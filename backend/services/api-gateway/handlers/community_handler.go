package handlers

import (
	"encoding/json"
	"net/http"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"

	commonv1 "github.com/constell/constell/backend/pkg/proto/common/v1"
	communityv1 "github.com/constell/constell/backend/pkg/proto/community/v1"
	communityv1connect "github.com/constell/constell/backend/pkg/proto/community/v1/communityv1connect"
)

// CommunityHandler handles REST API requests for community operations.
type CommunityHandler struct {
	client communityv1connect.CommunityServiceClient
}

// NewCommunityHandler creates a new CommunityHandler.
func NewCommunityHandler(client communityv1connect.CommunityServiceClient) *CommunityHandler {
	return &CommunityHandler{client: client}
}

// --- Server ---

// createServerRequest is the JSON body for POST /api/v1/servers.
type createServerRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IconURL     string `json:"icon_url"`
}

// serverResponse is the JSON representation of a server.
type serverResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IconURL     string `json:"icon_url"`
	OwnerID     string `json:"owner_id"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// serverToResponse converts a proto Server to a JSON response.
func serverToResponse(s *communityv1.Server) serverResponse {
	if s == nil {
		return serverResponse{}
	}
	return serverResponse{
		ID:          s.Id,
		Name:        s.Name,
		Description: s.Description,
		IconURL:     s.IconUrl,
		OwnerID:     s.OwnerId,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}

// CreateServer handles POST /api/v1/servers.
func (h *CommunityHandler) CreateServer(w http.ResponseWriter, r *http.Request) {
	var req createServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	cr := connect.NewRequest(&communityv1.CreateServerRequest{
		Name:        req.Name,
		Description: req.Description,
		IconUrl:     req.IconURL,
	})
	forwardAuth(r, cr)

	resp, err := h.client.CreateServer(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, serverToResponse(resp.Msg.Server))
}

// GetServer handles GET /api/v1/servers/:id.
func (h *CommunityHandler) GetServer(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		writeError(w, http.StatusBadRequest, "server id is required")
		return
	}

	cr := connect.NewRequest(&communityv1.GetServerRequest{
		ServerId: serverID,
	})
	forwardAuth(r, cr)

	resp, err := h.client.GetServer(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, serverToResponse(resp.Msg.Server))
}

// updateServerRequest is the JSON body for PATCH /api/v1/servers/:id.
type updateServerRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IconURL     string `json:"icon_url"`
}

// UpdateServer handles PATCH /api/v1/servers/:id.
func (h *CommunityHandler) UpdateServer(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		writeError(w, http.StatusBadRequest, "server id is required")
		return
	}

	var req updateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cr := connect.NewRequest(&communityv1.UpdateServerRequest{
		ServerId:    serverID,
		Name:        req.Name,
		Description: req.Description,
		IconUrl:     req.IconURL,
	})
	forwardAuth(r, cr)

	resp, err := h.client.UpdateServer(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, serverToResponse(resp.Msg.Server))
}

// --- Channels ---

// createChannelRequest is the JSON body for POST /api/v1/servers/:id/channels.
type createChannelRequest struct {
	Name     string `json:"name"`
	Topic    string `json:"topic"`
	Type     string `json:"type"`
	Position int32  `json:"position"`
}

// channelResponse is the JSON representation of a channel.
type channelResponse struct {
	ID        string `json:"id"`
	ServerID  string `json:"server_id"`
	Name      string `json:"name"`
	Topic     string `json:"topic"`
	Type      string `json:"type"`
	Position  int32  `json:"position"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// channelToResponse converts a proto Channel to a JSON response.
func channelToResponse(c *communityv1.Channel) channelResponse {
	if c == nil {
		return channelResponse{}
	}
	typeStr := "text"
	switch c.Type {
	case communityv1.ChannelType_CHANNEL_TYPE_ANNOUNCEMENT:
		typeStr = "announcement"
	}
	return channelResponse{
		ID:        c.Id,
		ServerID:  c.ServerId,
		Name:      c.Name,
		Topic:     c.Topic,
		Type:      typeStr,
		Position:  c.Position,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// channelTypeFromString converts a string to a ChannelType proto enum.
func channelTypeFromString(s string) communityv1.ChannelType {
	switch s {
	case "announcement":
		return communityv1.ChannelType_CHANNEL_TYPE_ANNOUNCEMENT
	default:
		return communityv1.ChannelType_CHANNEL_TYPE_TEXT
	}
}

// CreateChannel handles POST /api/v1/servers/:id/channels.
func (h *CommunityHandler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		writeError(w, http.StatusBadRequest, "server id is required")
		return
	}

	var req createChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	cr := connect.NewRequest(&communityv1.CreateChannelRequest{
		ServerId: serverID,
		Name:     req.Name,
		Topic:    req.Topic,
		Type:     channelTypeFromString(req.Type),
		Position: req.Position,
	})
	forwardAuth(r, cr)

	resp, err := h.client.CreateChannel(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, channelToResponse(resp.Msg.Channel))
}

// GetChannels handles GET /api/v1/servers/:id/channels.
func (h *CommunityHandler) GetChannels(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		writeError(w, http.StatusBadRequest, "server id is required")
		return
	}

	limit := int32FromQuery(r, "limit", 50)
	offset := int32FromQuery(r, "offset", 0)

	cr := connect.NewRequest(&communityv1.ListChannelsRequest{
		ServerId: serverID,
		Pagination: &commonv1.PaginationRequest{
			Limit:  limit,
			Offset: offset,
		},
	})
	forwardAuth(r, cr)

	resp, err := h.client.ListChannels(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	channels := make([]channelResponse, 0, len(resp.Msg.Channels))
	for _, c := range resp.Msg.Channels {
		channels = append(channels, channelToResponse(c))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"channels": channels,
	})
}

// UpdateChannel handles PATCH /api/v1/channels/:id.
func (h *CommunityHandler) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "id")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel id is required")
		return
	}

	var req createChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cr := connect.NewRequest(&communityv1.UpdateChannelRequest{
		ChannelId: channelID,
		Name:      req.Name,
		Topic:     req.Topic,
		Type:      channelTypeFromString(req.Type),
		Position:  req.Position,
	})
	forwardAuth(r, cr)

	resp, err := h.client.UpdateChannel(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, channelToResponse(resp.Msg.Channel))
}

// --- Membership ---

// addMemberRequest is the JSON body for POST /api/v1/servers/:id/members.
type addMemberRequest struct {
	UserID string `json:"user_id"`
}

// memberResponse is the JSON representation of a server member.
type memberResponse struct {
	ServerID string   `json:"server_id"`
	UserID   string   `json:"user_id"`
	Nickname string   `json:"nickname"`
	RoleIDs  []string `json:"role_ids"`
	JoinedAt int64    `json:"joined_at"`
}

// memberToResponse converts a proto ServerMember to a JSON response.
func memberToResponse(m *communityv1.ServerMember) memberResponse {
	if m == nil {
		return memberResponse{}
	}
	return memberResponse{
		ServerID: m.ServerId,
		UserID:   m.UserId,
		Nickname: m.Nickname,
		RoleIDs:  m.RoleIds,
		JoinedAt: m.JoinedAt,
	}
}

// AddMember handles POST /api/v1/servers/:id/members.
func (h *CommunityHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		writeError(w, http.StatusBadRequest, "server id is required")
		return
	}

	var req addMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	cr := connect.NewRequest(&communityv1.JoinServerRequest{
		ServerId: serverID,
	})
	forwardAuth(r, cr)

	resp, err := h.client.JoinServer(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, memberToResponse(resp.Msg.Member))
}

// RemoveMember handles DELETE /api/v1/servers/:id/members/:uid.
func (h *CommunityHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		writeError(w, http.StatusBadRequest, "server id is required")
		return
	}

	cr := connect.NewRequest(&communityv1.LeaveServerRequest{
		ServerId: serverID,
	})
	forwardAuth(r, cr)

	resp, err := h.client.LeaveServer(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	_ = resp.Msg
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// ListMembers handles GET /api/v1/servers/:id/members.
func (h *CommunityHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if serverID == "" {
		writeError(w, http.StatusBadRequest, "server id is required")
		return
	}

	limit := int32FromQuery(r, "limit", 50)
	offset := int32FromQuery(r, "offset", 0)

	cr := connect.NewRequest(&communityv1.ListMembersRequest{
		ServerId: serverID,
		Pagination: &commonv1.PaginationRequest{
			Limit:  limit,
			Offset: offset,
		},
	})
	forwardAuth(r, cr)

	resp, err := h.client.ListMembers(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	members := make([]memberResponse, 0, len(resp.Msg.Members))
	for _, m := range resp.Msg.Members {
		members = append(members, memberToResponse(m))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"members": members,
	})
}

// --- Messages ---

// sendMessageRequest is the JSON body for POST /api/v1/channels/:id/messages.
type sendMessageRequest struct {
	Content string `json:"content"`
}

// messageResponse is the JSON representation of a channel message.
type messageResponse struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	AuthorID  string `json:"author_id"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// messageToResponse converts a proto ChannelMessage to a JSON response.
func messageToResponse(m *communityv1.ChannelMessage) messageResponse {
	if m == nil {
		return messageResponse{}
	}
	return messageResponse{
		ID:        m.Id,
		ChannelID: m.ChannelId,
		AuthorID:  m.AuthorId,
		Content:   m.Content,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

// SendMessage handles POST /api/v1/channels/:id/messages.
func (h *CommunityHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "id")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel id is required")
		return
	}

	var req sendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	cr := connect.NewRequest(&communityv1.SendMessageRequest{
		ChannelId: channelID,
		Content:   req.Content,
	})
	forwardAuth(r, cr)

	resp, err := h.client.SendMessage(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, messageToResponse(resp.Msg.Message))
}

// GetHistory handles GET /api/v1/channels/:id/messages.
func (h *CommunityHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "id")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel id is required")
		return
	}

	limit := int32FromQuery(r, "limit", 50)
	offset := int32FromQuery(r, "offset", 0)

	cr := connect.NewRequest(&communityv1.GetMessagesRequest{
		ChannelId: channelID,
		Pagination: &commonv1.PaginationRequest{
			Limit:  limit,
			Offset: offset,
		},
	})
	forwardAuth(r, cr)

	resp, err := h.client.GetMessages(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	messages := make([]messageResponse, 0, len(resp.Msg.Messages))
	for _, m := range resp.Msg.Messages {
		messages = append(messages, messageToResponse(m))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"messages": messages,
	})
}
