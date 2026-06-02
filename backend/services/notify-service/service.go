package main

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	notifyv1 "github.com/constell/constell/backend/pkg/proto/notify/v1"
	"github.com/constell/constell/backend/pkg/proto/notify/v1/notifyv1connect"
	"github.com/constell/constell/backend/pkg/middleware"
)

// NotifyService implements the NotifyServiceHandler interface.
type NotifyService struct {
	store *Store
}

// Verify interface compliance at compile time.
var _ notifyv1connect.NotifyServiceHandler = (*NotifyService)(nil)

// NewNotifyService creates a new NotifyService backed by the given Store.
func NewNotifyService(store *Store) *NotifyService {
	return &NotifyService{store: store}
}

// GetUnreadCounts returns all unread channel and DM counts for the authenticated user.
func (s *NotifyService) GetUnreadCounts(
	ctx context.Context,
	req *connect.Request[notifyv1.GetUnreadCountsRequest],
) (*connect.Response[notifyv1.GetUnreadCountsResponse], error) {
	userID := middleware.UserIDFromContext(ctx)
	if userID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("missing user ID"))
	}

	// Fetch unread channels.
	unreadChannels, err := s.store.GetUnreadChannels(ctx, userID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get unread channels: %w", err))
	}

	// Fetch unread DMs.
	unreadDMs, err := s.store.GetUnreadDMs(ctx, userID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get unread DMs: %w", err))
	}

	// Convert to proto types and compute totals.
	var dmTotal int32
	dmConversations := make([]*notifyv1.UnreadDMConversation, 0, len(unreadDMs))
	for _, dm := range unreadDMs {
		dmTotal += dm.Count
		dmConversations = append(dmConversations, &notifyv1.UnreadDMConversation{
			ConversationId: dm.ConversationID,
			PeerId:         dm.PeerID,
			Count:          dm.Count,
		})
	}

	var channelTotal int32
	channels := make([]*notifyv1.UnreadChannel, 0, len(unreadChannels))
	for _, ch := range unreadChannels {
		channelTotal += ch.Count
		channels = append(channels, &notifyv1.UnreadChannel{
			ChannelId: ch.ChannelID,
			ServerId:  ch.ServerID,
			Count:     ch.Count,
		})
	}

	resp := &notifyv1.GetUnreadCountsResponse{
		DmTotal:         dmTotal,
		DmConversations: dmConversations,
		ChannelTotal:    channelTotal,
		Channels:        channels,
	}

	return connect.NewResponse(resp), nil
}

// MarkDMRead marks all messages in a DM conversation as read for the authenticated user.
func (s *NotifyService) MarkDMRead(
	ctx context.Context,
	req *connect.Request[notifyv1.MarkDMReadRequest],
) (*connect.Response[notifyv1.MarkDMReadResponse], error) {
	userID := middleware.UserIDFromContext(ctx)
	if userID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("missing user ID"))
	}

	convID := req.Msg.GetConversationId()
	if convID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("conversation_id is required"))
	}

	if err := s.store.MarkDMRead(ctx, userID, convID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("mark DM read: %w", err))
	}

	return connect.NewResponse(&notifyv1.MarkDMReadResponse{}), nil
}

// MarkChannelRead marks all messages in a channel as read for the authenticated user.
func (s *NotifyService) MarkChannelRead(
	ctx context.Context,
	req *connect.Request[notifyv1.MarkChannelReadRequest],
) (*connect.Response[notifyv1.MarkChannelReadResponse], error) {
	userID := middleware.UserIDFromContext(ctx)
	if userID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("missing user ID"))
	}

	channelID := req.Msg.GetChannelId()
	if channelID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("channel_id is required"))
	}

	if err := s.store.MarkChannelRead(ctx, userID, channelID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("mark channel read: %w", err))
	}

	return connect.NewResponse(&notifyv1.MarkChannelReadResponse{}), nil
}
