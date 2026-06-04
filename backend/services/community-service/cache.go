package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	groupcache "github.com/constell/constell/backend/pkg/groupcache"
	"github.com/constell/constell/backend/pkg/registry"
)

// CommunityCache caches community data partitioned by community_id.
type CommunityCache struct {
	cache *groupcache.Cache[string, []byte]
	repo  *Repository
}

// NewCommunityCache creates a CommunityCache backed by the repository.
func NewCommunityCache(repo *Repository, peers []string, reg registry.Registry, regServiceName string) *CommunityCache {
	opts := []groupcache.Option[string, []byte]{
		groupcache.WithLocalCapacity[string, []byte](10000),
		groupcache.WithFiller[string, []byte](func(ctx context.Context, key string) ([]byte, error) {
			communityID := key[len("community:"):]
			community, err := repo.GetCommunity(ctx, communityID)
			if err != nil {
				return nil, err
			}
			return MarshalCommunity(community)
		}),
	}
	if reg != nil {
		opts = append(opts, groupcache.WithRegistry[string, []byte](reg, regServiceName))
	} else if len(peers) > 0 {
		opts = append(opts, groupcache.WithPeers[string, []byte](peers))
	}
	c := groupcache.NewCache[string, []byte](opts...)
	return &CommunityCache{cache: c, repo: repo}
}

// Get retrieves a community from cache or fills from DB.
func (c *CommunityCache) Get(ctx context.Context, communityID string) (*CommunityRow, error) {
	key := fmt.Sprintf("community:%s", communityID)
	data, err := c.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return UnmarshalCommunity(data)
}

// Set stores a community in the local cache.
func (c *CommunityCache) Set(ctx context.Context, community *CommunityRow) {
	key := fmt.Sprintf("community:%s", community.ID)
	data, err := MarshalCommunity(community)
	if err != nil {
		log.Printf("cache: marshal community %s: %v", community.ID, err)
		return
	}
	c.cache.Set(ctx, key, data)
}

// Invalidate removes a community from the local cache.
func (c *CommunityCache) Invalidate(ctx context.Context, communityID string) {
	key := fmt.Sprintf("community:%s", communityID)
	c.cache.Remove(ctx, key)
}

// MembersCache caches member lists partitioned by community_id.
type MembersCache struct {
	cache *groupcache.Cache[string, []byte]
	repo  *Repository
}

// NewMembersCache creates a MembersCache backed by the repository.
func NewMembersCache(repo *Repository, peers []string, reg registry.Registry, regServiceName string) *MembersCache {
	opts := []groupcache.Option[string, []byte]{
		groupcache.WithLocalCapacity[string, []byte](10000),
		groupcache.WithFiller[string, []byte](func(ctx context.Context, key string) ([]byte, error) {
			communityID := key[len("members:"):]
			members, _, err := repo.ListMembersByCommunity(ctx, communityID, 1000, "")
			if err != nil {
				return nil, err
			}
			return MarshalMembers(members)
		}),
	}
	if reg != nil {
		opts = append(opts, groupcache.WithRegistry[string, []byte](reg, regServiceName))
	} else if len(peers) > 0 {
		opts = append(opts, groupcache.WithPeers[string, []byte](peers))
	}
	c := groupcache.NewCache[string, []byte](opts...)
	return &MembersCache{cache: c, repo: repo}
}

// Get retrieves the member list for a community.
func (c *MembersCache) Get(ctx context.Context, communityID string) ([]*MemberRow, error) {
	key := fmt.Sprintf("members:%s", communityID)
	data, err := c.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return UnmarshalMembers(data)
}

// Invalidate removes the member list for a community.
func (c *MembersCache) Invalidate(ctx context.Context, communityID string) {
	key := fmt.Sprintf("members:%s", communityID)
	c.cache.Remove(ctx, key)
}

// RolesCache caches role lists partitioned by community_id.
type RolesCache struct {
	cache *groupcache.Cache[string, []byte]
	repo  *Repository
}

// NewRolesCache creates a RolesCache backed by the repository.
func NewRolesCache(repo *Repository, peers []string, reg registry.Registry, regServiceName string) *RolesCache {
	opts := []groupcache.Option[string, []byte]{
		groupcache.WithLocalCapacity[string, []byte](10000),
		groupcache.WithFiller[string, []byte](func(ctx context.Context, key string) ([]byte, error) {
			communityID := key[len("roles:"):]
			roles, err := repo.ListRolesByCommunity(ctx, communityID)
			if err != nil {
				return nil, err
			}
			return MarshalRoles(roles)
		}),
	}
	if reg != nil {
		opts = append(opts, groupcache.WithRegistry[string, []byte](reg, regServiceName))
	} else if len(peers) > 0 {
		opts = append(opts, groupcache.WithPeers[string, []byte](peers))
	}
	c := groupcache.NewCache[string, []byte](opts...)
	return &RolesCache{cache: c, repo: repo}
}

// Get retrieves the role list for a community.
func (c *RolesCache) Get(ctx context.Context, communityID string) ([]*RoleRow, error) {
	key := fmt.Sprintf("roles:%s", communityID)
	data, err := c.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return UnmarshalRoles(data)
}

// Invalidate removes the role list for a community.
func (c *RolesCache) Invalidate(ctx context.Context, communityID string) {
	key := fmt.Sprintf("roles:%s", communityID)
	c.cache.Remove(ctx, key)
}

// cachedMembersSet returns a set of user IDs for quick membership checks.
func cachedMembersSet(members []*MemberRow) map[string]bool {
	set := make(map[string]bool, len(members))
	for _, m := range members {
		set[m.UserID] = true
	}
	return set
}

// marshalJSON is a helper to marshal any value to JSON bytes.
func marshalJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
