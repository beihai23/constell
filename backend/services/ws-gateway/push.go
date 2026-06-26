package main

import (
	"encoding/json"
	"fmt"
	"log"

	gatewayv1 "github.com/constell/constell/backend/pkg/proto/gateway/v1"
	"github.com/nats-io/nats.go"
)

// PushPayload represents the NATS message payload for gw.push.{gw_id} events.
type PushPayload struct {
	Targets   []string               `json:"targets"`
	EventType string                 `json:"event_type"`
	Payload   map[string]interface{} `json:"payload"`
}

// PushSubscriber subscribes to the gateway's NATS push topic and delivers events to local connections.
type PushSubscriber struct {
	nc      *nats.Conn
	connMgr *ConnManager
	sub     *nats.Subscription
}

// NewPushSubscriber creates a new PushSubscriber.
func NewPushSubscriber(nc *nats.Conn, connMgr *ConnManager) *PushSubscriber {
	return &PushSubscriber{
		nc:      nc,
		connMgr: connMgr,
	}
}

func pushTopic(gwID string) string {
	return "gw.push." + gwID
}

// Subscribe starts listening for push messages on the gateway's NATS topic.
func (ps *PushSubscriber) Subscribe(gwID string) error {
	if ps.nc == nil {
		return fmt.Errorf("NATS connection is nil")
	}

	topic := pushTopic(gwID)
	sub, err := ps.nc.Subscribe(topic, ps.handleNATSMessage)
	if err != nil {
		return fmt.Errorf("subscribe to %s: %w", topic, err)
	}

	ps.sub = sub
	log.Printf("subscribed to push topic: %s", topic)
	return nil
}

// Unsubscribe stops listening for push messages.
func (ps *PushSubscriber) Unsubscribe() error {
	if ps.sub == nil {
		return nil
	}
	if err := ps.sub.Unsubscribe(); err != nil {
		return fmt.Errorf("unsubscribe: %w", err)
	}
	ps.sub = nil
	return nil
}

func (ps *PushSubscriber) handleNATSMessage(msg *nats.Msg) {
	var payload PushPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		log.Printf("failed to unmarshal push payload: %v", err)
		return
	}

	delivered := ps.DeliverToLocal(payload)
	log.Printf("push delivered: targets=%d delivered=%d type=%s",
		len(payload.Targets), delivered, payload.EventType)
}

// DeliverToLocal looks up target users and writes the event to each matching WebSocket.
func (ps *PushSubscriber) DeliverToLocal(payload PushPayload) int {
	event, err := ps.buildServerEvent(payload)
	if err != nil {
		log.Printf("failed to build server event: %v", err)
		return 0
	}

	delivered := 0
	for _, targetUserID := range payload.Targets {
		entry, ok := ps.connMgr.Get(targetUserID)
		if !ok {
			continue
		}

		if err := WriteMessage(entry.Conn, event); err != nil {
			log.Printf("failed to write to user %s: %v", targetUserID, err)
			continue
		}
		delivered++
	}

	return delivered
}

func (ps *PushSubscriber) buildServerEvent(payload PushPayload) (*gatewayv1.ServerEvent, error) {
	switch payload.EventType {
	case "DM_RECEIVED":
		return ps.buildDMReceivedEvent(payload.Payload)
	case "CHANNEL_MESSAGE_RECEIVED":
		return ps.buildChannelMessageEvent(payload.Payload)
	case "USER_ONLINE":
		return ps.buildUserOnlineEvent(payload.Payload)
	case "USER_OFFLINE":
		return ps.buildUserOfflineEvent(payload.Payload)
	case "NOTIFICATION":
		return ps.buildNotificationEvent(payload.Payload)
	default:
		return nil, fmt.Errorf("unknown event type: %s", payload.EventType)
	}
}

func (ps *PushSubscriber) buildDMReceivedEvent(p map[string]interface{}) (*gatewayv1.ServerEvent, error) {
	return &gatewayv1.ServerEvent{
		Type: gatewayv1.ServerEventType_SERVER_EVENT_TYPE_DM_RECEIVED,
		DmReceivedEvent: &gatewayv1.DMReceivedEvent{
			MessageId:      getStringField(p, "message_id"),
			SenderId:       getStringField(p, "sender_id"),
			SenderNickname: getStringField(p, "sender_nickname"),
			Content:        getStringField(p, "content"),
			CreatedAt:      getInt64Field(p, "created_at"),
			Seq:            getInt64Field(p, "seq"),
		},
	}, nil
}

func (ps *PushSubscriber) buildChannelMessageEvent(p map[string]interface{}) (*gatewayv1.ServerEvent, error) {
	return &gatewayv1.ServerEvent{
		Type: gatewayv1.ServerEventType_SERVER_EVENT_TYPE_CHANNEL_MESSAGE_RECEIVED,
		ChannelMessageEvent: &gatewayv1.ChannelMessageReceivedEvent{
			MessageId:      getStringField(p, "message_id"),
			ChannelId:      getStringField(p, "channel_id"),
			SenderId:       getStringField(p, "sender_id"),
			SenderNickname: getStringField(p, "sender_nickname"),
			Content:        getStringField(p, "content"),
			CreatedAt:      getInt64Field(p, "created_at"),
			Seq:            getInt64Field(p, "seq"),
		},
	}, nil
}

func (ps *PushSubscriber) buildUserOnlineEvent(p map[string]interface{}) (*gatewayv1.ServerEvent, error) {
	return &gatewayv1.ServerEvent{
		Type: gatewayv1.ServerEventType_SERVER_EVENT_TYPE_USER_ONLINE,
		UserOnlineEvent: &gatewayv1.UserOnlineEvent{
			UserId: getStringField(p, "user_id"),
		},
	}, nil
}

func (ps *PushSubscriber) buildUserOfflineEvent(p map[string]interface{}) (*gatewayv1.ServerEvent, error) {
	return &gatewayv1.ServerEvent{
		Type: gatewayv1.ServerEventType_SERVER_EVENT_TYPE_USER_OFFLINE,
		UserOfflineEvent: &gatewayv1.UserOfflineEvent{
			UserId: getStringField(p, "user_id"),
		},
	}, nil
}

func (ps *PushSubscriber) buildNotificationEvent(p map[string]interface{}) (*gatewayv1.ServerEvent, error) {
	return &gatewayv1.ServerEvent{
		Type: gatewayv1.ServerEventType_SERVER_EVENT_TYPE_NOTIFICATION,
		NotificationEvent: &gatewayv1.NotificationEvent{
			NotificationType: getStringField(p, "notification_type"),
			SourceId:         getStringField(p, "source_id"),
			CommunityId:      getStringField(p, "community_id"),
			SenderId:         getStringField(p, "sender_id"),
			SenderNickname:   getStringField(p, "sender_nickname"),
			Preview:          getStringField(p, "preview"),
			CreatedAt:        getInt64Field(p, "created_at"),
		},
	}, nil
}

func getStringField(m map[string]interface{}, key string) string {
	val, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := val.(string)
	if !ok {
		return ""
	}
	return s
}

func getInt64Field(m map[string]interface{}, key string) int64 {
	val, ok := m[key]
	if !ok {
		return 0
	}
	f, ok := val.(float64)
	if !ok {
		return 0
	}
	return int64(f)
}
