package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
)

// UnreadChannel represents an unread channel entry returned to the caller.
type UnreadChannel struct {
	ChannelID   string
	CommunityID string
	Count       int32
}

// UnreadDM represents an unread DM conversation entry returned to the caller.
type UnreadDM struct {
	ConversationID string
	PeerID         string
	Count          int32
}

// Store manages read-state in Postgres. The read cursor (last_read_seq) is the
// source of truth for unread: it is anchored to the monotonic message seq, so
// unread counts are always derivable and self-healing. The Redis client is
// retained only for the ws-gateway registry lookup used by push delivery
// (subscriber.go); it holds no notification state.
type Store struct {
	rdb  *goredis.Client
	pool *pgxpool.Pool
}

// NewStore creates a Store backed by the given Postgres pool (and Redis client,
// used solely for gateway lookups during push).
func NewStore(rdb *goredis.Client, pool *pgxpool.Pool) *Store {
	return &Store{rdb: rdb, pool: pool}
}

// ---------- Mark-read (cursor advances to the channel's latest message) ----------

// MarkChannelRead advances the user's read cursor for a channel to its newest
// message. Idempotent and monotonic: last_read_seq = GREATEST(existing, max(seq)).
func (s *Store) MarkChannelRead(ctx context.Context, userID, channelID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO channel_read_state (user_id, channel_id, last_read_seq)
		VALUES ($1, $2, (SELECT COALESCE(max(seq), 0) FROM channel_messages WHERE channel_id = $2))
		ON CONFLICT (user_id, channel_id) DO UPDATE
		SET last_read_seq = GREATEST(channel_read_state.last_read_seq, EXCLUDED.last_read_seq),
		    updated_at = now()`,
		userID, channelID)
	if err != nil {
		return fmt.Errorf("mark channel read: %w", err)
	}
	return nil
}

// MarkDMRead advances the user's read cursor for a DM conversation to its newest
// message. Idempotent and monotonic.
func (s *Store) MarkDMRead(ctx context.Context, userID, convID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO dm_read_state (user_id, conversation_id, last_read_seq)
		VALUES ($1, $2, (SELECT COALESCE(max(seq), 0) FROM dm_messages WHERE conversation_id = $2))
		ON CONFLICT (user_id, conversation_id) DO UPDATE
		SET last_read_seq = GREATEST(dm_read_state.last_read_seq, EXCLUDED.last_read_seq),
		    updated_at = now()`,
		userID, convID)
	if err != nil {
		return fmt.Errorf("mark DM read: %w", err)
	}
	return nil
}

// ---------- Cursor advance on send (the sender has seen their own message) ----------

// AdvanceChannelRead advances the sender's read cursor to a specific seq. Used
// when a user sends a channel message so their own message isn't flagged unread.
func (s *Store) AdvanceChannelRead(ctx context.Context, userID, channelID string, seq int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO channel_read_state (user_id, channel_id, last_read_seq)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, channel_id) DO UPDATE
		SET last_read_seq = GREATEST(channel_read_state.last_read_seq, EXCLUDED.last_read_seq),
		    updated_at = now()`,
		userID, channelID, seq)
	if err != nil {
		return fmt.Errorf("advance channel read: %w", err)
	}
	return nil
}

// AdvanceDMRead advances the sender's read cursor for a DM to a specific seq.
func (s *Store) AdvanceDMRead(ctx context.Context, userID, convID string, seq int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO dm_read_state (user_id, conversation_id, last_read_seq)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, conversation_id) DO UPDATE
		SET last_read_seq = GREATEST(dm_read_state.last_read_seq, EXCLUDED.last_read_seq),
		    updated_at = now()`,
		userID, convID, seq)
	if err != nil {
		return fmt.Errorf("advance DM read: %w", err)
	}
	return nil
}

// ---------- Unread computation ----------

// GetUnreadChannels returns the channels with unread messages for a user.
// Membership is sourced from Postgres (community_members × channels); for each
// channel the unread count is the messages whose seq exceeds the user's cursor.
// The correlated count scans only the unread tail via (channel_id, seq).
func (s *Store) GetUnreadChannels(ctx context.Context, userID string) ([]UnreadChannel, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id,
		       c.community_id,
		       (SELECT count(*) FROM channel_messages m
		         WHERE m.channel_id = c.id
		           AND m.seq > COALESCE(rs.last_read_seq, 0)) AS unread
		FROM channels c
		JOIN community_members cm ON cm.community_id = c.community_id AND cm.user_id = $1
		LEFT JOIN channel_read_state rs ON rs.user_id = $1 AND rs.channel_id = c.id`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("query unread channels: %w", err)
	}
	defer rows.Close()

	var result []UnreadChannel
	for rows.Next() {
		var chID, commID string
		var unread int64
		if err := rows.Scan(&chID, &commID, &unread); err != nil {
			return nil, fmt.Errorf("scan unread channel: %w", err)
		}
		if unread > 0 {
			result = append(result, UnreadChannel{
				ChannelID:   chID,
				CommunityID: commID,
				Count:       int32(unread),
			})
		}
	}
	return result, rows.Err()
}

// GetUnreadDMs returns the DM conversations with unread messages for a user.
// Conversations the user participates in are enumerated from dm_conversations
// (peer resolved inline); unread is the messages whose seq exceeds the cursor.
func (s *Store) GetUnreadDMs(ctx context.Context, userID string) ([]UnreadDM, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT dc.id,
		       CASE WHEN dc.user_a_id = $1 THEN dc.user_b_id ELSE dc.user_a_id END AS peer_id,
		       (SELECT count(*) FROM dm_messages m
		         WHERE m.conversation_id = dc.id
		           AND m.seq > COALESCE(rs.last_read_seq, 0)) AS unread
		FROM dm_conversations dc
		LEFT JOIN dm_read_state rs ON rs.user_id = $1 AND rs.conversation_id = dc.id
		WHERE dc.user_a_id = $1 OR dc.user_b_id = $1`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("query unread DMs: %w", err)
	}
	defer rows.Close()

	var result []UnreadDM
	for rows.Next() {
		var convID, peerID string
		var unread int64
		if err := rows.Scan(&convID, &peerID, &unread); err != nil {
			return nil, fmt.Errorf("scan unread DM: %w", err)
		}
		if unread > 0 {
			result = append(result, UnreadDM{
				ConversationID: convID,
				PeerID:         peerID,
				Count:          int32(unread),
			})
		}
	}
	return result, rows.Err()
}
