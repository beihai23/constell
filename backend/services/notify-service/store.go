package main

import (
	"context"
	"fmt"
	"strconv"

	goredis "github.com/redis/go-redis/v9"
)

// UnreadChannel represents an unread channel entry returned to the caller.
type UnreadChannel struct {
	ChannelID   string
	CommunityID string
	Count       int32
}

// UnreadDM represents an unread DM conversation entry returned to the caller.
type UnreadDM struct {
	ConversationID string
	PeerID         string
	Count          int32
}

// Store manages notification state in Redis: message counters, read pointers,
// and the user->channel/conversation membership sets.
type Store struct {
	rdb *goredis.Client
}

// NewStore creates a Store backed by the given Redis client.
func NewStore(rdb *goredis.Client) *Store {
	return &Store{rdb: rdb}
}

// ---------- Key helpers ----------

func channelMsgCountKey(channelID string) string {
	return "channel_msg_count:" + channelID
}

func dmMsgCountKey(convID string) string {
	return "dm_msg_count:" + convID
}

func readPtrChannelKey(userID, channelID string) string {
	return "read_ptr:ch:" + userID + ":" + channelID
}

func readPtrDMKey(userID, convID string) string {
	return "read_ptr:dm:" + userID + ":" + convID
}

func userChannelsKey(userID string) string {
	return "user:channels:" + userID
}

func userConversationsKey(userID string) string {
	return "user:conversations:" + userID
}

// ---------- Counter operations ----------

// IncrementChannelMsgCount atomically increments the message counter for a channel.
func (s *Store) IncrementChannelMsgCount(ctx context.Context, channelID string) error {
	return s.rdb.Incr(ctx, channelMsgCountKey(channelID)).Err()
}

// IncrementDMMsgCount atomically increments the message counter for a DM conversation.
func (s *Store) IncrementDMMsgCount(ctx context.Context, convID string) error {
	return s.rdb.Incr(ctx, dmMsgCountKey(convID)).Err()
}

// GetChannelMsgCount returns the total message count for a channel.
// Returns 0 when the key does not exist.
func (s *Store) GetChannelMsgCount(ctx context.Context, channelID string) (int64, error) {
	val, err := s.rdb.Get(ctx, channelMsgCountKey(channelID)).Int64()
	if err == goredis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("GET channel_msg_count:%s: %w", channelID, err)
	}
	return val, nil
}

// GetDMMsgCount returns the total message count for a DM conversation.
// Returns 0 when the key does not exist.
func (s *Store) GetDMMsgCount(ctx context.Context, convID string) (int64, error) {
	val, err := s.rdb.Get(ctx, dmMsgCountKey(convID)).Int64()
	if err == goredis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("GET dm_msg_count:%s: %w", convID, err)
	}
	return val, nil
}

// ---------- Mark-read (pointer) ----------

// MarkChannelRead sets the read pointer for a user in a channel to the current
// message count, effectively clearing the unread badge.
func (s *Store) MarkChannelRead(ctx context.Context, userID, channelID string) error {
	count, err := s.GetChannelMsgCount(ctx, channelID)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, readPtrChannelKey(userID, channelID), count, 0).Err()
}

// MarkDMRead sets the read pointer for a user in a DM conversation to the
// current message count, effectively clearing the unread badge.
func (s *Store) MarkDMRead(ctx context.Context, userID, convID string) error {
	count, err := s.GetDMMsgCount(ctx, convID)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, readPtrDMKey(userID, convID), count, 0).Err()
}

// ---------- Set membership ----------

// GetUserChannels returns all channel IDs in the user's channel membership set.
func (s *Store) GetUserChannels(ctx context.Context, userID string) ([]string, error) {
	vals, err := s.rdb.SMembers(ctx, userChannelsKey(userID)).Result()
	if err != nil {
		return nil, fmt.Errorf("SMEMBERS user:channels:%s: %w", userID, err)
	}
	return vals, nil
}

// GetUserConversations returns all conversation IDs in the user's conversation set.
func (s *Store) GetUserConversations(ctx context.Context, userID string) ([]string, error) {
	vals, err := s.rdb.SMembers(ctx, userConversationsKey(userID)).Result()
	if err != nil {
		return nil, fmt.Errorf("SMEMBERS user:conversations:%s: %w", userID, err)
	}
	return vals, nil
}

// AddChannelsToUser adds channel IDs to the user's channel membership set.
func (s *Store) AddChannelsToUser(ctx context.Context, userID string, channelIDs []string) error {
	if len(channelIDs) == 0 {
		return nil
	}
	members := make([]interface{}, len(channelIDs))
	for i, id := range channelIDs {
		members[i] = id
	}
	return s.rdb.SAdd(ctx, userChannelsKey(userID), members...).Err()
}

// RemoveChannelsFromUser removes channel IDs from the user's channel membership set.
func (s *Store) RemoveChannelsFromUser(ctx context.Context, userID string, channelIDs []string) error {
	if len(channelIDs) == 0 {
		return nil
	}
	members := make([]interface{}, len(channelIDs))
	for i, id := range channelIDs {
		members[i] = id
	}
	return s.rdb.SRem(ctx, userChannelsKey(userID), members...).Err()
}

// AddConversationToUser adds a conversation ID to the user's conversation set.
func (s *Store) AddConversationToUser(ctx context.Context, userID, convID string) error {
	return s.rdb.SAdd(ctx, userConversationsKey(userID), convID).Err()
}

// ---------- Unread computation ----------

// GetUnreadChannels returns the list of channels with unread messages for a user.
// It fetches the user's channel set, then batch-reads message counts and read
// pointers, computing the diff for each channel.
func (s *Store) GetUnreadChannels(ctx context.Context, userID string) ([]UnreadChannel, error) {
	channelIDs, err := s.GetUserChannels(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(channelIDs) == 0 {
		return nil, nil
	}

	// Batch GET message counts.
	countKeys := make([]string, len(channelIDs))
	for i, chID := range channelIDs {
		countKeys[i] = channelMsgCountKey(chID)
	}
	countVals, err := s.rdb.MGet(ctx, countKeys...).Result()
	if err != nil {
		return nil, fmt.Errorf("MGET channel counts: %w", err)
	}

	// Batch GET read pointers.
	ptrKeys := make([]string, len(channelIDs))
	for i, chID := range channelIDs {
		ptrKeys[i] = readPtrChannelKey(userID, chID)
	}
	ptrVals, err := s.rdb.MGet(ctx, ptrKeys...).Result()
	if err != nil {
		return nil, fmt.Errorf("MGET channel read pointers: %w", err)
	}

	var result []UnreadChannel
	for i, chID := range channelIDs {
		total := toInt64(countVals[i])
		ptr := toInt64(ptrVals[i])
		unread := total - ptr
		if unread > 0 {
			result = append(result, UnreadChannel{
				ChannelID: chID,
				Count:     int32(unread),
			})
		}
	}
	return result, nil
}

// GetUnreadDMs returns the list of DM conversations with unread messages for a user.
// Uses the same pointer-diff pattern as GetUnreadChannels.
func (s *Store) GetUnreadDMs(ctx context.Context, userID string) ([]UnreadDM, error) {
	convIDs, err := s.GetUserConversations(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(convIDs) == 0 {
		return nil, nil
	}

	// Batch GET message counts.
	countKeys := make([]string, len(convIDs))
	for i, convID := range convIDs {
		countKeys[i] = dmMsgCountKey(convID)
	}
	countVals, err := s.rdb.MGet(ctx, countKeys...).Result()
	if err != nil {
		return nil, fmt.Errorf("MGET dm counts: %w", err)
	}

	// Batch GET read pointers.
	ptrKeys := make([]string, len(convIDs))
	for i, convID := range convIDs {
		ptrKeys[i] = readPtrDMKey(userID, convID)
	}
	ptrVals, err := s.rdb.MGet(ctx, ptrKeys...).Result()
	if err != nil {
		return nil, fmt.Errorf("MGET dm read pointers: %w", err)
	}

	var result []UnreadDM
	for i, convID := range convIDs {
		total := toInt64(countVals[i])
		ptr := toInt64(ptrVals[i])
		unread := total - ptr
		if unread > 0 {
			result = append(result, UnreadDM{
				ConversationID: convID,
				Count:          int32(unread),
			})
		}
	}
	return result, nil
}

// toInt64 converts a Redis MGET value (string, nil, or int64) to int64.
// Returns 0 for nil or unparseable values.
func toInt64(v interface{}) int64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return n
	case string:
		i, err := strconv.ParseInt(n, 10, 64)
		if err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}
