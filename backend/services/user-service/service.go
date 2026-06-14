package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"connectrpc.com/connect"

	"github.com/nats-io/nats.go"

	commonv1 "github.com/constell/constell/backend/pkg/proto/common/v1"
	pbv1 "github.com/constell/constell/backend/pkg/proto/user/v1"
	"github.com/constell/constell/backend/pkg/proto/user/v1/userv1connect"
	"github.com/constell/constell/backend/pkg/middleware"
)

// UserService implements the Connect-RPC UserServiceHandler.
type UserService struct {
	repo          *Repository
	userCache     UserCacheReader
	relationCache RelationCacheReader
	userWriter    UserCacheWriter
	natsConn      *nats.Conn
}

// NewUserService creates a new UserService.
func NewUserService(
	repo *Repository,
	userCache *UserCache,
	relationCache *RelationCache,
	natsConn *nats.Conn,
) *UserService {
	return &UserService{
		repo:          repo,
		userCache:     userCache,
		relationCache: relationCache,
		userWriter:    userCache,
		natsConn:      natsConn,
	}
}

var _ userv1connect.UserServiceHandler = (*UserService)(nil)

// GetUser returns a user's profile.
func (s *UserService) GetUser(
	ctx context.Context,
	req *connect.Request[pbv1.GetUserRequest],
) (*connect.Response[pbv1.GetUserResponse], error) {
	userID := strings.TrimSpace(req.Msg.UserId)
	if userID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("user_id is required"))
	}

	user, err := s.userCache.Get(ctx, userID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound,
			fmt.Errorf("user not found: %w", err))
	}

	resp := connect.NewResponse(&pbv1.GetUserResponse{
		Id: user.ID, Email: user.Email, Nickname: user.Nickname,
		AvatarUrl: user.AvatarURL, StatusMessage: user.StatusMessage,
		CreatedAt: user.CreatedAt.Unix(), UpdatedAt: user.UpdatedAt.Unix(),
	})
	return resp, nil
}

// UpdateProfile modifies the current user's profile.
func (s *UserService) UpdateProfile(
	ctx context.Context,
	req *connect.Request[pbv1.UpdateProfileRequest],
) (*connect.Response[pbv1.UpdateProfileResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}

	msg := req.Msg
	err := s.repo.UpdateUserProfile(ctx, callerID,
		strings.TrimSpace(msg.Nickname),
		strings.TrimSpace(msg.AvatarUrl),
		strings.TrimSpace(msg.StatusMessage))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to update profile: %w", err))
	}

	s.userWriter.Invalidate(ctx, callerID)

	user, err := s.repo.GetUserByID(ctx, callerID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to fetch updated profile: %w", err))
	}

	resp := connect.NewResponse(&pbv1.UpdateProfileResponse{
		User: &pbv1.GetUserProfile{
			Id: user.ID, Email: user.Email, Nickname: user.Nickname,
			AvatarUrl: user.AvatarURL, StatusMessage: user.StatusMessage,
			CreatedAt: user.CreatedAt.Unix(), UpdatedAt: user.UpdatedAt.Unix(),
		},
	})
	return resp, nil
}

// ListFriends returns the current user's friend list.
func (s *UserService) ListFriends(
	ctx context.Context,
	req *connect.Request[pbv1.ListFriendsRequest],
) (*connect.Response[pbv1.ListFriendsResponse], error) {
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

	friends, nextCursor, err := s.repo.ListFriends(ctx, callerID, int(limit), cursor)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to list friends: %w", err))
	}

	pbFriends := make([]*commonv1.UserBrief, 0, len(friends))
	for _, f := range friends {
		pbFriends = append(pbFriends, &commonv1.UserBrief{
			Id: f.ID, Nickname: f.Nickname,
		})
	}

	hasMore := nextCursor != ""
	resp := connect.NewResponse(&pbv1.ListFriendsResponse{
		Friends: pbFriends,
		Pagination: &commonv1.PaginationResponse{
			HasMore: hasMore, NextCursor: nextCursor,
		},
	})
	return resp, nil
}

// SendDM sends a direct message to another user.
func (s *UserService) SendDM(
	ctx context.Context,
	req *connect.Request[pbv1.SendDMRequest],
) (*connect.Response[pbv1.SendDMResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}

	targetUserID := strings.TrimSpace(req.Msg.TargetUserId)
	content := strings.TrimSpace(req.Msg.Content)
	if targetUserID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("target_user_id is required"))
	}
	if content == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("content is required"))
	}

	// Check blocklist in both directions.
	rel, err := s.relationCache.Get(ctx, targetUserID, callerID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to check blocklist: %w", err))
	}
	if rel != nil && rel.Type == "blocked" {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("cannot send DM to this user"))
	}

	rel2, err := s.relationCache.Get(ctx, callerID, targetUserID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to check blocklist: %w", err))
	}
	if rel2 != nil && rel2.Type == "blocked" {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("cannot send DM to a blocked user"))
	}

	conv, err := s.repo.GetOrCreateConversation(ctx, callerID, targetUserID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to get conversation: %w", err))
	}

	msg, err := s.repo.InsertDMMessage(ctx, conv.ID, callerID, content)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to send DM: %w", err))
	}

	// Insert attachments if file_ids are provided
	if len(req.Msg.FileIds) > 0 {
		attachments := make([]*AttachmentRow, 0, len(req.Msg.FileIds))
		for _, fileID := range req.Msg.FileIds {
			attachments = append(attachments, &AttachmentRow{
				MessageType: "dm",
				MessageID:   msg.ID,
				FileID:      fileID,
			})
		}
		if err := s.repo.InsertAttachments(ctx, attachments); err != nil {
			slog.Warn("failed to insert attachments", "error", err)
		}
	}

	// Publish dm.created NATS event
	s.publishDMCreated(ctx, callerID, targetUserID, conv.ID, content, msg.CreatedAt.Unix())

	resp := connect.NewResponse(&pbv1.SendDMResponse{
		Message: &pbv1.DMMessage{
			Id: msg.ID, ConversationId: msg.ConversationID,
			SenderId: msg.SenderID, Content: msg.Content,
			CreatedAt: msg.CreatedAt.Unix(),
		},
	})
	return resp, nil
}

// GetDMHistory retrieves DM history with a specific user.
func (s *UserService) GetDMHistory(
	ctx context.Context,
	req *connect.Request[pbv1.GetDMHistoryRequest],
) (*connect.Response[pbv1.GetDMHistoryResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}

	targetUserID := strings.TrimSpace(req.Msg.TargetUserId)
	if targetUserID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("target_user_id is required"))
	}

	conv, err := s.repo.GetOrCreateConversation(ctx, callerID, targetUserID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to get conversation: %w", err))
	}

	limit := int32(50)
	var cursor string
	if req.Msg.Pagination != nil {
		if req.Msg.Pagination.Limit > 0 {
			limit = req.Msg.Pagination.Limit
		}
		cursor = req.Msg.Pagination.Cursor
	}

	pbMessages := make([]*pbv1.DMMessage, 0)
	var nextCursor string

	if req.Msg.SinceSeq > 0 {
		// Backfill path: messages newer than since_seq, ascending.
		sinceMsgs, err := s.repo.GetDMMessagesSince(ctx, conv.ID, req.Msg.SinceSeq, int(limit))
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal,
				fmt.Errorf("failed to get DM messages since: %w", err))
		}
		for _, m := range sinceMsgs {
			pbMessages = append(pbMessages, &pbv1.DMMessage{
				Id: m.ID, ConversationId: m.ConversationID,
				SenderId: m.SenderID, Content: m.Content,
				CreatedAt: m.CreatedAt.Unix(), Seq: m.Seq,
			})
		}
	} else {
		// History-scroll path: cursor pagination, descending.
		messages, c, err := s.repo.GetDMHistory(ctx, conv.ID, int(limit), cursor)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal,
				fmt.Errorf("failed to get DM history: %w", err))
		}
		nextCursor = c
		for _, m := range messages {
			pbMessages = append(pbMessages, &pbv1.DMMessage{
				Id: m.ID, ConversationId: m.ConversationID,
				SenderId: m.SenderID, Content: m.Content,
				CreatedAt: m.CreatedAt.Unix(), Seq: m.Seq,
			})
		}
	}

	hasMore := nextCursor != ""
	resp := connect.NewResponse(&pbv1.GetDMHistoryResponse{
		Messages: pbMessages,
		Pagination: &commonv1.PaginationResponse{
			HasMore: hasMore, NextCursor: nextCursor,
		},
	})
	return resp, nil
}

// GetDMConversations lists the caller's DM conversations. The peer_id is the
// other party in each conversation.
func (s *UserService) GetDMConversations(
	ctx context.Context,
	req *connect.Request[pbv1.GetDMConversationsRequest],
) (*connect.Response[pbv1.GetDMConversationsResponse], error) {
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

	convos, nextCursor, err := s.repo.GetDMConversations(ctx, callerID, int(limit), cursor)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to get DM conversations: %w", err))
	}

	pbConvos := make([]*pbv1.DMConversation, 0, len(convos))
	for _, c := range convos {
		peerID := c.UserBID
		if c.UserAID != callerID {
			peerID = c.UserAID
		}
		pbConvos = append(pbConvos, &pbv1.DMConversation{
			Id: c.ID, PeerId: peerID, CreatedAt: c.CreatedAt.Unix(),
		})
	}

	resp := connect.NewResponse(&pbv1.GetDMConversationsResponse{
		Conversations: pbConvos,
		Pagination: &commonv1.PaginationResponse{
			HasMore: nextCursor != "", NextCursor: nextCursor,
		},
	})
	return resp, nil
}

// publishDMCreated publishes a constell.dm.created NATS event.
func (s *UserService) publishDMCreated(ctx context.Context, senderID, receiverID, conversationID, content string, createdAt int64) {
	if s.natsConn == nil {
		return
	}
	payload := map[string]interface{}{
		"sender_id":       senderID,
		"receiver_id":     receiverID,
		"conversation_id": conversationID,
		"content":         content,
		"created_at":      createdAt,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("marshal dm.created event", "error", err)
		return
	}
	if err := s.natsConn.Publish("constell.dm.created", data); err != nil {
		slog.Warn("publish dm.created event", "error", err)
	}
}
