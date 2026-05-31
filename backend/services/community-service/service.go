package main

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	pbv1 "github.com/constell/constell/backend/pkg/proto/community/v1"
	"github.com/constell/constell/backend/pkg/proto/community/v1/communityv1connect"
	commonv1 "github.com/constell/constell/backend/pkg/proto/common/v1"
	"github.com/constell/constell/backend/pkg/middleware"
)

// CommunityService implements the Connect-RPC CommunityServiceHandler.
type CommunityService struct {
	repo         *Repository
	serverCache  *ServerCache
	membersCache *MembersCache
	rolesCache   *RolesCache
}

// NewCommunityService creates a new CommunityService.
func NewCommunityService(
	repo *Repository,
	serverCache *ServerCache,
	membersCache *MembersCache,
	rolesCache *RolesCache,
) *CommunityService {
	return &CommunityService{
		repo: repo, serverCache: serverCache,
		membersCache: membersCache, rolesCache: rolesCache,
	}
}

var _ communityv1connect.CommunityServiceHandler = (*CommunityService)(nil)

// CreateServer creates a new server. The caller becomes the owner.
func (s *CommunityService) CreateServer(
	ctx context.Context,
	req *connect.Request[pbv1.CreateServerRequest],
) (*connect.Response[pbv1.CreateServerResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}

	name := strings.TrimSpace(req.Msg.Name)
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("name is required"))
	}

	server, err := s.repo.CreateServer(ctx, name,
		strings.TrimSpace(req.Msg.Description),
		strings.TrimSpace(req.Msg.IconUrl), callerID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to create server: %w", err))
	}

	// Auto-add owner as member.
	if _, err := s.repo.AddMember(ctx, server.ID, callerID); err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to add owner as member: %w", err))
	}

	// Create default @everyone role.
	_, err = s.repo.CreateRole(ctx, server.ID, "@everyone", 0,
		PermissionReadMessages|PermissionSendMessages, 0)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to create default role: %w", err))
	}

	s.serverCache.Set(ctx, server)

	resp := connect.NewResponse(&pbv1.CreateServerResponse{
		Server: toPBServer(server),
	})
	return resp, nil
}

// GetServer returns a server by ID.
func (s *CommunityService) GetServer(
	ctx context.Context,
	req *connect.Request[pbv1.GetServerRequest],
) (*connect.Response[pbv1.GetServerResponse], error) {
	serverID := strings.TrimSpace(req.Msg.ServerId)
	if serverID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("server_id is required"))
	}

	server, err := s.serverCache.Get(ctx, serverID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound,
			fmt.Errorf("server not found: %w", err))
	}

	resp := connect.NewResponse(&pbv1.GetServerResponse{Server: toPBServer(server)})
	return resp, nil
}

// UpdateServer updates a server.
func (s *CommunityService) UpdateServer(
	ctx context.Context,
	req *connect.Request[pbv1.UpdateServerRequest],
) (*connect.Response[pbv1.UpdateServerResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	serverID := strings.TrimSpace(req.Msg.ServerId)
	if serverID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("server_id is required"))
	}
	if err := s.checkPermission(ctx, serverID, callerID, PermissionManageServer); err != nil {
		return nil, err
	}

	server, err := s.repo.UpdateServer(ctx, serverID,
		strings.TrimSpace(req.Msg.Name),
		strings.TrimSpace(req.Msg.Description),
		strings.TrimSpace(req.Msg.IconUrl))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to update server: %w", err))
	}
	s.serverCache.Invalidate(ctx, serverID)

	resp := connect.NewResponse(&pbv1.UpdateServerResponse{Server: toPBServer(server)})
	return resp, nil
}

// DeleteServer deletes a server. Only the owner can delete.
func (s *CommunityService) DeleteServer(
	ctx context.Context,
	req *connect.Request[pbv1.DeleteServerRequest],
) (*connect.Response[pbv1.DeleteServerResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	serverID := strings.TrimSpace(req.Msg.ServerId)
	if serverID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("server_id is required"))
	}

	server, err := s.serverCache.Get(ctx, serverID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server not found"))
	}
	if server.OwnerID != callerID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the server owner can delete the server"))
	}

	if err := s.repo.DeleteServer(ctx, serverID); err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to delete server: %w", err))
	}
	s.serverCache.Invalidate(ctx, serverID)
	s.membersCache.Invalidate(ctx, serverID)
	s.rolesCache.Invalidate(ctx, serverID)

	resp := connect.NewResponse(&pbv1.DeleteServerResponse{})
	return resp, nil
}

// ListServers lists servers the caller is a member of.
func (s *CommunityService) ListServers(
	ctx context.Context,
	req *connect.Request[pbv1.ListServersRequest],
) (*connect.Response[pbv1.ListServersResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}

	limit := int32(50)
	var cursor string
	if req.Msg.Pagination != nil {
		if req.Msg.Pagination.Limit > 0 {
			limit = req.Msg.Pagination.Limit
		}
		cursor = req.Msg.Pagination.Cursor
	}

	servers, nextCursor, err := s.repo.ListServersByUser(ctx, callerID, int(limit), cursor)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to list servers: %w", err))
	}

	pbServers := make([]*pbv1.Server, 0, len(servers))
	for _, srv := range servers {
		pbServers = append(pbServers, toPBServer(srv))
	}

	hasMore := nextCursor != ""
	resp := connect.NewResponse(&pbv1.ListServersResponse{
		Servers: pbServers,
		Pagination: &commonv1.PaginationResponse{
			HasMore: hasMore, NextCursor: nextCursor,
		},
	})
	return resp, nil
}

// CreateChannel creates a new channel in a server.
func (s *CommunityService) CreateChannel(
	ctx context.Context,
	req *connect.Request[pbv1.CreateChannelRequest],
) (*connect.Response[pbv1.CreateChannelResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	serverID := strings.TrimSpace(req.Msg.ServerId)
	name := strings.TrimSpace(req.Msg.Name)
	if serverID == "" || name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("server_id and name are required"))
	}
	if err := s.checkPermission(ctx, serverID, callerID, PermissionManageChannels); err != nil {
		return nil, err
	}

	channelType := "text"
	if req.Msg.Type == pbv1.ChannelType_CHANNEL_TYPE_ANNOUNCEMENT {
		channelType = "announcement"
	}

	ch, err := s.repo.CreateChannel(ctx, serverID, name,
		strings.TrimSpace(req.Msg.Topic), channelType, req.Msg.Position)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to create channel: %w", err))
	}

	resp := connect.NewResponse(&pbv1.CreateChannelResponse{Channel: toPBChannel(ch)})
	return resp, nil
}

// GetChannel returns a channel by ID.
func (s *CommunityService) GetChannel(
	ctx context.Context,
	req *connect.Request[pbv1.GetChannelRequest],
) (*connect.Response[pbv1.GetChannelResponse], error) {
	channelID := strings.TrimSpace(req.Msg.ChannelId)
	if channelID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("channel_id is required"))
	}
	ch, err := s.repo.GetChannel(ctx, channelID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound,
			fmt.Errorf("channel not found: %w", err))
	}
	resp := connect.NewResponse(&pbv1.GetChannelResponse{Channel: toPBChannel(ch)})
	return resp, nil
}

// UpdateChannel updates a channel.
func (s *CommunityService) UpdateChannel(
	ctx context.Context,
	req *connect.Request[pbv1.UpdateChannelRequest],
) (*connect.Response[pbv1.UpdateChannelResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	channelID := strings.TrimSpace(req.Msg.ChannelId)
	if channelID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("channel_id is required"))
	}
	ch, err := s.repo.GetChannel(ctx, channelID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("channel not found"))
	}
	if err := s.checkPermission(ctx, ch.ServerID, callerID, PermissionManageChannels); err != nil {
		return nil, err
	}

	channelType := "text"
	if req.Msg.Type == pbv1.ChannelType_CHANNEL_TYPE_ANNOUNCEMENT {
		channelType = "announcement"
	}

	updated, err := s.repo.UpdateChannel(ctx, channelID,
		strings.TrimSpace(req.Msg.Name), strings.TrimSpace(req.Msg.Topic),
		channelType, req.Msg.Position)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to update channel: %w", err))
	}

	resp := connect.NewResponse(&pbv1.UpdateChannelResponse{Channel: toPBChannel(updated)})
	return resp, nil
}

// DeleteChannel deletes a channel.
func (s *CommunityService) DeleteChannel(
	ctx context.Context,
	req *connect.Request[pbv1.DeleteChannelRequest],
) (*connect.Response[pbv1.DeleteChannelResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	channelID := strings.TrimSpace(req.Msg.ChannelId)
	if channelID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("channel_id is required"))
	}
	ch, err := s.repo.GetChannel(ctx, channelID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("channel not found"))
	}
	if err := s.checkPermission(ctx, ch.ServerID, callerID, PermissionManageChannels); err != nil {
		return nil, err
	}
	if err := s.repo.DeleteChannel(ctx, channelID); err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to delete channel: %w", err))
	}
	resp := connect.NewResponse(&pbv1.DeleteChannelResponse{})
	return resp, nil
}

// ListChannels lists channels in a server.
func (s *CommunityService) ListChannels(
	ctx context.Context,
	req *connect.Request[pbv1.ListChannelsRequest],
) (*connect.Response[pbv1.ListChannelsResponse], error) {
	serverID := strings.TrimSpace(req.Msg.ServerId)
	if serverID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("server_id is required"))
	}
	channels, err := s.repo.ListChannelsByServer(ctx, serverID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to list channels: %w", err))
	}
	pbChannels := make([]*pbv1.Channel, 0, len(channels))
	for _, ch := range channels {
		pbChannels = append(pbChannels, toPBChannel(ch))
	}
	resp := connect.NewResponse(&pbv1.ListChannelsResponse{Channels: pbChannels})
	return resp, nil
}

// JoinServer adds the caller to a server.
func (s *CommunityService) JoinServer(
	ctx context.Context,
	req *connect.Request[pbv1.JoinServerRequest],
) (*connect.Response[pbv1.JoinServerResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	serverID := strings.TrimSpace(req.Msg.ServerId)
	if serverID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("server_id is required"))
	}
	if _, err := s.serverCache.Get(ctx, serverID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server not found"))
	}

	member, err := s.repo.AddMember(ctx, serverID, callerID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to join server: %w", err))
	}
	s.membersCache.Invalidate(ctx, serverID)

	resp := connect.NewResponse(&pbv1.JoinServerResponse{
		Member: &pbv1.ServerMember{
			ServerId: member.ServerID, UserId: member.UserID,
			Nickname: member.Nickname, JoinedAt: member.JoinedAt.Unix(),
		},
	})
	return resp, nil
}

// LeaveServer removes the caller from a server.
func (s *CommunityService) LeaveServer(
	ctx context.Context,
	req *connect.Request[pbv1.LeaveServerRequest],
) (*connect.Response[pbv1.LeaveServerResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	serverID := strings.TrimSpace(req.Msg.ServerId)
	if serverID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("server_id is required"))
	}

	server, err := s.serverCache.Get(ctx, serverID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server not found"))
	}
	if server.OwnerID == callerID {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("server owner cannot leave; transfer ownership or delete"))
	}

	if err := s.repo.RemoveMember(ctx, serverID, callerID); err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to leave server: %w", err))
	}
	s.membersCache.Invalidate(ctx, serverID)

	resp := connect.NewResponse(&pbv1.LeaveServerResponse{})
	return resp, nil
}

// ListMembers lists members of a server.
func (s *CommunityService) ListMembers(
	ctx context.Context,
	req *connect.Request[pbv1.ListMembersRequest],
) (*connect.Response[pbv1.ListMembersResponse], error) {
	serverID := strings.TrimSpace(req.Msg.ServerId)
	if serverID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("server_id is required"))
	}

	limit := int32(50)
	var cursor string
	if req.Msg.Pagination != nil {
		if req.Msg.Pagination.Limit > 0 {
			limit = req.Msg.Pagination.Limit
		}
		cursor = req.Msg.Pagination.Cursor
	}

	members, nextCursor, err := s.repo.ListMembersByServer(ctx, serverID, int(limit), cursor)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to list members: %w", err))
	}

	pbMembers := make([]*pbv1.ServerMember, 0, len(members))
	for _, m := range members {
		pbMembers = append(pbMembers, &pbv1.ServerMember{
			ServerId: m.ServerID, UserId: m.UserID,
			Nickname: m.Nickname, JoinedAt: m.JoinedAt.Unix(),
		})
	}

	hasMore := nextCursor != ""
	resp := connect.NewResponse(&pbv1.ListMembersResponse{
		Members: pbMembers,
		Pagination: &commonv1.PaginationResponse{
			HasMore: hasMore, NextCursor: nextCursor,
		},
	})
	return resp, nil
}

// SendMessage sends a message to a channel.
func (s *CommunityService) SendMessage(
	ctx context.Context,
	req *connect.Request[pbv1.SendMessageRequest],
) (*connect.Response[pbv1.SendMessageResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	channelID := strings.TrimSpace(req.Msg.ChannelId)
	content := strings.TrimSpace(req.Msg.Content)
	if channelID == "" || content == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("channel_id and content are required"))
	}

	ch, err := s.repo.GetChannel(ctx, channelID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("channel not found"))
	}

	members, err := s.membersCache.Get(ctx, ch.ServerID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to check membership: %w", err))
	}
	if !cachedMembersSet(members)[callerID] {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("not a member of this server"))
	}

	if err := s.checkPermission(ctx, ch.ServerID, callerID, PermissionSendMessages); err != nil {
		return nil, err
	}

	msg, err := s.repo.InsertChannelMessage(ctx, channelID, callerID, content)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to send message: %w", err))
	}

	resp := connect.NewResponse(&pbv1.SendMessageResponse{
		Message: &pbv1.ChannelMessage{
			Id: msg.ID, ChannelId: msg.ChannelID, AuthorId: msg.AuthorID,
			Content: msg.Content, CreatedAt: msg.CreatedAt.Unix(),
			UpdatedAt: msg.UpdatedAt.Unix(),
		},
	})
	return resp, nil
}

// GetMessages retrieves messages from a channel.
func (s *CommunityService) GetMessages(
	ctx context.Context,
	req *connect.Request[pbv1.GetMessagesRequest],
) (*connect.Response[pbv1.GetMessagesResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	channelID := strings.TrimSpace(req.Msg.ChannelId)
	if channelID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("channel_id is required"))
	}

	ch, err := s.repo.GetChannel(ctx, channelID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("channel not found"))
	}

	members, err := s.membersCache.Get(ctx, ch.ServerID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to check membership: %w", err))
	}
	if !cachedMembersSet(members)[callerID] {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("not a member of this server"))
	}

	if err := s.checkPermission(ctx, ch.ServerID, callerID, PermissionReadMessages); err != nil {
		return nil, err
	}

	limit := int32(50)
	var cursor string
	if req.Msg.Pagination != nil {
		if req.Msg.Pagination.Limit > 0 {
			limit = req.Msg.Pagination.Limit
		}
		cursor = req.Msg.Pagination.Cursor
	}

	messages, nextCursor, err := s.repo.GetChannelMessages(ctx, channelID, int(limit), cursor)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to get messages: %w", err))
	}

	pbMessages := make([]*pbv1.ChannelMessage, 0, len(messages))
	for _, m := range messages {
		pbMessages = append(pbMessages, &pbv1.ChannelMessage{
			Id: m.ID, ChannelId: m.ChannelID, AuthorId: m.AuthorID,
			Content: m.Content, CreatedAt: m.CreatedAt.Unix(),
			UpdatedAt: m.UpdatedAt.Unix(),
		})
	}

	hasMore := nextCursor != ""
	resp := connect.NewResponse(&pbv1.GetMessagesResponse{
		Messages: pbMessages,
		Pagination: &commonv1.PaginationResponse{
			HasMore: hasMore, NextCursor: nextCursor,
		},
	})
	return resp, nil
}

// checkPermission verifies the caller has the required permission.
func (s *CommunityService) checkPermission(ctx context.Context, serverID, userID string, required int64) error {
	server, err := s.serverCache.Get(ctx, serverID)
	if err != nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("server not found"))
	}
	if server.OwnerID == userID {
		return nil
	}

	roles, err := s.repo.ListMemberRoles(ctx, serverID, userID)
	if err != nil {
		return connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to get member roles: %w", err))
	}

	member := &MemberRow{ServerID: serverID, UserID: userID}
	perms := ComputePermissions(member, roles, server.OwnerID)

	if !HasPermission(perms, required) {
		return connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("insufficient permissions"))
	}
	return nil
}

func toPBServer(s *ServerRow) *pbv1.Server {
	return &pbv1.Server{
		Id: s.ID, Name: s.Name, Description: s.Description,
		IconUrl: s.IconURL, OwnerId: s.OwnerID,
		CreatedAt: s.CreatedAt.Unix(), UpdatedAt: s.UpdatedAt.Unix(),
	}
}

func toPBChannel(c *ChannelRow) *pbv1.Channel {
	var chType pbv1.ChannelType
	if c.Type == "announcement" {
		chType = pbv1.ChannelType_CHANNEL_TYPE_ANNOUNCEMENT
	} else {
		chType = pbv1.ChannelType_CHANNEL_TYPE_TEXT
	}
	return &pbv1.Channel{
		Id: c.ID, ServerId: c.ServerID, Name: c.Name, Topic: c.Topic,
		Type: chType, Position: c.Position,
		CreatedAt: c.CreatedAt.Unix(), UpdatedAt: c.UpdatedAt.Unix(),
	}
}
