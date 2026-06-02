package handlers

import (
	"net/http"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"

	notifyv1 "github.com/constell/constell/backend/pkg/proto/notify/v1"
	notifyv1connect "github.com/constell/constell/backend/pkg/proto/notify/v1/notifyv1connect"
)

// NotifyHandler handles REST API requests for notification operations.
type NotifyHandler struct {
	client notifyv1connect.NotifyServiceClient
}

// NewNotifyHandler creates a new NotifyHandler.
func NewNotifyHandler(client notifyv1connect.NotifyServiceClient) *NotifyHandler {
	return &NotifyHandler{client: client}
}

// unreadDMConversation is the JSON representation of an unread DM conversation.
type unreadDMConversation struct {
	ConversationID string `json:"conversation_id"`
	PeerID         string `json:"peer_id"`
	Count          int32  `json:"count"`
}

// unreadChannel is the JSON representation of an unread channel.
type unreadChannel struct {
	ChannelID string `json:"channel_id"`
	ServerID  string `json:"server_id"`
	Count     int32  `json:"count"`
}

// getUnreadResponse is the JSON response for GET /api/v1/notify/unread.
type getUnreadResponse struct {
	DMTotal        int32                 `json:"dm_total"`
	DMConversations []unreadDMConversation `json:"dm_conversations"`
	ChannelTotal   int32                 `json:"channel_total"`
	Channels       []unreadChannel       `json:"channels"`
}

// GetUnread handles GET /api/v1/notify/unread.
func (h *NotifyHandler) GetUnread(w http.ResponseWriter, r *http.Request) {
	cr := connect.NewRequest(&notifyv1.GetUnreadCountsRequest{})
	forwardAuth(r, cr)

	resp, err := h.client.GetUnreadCounts(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	msg := resp.Msg
	dmConvs := make([]unreadDMConversation, 0, len(msg.DmConversations))
	for _, c := range msg.DmConversations {
		dmConvs = append(dmConvs, unreadDMConversation{
			ConversationID: c.GetConversationId(),
			PeerID:         c.GetPeerId(),
			Count:          c.GetCount(),
		})
	}

	channels := make([]unreadChannel, 0, len(msg.Channels))
	for _, c := range msg.Channels {
		channels = append(channels, unreadChannel{
			ChannelID: c.GetChannelId(),
			ServerID:  c.GetServerId(),
			Count:     c.GetCount(),
		})
	}

	writeJSON(w, http.StatusOK, getUnreadResponse{
		DMTotal:         msg.GetDmTotal(),
		DMConversations: dmConvs,
		ChannelTotal:    msg.GetChannelTotal(),
		Channels:        channels,
	})
}

// MarkDMRead handles POST /api/v1/notify/dm/{conv_id}/read.
func (h *NotifyHandler) MarkDMRead(w http.ResponseWriter, r *http.Request) {
	convID := chi.URLParam(r, "conv_id")
	if convID == "" {
		writeError(w, http.StatusBadRequest, "conversation id is required")
		return
	}

	cr := connect.NewRequest(&notifyv1.MarkDMReadRequest{
		ConversationId: convID,
	})
	forwardAuth(r, cr)

	_, err := h.client.MarkDMRead(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "marked_read"})
}

// MarkChannelRead handles POST /api/v1/notify/channel/{ch_id}/read.
func (h *NotifyHandler) MarkChannelRead(w http.ResponseWriter, r *http.Request) {
	chID := chi.URLParam(r, "ch_id")
	if chID == "" {
		writeError(w, http.StatusBadRequest, "channel id is required")
		return
	}

	cr := connect.NewRequest(&notifyv1.MarkChannelReadRequest{
		ChannelId: chID,
	})
	forwardAuth(r, cr)

	_, err := h.client.MarkChannelRead(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "marked_read"})
}
