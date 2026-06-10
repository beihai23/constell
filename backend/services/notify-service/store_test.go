package main

import (
	"context"
	"sort"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func newTestStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return NewStore(rdb), mr
}

func TestStore_IncrementAndGet(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	// Increment channel message count 3 times.
	if err := store.IncrementChannelMsgCount(ctx, "ch1"); err != nil {
		t.Fatalf("INCR 1: %v", err)
	}
	if err := store.IncrementChannelMsgCount(ctx, "ch1"); err != nil {
		t.Fatalf("INCR 2: %v", err)
	}
	if err := store.IncrementChannelMsgCount(ctx, "ch1"); err != nil {
		t.Fatalf("INCR 3: %v", err)
	}

	count, err := store.GetChannelMsgCount(ctx, "ch1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if count != 3 {
		t.Errorf("expected count=3, got %d", count)
	}

	// Non-existent key returns 0.
	count, err = store.GetChannelMsgCount(ctx, "ch_missing")
	if err != nil {
		t.Fatalf("GET missing: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 for missing key, got %d", count)
	}
}

func TestStore_MarkChannelRead(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	// Increment 2 times, mark read, then increment 1 more.
	store.IncrementChannelMsgCount(ctx, "ch1")
	store.IncrementChannelMsgCount(ctx, "ch1")

	// Add channel to user's set so GetUnreadChannels picks it up.
	store.AddChannelsToUser(ctx, "user1", []string{"ch1"})

	if err := store.MarkChannelRead(ctx, "user1", "ch1"); err != nil {
		t.Fatalf("MarkChannelRead: %v", err)
	}

	// Unread should be 0 now.
	unreads, err := store.GetUnreadChannels(ctx, "user1")
	if err != nil {
		t.Fatalf("GetUnreadChannels: %v", err)
	}
	if len(unreads) != 0 {
		t.Errorf("expected 0 unreads after mark-read, got %d", len(unreads))
	}

	// Increment once more.
	store.IncrementChannelMsgCount(ctx, "ch1")

	unreads, err = store.GetUnreadChannels(ctx, "user1")
	if err != nil {
		t.Fatalf("GetUnreadChannels after new msg: %v", err)
	}
	if len(unreads) != 1 {
		t.Fatalf("expected 1 unread, got %d", len(unreads))
	}
	if unreads[0].Count != 1 {
		t.Errorf("expected unread count=1, got %d", unreads[0].Count)
	}
}

func TestStore_AddChannelsToUser(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	// Add 3 channels.
	err := store.AddChannelsToUser(ctx, "user1", []string{"ch1", "ch2", "ch3"})
	if err != nil {
		t.Fatalf("AddChannelsToUser: %v", err)
	}

	channels, err := store.GetUserChannels(ctx, "user1")
	if err != nil {
		t.Fatalf("GetUserChannels: %v", err)
	}
	sort.Strings(channels)
	if len(channels) != 3 {
		t.Fatalf("expected 3 channels, got %d", len(channels))
	}
	expected := []string{"ch1", "ch2", "ch3"}
	for i, ch := range expected {
		if channels[i] != ch {
			t.Errorf("channel[%d]: expected %s, got %s", i, ch, channels[i])
		}
	}

	// Remove 1 channel.
	err = store.RemoveChannelsFromUser(ctx, "user1", []string{"ch2"})
	if err != nil {
		t.Fatalf("RemoveChannelsFromUser: %v", err)
	}

	channels, err = store.GetUserChannels(ctx, "user1")
	if err != nil {
		t.Fatalf("GetUserChannels after remove: %v", err)
	}
	sort.Strings(channels)
	if len(channels) != 2 {
		t.Fatalf("expected 2 channels after removal, got %d", len(channels))
	}
}

func TestStore_DMUnread(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	// Increment DM count 2 times.
	store.IncrementDMMsgCount(ctx, "conv1")
	store.IncrementDMMsgCount(ctx, "conv1")

	// Add conversation to user.
	store.AddConversationToUser(ctx, "user1", "conv1")

	// Check unread = 2.
	unreads, err := store.GetUnreadDMs(ctx, "user1")
	if err != nil {
		t.Fatalf("GetUnreadDMs: %v", err)
	}
	if len(unreads) != 1 {
		t.Fatalf("expected 1 unread conversation, got %d", len(unreads))
	}
	if unreads[0].Count != 2 {
		t.Errorf("expected unread=2, got %d", unreads[0].Count)
	}

	// Mark as read.
	if err := store.MarkDMRead(ctx, "user1", "conv1"); err != nil {
		t.Fatalf("MarkDMRead: %v", err)
	}

	// Check unread = 0.
	unreads, err = store.GetUnreadDMs(ctx, "user1")
	if err != nil {
		t.Fatalf("GetUnreadDMs after mark-read: %v", err)
	}
	if len(unreads) != 0 {
		t.Errorf("expected 0 unreads after mark-read, got %d", len(unreads))
	}
}
