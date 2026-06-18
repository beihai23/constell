package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CommunityRow represents a row from the communities table.
type CommunityRow struct {
	ID          string
	Name        string
	Description string
	IconURL     string
	OwnerID     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ChannelRow represents a row from the channels table.
type ChannelRow struct {
	ID          string
	CommunityID string
	Name        string
	Topic       string
	Type        string
	Position    int32
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// MemberRow represents a row from community_members.
type MemberRow struct {
	CommunityID string
	UserID      string
	Nickname    string
	JoinedAt    time.Time
}

// RoleRow represents a row from roles.
type RoleRow struct {
	ID          string
	CommunityID string
	Name        string
	Color       int32
	Permissions int64
	Position    int32
	CreatedAt   time.Time
}

// ChannelMessageRow represents a row from channel_messages.
type ChannelMessageRow struct {
	ID        string
	ChannelID string
	AuthorID  string
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
	Seq       int64
}

// Repository handles database operations for the community service.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// CreateCommunity inserts a new community and returns the full row.
func (r *Repository) CreateCommunity(ctx context.Context, name, description, iconURL, ownerID string) (*CommunityRow, error) {
	var s CommunityRow
	err := r.pool.QueryRow(ctx,
		`INSERT INTO communities (name, description, icon_url, owner_id)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id, name, description, icon_url, owner_id, created_at, updated_at`,
		name, description, iconURL, ownerID,
	).Scan(&s.ID, &s.Name, &s.Description, &s.IconURL, &s.OwnerID,
		&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create community: %w", err)
	}
	return &s, nil
}

// IsCommunityPublic reports whether a community exists and is_public=true.
// Returns (false, nil) for a private community and (false, err) when the
// community does not exist — callers treat both as "not joinable".
func (r *Repository) IsCommunityPublic(ctx context.Context, communityID string) (bool, error) {
	var isPublic bool
	err := r.pool.QueryRow(ctx, `
		SELECT is_public FROM communities WHERE id = $1
	`, communityID).Scan(&isPublic)
	if err != nil {
		return false, err // pgx.ErrNoRows if not found
	}
	return isPublic, nil
}

// GetCommunity fetches a community by ID.
func (r *Repository) GetCommunity(ctx context.Context, communityID string) (*CommunityRow, error) {
	var s CommunityRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, description, icon_url, owner_id, created_at, updated_at
			 FROM communities WHERE id = $1`, communityID,
	).Scan(&s.ID, &s.Name, &s.Description, &s.IconURL, &s.OwnerID,
		&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("community not found")
		}
		return nil, fmt.Errorf("query community: %w", err)
	}
	return &s, nil
}

// UpdateCommunity updates a community's fields.
func (r *Repository) UpdateCommunity(ctx context.Context, communityID, name, description, iconURL string) (*CommunityRow, error) {
	var s CommunityRow
	err := r.pool.QueryRow(ctx,
		`UPDATE communities SET name = $2, description = $3, icon_url = $4, updated_at = now()
			 WHERE id = $1
			 RETURNING id, name, description, icon_url, owner_id, created_at, updated_at`,
		communityID, name, description, iconURL,
	).Scan(&s.ID, &s.Name, &s.Description, &s.IconURL, &s.OwnerID,
		&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("update community: %w", err)
	}
	return &s, nil
}

// DeleteCommunity deletes a community by ID.
func (r *Repository) DeleteCommunity(ctx context.Context, communityID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM communities WHERE id = $1`, communityID)
	if err != nil {
		return fmt.Errorf("delete community: %w", err)
	}
	return nil
}

// ListCommunitiesByUser lists communities the user is a member of.
func (r *Repository) ListCommunitiesByUser(ctx context.Context, userID string, limit int, cursor string) ([]*CommunityRow, string, error) {
	var args []interface{}
	args = append(args, userID)
	query := `
			SELECT c.id, c.name, c.description, c.icon_url, c.owner_id, c.created_at, c.updated_at
			FROM communities c JOIN community_members cm ON cm.community_id = c.id
			WHERE cm.user_id = $1`

	argIdx := 2
	if cursor != "" {
		query += fmt.Sprintf(` AND c.id > $%d`, argIdx)
		args = append(args, cursor)
		argIdx++
	}
	query += fmt.Sprintf(` ORDER BY c.id LIMIT $%d`, argIdx)
	args = append(args, limit+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("list communities: %w", err)
	}
	defer rows.Close()

	var communities []*CommunityRow
	for rows.Next() {
		var s CommunityRow
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.IconURL,
			&s.OwnerID, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, "", fmt.Errorf("scan community: %w", err)
		}
		communities = append(communities, &s)
	}

	var nextCursor string
	if len(communities) > limit {
		nextCursor = communities[limit-1].ID
		communities = communities[:limit]
	}
	return communities, nextCursor, nil
}

// CreateChannel inserts a new channel.
func (r *Repository) CreateChannel(ctx context.Context, communityID, name, topic, channelType string, position int32) (*ChannelRow, error) {
	var c ChannelRow
	err := r.pool.QueryRow(ctx,
		`INSERT INTO channels (community_id, name, topic, type, position)
			 VALUES ($1, $2, $3, $4, $5)
			 RETURNING id, community_id, name, topic, type, position, created_at, updated_at`,
		communityID, name, topic, channelType, position,
	).Scan(&c.ID, &c.CommunityID, &c.Name, &c.Topic, &c.Type, &c.Position,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create channel: %w", err)
	}
	return &c, nil
}

// GetChannel fetches a channel by ID.
func (r *Repository) GetChannel(ctx context.Context, channelID string) (*ChannelRow, error) {
	var c ChannelRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, community_id, name, topic, type, position, created_at, updated_at
			 FROM channels WHERE id = $1`, channelID,
	).Scan(&c.ID, &c.CommunityID, &c.Name, &c.Topic, &c.Type, &c.Position,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("channel not found")
		}
		return nil, fmt.Errorf("query channel: %w", err)
	}
	return &c, nil
}

// UpdateChannel updates a channel's fields.
func (r *Repository) UpdateChannel(ctx context.Context, channelID, name, topic, channelType string, position int32) (*ChannelRow, error) {
	var c ChannelRow
	err := r.pool.QueryRow(ctx,
		`UPDATE channels SET name = $2, topic = $3, type = $4, position = $5, updated_at = now()
			 WHERE id = $1
			 RETURNING id, community_id, name, topic, type, position, created_at, updated_at`,
		channelID, name, topic, channelType, position,
	).Scan(&c.ID, &c.CommunityID, &c.Name, &c.Topic, &c.Type, &c.Position,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("update channel: %w", err)
	}
	return &c, nil
}

// DeleteChannel deletes a channel.
func (r *Repository) DeleteChannel(ctx context.Context, channelID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM channels WHERE id = $1`, channelID)
	if err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}
	return nil
}

// ListChannelsByCommunity lists channels in a community.
func (r *Repository) ListChannelsByCommunity(ctx context.Context, communityID string) ([]*ChannelRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, community_id, name, topic, type, position, created_at, updated_at
			 FROM channels WHERE community_id = $1 ORDER BY position`, communityID)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer rows.Close()

	var channels []*ChannelRow
	for rows.Next() {
		var c ChannelRow
		if err := rows.Scan(&c.ID, &c.CommunityID, &c.Name, &c.Topic,
			&c.Type, &c.Position, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan channel: %w", err)
		}
		channels = append(channels, &c)
	}
	return channels, nil
}

// AddMember adds a user to a community. The user's current nickname is copied
// from the users table so member listings show a readable name.
func (r *Repository) AddMember(ctx context.Context, communityID, userID string) (*MemberRow, error) {
	var m MemberRow
	err := r.pool.QueryRow(ctx,
		`INSERT INTO community_members (community_id, user_id, nickname)
		 SELECT $1, $2, COALESCE((SELECT nickname FROM users WHERE id = $2), '')
		 ON CONFLICT (community_id, user_id) DO NOTHING
		 RETURNING community_id, user_id, nickname, joined_at`,
		communityID, userID,
	).Scan(&m.CommunityID, &m.UserID, &m.Nickname, &m.JoinedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return r.GetMember(ctx, communityID, userID)
		}
		return nil, fmt.Errorf("add member: %w", err)
	}
	return &m, nil
}

// GetMember fetches a member record. The nickname is resolved from the users
// table so it stays current even if the user renames themselves.
func (r *Repository) GetMember(ctx context.Context, communityID, userID string) (*MemberRow, error) {
	var m MemberRow
	err := r.pool.QueryRow(ctx,
		`SELECT cm.community_id, cm.user_id, COALESCE(u.nickname, ''), cm.joined_at
			 FROM community_members cm
			 LEFT JOIN users u ON u.id = cm.user_id
			 WHERE cm.community_id = $1 AND cm.user_id = $2`,
		communityID, userID,
	).Scan(&m.CommunityID, &m.UserID, &m.Nickname, &m.JoinedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get member: %w", err)
	}
	return &m, nil
}

// RemoveMember removes a user from a community.
func (r *Repository) RemoveMember(ctx context.Context, communityID, userID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM community_members WHERE community_id = $1 AND user_id = $2`,
		communityID, userID)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	return nil
}

// ListMembersByCommunity lists members with pagination. The nickname is
// resolved from the users table so it stays current.
func (r *Repository) ListMembersByCommunity(ctx context.Context, communityID string, limit int, cursor string) ([]*MemberRow, string, error) {
	var args []interface{}
	args = append(args, communityID)
	query := `SELECT cm.community_id, cm.user_id, COALESCE(u.nickname, ''), cm.joined_at
		FROM community_members cm
		LEFT JOIN users u ON u.id = cm.user_id
		WHERE cm.community_id = $1`

	argIdx := 2
	if cursor != "" {
		query += fmt.Sprintf(` AND cm.user_id > $%d`, argIdx)
		args = append(args, cursor)
		argIdx++
	}
	query += fmt.Sprintf(` ORDER BY cm.user_id LIMIT $%d`, argIdx)
	args = append(args, limit+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()

	var members []*MemberRow
	for rows.Next() {
		var m MemberRow
		if err := rows.Scan(&m.CommunityID, &m.UserID, &m.Nickname, &m.JoinedAt); err != nil {
			return nil, "", fmt.Errorf("scan member: %w", err)
		}
		members = append(members, &m)
	}

	var nextCursor string
	if len(members) > limit {
		nextCursor = members[limit-1].UserID
		members = members[:limit]
	}
	return members, nextCursor, nil
}

// CreateRole inserts a new role.
func (r *Repository) CreateRole(ctx context.Context, communityID, name string, color int32, permissions int64, position int32) (*RoleRow, error) {
	var role RoleRow
	err := r.pool.QueryRow(ctx,
		`INSERT INTO roles (community_id, name, color, permissions, position)
			 VALUES ($1, $2, $3, $4, $5)
			 RETURNING id, community_id, name, color, permissions, position, created_at`,
		communityID, name, color, permissions, position,
	).Scan(&role.ID, &role.CommunityID, &role.Name, &role.Color,
		&role.Permissions, &role.Position, &role.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create role: %w", err)
	}
	return &role, nil
}

// AssignRole assigns a role to a member.
func (r *Repository) AssignRole(ctx context.Context, communityID, userID, roleID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO member_roles (community_id, user_id, role_id) VALUES ($1, $2, $3)
			 ON CONFLICT (community_id, user_id, role_id) DO NOTHING`,
		communityID, userID, roleID)
	if err != nil {
		return fmt.Errorf("assign role: %w", err)
	}
	return nil
}

// GetDefaultRole returns the @everyone role for a community.
func (r *Repository) GetDefaultRole(ctx context.Context, communityID string) (*RoleRow, error) {
	var role RoleRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, community_id, name, color, permissions, position, created_at
		 FROM roles WHERE community_id = $1 AND name = '@everyone'`,
		communityID,
	).Scan(&role.ID, &role.CommunityID, &role.Name, &role.Color,
		&role.Permissions, &role.Position, &role.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get default role: %w", err)
	}
	return &role, nil
}

// ListRolesByCommunity lists all roles for a community.
func (r *Repository) ListRolesByCommunity(ctx context.Context, communityID string) ([]*RoleRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, community_id, name, color, permissions, position, created_at
			 FROM roles WHERE community_id = $1 ORDER BY position`, communityID)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	var roles []*RoleRow
	for rows.Next() {
		var role RoleRow
		if err := rows.Scan(&role.ID, &role.CommunityID, &role.Name, &role.Color,
			&role.Permissions, &role.Position, &role.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		roles = append(roles, &role)
	}
	return roles, nil
}

// ListMemberRoles lists roles assigned to a member.
func (r *Repository) ListMemberRoles(ctx context.Context, communityID, userID string) ([]*RoleRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT r.id, r.community_id, r.name, r.color, r.permissions, r.position, r.created_at
			 FROM roles r JOIN member_roles mr ON mr.role_id = r.id
			 WHERE mr.community_id = $1 AND mr.user_id = $2 ORDER BY r.position`,
		communityID, userID)
	if err != nil {
		return nil, fmt.Errorf("list member roles: %w", err)
	}
	defer rows.Close()

	var roles []*RoleRow
	for rows.Next() {
		var role RoleRow
		if err := rows.Scan(&role.ID, &role.CommunityID, &role.Name, &role.Color,
			&role.Permissions, &role.Position, &role.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan member role: %w", err)
		}
		roles = append(roles, &role)
	}
	return roles, nil
}

// InsertChannelMessage inserts a channel message.
func (r *Repository) InsertChannelMessage(ctx context.Context, channelID, authorID, content string) (*ChannelMessageRow, error) {
	var m ChannelMessageRow
	err := r.pool.QueryRow(ctx,
		`INSERT INTO channel_messages (channel_id, author_id, content)
			 VALUES ($1, $2, $3)
			 RETURNING id, channel_id, author_id, content, created_at, updated_at, seq`,
		channelID, authorID, content,
	).Scan(&m.ID, &m.ChannelID, &m.AuthorID, &m.Content, &m.CreatedAt, &m.UpdatedAt, &m.Seq)
	if err != nil {
		return nil, fmt.Errorf("insert channel message: %w", err)
	}
	return &m, nil
}

// GetChannelMessages fetches messages with cursor pagination. seq is included
// in the SELECT and Scan so initial history-load messages seed the client's
// cursor; without it the backfill path would never be entered.
func (r *Repository) GetChannelMessages(ctx context.Context, channelID string, limit int, cursor string) ([]*ChannelMessageRow, string, error) {
	var args []interface{}
	args = append(args, channelID)
	query := `SELECT id, channel_id, author_id, content, created_at, updated_at, seq
			FROM channel_messages WHERE channel_id = $1`

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
		return nil, "", fmt.Errorf("get channel messages: %w", err)
	}
	defer rows.Close()

	var messages []*ChannelMessageRow
	for rows.Next() {
		var m ChannelMessageRow
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.AuthorID, &m.Content,
			&m.CreatedAt, &m.UpdatedAt, &m.Seq); err != nil {
			return nil, "", fmt.Errorf("scan message: %w", err)
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

// GetChannelMessagesSince returns messages with seq > sinceSeq, ordered ascending
// (oldest first) so the client can append them in order during backfill.
func (r *Repository) GetChannelMessagesSince(ctx context.Context, channelID string, sinceSeq int64, limit int) ([]*ChannelMessageRow, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, channel_id, author_id, content, created_at, updated_at, seq
		 FROM channel_messages
		 WHERE channel_id = $1 AND seq > $2
		 ORDER BY seq ASC
		 LIMIT $3`,
		channelID, sinceSeq, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get channel messages since: %w", err)
	}
	defer rows.Close()

	var messages []*ChannelMessageRow
	for rows.Next() {
		var m ChannelMessageRow
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.AuthorID, &m.Content,
			&m.CreatedAt, &m.UpdatedAt, &m.Seq); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, &m)
	}
	return messages, nil
}

// MarshalCommunity serializes a CommunityRow to JSON.
func MarshalCommunity(s *CommunityRow) ([]byte, error) { return json.Marshal(s) }

// UnmarshalCommunity deserializes JSON to a CommunityRow.
func UnmarshalCommunity(data []byte) (*CommunityRow, error) {
	var s CommunityRow
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// MarshalMembers serializes a member list to JSON.
func MarshalMembers(members []*MemberRow) ([]byte, error) { return json.Marshal(members) }

// UnmarshalMembers deserializes JSON to a member list.
func UnmarshalMembers(data []byte) ([]*MemberRow, error) {
	var members []*MemberRow
	if err := json.Unmarshal(data, &members); err != nil {
		return nil, err
	}
	return members, nil
}

// MarshalRoles serializes a role list to JSON.
func MarshalRoles(roles []*RoleRow) ([]byte, error) { return json.Marshal(roles) }

// UnmarshalRoles deserializes JSON to a role list.
func UnmarshalRoles(data []byte) ([]*RoleRow, error) {
	var roles []*RoleRow
	if err := json.Unmarshal(data, &roles); err != nil {
		return nil, err
	}
	return roles, nil
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

// GetAttachmentsByMessage retrieves attachments for a message.
func (r *Repository) GetAttachmentsByMessage(ctx context.Context, messageType, messageID string) ([]*AttachmentRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, message_type, message_id, file_id, filename, content_type, size
		 FROM attachments WHERE message_type = $1 AND message_id = $2`,
		messageType, messageID)
	if err != nil {
		return nil, fmt.Errorf("get attachments: %w", err)
	}
	defer rows.Close()

	var attachments []*AttachmentRow
	for rows.Next() {
		var a AttachmentRow
		if err := rows.Scan(&a.ID, &a.MessageType, &a.MessageID, &a.FileID, &a.Filename, &a.ContentType, &a.Size); err != nil {
			return nil, fmt.Errorf("scan attachment: %w", err)
		}
		attachments = append(attachments, &a)
	}
	return attachments, nil
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
