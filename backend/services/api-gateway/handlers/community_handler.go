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

// --- Community ---

// createCommunityRequest is the JSON body for POST /api/v1/communities.
type createCommunityRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IconURL     string `json:"icon_url"`
}

// communityResponse is the JSON representation of a community.
type communityResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IconURL     string `json:"icon_url"`
	OwnerID     string `json:"owner_id"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// communityToResponse converts a proto Community to a JSON response.
func communityToResponse(s *communityv1.Community) communityResponse {
	if s == nil {
		return communityResponse{}
	}
	return communityResponse{
		ID:          s.Id,
		Name:        s.Name,
		Description: s.Description,
		IconURL:     s.IconUrl,
		OwnerID:     s.OwnerId,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}

// CreateCommunity handles POST /api/v1/communities.
func (h *CommunityHandler) CreateCommunity(w http.ResponseWriter, r *http.Request) {
	var req createCommunityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	cr := connect.NewRequest(&communityv1.CreateCommunityRequest{
		Name:        req.Name,
		Description: req.Description,
		IconUrl:     req.IconURL,
	})
	forwardAuth(r, cr)

	resp, err := h.client.CreateCommunity(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, communityToResponse(resp.Msg.Community))
}

// GetCommunity handles GET /api/v1/communities/:id.
func (h *CommunityHandler) GetCommunity(w http.ResponseWriter, r *http.Request) {
	communityID := chi.URLParam(r, "id")
	if communityID == "" {
		writeError(w, http.StatusBadRequest, "community id is required")
		return
	}

	cr := connect.NewRequest(&communityv1.GetCommunityRequest{
		CommunityId: communityID,
	})
	forwardAuth(r, cr)

	resp, err := h.client.GetCommunity(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, communityToResponse(resp.Msg.Community))
}

// updateCommunityRequest is the JSON body for PATCH /api/v1/communities/:id.
type updateCommunityRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IconURL     string `json:"icon_url"`
}

// UpdateCommunity handles PATCH /api/v1/communities/:id.
func (h *CommunityHandler) UpdateCommunity(w http.ResponseWriter, r *http.Request) {
	communityID := chi.URLParam(r, "id")
	if communityID == "" {
		writeError(w, http.StatusBadRequest, "community id is required")
		return
	}

	var req updateCommunityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cr := connect.NewRequest(&communityv1.UpdateCommunityRequest{
		CommunityId: communityID,
		Name:        req.Name,
		Description: req.Description,
		IconUrl:     req.IconURL,
	})
	forwardAuth(r, cr)

	resp, err := h.client.UpdateCommunity(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, communityToResponse(resp.Msg.Community))
}

// --- Channels ---

// createChannelRequest is the JSON body for POST /api/v1/communities/:id/channels.
type createChannelRequest struct {
	Name     string `json:"name"`
	Topic    string `json:"topic"`
	Type     string `json:"type"`
	Position int32  `json:"position"`
}

// channelResponse is the JSON representation of a channel.
type channelResponse struct {
	ID          string `json:"id"`
	CommunityID string `json:"community_id"`
	Name        string `json:"name"`
	Topic       string `json:"topic"`
	Type        string `json:"type"`
	Position    int32  `json:"position"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
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
		ID:          c.Id,
		CommunityID: c.CommunityId,
		Name:        c.Name,
		Topic:       c.Topic,
		Type:        typeStr,
		Position:    c.Position,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
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

// CreateChannel handles POST /api/v1/communities/:id/channels.
func (h *CommunityHandler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	communityID := chi.URLParam(r, "id")
	if communityID == "" {
		writeError(w, http.StatusBadRequest, "community id is required")
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
		CommunityId: communityID,
		Name:        req.Name,
		Topic:       req.Topic,
		Type:        channelTypeFromString(req.Type),
		Position:    req.Position,
	})
	forwardAuth(r, cr)

	resp, err := h.client.CreateChannel(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, channelToResponse(resp.Msg.Channel))
}

// GetChannels handles GET /api/v1/communities/:id/channels.
func (h *CommunityHandler) GetChannels(w http.ResponseWriter, r *http.Request) {
	communityID := chi.URLParam(r, "id")
	if communityID == "" {
		writeError(w, http.StatusBadRequest, "community id is required")
		return
	}

	limit := int32FromQuery(r, "limit", 50)
	offset := int32FromQuery(r, "offset", 0)

	cr := connect.NewRequest(&communityv1.ListChannelsRequest{
		CommunityId: communityID,
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

// addMemberRequest is the JSON body for POST /api/v1/communities/:id/members.
type addMemberRequest struct {
	UserID string `json:"user_id"`
}

// memberResponse is the JSON representation of a community member.
type memberResponse struct {
	CommunityID string   `json:"community_id"`
	UserID      string   `json:"user_id"`
	Nickname    string   `json:"nickname"`
	RoleIDs     []string `json:"role_ids"`
	JoinedAt    int64    `json:"joined_at"`
}

// memberToResponse converts a proto CommunityMember to a JSON response.
func memberToResponse(m *communityv1.CommunityMember) memberResponse {
	if m == nil {
		return memberResponse{}
	}
	return memberResponse{
		CommunityID: m.CommunityId,
		UserID:      m.UserId,
		Nickname:    m.Nickname,
		RoleIDs:     m.RoleIds,
		JoinedAt:    m.JoinedAt,
	}
}

// AddMember handles POST /api/v1/communities/:id/members.
func (h *CommunityHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	communityID := chi.URLParam(r, "id")
	if communityID == "" {
		writeError(w, http.StatusBadRequest, "community id is required")
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

	cr := connect.NewRequest(&communityv1.JoinCommunityRequest{
		CommunityId: communityID,
	})
	forwardAuth(r, cr)

	resp, err := h.client.JoinCommunity(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, memberToResponse(resp.Msg.Member))
}

// RemoveMember handles DELETE /api/v1/communities/:id/members/:uid.
func (h *CommunityHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	communityID := chi.URLParam(r, "id")
	if communityID == "" {
		writeError(w, http.StatusBadRequest, "community id is required")
		return
	}

	uid := chi.URLParam(r, "uid")
	if uid == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}

	cr := connect.NewRequest(&communityv1.KickMemberRequest{
		CommunityId: communityID,
		UserId:      uid,
	})
	forwardAuth(r, cr)

	resp, err := h.client.KickMember(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	_ = resp.Msg
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// ListMembers handles GET /api/v1/communities/:id/members.
func (h *CommunityHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	communityID := chi.URLParam(r, "id")
	if communityID == "" {
		writeError(w, http.StatusBadRequest, "community id is required")
		return
	}

	limit := int32FromQuery(r, "limit", 50)
	offset := int32FromQuery(r, "offset", 0)

	cr := connect.NewRequest(&communityv1.ListMembersRequest{
		CommunityId: communityID,
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
	Content string   `json:"content"`
	FileIDs []string `json:"file_ids"`
}

// attachmentResponse is the JSON representation of a message attachment.
type attachmentResponse struct {
	ID          string `json:"file_id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

// messageResponse is the JSON representation of a channel message.
type messageResponse struct {
	ID          string               `json:"id"`
	ChannelID   string               `json:"channel_id"`
	AuthorID    string               `json:"author_id"`
	Content     string               `json:"content"`
	CreatedAt   int64                `json:"created_at"`
	UpdatedAt   int64                `json:"updated_at"`
	Attachments []attachmentResponse `json:"attachments"`
}

// messageToResponse converts a proto ChannelMessage to a JSON response.
func messageToResponse(m *communityv1.ChannelMessage) messageResponse {
	if m == nil {
		return messageResponse{}
	}
	resp := messageResponse{
		ID:        m.Id,
		ChannelID: m.ChannelId,
		AuthorID:  m.AuthorId,
		Content:   m.Content,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
	for _, a := range m.Attachments {
		resp.Attachments = append(resp.Attachments, attachmentResponse{
			ID:          a.FileId,
			Filename:    a.Filename,
			ContentType: a.ContentType,
			Size:        a.Size,
		})
	}
	return resp
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
		FileIds:   req.FileIDs,
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
