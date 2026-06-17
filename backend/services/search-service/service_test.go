package main

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	pbv1 "github.com/constell/constell/backend/pkg/proto/search/v1"
	"github.com/constell/constell/backend/pkg/middleware"
)

type fakeRepo struct {
	users       []UserSearchResult
	messages    []MessageSearchResult
	dmMessages  []DMMessageSearchResult
	communities []CommunitySearchResult
}

func (f *fakeRepo) SearchUsers(ctx context.Context, query string, limit int) ([]UserSearchResult, error) {
	return f.users, nil
}
func (f *fakeRepo) SearchChannelMessages(ctx context.Context, query, userID string, limit int) ([]MessageSearchResult, error) {
	return f.messages, nil
}
func (f *fakeRepo) SearchDMMessages(ctx context.Context, query, userID string, limit int) ([]DMMessageSearchResult, error) {
	return f.dmMessages, nil
}
func (f *fakeRepo) SearchCommunities(ctx context.Context, query, userID string, limit int) ([]CommunitySearchResult, error) {
	return f.communities, nil
}

// withCaller returns a context carrying the given userID, mirroring the auth
// middleware. Uses middleware.WithUserIDForTesting, which plants the same
// unexported context key that middleware.UserIDFromContext reads.
func withCaller(userID string) context.Context {
	return middleware.WithUserIDForTesting(context.Background(), userID)
}

func TestSearchReturnsCommunities(t *testing.T) {
	svc := NewSearchService(&fakeRepo{
		communities: []CommunitySearchResult{{ID: "c1", Name: "Gophers", MemberCount: 5, Joined: false}},
	})
	resp, err := svc.Search(withCaller("u1"), connect.NewRequest(&pbv1.SearchRequest{Query: "gophers"}))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(resp.Msg.Communities) != 1 {
		t.Fatalf("expected 1 community, got %d", len(resp.Msg.Communities))
	}
	if resp.Msg.Communities[0].GetId() != "c1" {
		t.Errorf("id = %s", resp.Msg.Communities[0].GetId())
	}
	if resp.Msg.Communities[0].GetMemberCount() != 5 {
		t.Errorf("member_count = %d", resp.Msg.Communities[0].GetMemberCount())
	}
}

func TestSearchRequiresAuth(t *testing.T) {
	svc := NewSearchService(&fakeRepo{})
	_, err := svc.Search(context.Background(), connect.NewRequest(&pbv1.SearchRequest{Query: "x"}))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("expected CodeUnauthenticated, got %v", err)
	}
}
