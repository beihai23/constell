package main

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"golang.org/x/sync/errgroup"

	pbv1 "github.com/constell/constell/backend/pkg/proto/search/v1"
	"github.com/constell/constell/backend/pkg/proto/search/v1/searchv1connect"
	"github.com/constell/constell/backend/pkg/middleware"
)

// SearchService implements the Connect-RPC SearchServiceHandler.
type SearchService struct {
	repo SearchRepository
}

// NewSearchService creates a new SearchService.
func NewSearchService(repo SearchRepository) *SearchService {
	return &SearchService{repo: repo}
}

// Ensure SearchService implements the generated service handler interface.
var _ searchv1connect.SearchServiceHandler = (*SearchService)(nil)

// Search handles full-text search across users, channel messages, and DM messages.
func (s *SearchService) Search(
	ctx context.Context,
	req *connect.Request[pbv1.SearchRequest],
) (*connect.Response[pbv1.SearchResponse], error) {
	msg := req.Msg

	// 1. Verify authentication
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("authentication required"))
	}

	// 2. Validate query
	if msg.Query == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("query is required"))
	}

	// 3. Default limit
	limit := int(msg.Limit)
	if limit <= 0 {
		limit = 10
	}

	// 4. Determine which types to search
	types := msg.Types
	searchAll := len(types) == 0
	searchUsers := searchAll || containsType(types, pbv1.SearchType_SEARCH_TYPE_USERS)
	searchMessages := searchAll || containsType(types, pbv1.SearchType_SEARCH_TYPE_MESSAGES)
	searchDMs := searchAll || containsType(types, pbv1.SearchType_SEARCH_TYPE_DM_MESSAGES)
	searchCommunities := searchAll || containsType(types, pbv1.SearchType_SEARCH_TYPE_COMMUNITIES)

	// 5. Run searches in parallel
	var userResults []UserSearchResult
	var messageResults []MessageSearchResult
	var dmResults []DMMessageSearchResult
	var communityResults []CommunitySearchResult

	g, gctx := errgroup.WithContext(ctx)

	if searchUsers {
		g.Go(func() error {
			var err error
			userResults, err = s.repo.SearchUsers(gctx, msg.Query, limit)
			return err
		})
	}

	if searchMessages {
		g.Go(func() error {
			var err error
			messageResults, err = s.repo.SearchChannelMessages(gctx, msg.Query, callerID, limit)
			return err
		})
	}

	if searchDMs {
		g.Go(func() error {
			var err error
			dmResults, err = s.repo.SearchDMMessages(gctx, msg.Query, callerID, limit)
			return err
		})
	}

	if searchCommunities {
		g.Go(func() error {
			var err error
			communityResults, err = s.repo.SearchCommunities(gctx, msg.Query, callerID, limit)
			return err
		})
	}

	// 6. Wait for all goroutines
	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("search failed: %w", err))
	}

	// 7. Build response
	resp := connect.NewResponse(&pbv1.SearchResponse{
		Users:       toPBUserResults(userResults),
		Messages:    toPBMessageResults(messageResults),
		DmMessages:  toPBDMMessageResults(dmResults),
		Communities: toPBCommunityResults(communityResults),
	})
	return resp, nil
}

// containsType checks if the given SearchType is in the slice.
func containsType(types []pbv1.SearchType, t pbv1.SearchType) bool {
	for _, v := range types {
		if v == t {
			return true
		}
	}
	return false
}

// toPBUserResults converts internal user results to proto messages.
func toPBUserResults(results []UserSearchResult) []*pbv1.UserResult {
	if len(results) == 0 {
		return nil
	}
	out := make([]*pbv1.UserResult, len(results))
	for i, r := range results {
		out[i] = &pbv1.UserResult{
			Id:        r.ID,
			Nickname:  r.Nickname,
			AvatarUrl: r.AvatarURL,
			Relevance: r.Relevance,
		}
	}
	return out
}

// toPBMessageResults converts internal message results to proto messages.
func toPBMessageResults(results []MessageSearchResult) []*pbv1.MessageResult {
	if len(results) == 0 {
		return nil
	}
	out := make([]*pbv1.MessageResult, len(results))
	for i, r := range results {
		out[i] = &pbv1.MessageResult{
			Id:        r.ID,
			ChannelId: r.ChannelID,
			CommunityId: r.CommunityID,
			AuthorId:  r.AuthorID,
			Content:   r.Content,
			CreatedAt: r.CreatedAt,
			Relevance: r.Relevance,
		}
	}
	return out
}

// toPBDMMessageResults converts internal DM message results to proto messages.
func toPBDMMessageResults(results []DMMessageSearchResult) []*pbv1.DMMessageResult {
	if len(results) == 0 {
		return nil
	}
	out := make([]*pbv1.DMMessageResult, len(results))
	for i, r := range results {
		out[i] = &pbv1.DMMessageResult{
			Id:             r.ID,
			ConversationId: r.ConversationID,
			PeerId:         r.PeerID,
			Content:        r.Content,
			CreatedAt:      r.CreatedAt,
			Relevance:      r.Relevance,
		}
	}
	return out
}

// toPBCommunityResults converts internal community results to proto messages.
func toPBCommunityResults(results []CommunitySearchResult) []*pbv1.CommunityResult {
	if len(results) == 0 {
		return nil
	}
	out := make([]*pbv1.CommunityResult, len(results))
	for i, r := range results {
		out[i] = &pbv1.CommunityResult{
			Id:          r.ID,
			Name:        r.Name,
			IconUrl:     r.IconURL,
			Description: r.Description,
			MemberCount: r.MemberCount,
			Joined:      r.Joined,
			Relevance:   r.Relevance,
		}
	}
	return out
}
