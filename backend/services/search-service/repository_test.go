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
