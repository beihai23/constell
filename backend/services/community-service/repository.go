package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ServerRow represents a row from the servers table.
type ServerRow struct {
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
	ID        string
	ServerID  string
	Name      string
	Topic     string
	Type      string
	Position  int32
	CreatedAt time.Time
	UpdatedAt time.Time
}

// MemberRow represents a row from server_members.
type MemberRow struct {
	ServerID string
	UserID   string
	Nickname string
	JoinedAt time.Time
}

// RoleRow represents a row from roles.
type RoleRow struct {
	ID          string
	ServerID    string
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
}

// Repository handles database operations for the community service.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// CreateServer inserts a new server and returns the full row.
func (r *Repository) CreateServer(ctx context.Context, name, description, iconURL, ownerID string) (*ServerRow, error) {
	var s ServerRow
	err := r.pool.QueryRow(ctx,
		`INSERT INTO servers (name, description, icon_url, owner_id)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, name, description, icon_url, owner_id, created_at, updated_at`,
		name, description, iconURL, ownerID,
	).Scan(&s.ID, &s.Name, &s.Description, &s.IconURL, &s.OwnerID,
		&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create server: %w", err)
	}
	return &s, nil
}

// GetServer fetches a server by ID.
func (r *Repository) GetServer(ctx context.Context, serverID string) (*ServerRow, error) {
	var s ServerRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, description, icon_url, owner_id, created_at, updated_at
		 FROM servers WHERE id = $1`, serverID,
	).Scan(&s.ID, &s.Name, &s.Description, &s.IconURL, &s.OwnerID,
		&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("server not found")
		}
		return nil, fmt.Errorf("query server: %w", err)
	}
	return &s, nil
}

// UpdateServer updates a server's fields.
func (r *Repository) UpdateServer(ctx context.Context, serverID, name, description, iconURL string) (*ServerRow, error) {
	var s ServerRow
	err := r.pool.QueryRow(ctx,
		`UPDATE servers SET name = $2, description = $3, icon_url = $4, updated_at = now()
		 WHERE id = $1
		 RETURNING id, name, description, icon_url, owner_id, created_at, updated_at`,
		serverID, name, description, iconURL,
	).Scan(&s.ID, &s.Name, &s.Description, &s.IconURL, &s.OwnerID,
		&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("update server: %w", err)
	}
	return &s, nil
}

// DeleteServer deletes a server by ID.
func (r *Repository) DeleteServer(ctx context.Context, serverID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM servers WHERE id = $1`, serverID)
	if err != nil {
		return fmt.Errorf("delete server: %w", err)
	}
	return nil
}

// ListServersByUser lists servers the user is a member of.
func (r *Repository) ListServersByUser(ctx context.Context, userID string, limit int, cursor string) ([]*ServerRow, string, error) {
	var args []interface{}
	args = append(args, userID)
	query := `
		SELECT s.id, s.name, s.description, s.icon_url, s.owner_id, s.created_at, s.updated_at
		FROM servers s JOIN server_members sm ON sm.server_id = s.id
		WHERE sm.user_id = $1`

	argIdx := 2
	if cursor != "" {
		query += fmt.Sprintf(` AND s.id > $%d`, argIdx)
		args = append(args, cursor)
		argIdx++
	}
	query += fmt.Sprintf(` ORDER BY s.id LIMIT $%d`, argIdx)
	args = append(args, limit+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("list servers: %w", err)
	}
	defer rows.Close()

	var servers []*ServerRow
	for rows.Next() {
		var s ServerRow
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.IconURL,
			&s.OwnerID, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, "", fmt.Errorf("scan server: %w", err)
		}
		servers = append(servers, &s)
	}

	var nextCursor string
	if len(servers) > limit {
		nextCursor = servers[limit-1].ID
		servers = servers[:limit]
	}
	return servers, nextCursor, nil
}

// CreateChannel inserts a new channel.
func (r *Repository) CreateChannel(ctx context.Context, serverID, name, topic, channelType string, position int32) (*ChannelRow, error) {
	var c ChannelRow
	err := r.pool.QueryRow(ctx,
		`INSERT INTO channels (server_id, name, topic, type, position)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, server_id, name, topic, type, position, created_at, updated_at`,
		serverID, name, topic, channelType, position,
	).Scan(&c.ID, &c.ServerID, &c.Name, &c.Topic, &c.Type, &c.Position,
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
		`SELECT id, server_id, name, topic, type, position, created_at, updated_at
		 FROM channels WHERE id = $1`, channelID,
	).Scan(&c.ID, &c.ServerID, &c.Name, &c.Topic, &c.Type, &c.Position,
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
		 RETURNING id, server_id, name, topic, type, position, created_at, updated_at`,
		channelID, name, topic, channelType, position,
	).Scan(&c.ID, &c.ServerID, &c.Name, &c.Topic, &c.Type, &c.Position,
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

// ListChannelsByServer lists channels in a server.
func (r *Repository) ListChannelsByServer(ctx context.Context, serverID string) ([]*ChannelRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, server_id, name, topic, type, position, created_at, updated_at
		 FROM channels WHERE server_id = $1 ORDER BY position`, serverID)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer rows.Close()

	var channels []*ChannelRow
	for rows.Next() {
		var c ChannelRow
		if err := rows.Scan(&c.ID, &c.ServerID, &c.Name, &c.Topic,
			&c.Type, &c.Position, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan channel: %w", err)
		}
		channels = append(channels, &c)
	}
	return channels, nil
}

// AddMember adds a user to a server.
func (r *Repository) AddMember(ctx context.Context, serverID, userID string) (*MemberRow, error) {
	var m MemberRow
	err := r.pool.QueryRow(ctx,
		`INSERT INTO server_members (server_id, user_id)
		 VALUES ($1, $2) ON CONFLICT (server_id, user_id) DO NOTHING
		 RETURNING server_id, user_id, nickname, joined_at`,
		serverID, userID,
	).Scan(&m.ServerID, &m.UserID, &m.Nickname, &m.JoinedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return r.GetMember(ctx, serverID, userID)
		}
		return nil, fmt.Errorf("add member: %w", err)
	}
	return &m, nil
}

// GetMember fetches a member record.
func (r *Repository) GetMember(ctx context.Context, serverID, userID string) (*MemberRow, error) {
	var m MemberRow
	err := r.pool.QueryRow(ctx,
		`SELECT server_id, user_id, nickname, joined_at
		 FROM server_members WHERE server_id = $1 AND user_id = $2`,
		serverID, userID,
	).Scan(&m.ServerID, &m.UserID, &m.Nickname, &m.JoinedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get member: %w", err)
	}
	return &m, nil
}

// RemoveMember removes a user from a server.
func (r *Repository) RemoveMember(ctx context.Context, serverID, userID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM server_members WHERE server_id = $1 AND user_id = $2`,
		serverID, userID)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	return nil
}

// ListMembersByServer lists members with pagination.
func (r *Repository) ListMembersByServer(ctx context.Context, serverID string, limit int, cursor string) ([]*MemberRow, string, error) {
	var args []interface{}
	args = append(args, serverID)
	query := `SELECT server_id, user_id, nickname, joined_at FROM server_members WHERE server_id = $1`

	argIdx := 2
	if cursor != "" {
		query += fmt.Sprintf(` AND user_id > $%d`, argIdx)
		args = append(args, cursor)
		argIdx++
	}
	query += fmt.Sprintf(` ORDER BY user_id LIMIT $%d`, argIdx)
	args = append(args, limit+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()

	var members []*MemberRow
	for rows.Next() {
		var m MemberRow
		if err := rows.Scan(&m.ServerID, &m.UserID, &m.Nickname, &m.JoinedAt); err != nil {
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
func (r *Repository) CreateRole(ctx context.Context, serverID, name string, color int32, permissions int64, position int32) (*RoleRow, error) {
	var role RoleRow
	err := r.pool.QueryRow(ctx,
		`INSERT INTO roles (server_id, name, color, permissions, position)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, server_id, name, color, permissions, position, created_at`,
		serverID, name, color, permissions, position,
	).Scan(&role.ID, &role.ServerID, &role.Name, &role.Color,
		&role.Permissions, &role.Position, &role.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create role: %w", err)
	}
	return &role, nil
}

// AssignRole assigns a role to a member.
func (r *Repository) AssignRole(ctx context.Context, serverID, userID, roleID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO member_roles (server_id, user_id, role_id) VALUES ($1, $2, $3)
		 ON CONFLICT (server_id, user_id, role_id) DO NOTHING`,
		serverID, userID, roleID)
	if err != nil {
		return fmt.Errorf("assign role: %w", err)
	}
	return nil
}

// ListRolesByServer lists all roles for a server.
func (r *Repository) ListRolesByServer(ctx context.Context, serverID string) ([]*RoleRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, server_id, name, color, permissions, position, created_at
		 FROM roles WHERE server_id = $1 ORDER BY position`, serverID)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	var roles []*RoleRow
	for rows.Next() {
		var role RoleRow
		if err := rows.Scan(&role.ID, &role.ServerID, &role.Name, &role.Color,
			&role.Permissions, &role.Position, &role.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		roles = append(roles, &role)
	}
	return roles, nil
}

// ListMemberRoles lists roles assigned to a member.
func (r *Repository) ListMemberRoles(ctx context.Context, serverID, userID string) ([]*RoleRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT r.id, r.server_id, r.name, r.color, r.permissions, r.position, r.created_at
		 FROM roles r JOIN member_roles mr ON mr.role_id = r.id
		 WHERE mr.server_id = $1 AND mr.user_id = $2 ORDER BY r.position`,
		serverID, userID)
	if err != nil {
		return nil, fmt.Errorf("list member roles: %w", err)
	}
	defer rows.Close()

	var roles []*RoleRow
	for rows.Next() {
		var role RoleRow
		if err := rows.Scan(&role.ID, &role.ServerID, &role.Name, &role.Color,
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
		 RETURNING id, channel_id, author_id, content, created_at, updated_at`,
		channelID, authorID, content,
	).Scan(&m.ID, &m.ChannelID, &m.AuthorID, &m.Content, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert channel message: %w", err)
	}
	return &m, nil
}

// GetChannelMessages fetches messages with cursor pagination.
func (r *Repository) GetChannelMessages(ctx context.Context, channelID string, limit int, cursor string) ([]*ChannelMessageRow, string, error) {
	var args []interface{}
	args = append(args, channelID)
	query := `SELECT id, channel_id, author_id, content, created_at, updated_at
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
			&m.CreatedAt, &m.UpdatedAt); err != nil {
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

// MarshalServer serializes a ServerRow to JSON.
func MarshalServer(s *ServerRow) ([]byte, error) { return json.Marshal(s) }

// UnmarshalServer deserializes JSON to a ServerRow.
func UnmarshalServer(data []byte) (*ServerRow, error) {
	var s ServerRow
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
