package main

import (
	"context"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

func newTestRedisClient(t *testing.T) *goredis.Client {
	t.Helper()

	client := goredis.NewClient(&goredis.Options{
		Addr: "localhost:6379",
	})
	t.Cleanup(func() { client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available, skipping: %v", err)
	}

	return client
}

func TestRegistry_RegisterConnection(t *testing.T) {
	client := newTestRedisClient(t)
	ctx := context.Background()

	reg := NewRegistry(client, 5*time.Minute)

	userID := "user-reg-test"
	gwID := "gw-test-001"

	err := reg.RegisterConnection(ctx, userID, gwID)
	if err != nil {
		t.Fatalf("RegisterConnection failed: %v", err)
	}

	val, err := client.Get(ctx, "ws:uid:"+userID).Result()
	if err != nil {
		t.Fatalf("GET key failed: %v", err)
	}
	if val != gwID {
		t.Fatalf("expected %q, got %q", gwID, val)
	}

	client.Del(ctx, "ws:uid:"+userID)
	t.Logf("register connection OK: user=%s gw=%s", userID, gwID)
}

func TestRegistry_UnregisterConnection(t *testing.T) {
	client := newTestRedisClient(t)
	ctx := context.Background()

	reg := NewRegistry(client, 5*time.Minute)

	userID := "user-unreg-test"
	gwID := "gw-test-002"

	reg.RegisterConnection(ctx, userID, gwID)

	err := reg.UnregisterConnection(ctx, userID)
	if err != nil {
		t.Fatalf("UnregisterConnection failed: %v", err)
	}

	_, err = client.Get(ctx, "ws:uid:"+userID).Result()
	if err == nil {
		t.Fatal("expected key to be deleted after unregister")
	}

	t.Logf("unregister connection OK: user=%s", userID)
}

func TestRegistry_GetGatewayID(t *testing.T) {
	client := newTestRedisClient(t)
	ctx := context.Background()

	reg := NewRegistry(client, 5*time.Minute)

	userID := "user-getgw-test"
	gwID := "gw-test-003"

	reg.RegisterConnection(ctx, userID, gwID)
	defer client.Del(ctx, "ws:uid:"+userID)

	result, err := reg.GetGatewayID(ctx, userID)
	if err != nil {
		t.Fatalf("GetGatewayID failed: %v", err)
	}
	if result != gwID {
		t.Fatalf("expected %q, got %q", gwID, result)
	}

	t.Logf("get gateway ID OK: user=%s gw=%s", userID, result)
}

func TestRegistry_GetGatewayID_NotFound(t *testing.T) {
	client := newTestRedisClient(t)
	ctx := context.Background()

	reg := NewRegistry(client, 5*time.Minute)

	_, err := reg.GetGatewayID(ctx, "nonexistent-user")
	if err == nil {
		t.Fatal("expected error for nonexistent user, got nil")
	}

	t.Logf("correctly returned error for nonexistent user: %v", err)
}

func TestRegistry_GetGatewayIDs_Batch(t *testing.T) {
	client := newTestRedisClient(t)
	ctx := context.Background()

	reg := NewRegistry(client, 5*time.Minute)

	userIDs := []string{"user-batch-1", "user-batch-2", "user-batch-3"}
	gwIDs := []string{"gw-a", "gw-b", "gw-a"}

	for i, uid := range userIDs {
		reg.RegisterConnection(ctx, uid, gwIDs[i])
	}
	defer func() {
		for _, uid := range userIDs {
			client.Del(ctx, "ws:uid:"+uid)
		}
	}()

	result, err := reg.GetGatewayIDs(ctx, userIDs)
	if err != nil {
		t.Fatalf("GetGatewayIDs failed: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
	if result["user-batch-1"] != "gw-a" {
		t.Fatalf("expected user-batch-1 -> gw-a, got %q", result["user-batch-1"])
	}
	if result["user-batch-2"] != "gw-b" {
		t.Fatalf("expected user-batch-2 -> gw-b, got %q", result["user-batch-2"])
	}
	if result["user-batch-3"] != "gw-a" {
		t.Fatalf("expected user-batch-3 -> gw-a, got %q", result["user-batch-3"])
	}

	t.Logf("batch get gateway IDs OK: %v", result)
}

func TestRegistry_GetGatewayIDs_PartialMissing(t *testing.T) {
	client := newTestRedisClient(t)
	ctx := context.Background()

	reg := NewRegistry(client, 5*time.Minute)

	reg.RegisterConnection(ctx, "user-partial-1", "gw-x")
	defer client.Del(ctx, "ws:uid:user-partial-1")

	result, err := reg.GetGatewayIDs(ctx, []string{"user-partial-1", "user-partial-2"})
	if err != nil {
		t.Fatalf("GetGatewayIDs failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result (partial missing), got %d", len(result))
	}
	if result["user-partial-1"] != "gw-x" {
		t.Fatalf("expected user-partial-1 -> gw-x, got %q", result["user-partial-1"])
	}

	t.Logf("partial missing batch OK: %v", result)
}

func TestRegistry_TTLRefresh(t *testing.T) {
	client := newTestRedisClient(t)
	ctx := context.Background()

	ttl := 10 * time.Second
	reg := NewRegistry(client, ttl)

	userID := "user-ttl-test"
	gwID := "gw-ttl"

	reg.RegisterConnection(ctx, userID, gwID)
	defer client.Del(ctx, "ws:uid:"+userID)

	ttlVal, err := client.TTL(ctx, "ws:uid:"+userID).Result()
	if err != nil {
		t.Fatalf("TTL check failed: %v", err)
	}
	if ttlVal < 5*time.Second || ttlVal > 10*time.Second {
		t.Fatalf("expected TTL ~10s, got %v", ttlVal)
	}

	time.Sleep(2 * time.Second)
	reg.RegisterConnection(ctx, userID, gwID)

	ttlVal2, err := client.TTL(ctx, "ws:uid:"+userID).Result()
	if err != nil {
		t.Fatalf("TTL refresh check failed: %v", err)
	}
	if ttlVal2 <= ttlVal {
		t.Fatalf("expected TTL to be refreshed (higher than %v), got %v", ttlVal, ttlVal2)
	}

	t.Logf("TTL refresh OK: initial=%v refreshed=%v", ttlVal, ttlVal2)
}
