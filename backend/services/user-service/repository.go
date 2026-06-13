package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRow represents a row from the users table.
type UserRow struct {
	ID            string
	Email         string
	Nickname      string
	AvatarURL     string
	StatusMessage string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// RelationRow represents a row from user_relations.
type RelationRow struct {
	UserID       string
	TargetUserID string
	Type         string // "friend" or "blocked"
	CreatedAt    time.Time
}

// DMConversationRow represents a row from dm_conversations.
type DMConversationRow struct {
	ID        string
	UserAID   string
	UserBID   string
	CreatedAt time.Time
}

// DMMessageRow represents a row from dm_messages.
type DMMessageRow struct {
	ID             string
	ConversationID string
	SenderID       string
	Content        string
	CreatedAt      time.Time
}

// Repository handles database operations for the user service.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// GetUserByID fetches a user by ID.
func (r *Repository) GetUserByID(ctx context.Context, userID string) (*UserRow, error) {
	var u UserRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, nickname, avatar_url, status_message, created_at, updated_at
		 FROM users WHERE id = $1`, userID,
	).Scan(&u.ID, &u.Email, &u.Nickname, &u.AvatarURL, &u.StatusMessage,
		&u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user not found: %w", err)
		}
		return nil, fmt.Errorf("query user: %w", err)
	}
	return &u, nil
}

// UpdateUserProfile updates a user's profile fields.
func (r *Repository) UpdateUserProfile(ctx context.Context, userID, nickname, avatarURL, statusMessage string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET nickname = $2, avatar_url = $3, status_message = $4, updated_at = now()
		 WHERE id = $1`,
		userID, nickname, avatarURL, statusMessage)
	if err != nil {
		return fmt.Errorf("update user profile: %w", err)
	}
	return nil
}

// GetRelation fetches the relation between two users.
func (r *Repository) GetRelation(ctx context.Context, userID, targetUserID string) (*RelationRow, error) {
	var rel RelationRow
	err := r.pool.QueryRow(ctx,
		`SELECT user_id, target_user_id, type, created_at
		 FROM user_relations WHERE user_id = $1 AND target_user_id = $2`,
		userID, targetUserID,
	).Scan(&rel.UserID, &rel.TargetUserID, &rel.Type, &rel.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // no relation
		}
		return nil, fmt.Errorf("query relation: %w", err)
	}
	return &rel, nil
}

// CreateRelation creates a friend or block relation.
func (r *Repository) CreateRelation(ctx context.Context, userID, targetUserID, relationType string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_relations (user_id, target_user_id, type) VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, target_user_id) DO UPDATE SET type = $3`,
		userID, targetUserID, relationType)
	if err != nil {
		return fmt.Errorf("create relation: %w", err)
	}
	return nil
}

// DeleteRelation removes a relation between two users.
func (r *Repository) DeleteRelation(ctx context.Context, userID, targetUserID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM user_relations WHERE user_id = $1 AND target_user_id = $2`,
		userID, targetUserID)
	if err != nil {
		return fmt.Errorf("delete relation: %w", err)
	}
	return nil
}

// ListFriends returns the user's friends with pagination.
func (r *Repository) ListFriends(ctx context.Context, userID string, limit int, cursor string) ([]*UserRow, string, error) {
	var args []interface{}
	args = append(args, userID)
	query := `
		SELECT u.id, u.email, u.nickname, u.avatar_url, u.status_message, u.created_at, u.updated_at
		FROM user_relations r
		JOIN users u ON u.id = r.target_user_id
		WHERE r.user_id = $1 AND r.type = 'friend'`

	argIdx := 2
	if cursor != "" {
		query += fmt.Sprintf(` AND u.id > $%d`, argIdx)
		args = append(args, cursor)
		argIdx++
	}
	query += fmt.Sprintf(` ORDER BY u.id LIMIT $%d`, argIdx)
	args = append(args, limit+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("list friends: %w", err)
	}
	defer rows.Close()

	var friends []*UserRow
	for rows.Next() {
		var u UserRow
		if err := rows.Scan(&u.ID, &u.Email, &u.Nickname, &u.AvatarURL,
			&u.StatusMessage, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, "", fmt.Errorf("scan friend: %w", err)
		}
		friends = append(friends, &u)
	}

	var nextCursor string
	if len(friends) > limit {
		nextCursor = friends[limit-1].ID
		friends = friends[:limit]
	}
	return friends, nextCursor, nil
}

// GetOrCreateConversation returns the DM conversation between two users,
// creating it if it does not exist.
func (r *Repository) GetOrCreateConversation(ctx context.Context, userAID, userBID string) (*DMConversationRow, error) {
	small, big := userAID, userBID
	if small > big {
		small, big = big, small
	}

	var conv DMConversationRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_a_id, user_b_id, created_at
		 FROM dm_conversations WHERE user_a_id = $1 AND user_b_id = $2`,
		small, big,
	).Scan(&conv.ID, &conv.UserAID, &conv.UserBID, &conv.CreatedAt)
	if err == nil {
		return &conv, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("query conversation: %w", err)
	}

	err = r.pool.QueryRow(ctx,
		`INSERT INTO dm_conversations (user_a_id, user_b_id) VALUES ($1, $2)
		 RETURNING id, user_a_id, user_b_id, created_at`,
		small, big,
	).Scan(&conv.ID, &conv.UserAID, &conv.UserBID, &conv.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	return &conv, nil
}

// InsertDMMessage inserts a DM message and returns the full row.
func (r *Repository) InsertDMMessage(ctx context.Context, conversationID, senderID, content string) (*DMMessageRow, error) {
	var msg DMMessageRow
	err := r.pool.QueryRow(ctx,
		`INSERT INTO dm_messages (conversation_id, sender_id, content)
		 VALUES ($1, $2, $3) RETURNING id, conversation_id, sender_id, content, created_at`,
		conversationID, senderID, content,
	).Scan(&msg.ID, &msg.ConversationID, &msg.SenderID, &msg.Content, &msg.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert dm message: %w", err)
	}
	return &msg, nil
}

// GetDMHistory fetches DM messages for a conversation with cursor pagination.
func (r *Repository) GetDMHistory(ctx context.Context, conversationID string, limit int, cursor string) ([]*DMMessageRow, string, error) {
	var args []interface{}
	args = append(args, conversationID)
	query := `
		SELECT id, conversation_id, sender_id, content, created_at
		FROM dm_messages WHERE conversation_id = $1`

	argIdx := 2
	if cursor != "" {
		query += fmt.Sprintf(` AND created_at < $%d`, argIdx)
		args = append(args, cursor)
		argIdx++
	}
	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d`, argIdx)
	args = append(args, limit+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("get dm history: %w", err)
	}
	defer rows.Close()

	var messages []*DMMessageRow
	for rows.Next() {
		var m DMMessageRow
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.SenderID,
			&m.Content, &m.CreatedAt); err != nil {
			return nil, "", fmt.Errorf("scan dm message: %w", err)
		}
		messages = append(messages, &m)
	}

	var nextCursor string
	if len(messages) > limit {
		nextCursor = messages[limit].CreatedAt.Format(time.RFC3339Nano)
		messages = messages[:limit]
	}
	return messages, nextCursor, nil
}

// GetDMConversations returns conversations for a user, newest first.
// `cursor` is an RFC3339Nano timestamp; conversations older than it are returned.
// The peer is whichever of user_a_id / user_b_id is not the caller.
func (r *Repository) GetDMConversations(ctx context.Context, userID string, limit int, cursor string) ([]*DMConversationRow, string, error) {
	var args []interface{}
	args = append(args, userID)
	query := `
		SELECT id, user_a_id, user_b_id, created_at
		FROM dm_conversations WHERE user_a_id = $1 OR user_b_id = $1`

	argIdx := 2
	if cursor != "" {
		query += fmt.Sprintf(` AND created_at < $%d`, argIdx)
		args = append(args, cursor)
		argIdx++
	}
	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d`, argIdx)
	args = append(args, limit+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("get dm conversations: %w", err)
	}
	defer rows.Close()

	var convos []*DMConversationRow
	for rows.Next() {
		var c DMConversationRow
		if err := rows.Scan(&c.ID, &c.UserAID, &c.UserBID, &c.CreatedAt); err != nil {
			return nil, "", fmt.Errorf("scan conversation: %w", err)
		}
		convos = append(convos, &c)
	}

	var nextCursor string
	if len(convos) > limit {
		nextCursor = convos[limit].CreatedAt.Format(time.RFC3339Nano)
		convos = convos[:limit]
	}
	return convos, nextCursor, nil
}

// MarshalUser serializes a UserRow to JSON bytes.
func MarshalUser(u *UserRow) ([]byte, error) {
	return json.Marshal(u)
}

// UnmarshalUser deserializes JSON bytes to a UserRow.
func UnmarshalUser(data []byte) (*UserRow, error) {
	var u UserRow
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// MarshalRelation serializes a RelationRow to JSON bytes.
func MarshalRelation(r *RelationRow) ([]byte, error) {
	return json.Marshal(r)
}

// UnmarshalRelation deserializes JSON bytes to a RelationRow.
func UnmarshalRelation(data []byte) (*RelationRow, error) {
	var rel RelationRow
	if err := json.Unmarshal(data, &rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// AttachmentRow represents a row from the attachments table.
type AttachmentRow struct {
	ID          string
	MessageType string
	MessageID   string
	FileID      string
	Filename    string
	ContentType string
	Size        int64
}

// InsertAttachments inserts multiple attachment rows.
func (r *Repository) InsertAttachments(ctx context.Context, attachments []*AttachmentRow) error {
	for _, a := range attachments {
		_, err := r.pool.Exec(ctx,
			`INSERT INTO attachments (id, message_type, message_id, file_id, filename, content_type, size)
			 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6)`,
			a.MessageType, a.MessageID, a.FileID, a.Filename, a.ContentType, a.Size)
		if err != nil {
			return fmt.Errorf("insert attachment: %w", err)
		}
	}
	return nil
}
