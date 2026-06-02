package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
)

// ---------- NATS event types ----------

// DMCreatedEvent represents a new DM message event consumed from NATS.
type DMCreatedEvent struct {
	SenderID       string `json:"sender_id"`
	ReceiverID     string `json:"receiver_id"`
	ConversationID string `json:"conversation_id"`
	Content        string `json:"content"`
	CreatedAt      int64  `json:"created_at"`
}

// MessageCreatedEvent represents a new channel message event consumed from NATS.
type MessageCreatedEvent struct {
	MessageID string   `json:"message_id"`
	ChannelID string   `json:"channel_id"`
	ServerID  string   `json:"server_id"`
	SenderID  string   `json:"sender_id"`
	Content   string   `json:"content"`
	MemberIDs []string `json:"member_ids"`
	CreatedAt int64    `json:"created_at"`
}

// MemberJoinedEvent represents a member joining a server.
type MemberJoinedEvent struct {
	ServerID   string   `json:"server_id"`
	UserID     string   `json:"user_id"`
	ChannelIDs []string `json:"channel_ids"`
}

// MemberLeftEvent represents a member leaving a server.
type MemberLeftEvent struct {
	ServerID   string   `json:"server_id"`
	UserID     string   `json:"user_id"`
	ChannelIDs []string `json:"channel_ids"`
}

// ---------- Push payload ----------

// NotificationPush matches the WS Gateway PushPayload format.
type NotificationPush struct {
	Targets   []string               `json:"targets"`
	EventType string                 `json:"event_type"`
	Payload   map[string]interface{} `json:"payload"`
}

// ---------- Subscriber ----------

// Subscriber consumes events from NATS JetStream and updates the Redis store
// plus pushes real-time notifications through the WS Gateway.
type Subscriber struct {
	nc    *nats.Conn
	js    nats.JetStreamContext
	store *Store
}

// NewSubscriber creates a new Subscriber.
func NewSubscriber(nc *nats.Conn, js nats.JetStreamContext, store *Store) *Subscriber {
	return &Subscriber{
		nc:    nc,
		js:    js,
		store: store,
	}
}

// SubscribeAll registers durable JetStream consumers for all notification subjects.
func (s *Subscriber) SubscribeAll() error {
	subscriptions := []struct {
		subject string
		handler nats.MsgHandler
	}{
		{"constell.dm.created", s.handleDMCreated},
		{"constell.message.created", s.handleMessageCreated},
		{"constell.member.joined", s.handleMemberJoined},
		{"constell.member.left", s.handleMemberLeft},
	}

	for _, sub := range subscriptions {
		durable := "notify-" + strings.ReplaceAll(sub.subject, ".", "-")
		_, err := s.js.Subscribe(sub.subject, sub.handler,
			nats.Durable(durable),
			nats.ManualAck(),
		)
		if err != nil {
			return fmt.Errorf("subscribe to %s: %w", sub.subject, err)
		}
		slog.Info("subscribed to NATS subject", "subject", sub.subject, "durable", durable)
	}

	return nil
}

// ---------- Event handlers ----------

func (s *Subscriber) handleDMCreated(msg *nats.Msg) {
	ctx := context.Background()

	var evt DMCreatedEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		slog.Error("unmarshal DM event", "error", err)
		msg.Ack()
		return
	}

	// 1. INCR dm_msg_count for the conversation.
	if err := s.store.IncrementDMMsgCount(ctx, evt.ConversationID); err != nil {
		slog.Error("increment DM count", "error", err, "conv_id", evt.ConversationID)
	}

	// 2. SADD conversation to both sender and receiver.
	if err := s.store.AddConversationToUser(ctx, evt.SenderID, evt.ConversationID); err != nil {
		slog.Error("add conversation to sender", "error", err)
	}
	if err := s.store.AddConversationToUser(ctx, evt.ReceiverID, evt.ConversationID); err != nil {
		slog.Error("add conversation to receiver", "error", err)
	}

	// 3. Push notification to the receiver.
	push := NotificationPush{
		Targets:   []string{evt.ReceiverID},
		EventType: "DM_RECEIVED",
		Payload: map[string]interface{}{
			"message_id": fmt.Sprintf("dm-%s-%d", evt.ConversationID, evt.CreatedAt),
			"sender_id":  evt.SenderID,
			"content":    truncate(evt.Content, 200),
			"created_at": evt.CreatedAt,
		},
	}
	s.pushToUser(ctx, evt.ReceiverID, push)

	msg.Ack()
}

func (s *Subscriber) handleMessageCreated(msg *nats.Msg) {
	ctx := context.Background()

	var evt MessageCreatedEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		slog.Error("unmarshal message event", "error", err)
		msg.Ack()
		return
	}

	// 1. INCR channel_msg_count.
	if err := s.store.IncrementChannelMsgCount(ctx, evt.ChannelID); err != nil {
		slog.Error("increment channel count", "error", err, "channel_id", evt.ChannelID)
	}

	// 2. Push notification to all members except sender.
	targets := make([]string, 0, len(evt.MemberIDs))
	for _, mid := range evt.MemberIDs {
		if mid != evt.SenderID {
			targets = append(targets, mid)
		}
	}

	if len(targets) > 0 {
		push := NotificationPush{
			Targets:   targets,
			EventType: "CHANNEL_MESSAGE_RECEIVED",
			Payload: map[string]interface{}{
				"message_id": evt.MessageID,
				"channel_id": evt.ChannelID,
				"sender_id":  evt.SenderID,
				"content":    truncate(evt.Content, 200),
				"created_at": evt.CreatedAt,
			},
		}
		for _, target := range targets {
			s.pushToUser(ctx, target, push)
		}
	}

	msg.Ack()
}

func (s *Subscriber) handleMemberJoined(msg *nats.Msg) {
	ctx := context.Background()

	var evt MemberJoinedEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		slog.Error("unmarshal member joined event", "error", err)
		msg.Ack()
		return
	}

	// SADD channels to user's channel set.
	if err := s.store.AddChannelsToUser(ctx, evt.UserID, evt.ChannelIDs); err != nil {
		slog.Error("add channels to user", "error", err, "user_id", evt.UserID)
	}

	msg.Ack()
}

func (s *Subscriber) handleMemberLeft(msg *nats.Msg) {
	ctx := context.Background()

	var evt MemberLeftEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		slog.Error("unmarshal member left event", "error", err)
		msg.Ack()
		return
	}

	// SREM channels from user's channel set.
	if err := s.store.RemoveChannelsFromUser(ctx, evt.UserID, evt.ChannelIDs); err != nil {
		slog.Error("remove channels from user", "error", err, "user_id", evt.UserID)
	}

	msg.Ack()
}

// ---------- Push ----------

// pushToUser looks up which WS Gateway instance a user is connected to and
// publishes a push notification on the appropriate NATS subject.
func (s *Subscriber) pushToUser(ctx context.Context, userID string, payload NotificationPush) {
	// Look up the user's gateway via the ws-gateway registry key pattern: ws:uid:{userID}.
	gwID, err := s.store.rdb.Get(ctx, "ws:uid:"+userID).Result()
	if err != nil {
		// User is offline — nothing to push. This is not an error.
		return
	}

	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("marshal push payload", "error", err)
		return
	}

	topic := "gw.push." + gwID
	if err := s.nc.Publish(topic, data); err != nil {
		slog.Error("publish push", "error", err, "topic", topic, "user_id", userID)
	}
}

// truncate limits a string to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
