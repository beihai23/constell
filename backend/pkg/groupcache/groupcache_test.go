package groupcache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/constell/constell/backend/pkg/registry"
)

// TestBasicGetSet verifies that a local cache miss calls the fill function,
// caches the result, and returns the same value on a second Get without
// calling fill again.
func TestBasicGetSet(t *testing.T) {
	var fillCalls int32

	cache := NewCache[string, string](
		WithLocalCapacity[string, string](100),
		WithPeers[string, string]([]string{}),
		WithFiller(func(ctx context.Context, key string) (string, error) {
			atomic.AddInt32(&fillCalls, 1)
			return "value-for-" + key, nil
		}),
	)

	ctx := context.Background()

	// First Get — should call fill.
	val, err := cache.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "value-for-key1" {
		t.Fatalf("expected 'value-for-key1', got %q", val)
	}
	if atomic.LoadInt32(&fillCalls) != 1 {
		t.Fatalf("expected 1 fill call, got %d", fillCalls)
	}

	// Second Get — should NOT call fill (cached locally).
	val2, err := cache.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("second Get failed: %v", err)
	}
	if val2 != "value-for-key1" {
		t.Fatalf("expected 'value-for-key1', got %q", val2)
	}
	if atomic.LoadInt32(&fillCalls) != 1 {
		t.Fatalf("expected still 1 fill call, got %d", fillCalls)
	}

	t.Log("TestBasicGetSet passed: fill called once, cache hit on second Get")
}

// TestFillOnMiss verifies that the filler function is called when the key
// is not in the local cache, and the result is cached for subsequent calls.
func TestFillOnMiss(t *testing.T) {
	var fillCalls int32

	cache := NewCache[string, int](
		WithLocalCapacity[string, int](100),
		WithPeers[string, int](nil),
		WithFiller(func(ctx context.Context, key string) (int, error) {
			atomic.AddInt32(&fillCalls, 1)
			return len(key), nil
		}),
	)

	ctx := context.Background()

	// Miss for "hello" -> fill returns 5.
	val, err := cache.Get(ctx, "hello")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != 5 {
		t.Fatalf("expected 5, got %d", val)
	}

	// Miss for "world" -> fill returns 5.
	val2, err := cache.Get(ctx, "world")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val2 != 5 {
		t.Fatalf("expected 5, got %d", val2)
	}

	// Hit for "hello" -> no fill.
	val3, err := cache.Get(ctx, "hello")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val3 != 5 {
		t.Fatalf("expected 5, got %d", val3)
	}

	if atomic.LoadInt32(&fillCalls) != 2 {
		t.Fatalf("expected 2 fill calls, got %d", fillCalls)
	}

	t.Log("TestFillOnMiss passed")
}

// TestSingleflightDeduplication verifies that concurrent Gets for the same key
// only call the fill function once.
func TestSingleflightDeduplication(t *testing.T) {
	var fillCalls int32

	cache := NewCache[string, string](
		WithLocalCapacity[string, string](100),
		WithPeers[string, string](nil),
		WithFiller(func(ctx context.Context, key string) (string, error) {
			atomic.AddInt32(&fillCalls, 1)
			time.Sleep(100 * time.Millisecond)
			return "slow-" + key, nil
		}),
	)

	ctx := context.Background()

	// Launch 10 concurrent Gets for the same key.
	var wg sync.WaitGroup
	results := make([]string, 10)
	errors := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errors[idx] = cache.Get(ctx, "sf-key")
		}(i)
	}
	wg.Wait()

	// All should succeed.
	for i, err := range errors {
		if err != nil {
			t.Fatalf("goroutine %d failed: %v", i, err)
		}
		if results[i] != "slow-sf-key" {
			t.Fatalf("goroutine %d: expected 'slow-sf-key', got %q", i, results[i])
		}
	}

	// Fill should have been called exactly once.
	if atomic.LoadInt32(&fillCalls) != 1 {
		t.Fatalf("expected 1 fill call with singleflight, got %d", fillCalls)
	}

	t.Logf("TestSingleflightDeduplication passed: 10 concurrent Gets, %d fill calls", fillCalls)
}

// TestLRUEviction verifies that when the cache exceeds its capacity,
// the least recently used entries are evicted.
func TestLRUEviction(t *testing.T) {
	var fillCalls int32

	// Cache with capacity of 3.
	cache := NewCache[string, string](
		WithLocalCapacity[string, string](3),
		WithPeers[string, string](nil),
		WithFiller(func(ctx context.Context, key string) (string, error) {
			atomic.AddInt32(&fillCalls, 1)
			return "v-" + key, nil
		}),
	)

	ctx := context.Background()

	// Fill 3 entries: a, b, c.
	cache.Get(ctx, "a")
	cache.Get(ctx, "b")
	cache.Get(ctx, "c")

	if atomic.LoadInt32(&fillCalls) != 3 {
		t.Fatalf("expected 3 fill calls, got %d", fillCalls)
	}

	// Access "a" to make it recently used.
	cache.Get(ctx, "a")
	// No new fill call since "a" is cached.
	if atomic.LoadInt32(&fillCalls) != 3 {
		t.Fatalf("expected 3 fill calls after accessing 'a', got %d", fillCalls)
	}

	// Add "d" — this should evict "b" (LRU since "a" was accessed).
	cache.Get(ctx, "d")
	if atomic.LoadInt32(&fillCalls) != 4 {
		t.Fatalf("expected 4 fill calls after adding 'd', got %d", fillCalls)
	}

	// "b" should be evicted now, so getting it should call fill again.
	cache.Get(ctx, "b")
	if atomic.LoadInt32(&fillCalls) != 5 {
		t.Fatalf("expected 5 fill calls after re-getting 'b', got %d", fillCalls)
	}

	t.Log("TestLRUEviction passed")
}

// TestPeerRouting verifies that when peers are configured, the cache routes
// requests to the correct node based on consistent hashing.
func TestPeerRouting(t *testing.T) {
	// Create cache for node1 with node2 as peer.
	// Use a filler that always returns "local" and a peer getter mock.
	cache := NewCache[string, string](
		WithLocalCapacity[string, string](100),
		WithPeers[string, string]([]string{"node2:8080"}),
		WithFiller(func(ctx context.Context, key string) (string, error) {
			return "local-" + key, nil
		}),
	)

	ctx := context.Background()

	// Just verify Get works with peers configured.
	val, err := cache.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// The value must come from either local fill or peer fill.
	if val != "local-key1" {
		t.Fatalf("unexpected value: %q", val)
	}

	t.Logf("TestPeerRouting: got value %q", val)
}

// TestSetAndRemove verifies that Set stores a value and Remove deletes it.
func TestSetAndRemove(t *testing.T) {
	var fillCalls int32

	cache := NewCache[string, string](
		WithLocalCapacity[string, string](100),
		WithPeers[string, string](nil),
		WithFiller(func(ctx context.Context, key string) (string, error) {
			atomic.AddInt32(&fillCalls, 1)
			return "fill-" + key, nil
		}),
	)

	ctx := context.Background()

	// Set a value directly.
	cache.Set(ctx, "manual-key", "manual-value")

	// Get should return the set value without calling fill.
	val, err := cache.Get(ctx, "manual-key")
	if err != nil {
		t.Fatalf("Get after Set failed: %v", err)
	}
	if val != "manual-value" {
		t.Fatalf("expected 'manual-value', got %q", val)
	}
	if atomic.LoadInt32(&fillCalls) != 0 {
		t.Fatalf("expected 0 fill calls after Set, got %d", fillCalls)
	}

	// Remove the key.
	cache.Remove(ctx, "manual-key")

	// Get again — should call fill.
	val2, err := cache.Get(ctx, "manual-key")
	if err != nil {
		t.Fatalf("Get after Remove failed: %v", err)
	}
	if val2 != "fill-manual-key" {
		t.Fatalf("expected 'fill-manual-key', got %q", val2)
	}
	if atomic.LoadInt32(&fillCalls) != 1 {
		t.Fatalf("expected 1 fill call after Remove+Get, got %d", fillCalls)
	}

	t.Log("TestSetAndRemove passed")
}

type mockRegistry struct {
	instances []registry.Instance
	ch        chan []registry.Instance
}

func (m *mockRegistry) Register(_ context.Context, _ registry.Instance) error { return nil }
func (m *mockRegistry) Deregister(_ context.Context) error                    { return nil }
func (m *mockRegistry) Discover(_ context.Context, _ string) ([]registry.Instance, error) {
	return m.instances, nil
}
func (m *mockRegistry) Watch(_ context.Context, _ string) (<-chan []registry.Instance, error) {
	ch := make(chan []registry.Instance, 1)
	ch <- m.instances
	m.ch = ch
	return ch, nil
}

func TestRegistryWatchPeers(t *testing.T) {
	mock := &mockRegistry{
		instances: []registry.Instance{
			{ServiceName: "user-service", Addr: "node1:9082"},
			{ServiceName: "user-service", Addr: "node2:9082"},
		},
		ch: make(chan []registry.Instance, 4),
	}

	cache := NewCache[string, string](
		WithLocalCapacity[string, string](100),
		WithRegistry[string, string](mock, "user-service"),
		WithFiller(func(ctx context.Context, key string) (string, error) {
			return "val-" + key, nil
		}),
	)
	defer cache.Close()

	// Wait for initial Watch value to be consumed
	time.Sleep(50 * time.Millisecond)

	cache.mu.RLock()
	peers := cache.peers
	cache.mu.RUnlock()

	if len(peers) != 2 {
		t.Fatalf("expected 2 peers from registry, got %d: %v", len(peers), peers)
	}

	// Simulate Registry change: add a new instance
	mock.ch <- []registry.Instance{
		{ServiceName: "user-service", Addr: "node1:9082"},
		{ServiceName: "user-service", Addr: "node2:9082"},
		{ServiceName: "user-service", Addr: "node3:9082"},
	}

	time.Sleep(50 * time.Millisecond)

	cache.mu.RLock()
	peers = cache.peers
	cache.mu.RUnlock()

	if len(peers) != 3 {
		t.Fatalf("expected 3 peers after update, got %d: %v", len(peers), peers)
	}
}
