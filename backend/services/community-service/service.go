package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"connectrpc.com/connect"

	"github.com/nats-io/nats.go"

	pbv1 "github.com/constell/constell/backend/pkg/proto/community/v1"
	"github.com/constell/constell/backend/pkg/proto/community/v1/communityv1connect"
	commonv1 "github.com/constell/constell/backend/pkg/proto/common/v1"
	"github.com/constell/constell/backend/pkg/middleware"
)

// CommunityService implements the Connect-RPC CommunityServiceHandler.
type CommunityService struct {
	repo           *Repository
	communityCache *CommunityCache
	membersCache   *MembersCache
	rolesCache     *RolesCache
	natsConn       *nats.Conn
}

// NewCommunityService creates a new CommunityService.
func NewCommunityService(
	repo *Repository,
	communityCache *CommunityCache,
	membersCache *MembersCache,
	rolesCache *RolesCache,
	natsConn *nats.Conn,
) *CommunityService {
	return &CommunityService{
		repo: repo, communityCache: communityCache,
		membersCache: membersCache, rolesCache: rolesCache,
		natsConn: natsConn,
	}
}

var _ communityv1connect.CommunityServiceHandler = (*CommunityService)(nil)

// CreateCommunity creates a new community. The caller becomes the owner.
func (s *CommunityService) CreateCommunity(
	ctx context.Context,
	req *connect.Request[pbv1.CreateCommunityRequest],
) (*connect.Response[pbv1.CreateCommunityResponse], error) {
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

	community, err := s.repo.CreateCommunity(ctx, name,
		strings.TrimSpace(req.Msg.Description),
		strings.TrimSpace(req.Msg.IconUrl), callerID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to create community: %w", err))
	}

	// Auto-add owner as member.
	if _, err := s.repo.AddMember(ctx, community.ID, callerID); err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to add owner as member: %w", err))
	}

	// Create default @everyone role and assign it to the owner.
	defaultRole, err := s.repo.CreateRole(ctx, community.ID, "@everyone", 0,
		PermissionReadMessages|PermissionSendMessages, 0)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to create default role: %w", err))
	}
	if err := s.repo.AssignRole(ctx, community.ID, callerID, defaultRole.ID); err != nil {
		slog.Warn("failed to assign @everyone role to owner", "error", err)
	}

	s.communityCache.Set(ctx, community)

	resp := connect.NewResponse(&pbv1.CreateCommunityResponse{
		Community: toPBCommunity(community),
	})
	return resp, nil
}

// GetCommunity returns a community by ID.
func (s *CommunityService) GetCommunity(
	ctx context.Context,
	req *connect.Request[pbv1.GetCommunityRequest],
) (*connect.Response[pbv1.GetCommunityResponse], error) {
	communityID := strings.TrimSpace(req.Msg.CommunityId)
	if communityID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("community_id is required"))
	}

	community, err := s.communityCache.Get(ctx, communityID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound,
			fmt.Errorf("community not found: %w", err))
	}

	resp := connect.NewResponse(&pbv1.GetCommunityResponse{Community: toPBCommunity(community)})
	return resp, nil
}

// UpdateCommunity updates a community.
func (s *CommunityService) UpdateCommunity(
	ctx context.Context,
	req *connect.Request[pbv1.UpdateCommunityRequest],
) (*connect.Response[pbv1.UpdateCommunityResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	communityID := strings.TrimSpace(req.Msg.CommunityId)
	if communityID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("community_id is required"))
	}
	if err := s.checkPermission(ctx, communityID, callerID, PermissionManageCommunity); err != nil {
		return nil, err
	}

	community, err := s.repo.UpdateCommunity(ctx, communityID,
		strings.TrimSpace(req.Msg.Name),
		strings.TrimSpace(req.Msg.Description),
		strings.TrimSpace(req.Msg.IconUrl))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to update community: %w", err))
	}
	s.communityCache.Invalidate(ctx, communityID)

	resp := connect.NewResponse(&pbv1.UpdateCommunityResponse{Community: toPBCommunity(community)})
	return resp, nil
}

// DeleteCommunity deletes a community. Only the owner can delete.
func (s *CommunityService) DeleteCommunity(
	ctx context.Context,
	req *connect.Request[pbv1.DeleteCommunityRequest],
) (*connect.Response[pbv1.DeleteCommunityResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	communityID := strings.TrimSpace(req.Msg.CommunityId)
	if communityID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("community_id is required"))
	}

	community, err := s.communityCache.Get(ctx, communityID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("community not found"))
	}
	if community.OwnerID != callerID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the community owner can delete the community"))
	}

	if err := s.repo.DeleteCommunity(ctx, communityID); err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to delete community: %w", err))
	}
	s.communityCache.Invalidate(ctx, communityID)
	s.membersCache.Invalidate(ctx, communityID)
	s.rolesCache.Invalidate(ctx, communityID)

	resp := connect.NewResponse(&pbv1.DeleteCommunityResponse{})
	return resp, nil
}

// ListCommunities lists communities the caller is a member of.
func (s *CommunityService) ListCommunities(
	ctx context.Context,
	req *connect.Request[pbv1.ListCommunitiesRequest],
) (*connect.Response[pbv1.ListCommunitiesResponse], error) {
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

	communities, nextCursor, err := s.repo.ListCommunitiesByUser(ctx, callerID, int(limit), cursor)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to list communities: %w", err))
	}

	pbCommunities := make([]*pbv1.Community, 0, len(communities))
	for _, comm := range communities {
		pbCommunities = append(pbCommunities, toPBCommunity(comm))
	}

	hasMore := nextCursor != ""
	resp := connect.NewResponse(&pbv1.ListCommunitiesResponse{
		Communities: pbCommunities,
		Pagination: &commonv1.PaginationResponse{
			HasMore: hasMore, NextCursor: nextCursor,
		},
	})
	return resp, nil
}

// CreateChannel creates a new channel in a community.
func (s *CommunityService) CreateChannel(
	ctx context.Context,
	req *connect.Request[pbv1.CreateChannelRequest],
) (*connect.Response[pbv1.CreateChannelResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	communityID := strings.TrimSpace(req.Msg.CommunityId)
	name := strings.TrimSpace(req.Msg.Name)
	if communityID == "" || name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("community_id and name are required"))
	}
	if err := s.checkPermission(ctx, communityID, callerID, PermissionManageChannels); err != nil {
		return nil, err
	}

	channelType := "text"
	if req.Msg.Type == pbv1.ChannelType_CHANNEL_TYPE_ANNOUNCEMENT {
		channelType = "announcement"
	}

	ch, err := s.repo.CreateChannel(ctx, communityID, name,
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
	if err := s.checkPermission(ctx, ch.CommunityID, callerID, PermissionManageChannels); err != nil {
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
	if err := s.checkPermission(ctx, ch.CommunityID, callerID, PermissionManageChannels); err != nil {
		return nil, err
	}
	if err := s.repo.DeleteChannel(ctx, channelID); err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to delete channel: %w", err))
	}
	resp := connect.NewResponse(&pbv1.DeleteChannelResponse{})
	return resp, nil
}

// ListChannels lists channels in a community.
func (s *CommunityService) ListChannels(
	ctx context.Context,
	req *connect.Request[pbv1.ListChannelsRequest],
) (*connect.Response[pbv1.ListChannelsResponse], error) {
	communityID := strings.TrimSpace(req.Msg.CommunityId)
	if communityID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("community_id is required"))
	}
	channels, err := s.repo.ListChannelsByCommunity(ctx, communityID)
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

// JoinCommunity adds the caller to a community.
func (s *CommunityService) JoinCommunity(
	ctx context.Context,
	req *connect.Request[pbv1.JoinCommunityRequest],
) (*connect.Response[pbv1.JoinCommunityResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	communityID := strings.TrimSpace(req.Msg.CommunityId)
	if communityID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("community_id is required"))
	}
	if _, err := s.communityCache.Get(ctx, communityID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("community not found"))
	}

	member, err := s.repo.AddMember(ctx, communityID, callerID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to join community: %w", err))
	}
	s.membersCache.Invalidate(ctx, communityID)

	// Auto-assign @everyone role to the new member.
	if defaultRole, rerr := s.repo.GetDefaultRole(ctx, communityID); rerr != nil {
		slog.Warn("failed to find @everyone role", "community_id", communityID, "error", rerr)
	} else {
		if err := s.repo.AssignRole(ctx, communityID, callerID, defaultRole.ID); err != nil {
			slog.Warn("failed to assign @everyone role", "community_id", communityID, "user_id", callerID, "error", err)
		}
	}

	// Publish member.joined event
	s.publishMemberJoined(ctx, communityID, callerID, member.Nickname)

	resp := connect.NewResponse(&pbv1.JoinCommunityResponse{
		Member: &pbv1.CommunityMember{
			CommunityId: member.CommunityID, UserId: member.UserID,
			Nickname: member.Nickname, JoinedAt: member.JoinedAt.Unix(),
		},
	})
	return resp, nil
}

// LeaveCommunity removes the caller from a community.
func (s *CommunityService) LeaveCommunity(
	ctx context.Context,
	req *connect.Request[pbv1.LeaveCommunityRequest],
) (*connect.Response[pbv1.LeaveCommunityResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	communityID := strings.TrimSpace(req.Msg.CommunityId)
	if communityID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("community_id is required"))
	}

	community, err := s.communityCache.Get(ctx, communityID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("community not found"))
	}
	if community.OwnerID == callerID {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("community owner cannot leave; transfer ownership or delete"))
	}

	if err := s.repo.RemoveMember(ctx, communityID, callerID); err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to leave community: %w", err))
	}
	s.membersCache.Invalidate(ctx, communityID)

	// Publish member.left event
	s.publishMemberLeft(ctx, communityID, callerID)

	resp := connect.NewResponse(&pbv1.LeaveCommunityResponse{})
	return resp, nil
}

// KickMember removes a target user from a community. Only the owner or a member
// with KickMembers permission can kick.
func (s *CommunityService) KickMember(
	ctx context.Context,
	req *connect.Request[pbv1.KickMemberRequest],
) (*connect.Response[pbv1.KickMemberResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	communityID := strings.TrimSpace(req.Msg.CommunityId)
	targetUserID := strings.TrimSpace(req.Msg.UserId)
	if communityID == "" || targetUserID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("community_id and user_id are required"))
	}

	community, err := s.communityCache.Get(ctx, communityID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("community not found"))
	}

	// Self-leave: caller is removing themselves.
	if targetUserID == callerID {
		if community.OwnerID == callerID {
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("community owner cannot leave; transfer ownership or delete"))
		}
	} else {
		// Kicking someone else: owner can always kick, others need KickMembers permission.
		if community.OwnerID != callerID {
			if err := s.checkPermission(ctx, communityID, callerID, PermissionKickMembers); err != nil {
				return nil, err
			}
		}
		// Cannot kick the owner.
		if targetUserID == community.OwnerID {
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("cannot kick the community owner"))
		}
	}

	if err := s.repo.RemoveMember(ctx, communityID, targetUserID); err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to kick member: %w", err))
	}
	s.membersCache.Invalidate(ctx, communityID)

	// Publish member.left event
	s.publishMemberLeft(ctx, communityID, targetUserID)

	resp := connect.NewResponse(&pbv1.KickMemberResponse{})
	return resp, nil
}

// ListMembers lists members of a community.
func (s *CommunityService) ListMembers(
	ctx context.Context,
	req *connect.Request[pbv1.ListMembersRequest],
) (*connect.Response[pbv1.ListMembersResponse], error) {
	communityID := strings.TrimSpace(req.Msg.CommunityId)
	if communityID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("community_id is required"))
	}

	limit := int32(50)
	var cursor string
	if req.Msg.Pagination != nil {
		if req.Msg.Pagination.Limit > 0 {
			limit = req.Msg.Pagination.Limit
		}
		cursor = req.Msg.Pagination.Cursor
	}

	members, nextCursor, err := s.repo.ListMembersByCommunity(ctx, communityID, int(limit), cursor)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to list members: %w", err))
	}

	pbMembers := make([]*pbv1.CommunityMember, 0, len(members))
	for _, m := range members {
		pbMembers = append(pbMembers, &pbv1.CommunityMember{
			CommunityId: m.CommunityID, UserId: m.UserID,
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

	members, err := s.membersCache.Get(ctx, ch.CommunityID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to check membership: %w", err))
	}
	if !cachedMembersSet(members)[callerID] {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("not a member of this community"))
	}

	if err := s.checkPermission(ctx, ch.CommunityID, callerID, PermissionSendMessages); err != nil {
		return nil, err
	}

	msg, err := s.repo.InsertChannelMessage(ctx, channelID, callerID, content)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to send message: %w", err))
	}

	// Insert attachments if file_ids are provided
	if len(req.Msg.FileIds) > 0 {
		attachments := make([]*AttachmentRow, 0, len(req.Msg.FileIds))
		for _, fileID := range req.Msg.FileIds {
			attachments = append(attachments, &AttachmentRow{
				MessageType: "channel",
				MessageID:   msg.ID,
				FileID:      fileID,
			})
		}
		if err := s.repo.InsertAttachments(ctx, attachments); err != nil {
			slog.Warn("failed to insert attachments", "error", err)
		}
	}

	// Publish message.created NATS event
	memberIDs := make([]string, 0, len(members))
	for _, m := range members {
		memberIDs = append(memberIDs, m.UserID)
	}
	s.publishMessageCreated(ctx, msg.ID, channelID, ch.CommunityID, callerID, content, msg.CreatedAt.Unix(), memberIDs)

	// Fetch attachments for the response.
	var pbAttachments []*commonv1.Attachment
	dbAttachments, aerr := s.repo.GetAttachmentsByMessage(ctx, "channel", msg.ID)
	if aerr != nil {
		slog.Warn("failed to fetch attachments for response", "error", aerr)
	}
	for _, a := range dbAttachments {
		pbAttachments = append(pbAttachments, &commonv1.Attachment{
			Id:          a.ID,
			FileId:      a.FileID,
			Filename:    a.Filename,
			ContentType: a.ContentType,
			Size:        a.Size,
		})
	}

	resp := connect.NewResponse(&pbv1.SendMessageResponse{
		Message: &pbv1.ChannelMessage{
			Id: msg.ID, ChannelId: msg.ChannelID, AuthorId: msg.AuthorID,
			Content: msg.Content, CreatedAt: msg.CreatedAt.Unix(),
			UpdatedAt: msg.UpdatedAt.Unix(),
			Attachments: pbAttachments,
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

	members, err := s.membersCache.Get(ctx, ch.CommunityID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to check membership: %w", err))
	}
	if !cachedMembersSet(members)[callerID] {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("not a member of this community"))
	}

	if err := s.checkPermission(ctx, ch.CommunityID, callerID, PermissionReadMessages); err != nil {
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
func (s *CommunityService) checkPermission(ctx context.Context, communityID, userID string, required int64) error {
	community, err := s.communityCache.Get(ctx, communityID)
	if err != nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("community not found"))
	}
	if community.OwnerID == userID {
		return nil
	}

	roles, err := s.repo.ListMemberRoles(ctx, communityID, userID)
	if err != nil {
		return connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to get member roles: %w", err))
	}

	member := &MemberRow{CommunityID: communityID, UserID: userID}
	perms := ComputePermissions(member, roles, community.OwnerID)

	if !HasPermission(perms, required) {
		return connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("insufficient permissions"))
	}
	return nil
}

func toPBCommunity(s *CommunityRow) *pbv1.Community {
	return &pbv1.Community{
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
		Id: c.ID, CommunityId: c.CommunityID, Name: c.Name, Topic: c.Topic,
		Type: chType, Position: c.Position,
		CreatedAt: c.CreatedAt.Unix(), UpdatedAt: c.UpdatedAt.Unix(),
	}
}

// publishMessageCreated publishes a constell.message.created NATS event.
func (s *CommunityService) publishMessageCreated(ctx context.Context, messageID, channelID, communityID, senderID, content string, createdAt int64, memberIDs []string) {
	if s.natsConn == nil {
		return
	}
	payload := map[string]interface{}{
		"message_id":   messageID,
		"channel_id":   channelID,
		"community_id": communityID,
		"sender_id":    senderID,
		"content":      content,
		"member_ids":   memberIDs,
		"created_at":   createdAt,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("marshal message.created event", "error", err)
		return
	}
	if err := s.natsConn.Publish("constell.message.created", data); err != nil {
		slog.Warn("publish message.created event", "error", err)
	}
}

// publishMemberJoined publishes a constell.member.joined NATS event.
func (s *CommunityService) publishMemberJoined(ctx context.Context, communityID, userID, nickname string) {
	if s.natsConn == nil {
		return
	}
	channels, err := s.repo.ListChannelsByCommunity(ctx, communityID)
	if err != nil {
		slog.Warn("list channels for member.joined event", "error", err)
		return
	}
	channelIDs := make([]string, 0, len(channels))
	for _, ch := range channels {
		channelIDs = append(channelIDs, ch.ID)
	}
	payload := map[string]interface{}{
		"community_id": communityID,
		"user_id":      userID,
		"nickname":     nickname,
		"channel_ids":  channelIDs,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("marshal member.joined event", "error", err)
		return
	}
	if err := s.natsConn.Publish("constell.member.joined", data); err != nil {
		slog.Warn("publish member.joined event", "error", err)
	}
}

// publishMemberLeft publishes a constell.member.left NATS event.
func (s *CommunityService) publishMemberLeft(ctx context.Context, communityID, userID string) {
	if s.natsConn == nil {
		return
	}
	channels, err := s.repo.ListChannelsByCommunity(ctx, communityID)
	if err != nil {
		slog.Warn("list channels for member.left event", "error", err)
		return
	}
	channelIDs := make([]string, 0, len(channels))
	for _, ch := range channels {
		channelIDs = append(channelIDs, ch.ID)
	}
	payload := map[string]interface{}{
		"community_id": communityID,
		"user_id":      userID,
		"channel_ids":  channelIDs,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("marshal member.left event", "error", err)
		return
	}
	if err := s.natsConn.Publish("constell.member.left", data); err != nil {
		slog.Warn("publish member.left event", "error", err)
	}
}
