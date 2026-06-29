package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// uniqueEmail returns an email with a random suffix so concurrent runs,
// repeated -count iterations, and cross-process reruns never collide on
// the users.email unique index.
func uniqueEmail(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failing is a fatal test-harness problem
	}
	return fmt.Sprintf("%s-%s@example.com", prefix, hex.EncodeToString(b))
}

// testDSN returns the Postgres DSN for store tests. It honors DATABASE_URL (so
// CI/alternative ports work) and otherwise defaults to the Docker-exposed
// constell DB on localhost:15432 — the same DB the integration suite runs
// against, where migrations 013 (message seq) and 015 (read_state) have applied.
func testDSN() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://constell:constell_dev@localhost:15432/constell?sslmode=disable"
}

// newTestPool connects to the test Postgres; if it is unreachable the test is
// skipped rather than failed, so the suite still passes without Docker running.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testDSN())
	if err != nil {
		t.Skipf("postgres unavailable, skipping DB test: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("postgres ping failed, skipping DB test: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// newTestStore builds a Store over a real Postgres pool. The Redis client is
// nil: read-state methods only touch the pool, and push delivery (which needs
// Redis) is not exercised here.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(nil, newTestPool(t))
}

// deleteUsers removes the given users; ON DELETE CASCADE propagates to their
// read_state rows, messages, memberships, communities, and conversations.
func deleteUsers(t *testing.T, pool *pgxpool.Pool, userIDs ...string) {
	t.Helper()
	c := context.Background()
	for _, id := range userIDs {
		if _, err := pool.Exec(c, `DELETE FROM users WHERE id = $1`, id); err != nil {
			t.Logf("cleanup delete user %s: %v", id, err)
		}
	}
}

func mustInsertTestUser(t *testing.T, pool *pgxpool.Pool, email string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(),
		`INSERT INTO users (email, password_hash, nickname) VALUES ($1, 'x', $2) RETURNING id`,
		email, email,
	).Scan(&id); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	return id
}

func mustCreateCommunity(t *testing.T, pool *pgxpool.Pool, ownerID string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(),
		`INSERT INTO communities (name, owner_id) VALUES ('test-community', $1) RETURNING id`,
		ownerID,
	).Scan(&id); err != nil {
		t.Fatalf("insert test community: %v", err)
	}
	return id
}

func mustCreateChannel(t *testing.T, pool *pgxpool.Pool, communityID string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(),
		`INSERT INTO channels (community_id, name) VALUES ($1, 'test-channel') RETURNING id`,
		communityID,
	).Scan(&id); err != nil {
		t.Fatalf("insert test channel: %v", err)
	}
	return id
}

func mustAddMember(t *testing.T, pool *pgxpool.Pool, communityID, userID string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO community_members (community_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		communityID, userID,
	); err != nil {
		t.Fatalf("add community member: %v", err)
	}
}

// mustInsertChannelMsg inserts a channel message and returns its assigned seq.
func mustInsertChannelMsg(t *testing.T, pool *pgxpool.Pool, channelID, authorID string) int64 {
	t.Helper()
	var seq int64
	if err := pool.QueryRow(t.Context(),
		`INSERT INTO channel_messages (channel_id, author_id, content) VALUES ($1, $2, $3) RETURNING seq`,
		channelID, authorID, "msg",
	).Scan(&seq); err != nil {
		t.Fatalf("insert channel message: %v", err)
	}
	return seq
}

// mustInsertDMConversation inserts a conversation between two users in the
// canonical (user_a < user_b) order required by the dm_conversations CHECK.
func mustInsertDMConversation(t *testing.T, pool *pgxpool.Pool, id1, id2 string) string {
	t.Helper()
	a, b := id1, id2
	if a > b {
		a, b = b, a
	}
	var id string
	if err := pool.QueryRow(t.Context(),
		`INSERT INTO dm_conversations (user_a_id, user_b_id) VALUES ($1, $2) RETURNING id`,
		a, b,
	).Scan(&id); err != nil {
		t.Fatalf("insert dm conversation: %v", err)
	}
	return id
}

// mustInsertDMMsg inserts a DM message and returns its assigned seq.
func mustInsertDMMsg(t *testing.T, pool *pgxpool.Pool, convID, senderID string) int64 {
	t.Helper()
	var seq int64
	if err := pool.QueryRow(t.Context(),
		`INSERT INTO dm_messages (conversation_id, sender_id, content) VALUES ($1, $2, $3) RETURNING seq`,
		convID, senderID, "msg",
	).Scan(&seq); err != nil {
		t.Fatalf("insert dm message: %v", err)
	}
	return seq
}

func unreadChannelCount(unreads []UnreadChannel, channelID string) int32 {
	for _, u := range unreads {
		if u.ChannelID == channelID {
			return u.Count
		}
	}
	return 0
}

func unreadDMCount(unreads []UnreadDM, convID string) int32 {
	for _, u := range unreads {
		if u.ConversationID == convID {
			return u.Count
		}
	}
	return 0
}

// assertChannelUnread reads a user's channel unreads and asserts the count for
// a specific channel. Encapsulates the (value, error) capture so call sites
// stay one line.
func assertChannelUnread(t *testing.T, store *Store, ctx context.Context, userID, chID string, want int32) {
	t.Helper()
	unreads, err := store.GetUnreadChannels(ctx, userID)
	if err != nil {
		t.Fatalf("GetUnreadChannels: %v", err)
	}
	if got := unreadChannelCount(unreads, chID); got != want {
		t.Fatalf("unread for channel %s = %d, want %d", chID, got, want)
	}
}

// assertDMUnread reads a user's DM unreads and asserts the count for a specific
// conversation.
func assertDMUnread(t *testing.T, store *Store, ctx context.Context, userID, convID string, want int32) {
	t.Helper()
	unreads, err := store.GetUnreadDMs(ctx, userID)
	if err != nil {
		t.Fatalf("GetUnreadDMs: %v", err)
	}
	if got := unreadDMCount(unreads, convID); got != want {
		t.Fatalf("unread for conv %s = %d, want %d", convID, got, want)
	}
}

// TestStore_ChannelReadState exercises the full channel cursor lifecycle:
// unread grows with messages, mark-read advances the cursor to max(seq) and
// clears the badge, and a later message re-surfaces unread. The cursor must be
// monotonic/idempotent.
func TestStore_ChannelReadState(t *testing.T) {
	store := newTestStore(t)
	pool := newTestPool(t)
	ctx := t.Context()

	userID := mustInsertTestUser(t, pool, uniqueEmail("member"))
	t.Cleanup(func() { deleteUsers(t, pool, userID) })
	commID := mustCreateCommunity(t, pool, userID)
	chID := mustCreateChannel(t, pool, commID)
	mustAddMember(t, pool, commID, userID)

	// 3 messages → unread 3.
	for i := 0; i < 3; i++ {
		mustInsertChannelMsg(t, pool, chID, userID)
	}
	assertChannelUnread(t, store, ctx, userID, chID, 3)

	// Mark read → cursor advances to max(seq), unread clears.
	if err := store.MarkChannelRead(ctx, userID, chID); err != nil {
		t.Fatalf("MarkChannelRead: %v", err)
	}
	var cursor int64
	if err := pool.QueryRow(ctx,
		`SELECT last_read_seq FROM channel_read_state WHERE user_id = $1 AND channel_id = $2`,
		userID, chID,
	).Scan(&cursor); err != nil {
		t.Fatalf("read back cursor: %v", err)
	}
	var maxSeq int64
	if err := pool.QueryRow(ctx,
		`SELECT max(seq) FROM channel_messages WHERE channel_id = $1`, chID,
	).Scan(&maxSeq); err != nil {
		t.Fatalf("read back max seq: %v", err)
	}
	if cursor != maxSeq {
		t.Fatalf("cursor = %d, want max(seq) = %d", cursor, maxSeq)
	}
	assertChannelUnread(t, store, ctx, userID, chID, 0)

	// Mark-read again with no new messages → idempotent (cursor unchanged).
	if err := store.MarkChannelRead(ctx, userID, chID); err != nil {
		t.Fatalf("MarkChannelRead (idempotent): %v", err)
	}

	// One more message → unread 1.
	mustInsertChannelMsg(t, pool, chID, userID)
	assertChannelUnread(t, store, ctx, userID, chID, 1)
}

// TestStore_AdvanceChannelRead_Sender verifies the sender-advance-on-send path:
// the sender's cursor jumps to their message's seq so they never see their own
// message as unread, while a fellow member still does.
func TestStore_AdvanceChannelRead_Sender(t *testing.T) {
	store := newTestStore(t)
	pool := newTestPool(t)
	ctx := t.Context()

	senderID := mustInsertTestUser(t, pool, uniqueEmail("sender"))
	memberID := mustInsertTestUser(t, pool, uniqueEmail("member"))
	t.Cleanup(func() { deleteUsers(t, pool, senderID, memberID) })
	commID := mustCreateCommunity(t, pool, senderID)
	chID := mustCreateChannel(t, pool, commID)
	mustAddMember(t, pool, commID, senderID)
	mustAddMember(t, pool, commID, memberID)

	seq := mustInsertChannelMsg(t, pool, chID, senderID)
	if err := store.AdvanceChannelRead(ctx, senderID, chID, seq); err != nil {
		t.Fatalf("AdvanceChannelRead: %v", err)
	}

	// Sender: own message is read; member: still 1 unread.
	assertChannelUnread(t, store, ctx, senderID, chID, 0)
	assertChannelUnread(t, store, ctx, memberID, chID, 1)

	// Advance with a stale (lower) seq must NOT rewind the cursor.
	if err := store.AdvanceChannelRead(ctx, senderID, chID, 1); err != nil {
		t.Fatalf("AdvanceChannelRead (stale): %v", err)
	}
	assertChannelUnread(t, store, ctx, senderID, chID, 0)
}

// TestStore_ChannelMembershipScoped verifies unread is scoped to membership: a
// user not in a community never sees that community's channel as unread, even
// when messages exist in it.
func TestStore_ChannelMembershipScoped(t *testing.T) {
	store := newTestStore(t)
	pool := newTestPool(t)
	ctx := t.Context()

	memberID := mustInsertTestUser(t, pool, uniqueEmail("member"))
	outsideID := mustInsertTestUser(t, pool, uniqueEmail("outside"))
	t.Cleanup(func() { deleteUsers(t, pool, memberID, outsideID) })
	commID := mustCreateCommunity(t, pool, memberID)
	chID := mustCreateChannel(t, pool, commID)
	mustAddMember(t, pool, commID, memberID)

	mustInsertChannelMsg(t, pool, chID, memberID)
	mustInsertChannelMsg(t, pool, chID, memberID)

	assertChannelUnread(t, store, ctx, memberID, chID, 2)
	assertChannelUnread(t, store, ctx, outsideID, chID, 0) // not a member
}

// TestStore_DMReadState exercises the DM cursor lifecycle and peer resolution.
func TestStore_DMReadState(t *testing.T) {
	store := newTestStore(t)
	pool := newTestPool(t)
	ctx := t.Context()

	userID := mustInsertTestUser(t, pool, uniqueEmail("user"))
	peerID := mustInsertTestUser(t, pool, uniqueEmail("peer"))
	t.Cleanup(func() { deleteUsers(t, pool, userID, peerID) })
	convID := mustInsertDMConversation(t, pool, userID, peerID)

	// 2 messages from peer → unread 2, peer resolved correctly.
	mustInsertDMMsg(t, pool, convID, peerID)
	mustInsertDMMsg(t, pool, convID, peerID)
	unreads, err := store.GetUnreadDMs(ctx, userID)
	if err != nil {
		t.Fatalf("GetUnreadDMs: %v", err)
	}
	if got := unreadDMCount(unreads, convID); got != 2 {
		t.Fatalf("unread = %d, want 2", got)
	}
	for _, u := range unreads {
		if u.ConversationID == convID && u.PeerID != peerID {
			t.Fatalf("peer = %s, want %s", u.PeerID, peerID)
		}
	}

	// Mark read → 0; one more message → 1.
	if err := store.MarkDMRead(ctx, userID, convID); err != nil {
		t.Fatalf("MarkDMRead: %v", err)
	}
	assertDMUnread(t, store, ctx, userID, convID, 0)
	mustInsertDMMsg(t, pool, convID, peerID)
	assertDMUnread(t, store, ctx, userID, convID, 1)
}

// TestStore_AdvanceDMRead_Sender verifies the sender of a DM does not see their
// own message as unread.
func TestStore_AdvanceDMRead_Sender(t *testing.T) {
	store := newTestStore(t)
	pool := newTestPool(t)
	ctx := t.Context()

	senderID := mustInsertTestUser(t, pool, uniqueEmail("sender"))
	receiverID := mustInsertTestUser(t, pool, uniqueEmail("receiver"))
	t.Cleanup(func() { deleteUsers(t, pool, senderID, receiverID) })
	convID := mustInsertDMConversation(t, pool, senderID, receiverID)

	seq := mustInsertDMMsg(t, pool, convID, senderID)
	if err := store.AdvanceDMRead(ctx, senderID, convID, seq); err != nil {
		t.Fatalf("AdvanceDMRead: %v", err)
	}

	assertDMUnread(t, store, ctx, senderID, convID, 0)
	assertDMUnread(t, store, ctx, receiverID, convID, 1)
}
