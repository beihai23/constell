package main

import (
	"context"
	"fmt"

	gatewayv1 "github.com/constell/constell/backend/pkg/proto/gateway/v1"
)

// UserSvcClient defines the interface for calling User Service RPCs.
type UserSvcClient interface {
	SendDM(ctx context.Context, senderID, receiverID, content string) (messageID string, createdAt string, err error)
}

// CommunitySvcClient defines the interface for calling Community Service RPCs.
type CommunitySvcClient interface {
	SendMessage(ctx context.Context, senderID, channelID, content string) (messageID string, createdAt string, err error)
}

// Router translates gateway-layer client messages into Connect-RPC calls.
type Router struct {
	userClient      UserSvcClient
	communityClient CommunitySvcClient
	connMgr         *ConnManager
}

// NewRouter creates a new Router.
func NewRouter(userClient UserSvcClient, communityClient CommunitySvcClient, connMgr *ConnManager) *Router {
	return &Router{
		userClient:      userClient,
		communityClient: communityClient,
		connMgr:         connMgr,
	}
}

// Route dispatches a client message to the appropriate handler.
func (r *Router) Route(ctx context.Context, userID string, msg *gatewayv1.ClientMessage) (*gatewayv1.ServerEvent, error) {
	switch msg.Type {
	case gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_SEND_DM:
		return r.handleSendDM(ctx, userID, msg)
	case gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_SEND_CHANNEL_MESSAGE:
		return r.handleSendChannelMessage(ctx, userID, msg)
	case gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_SUBSCRIBE_CHANNEL:
		return r.handleSubscribeChannel(userID, msg)
	case gatewayv1.ClientMessageType_CLIENT_MESSAGE_TYPE_UNSUBSCRIBE_CHANNEL:
		return r.handleUnsubscribeChannel(userID, msg)
	default:
		return nil, fmt.Errorf("unknown message type: %v", msg.Type)
	}
}

func (r *Router) handleSendDM(ctx context.Context, userID string, msg *gatewayv1.ClientMessage) (*gatewayv1.ServerEvent, error) {
	req := msg.SendDmRequest
	if req == nil {
		return nil, fmt.Errorf("send_dm_request is nil")
	}
	if req.ReceiverId == "" {
		return nil, fmt.Errorf("receiver_id is required")
	}
	if req.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	_, _, err := r.userClient.SendDM(ctx, userID, req.ReceiverId, req.Content)
	if err != nil {
		return nil, fmt.Errorf("user service SendDM: %w", err)
	}

	return &gatewayv1.ServerEvent{
		Type:      gatewayv1.ServerEventType_SERVER_EVENT_TYPE_ACK,
		RequestId: msg.RequestId,
	}, nil
}

func (r *Router) handleSendChannelMessage(ctx context.Context, userID string, msg *gatewayv1.ClientMessage) (*gatewayv1.ServerEvent, error) {
	req := msg.SendChannelMessageRequest
	if req == nil {
		return nil, fmt.Errorf("send_channel_message_request is nil")
	}
	if req.ChannelId == "" {
		return nil, fmt.Errorf("channel_id is required")
	}
	if req.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	_, _, err := r.communityClient.SendMessage(ctx, userID, req.ChannelId, req.Content)
	if err != nil {
		return nil, fmt.Errorf("community service SendMessage: %w", err)
	}

	return &gatewayv1.ServerEvent{
		Type:      gatewayv1.ServerEventType_SERVER_EVENT_TYPE_ACK,
		RequestId: msg.RequestId,
	}, nil
}

func (r *Router) handleSubscribeChannel(userID string, msg *gatewayv1.ClientMessage) (*gatewayv1.ServerEvent, error) {
	req := msg.SubscribeChannelRequest
	if req == nil {
		return nil, fmt.Errorf("subscribe_channel_request is nil")
	}
	if req.ChannelId == "" {
		return nil, fmt.Errorf("channel_id is required")
	}

	r.connMgr.AddSubscribedChannel(userID, req.ChannelId)

	return &gatewayv1.ServerEvent{
		Type:      gatewayv1.ServerEventType_SERVER_EVENT_TYPE_ACK,
		RequestId: msg.RequestId,
	}, nil
}

func (r *Router) handleUnsubscribeChannel(userID string, msg *gatewayv1.ClientMessage) (*gatewayv1.ServerEvent, error) {
	req := msg.UnsubscribeChannelRequest
	if req == nil {
		return nil, fmt.Errorf("unsubscribe_channel_request is nil")
	}
	if req.ChannelId == "" {
		return nil, fmt.Errorf("channel_id is required")
	}

	r.connMgr.RemoveSubscribedChannel(userID, req.ChannelId)

	return &gatewayv1.ServerEvent{
		Type:      gatewayv1.ServerEventType_SERVER_EVENT_TYPE_ACK,
		RequestId: msg.RequestId,
	}, nil
}
