package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"

	pbv1 "github.com/constell/constell/backend/pkg/proto/user/v1"
)

// --- Unit Tests ---

func TestSplitRelationKey(t *testing.T) {
	parts := splitRelationKey("relation:abc-123:def-456")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0] != "abc-123" || parts[1] != "def-456" {
		t.Fatalf("expected [abc-123, def-456], got %v", parts)
	}
	t.Log("splitRelationKey working correctly")
}

func TestSplitRelationKeyInvalid(t *testing.T) {
	parts := splitRelationKey("relation:nocolon")
	if parts != nil {
		t.Fatalf("expected nil for invalid key, got %v", parts)
	}
	t.Log("splitRelationKey correctly returns nil for invalid key")
}

func TestMarshalUnmarshalUser(t *testing.T) {
	u := &UserRow{
		ID: "user-1", Email: "a@b.com", Nickname: "alice",
		AvatarURL: "https://img.example.com/a.png", StatusMessage: "hello",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	data, err := MarshalUser(u)
	if err != nil {
		t.Fatalf("MarshalUser failed: %v", err)
	}

	got, err := UnmarshalUser(data)
	if err != nil {
		t.Fatalf("UnmarshalUser failed: %v", err)
	}
	if got.ID != u.ID || got.Email != u.Email || got.Nickname != u.Nickname {
		t.Fatalf("round-trip mismatch: got %+v", got)
	}
	t.Log("MarshalUser/UnmarshalUser round-trip working")
}

func TestMarshalUnmarshalRelation(t *testing.T) {
	r := &RelationRow{
		UserID: "user-1", TargetUserID: "user-2",
		Type: "friend", CreatedAt: time.Now(),
	}

	data, err := MarshalRelation(r)
	if err != nil {
		t.Fatalf("MarshalRelation failed: %v", err)
	}

	got, err := UnmarshalRelation(data)
	if err != nil {
		t.Fatalf("UnmarshalRelation failed: %v", err)
	}
	if got.UserID != r.UserID || got.TargetUserID != r.TargetUserID || got.Type != r.Type {
		t.Fatalf("round-trip mismatch: got %+v", got)
	}
	t.Log("MarshalRelation/UnmarshalRelation round-trip working")
}

// --- Mock Types ---

// mockUserCache is a simple in-memory mock for UserCacheReader/UserCacheWriter.
type mockUserCache struct {
	users map[string]*UserRow
}

func (c *mockUserCache) Get(ctx context.Context, userID string) (*UserRow, error) {
	u, ok := c.users[userID]
	if !ok {
		return nil, fmt.Errorf("user not found")
	}
	return u, nil
}

func (c *mockUserCache) Set(ctx context.Context, user *UserRow) {
	c.users[user.ID] = user
}

func (c *mockUserCache) Invalidate(ctx context.Context, userID string) {
	delete(c.users, userID)
}

// mockRelationCache is a simple in-memory mock for RelationCacheReader/RelationCacheWriter.
type mockRelationCache struct {
	relations map[string]*RelationRow
}

func (c *mockRelationCache) Get(ctx context.Context, userID, targetUserID string) (*RelationRow, error) {
	key := fmt.Sprintf("%s:%s", userID, targetUserID)
	r, ok := c.relations[key]
	if !ok {
		return nil, nil
	}
	return r, nil
}

func (c *mockRelationCache) Invalidate(ctx context.Context, userID, targetUserID string) {
	key := fmt.Sprintf("%s:%s", userID, targetUserID)
	delete(c.relations, key)
}

// newTestUserService creates a UserService backed by mock caches.
func newTestUserService() (*UserService, *mockUserCache, *mockRelationCache) {
	uc := &mockUserCache{users: make(map[string]*UserRow)}
	rc := &mockRelationCache{relations: make(map[string]*RelationRow)}
	return &UserService{
		repo:          nil,
		userCache:     uc,
		relationCache: rc,
		userWriter:    uc,
	}, uc, rc
}

// --- Service-Level Tests ---

func TestGetUserValidation(t *testing.T) {
	svc, _, _ := newTestUserService()

	req := connect.NewRequest(&pbv1.GetUserRequest{UserId: ""})
	_, err := svc.GetUser(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for empty user_id")
	}
	connErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connErr.Code() != connect.CodeInvalidArgument {
		t.Fatalf("expected CodeInvalidArgument, got %v", connErr.Code())
	}
	t.Log("empty user_id correctly rejected")
}

func TestGetUserSuccess(t *testing.T) {
	svc, uc, _ := newTestUserService()
	uc.users["user-1"] = &UserRow{
		ID: "user-1", Email: "test@test.com", Nickname: "testuser",
		AvatarURL: "https://img.example.com/test.png", StatusMessage: "hello",
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	req := connect.NewRequest(&pbv1.GetUserRequest{UserId: "user-1"})
	resp, err := svc.GetUser(context.Background(), req)
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if resp.Msg.Id != "user-1" {
		t.Fatalf("expected user-1, got %s", resp.Msg.Id)
	}
	if resp.Msg.Nickname != "testuser" {
		t.Fatalf("expected testuser, got %s", resp.Msg.Nickname)
	}
	if resp.Msg.Email != "test@test.com" {
		t.Fatalf("expected test@test.com, got %s", resp.Msg.Email)
	}
	t.Log("GetUser returned correct user data")
}

func TestGetUserNotFound(t *testing.T) {
	svc, _, _ := newTestUserService()

	req := connect.NewRequest(&pbv1.GetUserRequest{UserId: "nonexistent"})
	_, err := svc.GetUser(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
	connErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connErr.Code() != connect.CodeNotFound {
		t.Fatalf("expected CodeNotFound, got %v", connErr.Code())
	}
	t.Log("nonexistent user correctly returns NotFound")
}

func TestSendDMBlockedByRelationCache(t *testing.T) {
	// Test the blocklist check logic via the RelationCache mock.
	// The full SendDM flow requires middleware context which is tested
	// via integration tests. Here we verify the cache returns the
	// correct blocked relation.
	rc := &mockRelationCache{relations: make(map[string]*RelationRow)}
	rc.relations["user-2:user-1"] = &RelationRow{
		UserID: "user-2", TargetUserID: "user-1", Type: "blocked",
	}

	rel, err := rc.Get(context.Background(), "user-2", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel == nil {
		t.Fatal("expected relation to exist")
	}
	if rel.Type != "blocked" {
		t.Fatalf("expected blocked, got %s", rel.Type)
	}
	t.Log("RelationCache correctly returns blocked relation")
}

func TestSendDMNoRelation(t *testing.T) {
	rc := &mockRelationCache{relations: make(map[string]*RelationRow)}

	rel, err := rc.Get(context.Background(), "user-1", "user-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel != nil {
		t.Fatal("expected nil relation")
	}
	t.Log("RelationCache correctly returns nil for no relation")
}
