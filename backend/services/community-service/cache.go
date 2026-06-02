package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	groupcache "github.com/constell/constell/backend/pkg/groupcache"
	"github.com/constell/constell/backend/pkg/registry"
)

// ServerCache caches server data partitioned by server_id.
type ServerCache struct {
	cache *groupcache.Cache[string, []byte]
	repo  *Repository
}

// NewServerCache creates a ServerCache backed by the repository.
func NewServerCache(repo *Repository, peers []string, reg registry.Registry, regServiceName string) *ServerCache {
	opts := []groupcache.Option[string, []byte]{
		groupcache.WithLocalCapacity[string, []byte](10000),
		groupcache.WithFiller[string, []byte](func(ctx context.Context, key string) ([]byte, error) {
			serverID := key[len("server:"):]
			server, err := repo.GetServer(ctx, serverID)
			if err != nil {
				return nil, err
			}
			return MarshalServer(server)
		}),
	}
	if reg != nil {
		opts = append(opts, groupcache.WithRegistry[string, []byte](reg, regServiceName))
	} else if len(peers) > 0 {
		opts = append(opts, groupcache.WithPeers[string, []byte](peers))
	}
	c := groupcache.NewCache[string, []byte](opts...)
	return &ServerCache{cache: c, repo: repo}
}

// Get retrieves a server from cache or fills from DB.
func (c *ServerCache) Get(ctx context.Context, serverID string) (*ServerRow, error) {
	key := fmt.Sprintf("server:%s", serverID)
	data, err := c.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return UnmarshalServer(data)
}

// Set stores a server in the local cache.
func (c *ServerCache) Set(ctx context.Context, server *ServerRow) {
	key := fmt.Sprintf("server:%s", server.ID)
	data, err := MarshalServer(server)
	if err != nil {
		log.Printf("cache: marshal server %s: %v", server.ID, err)
		return
	}
	c.cache.Set(ctx, key, data)
}

// Invalidate removes a server from the local cache.
func (c *ServerCache) Invalidate(ctx context.Context, serverID string) {
	key := fmt.Sprintf("server:%s", serverID)
	c.cache.Remove(ctx, key)
}

// MembersCache caches member lists partitioned by server_id.
type MembersCache struct {
	cache *groupcache.Cache[string, []byte]
	repo  *Repository
}

// NewMembersCache creates a MembersCache backed by the repository.
func NewMembersCache(repo *Repository, peers []string, reg registry.Registry, regServiceName string) *MembersCache {
	opts := []groupcache.Option[string, []byte]{
		groupcache.WithLocalCapacity[string, []byte](10000),
		groupcache.WithFiller[string, []byte](func(ctx context.Context, key string) ([]byte, error) {
			serverID := key[len("members:"):]
			members, _, err := repo.ListMembersByServer(ctx, serverID, 1000, "")
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

// Get retrieves the member list for a server.
func (c *MembersCache) Get(ctx context.Context, serverID string) ([]*MemberRow, error) {
	key := fmt.Sprintf("members:%s", serverID)
	data, err := c.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return UnmarshalMembers(data)
}

// Invalidate removes the member list for a server.
func (c *MembersCache) Invalidate(ctx context.Context, serverID string) {
	key := fmt.Sprintf("members:%s", serverID)
	c.cache.Remove(ctx, key)
}

// RolesCache caches role lists partitioned by server_id.
type RolesCache struct {
	cache *groupcache.Cache[string, []byte]
	repo  *Repository
}

// NewRolesCache creates a RolesCache backed by the repository.
func NewRolesCache(repo *Repository, peers []string, reg registry.Registry, regServiceName string) *RolesCache {
	opts := []groupcache.Option[string, []byte]{
		groupcache.WithLocalCapacity[string, []byte](10000),
		groupcache.WithFiller[string, []byte](func(ctx context.Context, key string) ([]byte, error) {
			serverID := key[len("roles:"):]
			roles, err := repo.ListRolesByServer(ctx, serverID)
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

// Get retrieves the role list for a server.
func (c *RolesCache) Get(ctx context.Context, serverID string) ([]*RoleRow, error) {
	key := fmt.Sprintf("roles:%s", serverID)
	data, err := c.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return UnmarshalRoles(data)
}

// Invalidate removes the role list for a server.
func (c *RolesCache) Invalidate(ctx context.Context, serverID string) {
	key := fmt.Sprintf("roles:%s", serverID)
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
