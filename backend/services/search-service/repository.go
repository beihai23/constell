package main

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UserSearchResult holds a row from user search.
type UserSearchResult struct {
	ID        string
	Nickname  string
	AvatarURL string
	Relevance float64
}

// MessageSearchResult holds a row from channel message search.
type MessageSearchResult struct {
	ID          string
	ChannelID   string
	CommunityID string
	AuthorID    string
	Content     string
	CreatedAt   int64
	Relevance   float64
}

// DMMessageSearchResult holds a row from DM message search.
type DMMessageSearchResult struct {
	ID             string
	ConversationID string
	PeerID         string
	Content        string
	CreatedAt      int64
	Relevance      float64
}

// CommunitySearchResult holds a row from community discovery search.
type CommunitySearchResult struct {
	ID          string
	Name        string
	IconURL     string
	Description string
	MemberCount int64
	Joined      bool
	Relevance   float64
}

// SearchRepository defines the database operations for search.
type SearchRepository interface {
	SearchUsers(ctx context.Context, query string, limit int) ([]UserSearchResult, error)
	SearchChannelMessages(ctx context.Context, query string, userID string, limit int) ([]MessageSearchResult, error)
	SearchDMMessages(ctx context.Context, query string, userID string, limit int) ([]DMMessageSearchResult, error)
	SearchCommunities(ctx context.Context, query string, userID string, limit int) ([]CommunitySearchResult, error)
}

// Repository implements SearchRepository backed by PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new Repository backed by the given connection pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// prefixTsquery turns a free-form user query into a tsquery string suitable
// for to_tsquery('simple', ...) that matches on token PREFIXES, so "comm"
// finds "Community" / "Commons" (not just the exact token "comm").
//
// Each whitespace-separated word is stripped of tsquery metacharacters
// (& | : ! ( ) ' etc.), and the surviving token is suffixed with ":*" to make
// it a prefix match. Words are joined with " & " so multi-word queries AND
// their tokens. An all-stopword/all-symbol input yields "" — to_tsquery then
// returns an empty query that matches nothing (the caller's empty-query guard
// means this only happens for degenerate input like "&&&").
func prefixTsquery(q string) string {
	var b strings.Builder
	first := true
	for _, field := range strings.Fields(q) {
		var tok strings.Builder
		for _, r := range field {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
				tok.WriteRune(r)
			}
		}
		if tok.Len() == 0 {
			continue
		}
		if !first {
			b.WriteString(" & ")
		}
		first = false
		b.WriteString(tok.String())
		b.WriteString(":*")
	}
	return b.String()
}

// SearchUsers searches the users table by tsvector full-text search with
// PREFIX matching (so "goph" matches "gophers"). No permission filtering —
// all matching users are returned.
func (r *Repository) SearchUsers(ctx context.Context, query string, limit int) ([]UserSearchResult, error) {
	tsq := prefixTsquery(query)
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.nickname, COALESCE(u.avatar_url, ''),
		       ts_rank(u.search_vector, to_tsquery('simple', $1)) AS relevance
		FROM users u
		WHERE u.search_vector @@ to_tsquery('simple', $1)
		ORDER BY relevance DESC
		LIMIT $2
	`, tsq, limit)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	defer rows.Close()

	var results []UserSearchResult
	for rows.Next() {
		var r UserSearchResult
		if err := rows.Scan(&r.ID, &r.Nickname, &r.AvatarURL, &r.Relevance); err != nil {
			return nil, fmt.Errorf("scan user result: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// SearchChannelMessages searches channel messages with community membership
// check and PREFIX matching (so "golan" matches "golang"). Only returns
// messages from channels in communities where the user is a member.
func (r *Repository) SearchChannelMessages(ctx context.Context, query string, userID string, limit int) ([]MessageSearchResult, error) {
	tsq := prefixTsquery(query)
	rows, err := r.pool.Query(ctx, `
		SELECT cm.id, cm.channel_id, c.community_id, cm.author_id, cm.content,
		       EXTRACT(EPOCH FROM cm.created_at)::bigint AS created_at,
		       ts_rank(cm.search_vector, to_tsquery('simple', $1)) AS relevance
		FROM channel_messages cm
		JOIN channels c ON c.id = cm.channel_id
		JOIN community_members mb ON mb.community_id = c.community_id AND mb.user_id = $2
		WHERE cm.search_vector @@ to_tsquery('simple', $1)
		ORDER BY relevance DESC
		LIMIT $3
	`, tsq, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("search channel messages: %w", err)
	}
	defer rows.Close()

	var results []MessageSearchResult
	for rows.Next() {
		var r MessageSearchResult
		if err := rows.Scan(&r.ID, &r.ChannelID, &r.CommunityID, &r.AuthorID, &r.Content, &r.CreatedAt, &r.Relevance); err != nil {
			return nil, fmt.Errorf("scan message result: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// SearchDMMessages searches DM messages for conversations where the user is a
// participant, with PREFIX matching (so "hel" matches "hello"). Peer ID is
// computed as the other party in the conversation.
func (r *Repository) SearchDMMessages(ctx context.Context, query string, userID string, limit int) ([]DMMessageSearchResult, error) {
	tsq := prefixTsquery(query)
	rows, err := r.pool.Query(ctx, `
		SELECT dm.id, dm.conversation_id,
		       CASE WHEN dc.user_a_id = $2 THEN dc.user_b_id ELSE dc.user_a_id END AS peer_id,
		       dm.content,
		       EXTRACT(EPOCH FROM dm.created_at)::bigint AS created_at,
		       ts_rank(dm.search_vector, to_tsquery('simple', $1)) AS relevance
		FROM dm_messages dm
		JOIN dm_conversations dc ON dc.id = dm.conversation_id
		WHERE (dc.user_a_id = $2 OR dc.user_b_id = $2)
		  AND dm.search_vector @@ to_tsquery('simple', $1)
		ORDER BY relevance DESC
		LIMIT $3
	`, tsq, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("search dm messages: %w", err)
	}
	defer rows.Close()

	var results []DMMessageSearchResult
	for rows.Next() {
		var r DMMessageSearchResult
		if err := rows.Scan(&r.ID, &r.ConversationID, &r.PeerID, &r.Content, &r.CreatedAt, &r.Relevance); err != nil {
			return nil, fmt.Errorf("scan dm message result: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// SearchCommunities searches public communities by name/description via the
// generated search_vector, using PREFIX matching so partial typing ("comm")
// matches longer tokens ("Community"). Only is_public=true communities are
// returned. member_count is the member total for the community; joined
// reflects whether userID is currently a member. Results are ordered by
// ts_rank relevance desc.
func (r *Repository) SearchCommunities(ctx context.Context, query string, userID string, limit int) ([]CommunitySearchResult, error) {
	tsq := prefixTsquery(query)
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.name, COALESCE(c.icon_url, ''), COALESCE(c.description, ''),
		       COUNT(cm.user_id)::bigint AS member_count,
		       EXISTS(SELECT 1 FROM community_members cm2
		              WHERE cm2.community_id = c.id AND cm2.user_id = $2) AS joined,
		       ts_rank(c.search_vector, to_tsquery('simple', $1)) AS relevance
		FROM communities c
		LEFT JOIN community_members cm ON cm.community_id = c.id
		WHERE c.is_public = true
		  AND c.search_vector @@ to_tsquery('simple', $1)
		GROUP BY c.id
		ORDER BY relevance DESC
		LIMIT $3
	`, tsq, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("search communities: %w", err)
	}
	defer rows.Close()

	var results []CommunitySearchResult
	for rows.Next() {
		var r CommunitySearchResult
		if err := rows.Scan(&r.ID, &r.Name, &r.IconURL, &r.Description, &r.MemberCount, &r.Joined, &r.Relevance); err != nil {
			return nil, fmt.Errorf("scan community result: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
