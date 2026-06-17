# Community Search (Discovery Mode) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an authenticated user search communities by name/description and discover communities they have not joined (Discord-style server discovery).

**Architecture:** Add a binary `is_public` (default true) + a generated tsvector `search_vector` to `communities`. search-service gains a `SearchCommunities` repository method (one query: `is_public` filter + full-text match + `member_count` via JOIN + `joined` via EXISTS), wired into the existing parallel `Search` RPC. The new `CommunityResult` flows through proto → api-gateway JSON → SDK → web `SearchDialog`. The SDK also gains a `joinCommunity` method used by the web's Join affordance.

**Tech Stack:** Go (Connect-RPC, pgx/v5), PostgreSQL (tsvector/GIN), Buf/proto, TypeScript SDK (tsup), React + zustand web (Vite). Postgres is on Docker host port **15432** (not 5432).

**Spec:** `docs/superpowers/specs/2026-06-17-community-search-design.md`

**Note (spec correction):** The api-gateway search handler is **not** pure proto passthrough — it explicitly maps proto fields into JSON structs (`userSearchResult`, etc.). Task 5 adds the parallel `communitySearchResult` struct + mapping. Also, the REST/SDK layer drops `relevance` for communities to match the existing user/message/dm results (the api-gateway omits `relevance` for all of them); the proto `CommunityResult` still carries it.

---

## File Structure

**Create:**
- `deploy/migrations/014_community_visibility.up.sql` — add `is_public`, `search_vector`, GIN + btree indexes.
- `deploy/migrations/014_community_visibility.down.sql` — reverse.
- `backend/services/search-service/repository_test.go` — DB-backed repo test (new file; search-service has no tests today). Mirrors the `community-service/repository_test.go` helper pattern (`testDSN`, `newTestPool`, `mustInsertTestUser`).
- `backend/services/search-service/service_test.go` — unit test for `Search` wiring using a fake repository.

**Modify:**
- `proto/search/v1/search.proto` — `SEARCH_TYPE_COMMUNITIES`, `CommunityResult` message, `communities` field on `SearchResponse`.
- `backend/pkg/proto/search/v1/search.pb.go` — regenerated via `make proto-gen` (do not hand-edit).
- `backend/services/search-service/repository.go` — `CommunitySearchResult` struct, `SearchCommunities` on interface + impl.
- `backend/services/search-service/service.go` — `searchCommunities` flag, errgroup goroutine, `toPBCommunityResults`, wire `Communities`.
- `backend/services/api-gateway/handlers/search_handler.go` — `communitySearchResult` struct, `Communities` on `searchResponse`, mapping loop.
- `sdk/sdk-js/src/types.ts` — `CommunitySearchResult`, `communities` on `SearchResults`.
- `sdk/sdk-js/src/client.ts` — `communities` mapping in `search()`; new `joinCommunity(id)` method.
- `sdk/sdk-js/tests/client.test.ts` — extend search test + add join test.
- `clients/web/src/components/search/SearchDialog.tsx` — Communities result group + join/open affordance.

---

## Task 1: Database migration (is_public + search_vector)

**Files:**
- Create: `deploy/migrations/014_community_visibility.up.sql`
- Create: `deploy/migrations/014_community_visibility.down.sql`

- [ ] **Step 1: Write the up migration**

Create `deploy/migrations/014_community_visibility.up.sql`:

```sql
-- Community discovery: visibility flag + full-text search vector.
ALTER TABLE communities ADD COLUMN is_public BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE communities ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (
    to_tsvector('simple', coalesce(name, '') || ' ' || coalesce(description, ''))
  ) STORED;

CREATE INDEX idx_communities_search ON communities USING GIN (search_vector);
CREATE INDEX idx_communities_public ON communities (is_public);
```

- [ ] **Step 2: Write the down migration**

Create `deploy/migrations/014_community_visibility.down.sql`:

```sql
DROP INDEX IF EXISTS idx_communities_public;
DROP INDEX IF EXISTS idx_communities_search;
ALTER TABLE communities DROP COLUMN IF EXISTS search_vector;
ALTER TABLE communities DROP COLUMN IF EXISTS is_public;
```

- [ ] **Step 3: Apply the migration**

`make migrate-up` is broken (wrong dir + port 5432). Use the working command (Postgres on 15432):

```bash
cd backend && go run ./tools/migrate/main.go \
  -dir "$(git rev-parse --show-toplevel)/deploy/migrations" \
  -dsn "postgres://constell:constell_dev@localhost:15432/constell?sslmode=disable" up
```

Expected: prints applying `014_community_visibility` with no error.

- [ ] **Step 4: Verify the columns/indexes exist**

```bash
docker exec constell-postgres psql -U constell -d constell -c "\d communities"
```

Expected: `is_public | boolean | not null default true`, `search_vector | tsvector`, and indexes `idx_communities_search`, `idx_communities_public`.

- [ ] **Step 5: Verify round-trip (down then up)**

```bash
cd backend && go run ./tools/migrate/main.go \
  -dir "$(git rev-parse --show-toplevel)/deploy/migrations" \
  -dsn "postgres://constell:constell_dev@localhost:15432/constell?sslmode=disable" down
```

Expected: rolls back 014 with no error; re-running the `up` command restores it. Leave it applied (`up`) when done.

- [ ] **Step 6: Commit**

```bash
git add deploy/migrations/014_community_visibility.up.sql deploy/migrations/014_community_visibility.down.sql
git commit -m "feat(db): add communities.is_public + search_vector (migration 014)"
```

---

## Task 2: Proto — CommunityResult + SEARCH_TYPE_COMMUNITIES

**Files:**
- Modify: `proto/search/v1/search.proto`
- Regenerated: `backend/pkg/proto/search/v1/search.pb.go` (+ `searchv1connect` if interface changes — it does not)

- [ ] **Step 1: Edit the enum**

In `proto/search/v1/search.proto`, add the community type to the `SearchType` enum:

```proto
enum SearchType {
  SEARCH_TYPE_UNSPECIFIED = 0;
  SEARCH_TYPE_USERS = 1;
  SEARCH_TYPE_MESSAGES = 2;
  SEARCH_TYPE_DM_MESSAGES = 3;
  SEARCH_TYPE_COMMUNITIES = 4;
}
```

- [ ] **Step 2: Add the field to SearchResponse**

```proto
message SearchResponse {
  repeated UserResult users = 1;
  repeated MessageResult messages = 2;
  repeated DMMessageResult dm_messages = 3;
  repeated CommunityResult communities = 4;
}
```

- [ ] **Step 3: Add the CommunityResult message**

Place near the other result messages:

```proto
message CommunityResult {
  string id           = 1;
  string name         = 2;
  string icon_url     = 3;
  string description  = 4;
  int64  member_count = 5;
  bool   joined       = 6;
  double relevance    = 7;
}
```

- [ ] **Step 4: Regenerate**

```bash
make proto-gen
```

Expected: `backend/pkg/proto/search/v1/search.pb.go` now contains `CommunityResult` and `SearchResponse.Communities`; no errors.

- [ ] **Step 5: Verify the generated type**

```bash
grep -n "type CommunityResult" backend/pkg/proto/search/v1/search.pb.go
grep -n "Communities " backend/pkg/proto/search/v1/search.pb.go | head
```

Expected: both return matches.

- [ ] **Step 6: Verify backend still builds**

```bash
cd backend && go build ./...
```

Expected: success (the new proto field is additive; existing code compiles).

- [ ] **Step 7: Commit**

```bash
git add proto/search/v1/search.proto backend/pkg/proto/
git commit -m "feat(proto): add CommunityResult + SEARCH_TYPE_COMMUNITIES to search"
```

---

## Task 3: search-service repository — SearchCommunities (TDD)

**Files:**
- Modify: `backend/services/search-service/repository.go`
- Create: `backend/services/search-service/repository_test.go`

- [ ] **Step 1: Write the failing repo test**

Create `backend/services/search-service/repository_test.go`. It mirrors the DB-test helper pattern from `backend/services/community-service/repository_test.go` (`testDSN`, `newTestPool`, `mustInsertTestUser`):

```go
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

func testDSN() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://constell:constell_dev@localhost:15432/constell?sslmode=disable"
}

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

func uniqueEmail(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s-%s@example.com", prefix, hex.EncodeToString(b))
}

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
		pool.Exec(ctx, "DELETE FROM communities WHERE owner_id = $1", owner)
		pool.Exec(ctx, "DELETE FROM users WHERE id = $1", owner)
		pool.Exec(ctx, "DELETE FROM users WHERE id = $1", member)
		pool.Exec(ctx, "DELETE FROM users WHERE id = $1", lone)
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
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd backend && go test ./services/search-service/ -run TestSearchCommunities -v
```

Expected: FAIL — `repo.SearchCommunities undefined` (compile error).

- [ ] **Step 3: Add the result struct + interface method**

In `backend/services/search-service/repository.go`, add the struct after `DMMessageSearchResult` (around line 37) and extend the `SearchRepository` interface (around line 40–44):

```go
// CommunitySearchResult holds a row from community discovery search.
type CommunitySearchResult struct {
	ID          string
	Name        string
	IconURL     string
	Description string
	MemberCount int64
	Joined      bool
	Relevance   float64
}
```

Add to the `SearchRepository` interface:

```go
	SearchCommunities(ctx context.Context, query string, userID string, limit int) ([]CommunitySearchResult, error)
```

- [ ] **Step 4: Implement SearchCommunities**

Add the method to `Repository` (after `SearchDMMessages`):

```go
// SearchCommunities searches public communities by name/description.
// Only is_public=true communities are returned. member_count is the member
// total; joined reflects whether userID is a member.
func (r *Repository) SearchCommunities(ctx context.Context, query string, userID string, limit int) ([]CommunitySearchResult, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.name, COALESCE(c.icon_url, ''), COALESCE(c.description, ''),
		       COUNT(cm.user_id)::bigint AS member_count,
		       EXISTS(SELECT 1 FROM community_members cm2
		              WHERE cm2.community_id = c.id AND cm2.user_id = $2) AS joined,
		       ts_rank(c.search_vector, plainto_tsquery('simple', $1)) AS relevance
		FROM communities c
		LEFT JOIN community_members cm ON cm.community_id = c.id
		WHERE c.is_public = true
		  AND c.search_vector @@ plainto_tsquery('simple', $1)
		GROUP BY c.id
		ORDER BY relevance DESC
		LIMIT $3
	`, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("search communities: %w", err)
	}
	defer rows.Close()

	var results []CommunitySearchResult
	for rows.Next() {
		var r CommunitySearchResult
		if err := rows.Scan(&r.ID, &r.Name, &r.IconURL, &r.Description, &r.MemberCount, &r.Joined, &r.Relevance); err != nil {
			return nil, fmt.Errorf("scan community result: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
```

- [ ] **Step 5: Run the test to verify it passes**

```bash
cd backend && go test ./services/search-service/ -run TestSearchCommunities -v
```

Expected: PASS. (If it SKIPs, Postgres on 15432 is down — start it with `make docker-up` and rerun.)

- [ ] **Step 6: Commit**

```bash
git add backend/services/search-service/repository.go backend/services/search-service/repository_test.go
git commit -m "feat(search-service): add SearchCommunities repo method"
```

---

## Task 4: search-service service.go — wire communities into Search (TDD)

**Files:**
- Modify: `backend/services/search-service/service.go`
- Create: `backend/services/search-service/service_test.go`

- [ ] **Step 1: Write the failing service test**

Create `backend/services/search-service/service_test.go`. Uses a fake repository so it needs no DB:

```go
package main

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	pbv1 "github.com/constell/constell/backend/pkg/proto/search/v1"
	"github.com/constell/constell/backend/pkg/middleware"
)

type fakeRepo struct {
	users      []UserSearchResult
	messages   []MessageSearchResult
	dmMessages []DMMessageSearchResult
	communities []CommunitySearchResult
}

func (f *fakeRepo) SearchUsers(ctx context.Context, query string, limit int) ([]UserSearchResult, error) {
	return f.users, nil
}
func (f *fakeRepo) SearchChannelMessages(ctx context.Context, query, userID string, limit int) ([]MessageSearchResult, error) {
	return f.messages, nil
}
func (f *fakeRepo) SearchDMMessages(ctx context.Context, query, userID string, limit int) ([]DMMessageSearchResult, error) {
	return f.dmMessages, nil
}
func (f *fakeRepo) SearchCommunities(ctx context.Context, query, userID string, limit int) ([]CommunitySearchResult, error) {
	return f.communities, nil
}

// withCaller returns a context carrying the given userID, mirroring the auth middleware.
func withCaller(userID string) context.Context {
	return context.WithValue(context.Background(), middleware.UserIDKey{}, userID)
}

func TestSearchReturnsCommunities(t *testing.T) {
	svc := NewSearchService(&fakeRepo{
		communities: []CommunitySearchResult{{ID: "c1", Name: "Gophers", MemberCount: 5, Joined: false}},
	})
	resp, err := svc.Search(withCaller("u1"), connect.NewRequest(&pbv1.SearchRequest{Query: "gophers"}))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(resp.Msg.Communities) != 1 {
		t.Fatalf("expected 1 community, got %d", len(resp.Msg.Communities))
	}
	if resp.Msg.Communities[0].GetId() != "c1" {
		t.Errorf("id = %s", resp.Msg.Communities[0].GetId())
	}
	if resp.Msg.Communities[0].GetMemberCount() != 5 {
		t.Errorf("member_count = %d", resp.Msg.Communities[0].GetMemberCount())
	}
}

func TestSearchRequiresAuth(t *testing.T) {
	svc := NewSearchService(&fakeRepo{})
	if _, err := svc.Search(context.Background(), connect.NewRequest(&pbv1.SearchRequest{Query: "x"})); err == nil {
		t.Fatal("expected Unauthenticated error, got nil")
	}
}
```

> **Verify before running:** the `middleware.UserIDKey{}` type name and `context.WithValue` key must match what `middleware.UserIDFromContext` reads. Confirm by reading `backend/pkg/middleware/` — if the key type differs (e.g., a string key or unexported type), adjust `withCaller` to use the same mechanism the middleware uses (the test must plant the same key the handler reads). If the middleware key is unexported and inaccessible, drop `TestSearchRequiresAuth` and rely on the existing auth check being unchanged (it is).

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd backend && go test ./services/search-service/ -run TestSearch -v
```

Expected: FAIL — `Communities` not populated (returns 0) or compile error on `resp.Msg.Communities` if proto not regenerated (do Task 2 first).

- [ ] **Step 3: Wire searchCommunities into service.Search**

In `backend/services/search-service/service.go`:

After the existing type flags (around line 59), add:

```go
	searchCommunities := searchAll || containsType(types, pbv1.SearchType_SEARCH_TYPE_COMMUNITIES)
```

After the `dmResults` accumulator declaration (around line 64), add:

```go
	var communityResults []CommunitySearchResult
```

After the `searchDMs` goroutine block (around line 90), add:

```go
	if searchCommunities {
		g.Go(func() error {
			var err error
			communityResults, err = s.repo.SearchCommunities(gctx, msg.Query, callerID, limit)
			return err
		})
	}
```

In the response construction (around line 99–104), add `Communities:`:

```go
	resp := connect.NewResponse(&pbv1.SearchResponse{
		Users:       toPBUserResults(userResults),
		Messages:    toPBMessageResults(messageResults),
		DmMessages:  toPBDMMessageResults(dmResults),
		Communities: toPBCommunityResults(communityResults),
	})
```

Add the converter at the end of the file (after `toPBDMMessageResults`):

```go
// toPBCommunityResults converts internal community results to proto messages.
func toPBCommunityResults(results []CommunitySearchResult) []*pbv1.CommunityResult {
	if len(results) == 0 {
		return nil
	}
	out := make([]*pbv1.CommunityResult, len(results))
	for i, r := range results {
		out[i] = &pbv1.CommunityResult{
			Id:          r.ID,
			Name:        r.Name,
			IconUrl:     r.IconURL,
			Description: r.Description,
			MemberCount: r.MemberCount,
			Joined:      r.Joined,
			Relevance:   r.Relevance,
		}
	}
	return out
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd backend && go test ./services/search-service/ -v
```

Expected: PASS (both the repo DB test from Task 3 and the service unit tests).

- [ ] **Step 5: Commit**

```bash
git add backend/services/search-service/service.go backend/services/search-service/service_test.go
git commit -m "feat(search-service): wire community discovery into Search RPC"
```

---

## Task 5: api-gateway — surface communities in the REST response

**Files:**
- Modify: `backend/services/api-gateway/handlers/search_handler.go`

- [ ] **Step 1: Add the JSON struct**

In `backend/services/api-gateway/handlers/search_handler.go`, after the `dmMessageSearchResult` struct (around line 48) and before `searchResponse`, add:

```go
// communitySearchResult is the JSON representation of a community discovery result.
type communitySearchResult struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	IconURL     string `json:"icon_url"`
	Description string `json:"description"`
	MemberCount int64  `json:"member_count"`
	Joined      bool   `json:"joined"`
}
```

- [ ] **Step 2: Add the field to searchResponse**

```go
// searchResponse is the JSON response for GET /api/v1/search.
type searchResponse struct {
	Users       []userSearchResult      `json:"users"`
	Messages    []messageSearchResult   `json:"messages"`
	DMMessages  []dmMessageSearchResult `json:"dm_messages"`
	Communities []communitySearchResult `json:"communities"`
}
```

- [ ] **Step 3: Add the mapping loop + wire into response**

In the `Search` handler body, after the `dmMessages` loop (around line 100) and before `writeJSON`, add:

```go
	communities := make([]communitySearchResult, 0, len(msg.Communities))
	for _, c := range msg.Communities {
		communities = append(communities, communitySearchResult{
			ID:          c.GetId(),
			Name:        c.GetName(),
			IconURL:     c.GetIconUrl(),
			Description: c.GetDescription(),
			MemberCount: c.GetMemberCount(),
			Joined:      c.GetJoined(),
		})
	}
```

Change the `writeJSON` call to include `Communities`:

```go
	writeJSON(w, http.StatusOK, searchResponse{
		Users:       users,
		Messages:    messages,
		DMMessages:  dmMessages,
		Communities: communities,
	})
```

- [ ] **Step 4: Build to verify**

```bash
cd backend && go build ./services/api-gateway/
```

Expected: success.

- [ ] **Step 5: Commit**

```bash
git add backend/services/api-gateway/handlers/search_handler.go
git commit -m "feat(api-gateway): include communities in /api/v1/search response"
```

---

## Task 6: SDK — types, mapping, joinCommunity (TDD)

**Files:**
- Modify: `sdk/sdk-js/src/types.ts`
- Modify: `sdk/sdk-js/src/client.ts`
- Modify: `sdk/sdk-js/tests/client.test.ts`

- [ ] **Step 1: Add the type**

In `sdk/sdk-js/src/types.ts`, add the interface (near `UserSearchResult`) and extend `SearchResults` (line 154):

```ts
export interface CommunitySearchResult {
  id: string;
  name: string;
  iconUrl: string;
  description: string;
  memberCount: number;
  joined: boolean;
}
```

Change `SearchResults` to:

```ts
export interface SearchResults {
  users: UserSearchResult[];
  messages: MessageSearchResult[];
  dmMessages: DMMessageSearchResult[];
  communities: CommunitySearchResult[];
}
```

- [ ] **Step 2: Extend the failing test**

In `sdk/sdk-js/tests/client.test.ts`, find the `it("search calls correct endpoint and maps response", …)` block (around line 857). Add `communities` to the `restResponse` and an assertion. Updated block:

```ts
    it("search calls correct endpoint and maps response", async () => {
      const restResponse = {
        users: [{ id: "u2", nickname: "Alice", avatar_url: "", relevance: 0.9 }],
        messages: [{ id: "m1", channel_id: "ch1", community_id: "s1", author_id: "u1", content: "hello", created_at: 0, relevance: 0.8 }],
        dm_messages: [{ id: "dm1", conversation_id: "conv1", peer_id: "u2", content: "hi", created_at: 0, relevance: 0.7 }],
        communities: [{ id: "c1", name: "Gophers", icon_url: "u", description: "d", member_count: 5, joined: false }],
      };

      vi.spyOn(client.rest, "get").mockResolvedValueOnce(restResponse);

      const result = await client.search("hello", { limit: 10 });

      expect(client.rest.get).toHaveBeenCalledWith("/api/v1/search?q=hello&limit=10");
      expect(result.communities).toEqual([
        { id: "c1", name: "Gophers", iconUrl: "u", description: "d", memberCount: 5, joined: false },
      ]);
    });
```

Also add a join test after it:

```ts
    it("joinCommunity posts to the join endpoint", async () => {
      vi.spyOn(client.rest, "post").mockResolvedValueOnce({} as never);
      await client.joinCommunity("c1");
      expect(client.rest.post).toHaveBeenCalledWith("/api/v1/communities/c1/join", undefined);
    });
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
cd sdk/sdk-js && npm test
```

Expected: FAIL — `result.communities` undefined / `client.joinCommunity is not a function`.

- [ ] **Step 4: Add the communities mapping in search()**

In `sdk/sdk-js/src/client.ts`, inside `search()` (around line 600–620, after the `dmMessages` mapping), add to the returned object:

```ts
      communities: ((raw.communities ?? []) as Record<string, unknown>[]).map((c) => ({
        id: c.id as string,
        name: c.name as string,
        iconUrl: (c.icon_url ?? c.iconUrl ?? "") as string,
        description: (c.description ?? "") as string,
        memberCount: (c.member_count ?? c.memberCount ?? 0) as number,
        joined: (c.joined ?? false) as boolean,
      })),
```

- [ ] **Step 5: Add joinCommunity method**

In `sdk/sdk-js/src/client.ts`, add near the other community methods (e.g., after `getChannels`, around line 489):

```ts
  /** Join a community by id. */
  async joinCommunity(communityId: string): Promise<void> {
    await this.rest.post(`/api/v1/communities/${communityId}/join`, undefined);
  }
```

> **Confirm the REST shape:** verify `this.rest.post` accepts `(url, body)` and sends JSON. Check `createCommunity` at ~line 481 (`this.rest.post<...>("/api/v1/communities", {...})`) — the signature is `post(url, body)`, so passing `undefined` for a bodyless POST is correct. If `post` requires a body object, pass `{}` instead.

- [ ] **Step 6: Run tests to verify they pass**

```bash
cd sdk/sdk-js && npm test
```

Expected: PASS (all, including the two new assertions).

- [ ] **Step 7: Rebuild the SDK + verify typecheck + .d.ts emits**

```bash
cd sdk/sdk-js && npm run build && npx tsc --noEmit
```

Expected: `dist/index.d.ts` regenerated, `CommunitySearchResult` present, 0 type errors, 94+ tests pass.

- [ ] **Step 8: Commit**

```bash
git add sdk/sdk-js/src/types.ts sdk/sdk-js/src/client.ts sdk/sdk-js/tests/client.test.ts
git commit -m "feat(sdk-js): map communities in search() + add joinCommunity"
```

---

## Task 7: web — Communities group in SearchDialog

**Files:**
- Modify: `clients/web/src/components/search/SearchDialog.tsx`

> **No web unit-test runner exists** (Playwright e2e only). Verification here is `tsc -b` + build + manual click-through.

- [ ] **Step 1: Read the current render structure**

Read `clients/web/src/components/search/SearchDialog.tsx` fully to see how existing `CommandGroup`s (Users / Messages / DMs) render results, so the new Communities group matches the established pattern (icon, label, onSelect). Mirror that exact structure.

- [ ] **Step 2: Import the type + add a result type**

Ensure the SDK import includes `CommunitySearchResult` (extend the existing `import type { SearchResults, ... } from '@constell/sdk-js';` line). Add a local alias if the file uses one:

```ts
import type { SearchResults, CommunitySearchResult } from '@constell/sdk-js';
```

- [ ] **Step 3: Add navigation/join handler**

Near the existing `goToUser` / `goToChannelMessage` callbacks (around line 193), add:

```ts
  const goToCommunity = useCallback(
    async (community: CommunitySearchResult) => {
      if (!community.joined) {
        try {
          await client.joinCommunity(community.id);
        } catch {
          toast.error('Failed to join community');
          return;
        }
      }
      navigate(`/${community.id}`);
      onOpenChange(false);
    },
    [navigate, onOpenChange, client],
  );
```

(`toast` and `client` are already imported/available in this file — confirm when reading it in Step 1.)

- [ ] **Step 4: Render the Communities group**

In the results JSX, after the existing groups and gated on `results.communities.length > 0`, add a `CommandGroup` mirroring the existing group markup:

```tsx
          {results.communities.length > 0 && (
            <CommandGroup heading="Communities">
              {results.communities.map((c) => (
                <CommandItem
                  key={c.id}
                  value={`community ${c.name}`}
                  onSelect={() => goToCommunity(c)}
                >
                  <Hash className="mr-2 h-4 w-4" />
                  <span className="flex-1">{c.name}</span>
                  <span className="text-xs text-muted-foreground">
                    {c.memberCount} members
                  </span>
                  {c.joined ? (
                    <span className="ml-2 text-xs text-muted-foreground">Joined</span>
                  ) : (
                    <span className="ml-2 text-xs text-primary">Join</span>
                  )}
                </CommandItem>
              ))}
            </CommandGroup>
          )}
```

> Match the exact `CommandItem`/`CommandGroup` props and classes the existing groups use (read in Step 1). `Hash` is already imported from `lucide-react`. If a community avatar (`iconUrl`) should show, use the existing `Avatar` pattern — optional, keep first version text-only if the existing groups are text-only.

- [ ] **Step 5: Typecheck + build**

```bash
cd clients/web && npx tsc -b && npm run build
```

Expected: 0 errors; vite build succeeds.

- [ ] **Step 6: Commit**

```bash
git add clients/web/src/components/search/SearchDialog.tsx
git commit -m "feat(web): show discoverable communities in SearchDialog"
```

---

## Task 8: Rebuild services + end-to-end verification

**Files:** none (rebuild + verify)

- [ ] **Step 1: Rebuild search-service + api-gateway images**

```bash
cd deploy/docker && docker compose -p docker build search-service api-gateway
```

Expected: both images build.

- [ ] **Step 2: Recreate the two containers**

```bash
cd deploy/docker && docker compose -p docker up -d search-service api-gateway
```

Expected: both recreate and go `healthy`.

- [ ] **Step 3: Seed a discoverable community (if none)**

```bash
docker exec constell-postgres psql -U constell -d constell -c \
  "SELECT id, name, is_public FROM communities LIMIT 5;"
```

If empty or all private, insert a public one owned by an existing user (grab a real user id from `users` first):

```bash
UID=$(docker exec constell-postgres psql -U constell -d constell -tAc "SELECT id FROM users LIMIT 1;")
docker exec constell-postgres psql -U constell -d constell -c \
  "INSERT INTO communities (name, description, owner_id, is_public) VALUES ('Gophers United', 'a go community', '$UID', true);"
```

- [ ] **Step 4: Get a token + call search**

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"<existing-user-email>","password":"<password>"}' | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
curl -s "http://localhost:8080/api/v1/search?q=gophers&limit=10" \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool
```

Expected: a JSON object with a non-empty `"communities"` array containing `Gophers United` (`member_count`, `joined` present). A private community with "gophers" in its name must NOT appear.

- [ ] **Step 5: Manual web check**

Open `http://localhost:3000`, log in, open search (⌘K), type "gophers" — expect a **Communities** group showing `Gophers United` with member count and a `Join`/`Joined` label. Click it: if not joined, it joins then navigates to `/{id}`; if joined, navigates directly.

- [ ] **Step 6: Final commit (if any verification fixups)**

Only if Steps 1–5 surfaced code changes not already committed. Otherwise no commit — all tasks are committed individually.

---

## Done criteria

- `GET /api/v1/search?q=…` returns a `communities` array (public matches only).
- Private communities never appear in discovery.
- `member_count` and `joined` are correct for the caller.
- Web search shows a Communities group; clicking joins (if needed) and navigates.
- All backend tests pass (`go test ./services/search-service/...`); SDK tests pass (`npm test`); web `tsc -b` + `npm run build` clean.
