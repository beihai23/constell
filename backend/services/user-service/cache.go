package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	groupcache "github.com/constell/constell/backend/pkg/groupcache"
)

// UserCacheReader defines the read interface for the user cache.
type UserCacheReader interface {
	Get(ctx context.Context, userID string) (*UserRow, error)
}

// UserCacheWriter defines the write interface for the user cache.
type UserCacheWriter interface {
	Set(ctx context.Context, user *UserRow)
	Invalidate(ctx context.Context, userID string)
}

// RelationCacheReader defines the read interface for the relation cache.
type RelationCacheReader interface {
	Get(ctx context.Context, userID, targetUserID string) (*RelationRow, error)
}

// RelationCacheWriter defines the write interface for the relation cache.
type RelationCacheWriter interface {
	Invalidate(ctx context.Context, userID, targetUserID string)
}

// UserCache wraps a groupcache.Cache for user data.
type UserCache struct {
	cache *groupcache.Cache[string, []byte]
	repo  *Repository
}

// NewUserCache creates a UserCache that fills misses from the repository.
func NewUserCache(localCapacity int, peers []string, repo *Repository) *UserCache {
	c := groupcache.NewCache[string, []byte](
		groupcache.WithLocalCapacity[string, []byte](localCapacity),
		groupcache.WithPeers[string, []byte](peers),
		groupcache.WithFiller[string, []byte](func(ctx context.Context, key string) ([]byte, error) {
			userID := key[len("user:"):]
			user, err := repo.GetUserByID(ctx, userID)
			if err != nil {
				return nil, err
			}
			return MarshalUser(user)
		}),
	)
	return &UserCache{cache: c, repo: repo}
}

// Get retrieves a user from cache or fills it from the DB.
func (c *UserCache) Get(ctx context.Context, userID string) (*UserRow, error) {
	key := fmt.Sprintf("user:%s", userID)
	data, err := c.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return UnmarshalUser(data)
}

// Set stores a user in the local cache.
func (c *UserCache) Set(ctx context.Context, user *UserRow) {
	key := fmt.Sprintf("user:%s", user.ID)
	data, err := MarshalUser(user)
	if err != nil {
		log.Printf("cache: marshal user %s: %v", user.ID, err)
		return
	}
	c.cache.Set(ctx, key, data)
}

// Invalidate removes a user from the local cache.
func (c *UserCache) Invalidate(ctx context.Context, userID string) {
	key := fmt.Sprintf("user:%s", userID)
	c.cache.Remove(ctx, key)
}

// RelationCache wraps a groupcache.Cache for relation data.
type RelationCache struct {
	cache *groupcache.Cache[string, []byte]
	repo  *Repository
}

// NewRelationCache creates a RelationCache that fills misses from the repository.
func NewRelationCache(localCapacity int, peers []string, repo *Repository) *RelationCache {
	c := groupcache.NewCache[string, []byte](
		groupcache.WithLocalCapacity[string, []byte](localCapacity),
		groupcache.WithPeers[string, []byte](peers),
		groupcache.WithFiller[string, []byte](func(ctx context.Context, key string) ([]byte, error) {
			parts := splitRelationKey(key)
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid relation key: %s", key)
			}
			rel, err := repo.GetRelation(ctx, parts[0], parts[1])
			if err != nil {
				return nil, err
			}
			if rel == nil {
				return json.Marshal(nil)
			}
			return MarshalRelation(rel)
		}),
	)
	return &RelationCache{cache: c, repo: repo}
}

// Get retrieves a relation from cache or fills from DB.
func (c *RelationCache) Get(ctx context.Context, userID, targetUserID string) (*RelationRow, error) {
	key := fmt.Sprintf("relation:%s:%s", userID, targetUserID)
	data, err := c.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if data == nil || string(data) == "null" {
		return nil, nil
	}
	return UnmarshalRelation(data)
}

// Invalidate removes a relation from the local cache.
func (c *RelationCache) Invalidate(ctx context.Context, userID, targetUserID string) {
	key := fmt.Sprintf("relation:%s:%s", userID, targetUserID)
	c.cache.Remove(ctx, key)
}

// splitRelationKey parses "relation:{user_id}:{target_id}".
func splitRelationKey(key string) []string {
	rest := key[len("relation:"):]
	firstEnd := -1
	for i, ch := range rest {
		if ch == ':' {
			firstEnd = i
			break
		}
	}
	if firstEnd == -1 {
		return nil
	}
	return []string{rest[:firstEnd], rest[firstEnd+1:]}
}
