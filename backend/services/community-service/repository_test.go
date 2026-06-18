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

// testDSN returns the Postgres DSN for repo-level tests. It honors the
// DATABASE_URL env var (so CI/alternative ports work) and otherwise defaults
// to the Docker-exposed constell DB on localhost:15432 — the same DB the
// integration suite runs against, where migration 013 has applied the
// channel_messages.seq column this test exercises.
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
	// Verify the pool actually connects before returning it.
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("postgres ping failed, skipping DB test: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// mustInsertTestUser inserts a minimal user row so FKs on communities,
// channels, and channel_messages are satisfied. Returns the user id.
func mustInsertTestUser(t *testing.T, pool *pgxpool.Pool, email string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(t.Context(),
		`INSERT INTO users (email, password_hash, nickname) VALUES ($1, 'x', $2) RETURNING id`,
		email, email,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	return id
}

// mustCreateTestCommunity inserts a community row directly. The owner must
// already exist in users (FK communities.owner_id → users.id).
func mustCreateTestCommunity(t *testing.T, pool *pgxpool.Pool, ownerID string) *CommunityRow {
	t.Helper()
	return mustCreateTestCommunityWithVisibility(t, pool, ownerID, true)
}

// mustCreateTestCommunityWithVisibility inserts a community row with an
// explicit is_public value. Used to exercise the migration-014 visibility
// column without going through a CreateCommunity API that doesn't expose it.
func mustCreateTestCommunityWithVisibility(t *testing.T, pool *pgxpool.Pool, ownerID string, isPublic bool) *CommunityRow {
	t.Helper()
	var s CommunityRow
	err := pool.QueryRow(t.Context(),
		`INSERT INTO communities (name, owner_id, is_public)
		 VALUES ($1, $2, $3)
		 RETURNING id, name, description, icon_url, owner_id, created_at, updated_at`,
		"test-community", ownerID, isPublic,
	).Scan(&s.ID, &s.Name, &s.Description, &s.IconURL, &s.OwnerID,
		&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		t.Fatalf("insert test community: %v", err)
	}
	return &s
}

// mustCreateTestChannel inserts a channel row directly. The community must
// already exist (FK channels.community_id → communities.id).
func mustCreateTestChannel(t *testing.T, pool *pgxpool.Pool, communityID string) *ChannelRow {
	t.Helper()
	var c ChannelRow
	err := pool.QueryRow(t.Context(),
		`INSERT INTO channels (community_id, name)
		 VALUES ($1, $2)
		 RETURNING id, community_id, name, topic, type, position, created_at, updated_at`,
		communityID, "test-channel",
	).Scan(&c.ID, &c.CommunityID, &c.Name, &c.Topic, &c.Type, &c.Position,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		t.Fatalf("insert test channel: %v", err)
	}
	return &c
}

// mustInsertChannelMsg inserts a channel message via the repo so it returns
// the full row, including the seq column.
func mustInsertChannelMsg(t *testing.T, repo *Repository, channelID, authorID, content string) *ChannelMessageRow {
	t.Helper()
	m, err := repo.InsertChannelMessage(t.Context(), channelID, authorID, content)
	if err != nil {
		t.Fatalf("insert channel message %q: %v", content, err)
	}
	return m
}

func TestGetChannelMessagesSince(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()

	// Globally-unique email so concurrent test runs, repeated -count
	// iterations, and cross-process reruns never collide on the
	// users.email unique index.
	authorID := mustInsertTestUser(t, pool, uniqueEmail("author"))

	comm := mustCreateTestCommunity(t, pool, authorID)
	ch := mustCreateTestChannel(t, pool, comm.ID)
	repo := NewRepository(pool)

	// Cleanup the rows this test inserts, in dependency order. Do NOT rely
	// on ON DELETE CASCADE — it isn't set on every FK — so delete messages,
	// then channels, then communities, then users explicitly. Swallow
	// errors: a missing row is fine, and cleanup must not mask the real
	// failure.
	channelID := ch.ID
	communityID := comm.ID
	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM channel_messages WHERE channel_id = $1`, channelID)
		_, _ = pool.Exec(c, `DELETE FROM channels WHERE id = $1`, channelID)
		_, _ = pool.Exec(c, `DELETE FROM communities WHERE id = $1`, communityID)
		_, _ = pool.Exec(c, `DELETE FROM users WHERE id = $1`, authorID)
	})

	m1 := mustInsertChannelMsg(t, repo, ch.ID, authorID, "one")
	m2 := mustInsertChannelMsg(t, repo, ch.ID, authorID, "two")
	m3 := mustInsertChannelMsg(t, repo, ch.ID, authorID, "three")

	// Sanity: seq must be monotonically increasing.
	if !(m1.Seq < m2.Seq && m2.Seq < m3.Seq) {
		t.Fatalf("seq not monotonic: m1=%d m2=%d m3=%d", m1.Seq, m2.Seq, m3.Seq)
	}

	got, err := repo.GetChannelMessagesSince(ctx, ch.ID, m1.Seq, 100)
	if err != nil {
		t.Fatalf("GetChannelMessagesSince: %v", err)
	}
	if len(got) != 2 || got[0].ID != m2.ID || got[1].ID != m3.ID {
		t.Fatalf("expected [%s,%s] ascending, got %+v", m2.ID, m3.ID, got)
	}

	// Every returned row must carry its seq.
	for _, m := range got {
		if m.Seq == 0 {
			t.Fatalf("returned message %s has zero Seq", m.ID)
		}
	}
}

// TestIsCommunityPublic exercises the visibility gate JoinCommunity relies on.
// A public community reports (true, nil); a private one reports (false, nil);
// a nonexistent id reports (false, err). Callers treat both non-true results
// as "not self-joinable".
func TestIsCommunityPublic(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()

	ownerID := mustInsertTestUser(t, pool, uniqueEmail("owner"))
	pub := mustCreateTestCommunityWithVisibility(t, pool, ownerID, true)
	priv := mustCreateTestCommunityWithVisibility(t, pool, ownerID, false)
	repo := NewRepository(pool)

	pubID := pub.ID
	privID := priv.ID
	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM communities WHERE id IN ($1, $2)`, pubID, privID)
		_, _ = pool.Exec(c, `DELETE FROM users WHERE id = $1`, ownerID)
	})

	// Public community is joinable.
	gotPublic, err := repo.IsCommunityPublic(ctx, pubID)
	if err != nil {
		t.Fatalf("IsCommunityPublic(public): unexpected error: %v", err)
	}
	if !gotPublic {
		t.Fatalf("IsCommunityPublic(public) = false, want true")
	}

	// Private community is NOT joinable. Error must be nil so the caller can
	// distinguish "private" from "doesn't exist" if it ever needs to; JoinCommunity
	// treats both the same (CodeNotFound) to avoid leaking existence.
	gotPrivate, err := repo.IsCommunityPublic(ctx, privID)
	if err != nil {
		t.Fatalf("IsCommunityPublic(private): unexpected error: %v", err)
	}
	if gotPrivate {
		t.Fatalf("IsCommunityPublic(private) = true, want false")
	}

	// Nonexistent id returns (false, err) — pgx.ErrNoRows. Callers gate on
	// `perr != nil || !public`, so this is covered.
	gotMissing, err := repo.IsCommunityPublic(ctx, "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatalf("IsCommunityPublic(missing): expected error, got nil")
	}
	if gotMissing {
		t.Fatalf("IsCommunityPublic(missing) = true, want false")
	}
}
