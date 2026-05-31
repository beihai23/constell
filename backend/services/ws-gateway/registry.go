package main

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Registry manages the Redis uid->gw_id mapping for the WS Gateway cluster.
type Registry struct {
	client *goredis.Client
	ttl    time.Duration
}

// NewRegistry creates a new Registry with the given Redis client and key TTL.
func NewRegistry(client *goredis.Client, ttl time.Duration) *Registry {
	return &Registry{
		client: client,
		ttl:    ttl,
	}
}

const registryKeyPrefix = "ws:uid:"

func registryKey(userID string) string {
	return registryKeyPrefix + userID
}

// RegisterConnection writes the uid->gw_id mapping to Redis with a TTL.
func (r *Registry) RegisterConnection(ctx context.Context, userID string, gwID string) error {
	key := registryKey(userID)
	if err := r.client.Set(ctx, key, gwID, r.ttl).Err(); err != nil {
		return fmt.Errorf("SET %s: %w", key, err)
	}
	return nil
}

// UnregisterConnection removes the uid->gw_id mapping from Redis.
func (r *Registry) UnregisterConnection(ctx context.Context, userID string) error {
	key := registryKey(userID)
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("DEL %s: %w", key, err)
	}
	return nil
}

// GetGatewayID looks up which gateway instance holds a user's connection.
func (r *Registry) GetGatewayID(ctx context.Context, userID string) (string, error) {
	key := registryKey(userID)
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == goredis.Nil {
			return "", fmt.Errorf("user %s not connected", userID)
		}
		return "", fmt.Errorf("GET %s: %w", key, err)
	}
	return val, nil
}

// GetGatewayIDs performs a batch MGET to resolve multiple user IDs to their gw_ids.
func (r *Registry) GetGatewayIDs(ctx context.Context, userIDs []string) (map[string]string, error) {
	if len(userIDs) == 0 {
		return make(map[string]string), nil
	}

	keys := make([]string, len(userIDs))
	for i, uid := range userIDs {
		keys[i] = registryKey(uid)
	}

	vals, err := r.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("MGET: %w", err)
	}

	result := make(map[string]string, len(userIDs))
	for i, val := range vals {
		if val == nil {
			continue
		}
		gwID, ok := val.(string)
		if !ok {
			continue
		}
		result[userIDs[i]] = gwID
	}

	return result, nil
}
