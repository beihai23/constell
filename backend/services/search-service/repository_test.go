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

// testDSN returns the Postgres DSN for repo-level tests. It honors the
// DATABASE_URL env var and otherwise defaults to the Docker-exposed
// constell DB on localhost:15432 — where migration 014 has applied the
// communities.is_public and communities.search_vector columns this test
// exercises.
func testDSN() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://constell:constell_dev@localhost:15432/constell?sslmode=disable"
}

// newTestPool connects to the test Postgres. Tests that need a DB call this;
// if Postgres is unreachable the test is skipped rather than failed, so the
// suite still passes in environments without Docker running.
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

// uniqueEmail returns an email with a random suffix so concurrent runs,
// repeated -count iterations, and cross-process reruns never collide on
// the users.email unique index.
func uniqueEmail(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s-%s@example.com", prefix, hex.EncodeToString(b))
}

// mustInsertTestUser inserts a minimal user row so the communities.owner_id
// and community_members.user_id FKs are satisfied. Returns the user id.
func mustInsertTestUser(t *testing.T, pool *pgxpool.Pool, email string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(t.Context(), `
		INSERT INTO users (email, password_hash, nickname)
		VALUES ($1, 'x', $2)
		RETURNING id
	`, email, email).Scan(&id)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

// mustInsertTestCommunity inserts a community with explicit name, description
// and is_public. The owner must already exist in users.
func mustInsertTestCommunity(t *testing.T, pool *pgxpool.Pool, ownerID, name, description string, isPublic bool) string {
	t.Helper()
	var id string
	err := pool.QueryRow(t.Context(), `
		INSERT INTO communities (name, description, owner_id, is_public)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, name, description, ownerID, isPublic).Scan(&id)
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}
	return id
}

// mustAddMember adds a row to community_members for the given community/user.
func mustAddMember(t *testing.T, pool *pgxpool.Pool, communityID, userID string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO community_members (community_id, user_id) VALUES ($1, $2)
	`, communityID, userID); err != nil {
		t.Fatalf("add member: %v", err)
	}
}

// mustInsertTestUserWithID inserts a user with an explicit id (so tests can
// control dm_conversations' CHECK(user_a_id < user_b_id) ordering) and a
// random-suffix email for uniqueness.
func mustInsertTestUserWithID(t *testing.T, pool *pgxpool.Pool, id, nickname string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO users (id, email, password_hash, nickname)
		VALUES ($1, $2, 'x', $3)
	`, id, uniqueEmail(nickname), nickname); err != nil {
		t.Fatalf("insert user %s: %v", id, err)
	}
}

// mustInsertTestDMConversation inserts a dm_conversations row. callerA must be
// lexicographically smaller than callerB per the table's CHECK constraint.
func mustInsertTestDMConversation(t *testing.T, pool *pgxpool.Pool, userAID, userBID string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(), `
		INSERT INTO dm_conversations (user_a_id, user_b_id)
		VALUES ($1, $2)
		RETURNING id
	`, userAID, userBID).Scan(&id); err != nil {
		t.Fatalf("insert dm_conversation: %v", err)
	}
	return id
}

// mustInsertTestDMMessage inserts a dm_message in the given conversation.
func mustInsertTestDMMessage(t *testing.T, pool *pgxpool.Pool, convID, senderID, content string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(), `
		INSERT INTO dm_messages (conversation_id, sender_id, content)
		VALUES ($1, $2, $3)
		RETURNING id
	`, convID, senderID, content).Scan(&id); err != nil {
		t.Fatalf("insert dm_message %q: %v", content, err)
	}
	return id
}

// mustInsertTestChannel inserts a channel in the given community.
func mustInsertTestChannel(t *testing.T, pool *pgxpool.Pool, communityID, name string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(), `
		INSERT INTO channels (community_id, name)
		VALUES ($1, $2)
		RETURNING id
	`, communityID, name).Scan(&id); err != nil {
		t.Fatalf("insert channel %q: %v", name, err)
	}
	return id
}

// mustInsertTestChannelMessage inserts a channel message.
func mustInsertTestChannelMessage(t *testing.T, pool *pgxpool.Pool, channelID, authorID, content string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(t.Context(), `
		INSERT INTO channel_messages (channel_id, author_id, content)
		VALUES ($1, $2, $3)
		RETURNING id
	`, channelID, authorID, content).Scan(&id); err != nil {
		t.Fatalf("insert channel_message %q: %v", content, err)
	}
	return id
}

// TestSearchDMMessages guards against a SQL operator-precedence bug where
// "WHERE user_a_id = $2 OR user_b_id = $2 AND <fts>" parsed as
// "WHERE user_a_id = $2 OR (user_b_id = $2 AND <fts>)" — so when the searching
// user was user_a_id, the full-text filter was bypassed entirely and every DM
// in the conversation came back regardless of the query. The searcher is
// deliberately placed as user_a_id to exercise that branch.
func TestSearchDMMessages(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()

	// self sorts before peer so self lands in user_a_id (the broken branch).
	const (
		self = "11111111-1111-1111-1111-111111111111"
		peer = "22222222-2222-2222-2222-222222222222"
	)
	mustInsertTestUserWithID(t, pool, self, "self")
	mustInsertTestUserWithID(t, pool, peer, "peer")
	conv := mustInsertTestDMConversation(t, pool, self, peer)

	matchA := mustInsertTestDMMessage(t, pool, conv, self, "golang is fun")     // matches "golang"
	noMatch := mustInsertTestDMMessage(t, pool, conv, peer, "hello there")      // no match
	matchB := mustInsertTestDMMessage(t, pool, conv, peer, "golang tips here")  // matches "golang"

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, "DELETE FROM dm_conversations WHERE id = $1", conv)
		_, _ = pool.Exec(c, "DELETE FROM users WHERE id IN ($1, $2)", self, peer)
	})

	repo := NewRepository(pool)
	results, err := repo.SearchDMMessages(ctx, "golang", self, 10)
	if err != nil {
		t.Fatalf("SearchDMMessages: %v", err)
	}

	// Only the two messages containing "golang" may be returned — the
	// non-matching "hello there" must be filtered out even though self is
	// user_a_id.
	if len(results) != 2 {
		t.Fatalf("expected 2 matching DMs, got %d: %+v", len(results), results)
	}
	got := map[string]bool{}
	for _, r := range results {
		got[r.ID] = true
		if r.PeerID != peer {
			t.Errorf("peer_id = %s, want %s", r.PeerID, peer)
		}
	}
	if !got[matchA] || !got[matchB] {
		t.Errorf("expected matches %s and %s, got %v", matchA, matchB, got)
	}
	if got[noMatch] {
		t.Errorf("non-matching message %s was returned: filter is leaking", noMatch)
	}
}

func TestSearchCommunities(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()

	owner := mustInsertTestUser(t, pool, uniqueEmail("owner"))
	member := mustInsertTestUser(t, pool, uniqueEmail("member"))
	lone := mustInsertTestUser(t, pool, uniqueEmail("lone"))

	// Public + matches query; `member` belongs to it.
	pubMatch := mustInsertTestCommunity(t, pool, owner, "Gophers United", "a go community", true)
	mustAddMember(t, pool, pubMatch, member)
	// Private + matches query -> must be excluded.
	privMatch := mustInsertTestCommunity(t, pool, owner, "Gophers Secret", "a go community", false)
	// Public + does not match query -> excluded.
	_ = mustInsertTestCommunity(t, pool, owner, "Rustaceans", "a rust community", true)
	// Owner is a member of their own communities.
	mustAddMember(t, pool, pubMatch, owner)
	mustAddMember(t, pool, privMatch, owner)

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, "DELETE FROM communities WHERE owner_id = $1", owner)
		_, _ = pool.Exec(c, "DELETE FROM users WHERE id = $1", owner)
		_, _ = pool.Exec(c, "DELETE FROM users WHERE id = $1", member)
		_, _ = pool.Exec(c, "DELETE FROM users WHERE id = $1", lone)
	})

	repo := NewRepository(pool)
	results, err := repo.SearchCommunities(ctx, "gophers", lone, 10)
	if err != nil {
		t.Fatalf("SearchCommunities: %v", err)
	}

	// Only the public matching community is returned.
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %+v", len(results), results)
	}
	got := results[0]
	if got.ID != pubMatch {
		t.Errorf("id = %s, want %s", got.ID, pubMatch)
	}
	if got.Name != "Gophers United" {
		t.Errorf("name = %q", got.Name)
	}
	// member_count: owner + member = 2.
	if got.MemberCount != 2 {
		t.Errorf("member_count = %d, want 2", got.MemberCount)
	}
	// `lone` is not a member -> joined false.
	if got.Joined {
		t.Errorf("joined = true, want false")
	}

	// From `member`'s perspective, joined should be true.
	resultsMember, err := repo.SearchCommunities(ctx, "gophers", member, 10)
	if err != nil {
		t.Fatalf("SearchCommunities(member): %v", err)
	}
	if len(resultsMember) != 1 || !resultsMember[0].Joined {
		t.Errorf("expected joined=true for member, got %+v", resultsMember)
	}
}

// TestSearchCommunitiesPrefix verifies that community search matches name
// PREFIXES, not just whole tokens — typing "zefron" finds "Zefronix Hub" /
// "Zefronics Commons". Under the old plainto_tsquery this returned nothing
// because "zefron" is not a complete token. Invented names ("zefron*") keep
// the test independent of whatever else lives in the shared dev DB.
func TestSearchCommunitiesPrefix(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()

	owner := mustInsertTestUser(t, pool, uniqueEmail("owner"))
	lone := mustInsertTestUser(t, pool, uniqueEmail("lone"))

	// Public communities whose name tokens start with "zefron" but are longer
	// tokens ("zefronix", "zefronics").
	hub := mustInsertTestCommunity(t, pool, owner, "Zefronix Hub", "a place to chat", true)
	commons := mustInsertTestCommunity(t, pool, owner, "Zefronics Commons", "shared space", true)
	// Public but shares no "zefron" prefix -> must be excluded.
	rust := mustInsertTestCommunity(t, pool, owner, "Rustaceans", "rust lovers", true)
	mustAddMember(t, pool, hub, owner)
	mustAddMember(t, pool, commons, owner)
	mustAddMember(t, pool, rust, owner)

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, "DELETE FROM communities WHERE owner_id = $1", owner)
		_, _ = pool.Exec(c, "DELETE FROM users WHERE id = $1", owner)
		_, _ = pool.Exec(c, "DELETE FROM users WHERE id = $1", lone)
	})

	repo := NewRepository(pool)

	// "zefron" is a proper prefix of "zefronix" and "zefronics" -> both match.
	results, err := repo.SearchCommunities(ctx, "zefron", lone, 10)
	if err != nil {
		t.Fatalf("SearchCommunities(zefron): %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 prefix matches, got %d: %+v", len(results), results)
	}
	have := make(map[string]bool, len(results))
	for _, r := range results {
		have[r.ID] = true
	}
	if !have[hub] || !have[commons] {
		t.Errorf("expected matches %s and %s, got %v", hub, commons, have)
	}
	if have[rust] {
		t.Errorf("prefix 'zefron' must not match 'Rustaceans' (id %s)", rust)
	}

	// A prefix that matches nothing returns empty.
	none, err := repo.SearchCommunities(ctx, "zzqqnotfound", lone, 10)
	if err != nil {
		t.Fatalf("SearchCommunities(zzqqnotfound): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected 0 results for unknown prefix, got %d: %+v", len(none), none)
	}
}

// TestSearchUsersPrefix verifies prefix matching on user nicknames: "zefron"
// finds a user whose nickname token is "zefronius". The email-based nickname
// from uniqueEmail("zefronius") tokenizes to include "zefronius", so no
// explicit nickname update is needed.
func TestSearchUsersPrefix(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()

	user := mustInsertTestUser(t, pool, uniqueEmail("zefronius"))

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user)
	})

	repo := NewRepository(pool)
	results, err := repo.SearchUsers(ctx, "zefron", 10)
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	found := false
	for _, r := range results {
		if r.ID == user {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("prefix 'zefron' should match user %s; got %d results", user, len(results))
	}
}

// TestSearchChannelMessagesPrefix verifies prefix matching on channel message
// content, scoped to communities the searcher belongs to.
func TestSearchChannelMessagesPrefix(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()

	owner := mustInsertTestUser(t, pool, uniqueEmail("owner"))  // message author
	member := mustInsertTestUser(t, pool, uniqueEmail("member")) // searcher + community member
	comm := mustInsertTestCommunity(t, pool, owner, "Zefronix Club", "zefronix folks", true)
	mustAddMember(t, pool, comm, owner)
	mustAddMember(t, pool, comm, member)
	ch := mustInsertTestChannel(t, pool, comm, "zefronix-chat")

	match := mustInsertTestChannelMessage(t, pool, ch, owner, "zefronix is the future")
	noMatch := mustInsertTestChannelMessage(t, pool, ch, owner, "totally unrelated content")

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, "DELETE FROM communities WHERE owner_id = $1", owner)
		_, _ = pool.Exec(c, "DELETE FROM users WHERE id = $1", owner)
		_, _ = pool.Exec(c, "DELETE FROM users WHERE id = $1", member)
	})

	repo := NewRepository(pool)
	results, err := repo.SearchChannelMessages(ctx, "zefron", member, 10)
	if err != nil {
		t.Fatalf("SearchChannelMessages: %v", err)
	}
	if len(results) != 1 || results[0].ID != match {
		t.Fatalf("expected only the prefix-matching message %s, got %d: %+v", match, len(results), results)
	}
	if results[0].ID == noMatch {
		t.Errorf("non-matching message %s should not be returned", noMatch)
	}
}

// TestSearchDMMessagesPrefix verifies prefix matching on DM content. The
// searcher is user_a_id so this also exercises the previously-fixed OR/AND
// precedence together with prefix matching.
func TestSearchDMMessagesPrefix(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()

	const (
		self = "44444444-4444-4444-4444-444444444444"
		peer = "55555555-5555-5555-5555-555555555555"
	)
	mustInsertTestUserWithID(t, pool, self, "selfprefix")
	mustInsertTestUserWithID(t, pool, peer, "peerprefix")
	conv := mustInsertTestDMConversation(t, pool, self, peer)

	match := mustInsertTestDMMessage(t, pool, conv, peer, "zefronics are great")
	noMatch := mustInsertTestDMMessage(t, pool, conv, self, "nothing to see here")

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, "DELETE FROM dm_conversations WHERE id = $1", conv)
		_, _ = pool.Exec(c, "DELETE FROM users WHERE id IN ($1, $2)", self, peer)
	})

	repo := NewRepository(pool)
	results, err := repo.SearchDMMessages(ctx, "zefron", self, 10)
	if err != nil {
		t.Fatalf("SearchDMMessages: %v", err)
	}
	if len(results) != 1 || results[0].ID != match {
		t.Fatalf("expected only the prefix-matching DM %s, got %d: %+v", match, len(results), results)
	}
	if results[0].ID == noMatch {
		t.Errorf("non-matching DM %s should not be returned", noMatch)
	}
}
