package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"
	goredis "github.com/redis/go-redis/v9"

	commonv1 "github.com/constell/constell/backend/pkg/proto/common/v1"
	userv1 "github.com/constell/constell/backend/pkg/proto/user/v1"
	userv1connect "github.com/constell/constell/backend/pkg/proto/user/v1/userv1connect"
)

const presenceKeyPrefix = "ws:uid:"

// UserHandler handles REST API requests for user operations.
type UserHandler struct {
	client userv1connect.UserServiceClient
	redis  *goredis.Client // optional: nil means presence lookups return empty
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(client userv1connect.UserServiceClient, redis *goredis.Client) *UserHandler {
	return &UserHandler{client: client, redis: redis}
}

// GetPresence handles GET /api/v1/users/presence?ids=id1,id2,...
// Checks Redis ws:uid:{id} keys to determine which users are online.
func (h *UserHandler) GetPresence(w http.ResponseWriter, r *http.Request) {
	idsParam := r.URL.Query().Get("ids")
	if idsParam == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"online":  []string{},
			"offline": []string{},
		})
		return
	}

	ids := strings.Split(idsParam, ",")
	if len(ids) == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"online":  []string{},
			"offline": []string{},
		})
		return
	}

	online := make([]string, 0)
	offline := make([]string, 0)

	if h.redis != nil {
		for _, id := range ids {
			key := presenceKeyPrefix + id
			exists, err := h.redis.Exists(r.Context(), key).Result()
			if err != nil {
				offline = append(offline, id)
				continue
			}
			if exists == 1 {
				online = append(online, id)
			} else {
				offline = append(offline, id)
			}
		}
	} else {
		// No Redis — assume all offline
		offline = append(offline, ids...)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"online":  online,
		"offline": offline,
	})
}

// getUserResponse is the JSON response for GET /api/v1/users/:id.
type getUserResponse struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Nickname      string `json:"nickname"`
	AvatarURL     string `json:"avatar_url"`
	StatusMessage string `json:"status_message"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// GetUser handles GET /api/v1/users/:id.
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}

	cr := connect.NewRequest(&userv1.GetUserRequest{
		UserId: userID,
	})
	forwardAuth(r, cr)

	resp, err := h.client.GetUser(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	msg := resp.Msg
	writeJSON(w, http.StatusOK, getUserResponse{
		ID:            msg.Id,
		Email:         msg.Email,
		Nickname:      msg.Nickname,
		AvatarURL:     msg.AvatarUrl,
		StatusMessage: msg.StatusMessage,
		CreatedAt:     msg.CreatedAt,
		UpdatedAt:     msg.UpdatedAt,
	})
}

// updateProfileRequest is the JSON body for PATCH /api/v1/users/:id.
type updateProfileRequest struct {
	Nickname      string `json:"nickname"`
	AvatarURL     string `json:"avatar_url"`
	StatusMessage string `json:"status_message"`
}

// updateProfileResponse is the JSON response for PATCH /api/v1/users/:id.
type updateProfileResponse struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Nickname      string `json:"nickname"`
	AvatarURL     string `json:"avatar_url"`
	StatusMessage string `json:"status_message"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// UpdateProfile handles PATCH /api/v1/users/:id.
func (h *UserHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user id is required")
		return
	}
	_ = userID // authenticated user ID comes from context, not URL

	var req updateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cr := connect.NewRequest(&userv1.UpdateProfileRequest{
		Nickname:      req.Nickname,
		AvatarUrl:     req.AvatarURL,
		StatusMessage: req.StatusMessage,
	})
	forwardAuth(r, cr)

	resp, err := h.client.UpdateProfile(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	profile := resp.Msg.User
	writeJSON(w, http.StatusOK, updateProfileResponse{
		ID:            profile.GetId(),
		Email:         profile.GetEmail(),
		Nickname:      profile.GetNickname(),
		AvatarURL:     profile.GetAvatarUrl(),
		StatusMessage: profile.GetStatusMessage(),
		CreatedAt:     profile.GetCreatedAt(),
		UpdatedAt:     profile.GetUpdatedAt(),
	})
}

// listFriendsResponse is the JSON response for GET /api/v1/users/:id/friends.
type listFriendsResponse struct {
	Friends []friendBrief `json:"friends"`
	HasMore bool          `json:"has_more"`
}

type friendBrief struct {
	ID        string `json:"id"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}

// ListFriends handles GET /api/v1/users/:id/friends.
func (h *UserHandler) ListFriends(w http.ResponseWriter, r *http.Request) {
	limit := int32FromQuery(r, "limit", 20)
	offset := int32FromQuery(r, "offset", 0)

	cr := connect.NewRequest(&userv1.ListFriendsRequest{
		Pagination: &commonv1.PaginationRequest{
			Limit:  limit,
			Offset: offset,
		},
	})
	forwardAuth(r, cr)

	resp, err := h.client.ListFriends(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	msg := resp.Msg
	friends := make([]friendBrief, 0, len(msg.Friends))
	for _, f := range msg.Friends {
		friends = append(friends, friendBrief{
			ID:        f.Id,
			Nickname:  f.Nickname,
			AvatarURL: f.AvatarUrl,
		})
	}

	hasMore := false
	if msg.Pagination != nil {
		hasMore = msg.Pagination.HasMore
	}

	writeJSON(w, http.StatusOK, listFriendsResponse{
		Friends: friends,
		HasMore: hasMore,
	})
}

// sendDMRequest is the JSON body for POST /api/v1/dm/send.
type sendDMRequest struct {
	TargetUserID string `json:"target_user_id"`
	Content      string `json:"content"`
}

// dmMessageResponse is the JSON response for DM messages.
type dmMessageResponse struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	SenderID       string `json:"sender_id"`
	Content        string `json:"content"`
	CreatedAt      int64  `json:"created_at"`
}

// SendDM handles POST /api/v1/dm/send.
func (h *UserHandler) SendDM(w http.ResponseWriter, r *http.Request) {
	var req sendDMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TargetUserID == "" || req.Content == "" {
		writeError(w, http.StatusBadRequest, "target_user_id and content are required")
		return
	}

	cr := connect.NewRequest(&userv1.SendDMRequest{
		TargetUserId: req.TargetUserID,
		Content:      req.Content,
	})
	forwardAuth(r, cr)

	resp, err := h.client.SendDM(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	msg := resp.Msg
	dm := msg.Message
	writeJSON(w, http.StatusCreated, dmMessageResponse{
		ID:             dm.GetId(),
		ConversationID: dm.GetConversationId(),
		SenderID:       dm.GetSenderId(),
		Content:        dm.GetContent(),
		CreatedAt:      dm.GetCreatedAt(),
	})
}

// GetDMHistory handles GET /api/v1/dm/history/:peerId.
func (h *UserHandler) GetDMHistory(w http.ResponseWriter, r *http.Request) {
	peerID := chi.URLParam(r, "peerId")
	if peerID == "" {
		writeError(w, http.StatusBadRequest, "peer id is required")
		return
	}

	limit := int32FromQuery(r, "limit", 50)
	offset := int32FromQuery(r, "offset", 0)

	cr := connect.NewRequest(&userv1.GetDMHistoryRequest{
		TargetUserId: peerID,
		Pagination: &commonv1.PaginationRequest{
			Limit:  limit,
			Offset: offset,
		},
	})
	forwardAuth(r, cr)

	resp, err := h.client.GetDMHistory(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	msg := resp.Msg
	messages := make([]dmMessageResponse, 0, len(msg.Messages))
	for _, m := range msg.Messages {
		messages = append(messages, dmMessageResponse{
			ID:             m.GetId(),
			ConversationID: m.GetConversationId(),
			SenderID:       m.GetSenderId(),
			Content:        m.GetContent(),
			CreatedAt:      m.GetCreatedAt(),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"messages": messages,
	})
}

// int32FromQuery parses an int32 query parameter with a default fallback.
func int32FromQuery(r *http.Request, key string, defaultVal int32) int32 {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return defaultVal
	}
	return int32(v)
}
