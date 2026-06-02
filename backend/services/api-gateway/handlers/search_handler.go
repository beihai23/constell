package handlers

import (
	"net/http"

	"connectrpc.com/connect"

	searchv1 "github.com/constell/constell/backend/pkg/proto/search/v1"
	searchv1connect "github.com/constell/constell/backend/pkg/proto/search/v1/searchv1connect"
)

// SearchHandler handles REST API requests for search operations.
type SearchHandler struct {
	client searchv1connect.SearchServiceClient
}

// NewSearchHandler creates a new SearchHandler.
func NewSearchHandler(client searchv1connect.SearchServiceClient) *SearchHandler {
	return &SearchHandler{client: client}
}

// userSearchResult is the JSON representation of a user search result.
type userSearchResult struct {
	ID        string `json:"id"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}

// messageSearchResult is the JSON representation of a message search result.
type messageSearchResult struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	ServerID  string `json:"server_id"`
	AuthorID  string `json:"author_id"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
}

// dmMessageSearchResult is the JSON representation of a DM message search result.
type dmMessageSearchResult struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	PeerID         string `json:"peer_id"`
	Content        string `json:"content"`
	CreatedAt      int64  `json:"created_at"`
}

// searchResponse is the JSON response for GET /api/v1/search.
type searchResponse struct {
	Users      []userSearchResult      `json:"users"`
	Messages   []messageSearchResult   `json:"messages"`
	DMMessages []dmMessageSearchResult `json:"dm_messages"`
}

// Search handles GET /api/v1/search.
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter q is required")
		return
	}

	limit := int32FromQuery(r, "limit", 20)

	cr := connect.NewRequest(&searchv1.SearchRequest{
		Query: query,
		Limit: limit,
	})
	forwardAuth(r, cr)

	resp, err := h.client.Search(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	msg := resp.Msg
	users := make([]userSearchResult, 0, len(msg.Users))
	for _, u := range msg.Users {
		users = append(users, userSearchResult{
			ID:        u.GetId(),
			Nickname:  u.GetNickname(),
			AvatarURL: u.GetAvatarUrl(),
		})
	}

	messages := make([]messageSearchResult, 0, len(msg.Messages))
	for _, m := range msg.Messages {
		messages = append(messages, messageSearchResult{
			ID:        m.GetId(),
			ChannelID: m.GetChannelId(),
			ServerID:  m.GetServerId(),
			AuthorID:  m.GetAuthorId(),
			Content:   m.GetContent(),
			CreatedAt: m.GetCreatedAt(),
		})
	}

	dmMessages := make([]dmMessageSearchResult, 0, len(msg.DmMessages))
	for _, m := range msg.DmMessages {
		dmMessages = append(dmMessages, dmMessageSearchResult{
			ID:             m.GetId(),
			ConversationID: m.GetConversationId(),
			PeerID:         m.GetPeerId(),
			Content:        m.GetContent(),
			CreatedAt:      m.GetCreatedAt(),
		})
	}

	writeJSON(w, http.StatusOK, searchResponse{
		Users:      users,
		Messages:   messages,
		DMMessages: dmMessages,
	})
}
