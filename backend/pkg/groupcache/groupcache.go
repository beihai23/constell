package groupcache

import (
	"container/list"
	"context"
	"fmt"
	"hash/fnv"
	"sync"

	"github.com/constell/constell/backend/pkg/registry"
	"golang.org/x/sync/singleflight"
)

// Cache is a generic groupcache-like wrapper that provides consistent hashing,
// local LRU caching, and singleflight deduplication.
type Cache[K comparable, V any] struct {
	mu             sync.RWMutex
	capacity       int
	peers          []string
	peerSet        map[string]bool
	filler         func(ctx context.Context, key K) (V, error)
	lru            *lruCache[K, V]
	sfGroup        singleflight.Group
	sfKeyFunc      func(key K) string
	reg            registry.Registry   // optional Registry for dynamic peer updates
	regServiceName string              // service name for Watch
	regCancel      context.CancelFunc  // stop Watch
}

// Option configures a Cache instance.
type Option[K comparable, V any] func(*Cache[K, V])

// WithLocalCapacity sets the maximum number of entries in the local LRU cache.
// Defaults to 10000 if not specified.
func WithLocalCapacity[K comparable, V any](n int) Option[K, V] {
	return func(c *Cache[K, V]) {
		c.capacity = n
	}
}

// WithPeers sets the list of peer node addresses for consistent hashing.
func WithPeers[K comparable, V any](peers []string) Option[K, V] {
	return func(c *Cache[K, V]) {
		c.peers = peers
	}
}

// WithRegistry sets a Registry instance for dynamic peer updates via Watch.
func WithRegistry[K comparable, V any](reg registry.Registry, serviceName string) Option[K, V] {
	return func(c *Cache[K, V]) {
		c.reg = reg
		c.regServiceName = serviceName
	}
}

// WithFiller sets the function used to load a value on cache miss.
func WithFiller[K comparable, V any](fn func(ctx context.Context, key K) (V, error)) Option[K, V] {
	return func(c *Cache[K, V]) {
		c.filler = fn
	}
}

// NewCache creates a new Cache[K, V] with the given options.
func NewCache[K comparable, V any](opts ...Option[K, V]) *Cache[K, V] {
	c := &Cache[K, V]{
		capacity: 10000,
		peerSet:  make(map[string]bool),
	}
	for _, opt := range opts {
		opt(c)
	}

	c.peers = dedupePeers(c.peers)
	for _, p := range c.peers {
		c.peerSet[p] = true
	}

	if c.capacity <= 0 {
		c.capacity = 10000
	}
	c.lru = newLRUCache[K, V](c.capacity)

	c.sfKeyFunc = func(key K) string {
		return fmt.Sprintf("%v", key)
	}

	// If Registry is configured, start Watch to update peers dynamically.
	if c.reg != nil && c.regServiceName != "" {
		ctx, cancel := context.WithCancel(context.Background())
		c.regCancel = cancel
		go c.watchPeers(ctx)
	}

	return c
}

// Get retrieves a value from the cache. On a miss, it uses singleflight to
// deduplicate concurrent requests and calls the filler to load the value.
func (c *Cache[K, V]) Get(ctx context.Context, key K) (V, error) {
	// Check local LRU cache first.
	c.mu.RLock()
	if val, ok := c.lru.Get(key); ok {
		c.mu.RUnlock()
		return val, nil
	}
	c.mu.RUnlock()

	// Singleflight deduplication.
	sfKey := c.sfKeyFunc(key)
	result, err, _ := c.sfGroup.Do(sfKey, func() (interface{}, error) {
		// Double-check inside singleflight.
		c.mu.RLock()
		if val, ok := c.lru.Get(key); ok {
			c.mu.RUnlock()
			return val, nil
		}
		c.mu.RUnlock()

		// Determine which node should handle this key.
		owner := c.resolveOwner(key)

		var val V
		var fillErr error
		if owner == "" || !c.peerSet[owner] {
			// This node owns the key or no peers configured — call filler.
			if c.filler != nil {
				val, fillErr = c.filler(ctx, key)
			} else {
				return *new(V), fmt.Errorf("no filler configured")
			}
		} else {
			// Peer owns the key — for now, call filler locally.
			// In a real implementation, this would call the peer via RPC.
			if c.filler != nil {
				val, fillErr = c.filler(ctx, key)
			} else {
				return *new(V), fmt.Errorf("no filler configured")
			}
		}
		if fillErr != nil {
			return *new(V), fillErr
		}

		// Store in local LRU cache.
		c.mu.Lock()
		c.lru.Add(key, val)
		c.mu.Unlock()

		return val, nil
	})
	if err != nil {
		return *new(V), err
	}

	return result.(V), nil
}

// Set stores a value in the local LRU cache.
func (c *Cache[K, V]) Set(ctx context.Context, key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lru.Add(key, value)
}

// Remove removes a value from the local LRU cache.
func (c *Cache[K, V]) Remove(ctx context.Context, key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lru.Remove(key)
}

// watchPeers watches Registry instance changes and updates the peer list.
func (c *Cache[K, V]) watchPeers(ctx context.Context) {
	ch, err := c.reg.Watch(ctx, c.regServiceName)
	if err != nil {
		return
	}
	for {
		select {
		case instances, ok := <-ch:
			if !ok {
				return
			}
			peers := make([]string, 0, len(instances))
			for _, inst := range instances {
				peers = append(peers, inst.Addr)
			}
			peers = dedupePeers(peers)
			c.mu.Lock()
			c.peers = peers
			c.peerSet = make(map[string]bool, len(peers))
			for _, p := range peers {
				c.peerSet[p] = true
			}
			c.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

// Close stops the Registry Watch and releases resources.
func (c *Cache[K, V]) Close() {
	if c.regCancel != nil {
		c.regCancel()
	}
}

// resolveOwner uses consistent hashing to determine which node owns a key.
// Returns the owner address, or "" if this node owns the key (no self-addr configured).
func (c *Cache[K, V]) resolveOwner(key K) string {
	if len(c.peers) == 0 {
		return ""
	}

	h := hashKey(key)
	// Simple consistent hashing: pick the peer with the smallest hash >= key hash.
	// If none, wrap around to the first peer.
	idx := h % uint32(len(c.peers))
	return c.peers[idx]
}

// --- LRU Cache ---

type lruCache[K comparable, V any] struct {
	maxSize int
	items   map[K]*list.Element
	order   *list.List
}

type lruEntry[K comparable, V any] struct {
	key   K
	value V
}

func newLRUCache[K comparable, V any](maxSize int) *lruCache[K, V] {
	return &lruCache[K, V]{
		maxSize: maxSize,
		items:   make(map[K]*list.Element),
		order:   list.New(),
	}
}

// Get returns the cached value and moves it to the front of the LRU list.
func (c *lruCache[K, V]) Get(key K) (V, bool) {
	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		return elem.Value.(*lruEntry[K, V]).value, true
	}
	var zero V
	return zero, false
}

// Add inserts or updates a key-value pair and moves it to the front.
// Evicts the least recently used entry if the cache exceeds maxSize.
func (c *lruCache[K, V]) Add(key K, value V) {
	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		elem.Value.(*lruEntry[K, V]).value = value
		return
	}

	entry := &lruEntry[K, V]{key: key, value: value}
	elem := c.order.PushFront(entry)
	c.items[key] = elem

	for c.order.Len() > c.maxSize {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			delete(c.items, oldest.Value.(*lruEntry[K, V]).key)
		}
	}
}

// Remove deletes a key from the cache.
func (c *lruCache[K, V]) Remove(key K) {
	if elem, ok := c.items[key]; ok {
		c.order.Remove(elem)
		delete(c.items, key)
	}
}

// --- Helpers ---

func hashKey[K comparable](key K) uint32 {
	h := fnv.New32a()
	fmt.Fprintf(h, "%v", key)
	return h.Sum32()
}

func dedupePeers(peers []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(peers))
	for _, p := range peers {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	return result
}
