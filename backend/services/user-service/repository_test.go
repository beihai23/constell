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
// dm_messages.seq column this test exercises.
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

// mustInsertTestUser inserts a minimal user row so FKs on dm_conversations
// and dm_messages are satisfied. Returns the user id.
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

// mustCreateTestConversation inserts a DM conversation row directly.
func mustCreateTestConversation(t *testing.T, pool *pgxpool.Pool, userAID, userBID string) *DMConversationRow {
	t.Helper()
	small, big := userAID, userBID
	if small > big {
		small, big = big, small
	}
	var conv DMConversationRow
	err := pool.QueryRow(t.Context(),
		`INSERT INTO dm_conversations (user_a_id, user_b_id)
		 VALUES ($1, $2) RETURNING id, user_a_id, user_b_id, created_at`,
		small, big,
	).Scan(&conv.ID, &conv.UserAID, &conv.UserBID, &conv.CreatedAt)
	if err != nil {
		t.Fatalf("insert test conversation: %v", err)
	}
	return &conv
}

func TestGetDMMessagesSince(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()

	// Globally-unique emails so concurrent test runs, repeated -count
	// iterations, and cross-process reruns never collide on the
	// users.email unique index.
	userA := mustInsertTestUser(t, pool, uniqueEmail("a"))
	userB := mustInsertTestUser(t, pool, uniqueEmail("b"))

	conv := mustCreateTestConversation(t, pool, userA, userB)
	repo := NewRepository(pool)

	// Cleanup the rows this test inserts, in dependency order. Do NOT rely
	// on ON DELETE CASCADE — it isn't set on all FKs — so delete messages,
	// then conversations, then users explicitly. Swallow errors: a missing
	// row is fine, and cleanup must not mask the real failure.
	convID := conv.ID
	userAID := userA
	userBID := userB
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM dm_messages WHERE conversation_id = $1`, convID)
		_, _ = pool.Exec(ctx, `DELETE FROM dm_conversations WHERE id = $1`, convID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, userAID, userBID)
	})

	m1, err := repo.InsertDMMessage(ctx, conv.ID, userA, "one")
	if err != nil {
		t.Fatalf("insert m1: %v", err)
	}
	m2, err := repo.InsertDMMessage(ctx, conv.ID, userA, "two")
	if err != nil {
		t.Fatalf("insert m2: %v", err)
	}
	m3, err := repo.InsertDMMessage(ctx, conv.ID, userA, "three")
	if err != nil {
		t.Fatalf("insert m3: %v", err)
	}

	// Sanity: seq must be monotonically increasing.
	if !(m1.Seq < m2.Seq && m2.Seq < m3.Seq) {
		t.Fatalf("seq not monotonic: m1=%d m2=%d m3=%d", m1.Seq, m2.Seq, m3.Seq)
	}

	got, err := repo.GetDMMessagesSince(ctx, conv.ID, m1.Seq, 100)
	if err != nil {
		t.Fatalf("GetDMMessagesSince: %v", err)
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
