# Community Search (Discovery Mode) — Design

**Date:** 2026-06-17
**Status:** Approved (design)
**Author:** brainstorming session

## Goal

Let an authenticated user search for **communities by name/description** and discover communities they are **not** a member of — Discord-style server discovery. Today the search contract (`proto/search/v1`) covers only users, channel messages, and DM messages; communities are not searchable at all.

## Key decision

**Discovery scope:** all public communities, including ones the caller has not joined.

**Visibility model:** binary `is_public` boolean on `communities`, **default `true`**. Public communities appear in discovery; private ones are excluded. All existing communities become discoverable immediately (acceptable for this dev/OSS context; owners can flip a community private later).

## Approach

**search-service queries the `communities` table directly.** This matches how the service already works: its repository reads `users`, `channel_messages`, `dm_messages`, and already joins `community_members` for message-membership filtering. Adding a `SearchCommunities` repository method that queries `communities` + `community_members` is consistent, keeps a single unified `SearchResponse`, adds no new RPC, and minimizes latency.

**Alternatives considered and rejected:**
- New `SearchCommunities` RPC on community-service — cleaner service boundary, but search-service already crosses it (reads `community_members`), so using RPC for communities while using direct-DB for messages would be inconsistent and slower.
- community-service hosts discovery via a separate endpoint — splits the search UX across two calls and forces the web to merge results; breaks the unified contract.

## Components

### 1. Migration — `deploy/migrations/014_community_visibility.up.sql` (+ `.down.sql`)

Mirrors the `search_vector` pattern from `011_search_vectors.up.sql`.

```sql
-- 014_community_visibility.up.sql
ALTER TABLE communities ADD COLUMN is_public BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE communities ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (
    to_tsvector('simple', coalesce(name, '') || ' ' || coalesce(description, ''))
  ) STORED;

CREATE INDEX idx_communities_search ON communities USING GIN (search_vector);
CREATE INDEX idx_communities_public ON communities (is_public);
```

```sql
-- 014_community_visibility.down.sql
DROP INDEX IF EXISTS idx_communities_public;
DROP INDEX IF EXISTS idx_communities_search;
ALTER TABLE communities DROP COLUMN IF EXISTS search_vector;
ALTER TABLE communities DROP COLUMN IF EXISTS is_public;
```

### 2. Proto — `proto/search/v1/search.proto`

- Add to the `SearchType` enum: `SEARCH_TYPE_COMMUNITIES = 4;`
- Add to `SearchResponse`: `repeated CommunityResult communities = 4;`
- New message:

```proto
message CommunityResult {
  string id          = 1;
  string name        = 2;
  string icon_url    = 3;
  string description = 4;
  int64  member_count = 5;
  bool   joined      = 6;
  double relevance   = 7;
}
```

Run `make proto-gen` to regenerate Go (`backend/pkg/proto`) and SDK code.

### 3. search-service (`backend/services/search-service/`)

**`repository.go`:**
- New struct `CommunitySearchResult { ID, Name, IconURL, Description string; MemberCount int64; Joined bool; Relevance float64 }`.
- Add `SearchCommunities(ctx, query, userID string, limit int) ([]CommunitySearchResult, error)` to the `SearchRepository` interface and its `Repository` implementation:

```sql
SELECT c.id, c.name, COALESCE(c.icon_url,''), COALESCE(c.description,''),
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
```
(`$1` = query, `$2` = caller userID, `$3` = limit. `ts_rank(c.search_vector,…)` is functionally dependent on the grouped PK `c.id`, so Postgres accepts it.)

**`service.go`:**
- Add `searchCommunities := searchAll || containsType(types, SEARCH_TYPE_COMMUNITIES)`.
- Add a `CommunityResult` accumulator and an `errgroup` goroutine calling `s.repo.SearchCommunities`.
- Add `toPBCommunityResults` and wire `Communities:` into the `SearchResponse`.

### 4. api-gateway

Pure proto passthrough — no code change. The regenerated search proto propagates the new `communities` field through the existing `/api/v1/search` handler automatically.

### 5. SDK (`sdk/sdk-js`)

- `src/types.ts`: add `CommunitySearchResult` interface and a `communities: CommunitySearchResult[]` field to `SearchResults`.
- `src/client.ts` (in `search()`, ~line 600): add a `communities` mapping block parallel to the existing `users` / `messages` / `dmMessages` mappings, reading `raw.communities` and mapping snake_case → camelCase (`icon_url→iconUrl`, `member_count→memberCount`).

### 6. Web (`clients/web/src/components/search/SearchDialog.tsx`)

- Add a **Communities** `CommandGroup` to the results, rendering each result as: icon + name + member count + a `Joined`/`Join` affordance.
- On select:
  - if `joined` → `navigate('/' + communityId)`;
  - else → join via the existing `JoinCommunity` capability (api-gateway already exposes it; the SDK wraps it if a method exists, otherwise a thin REST wrapper is added — confirmed in the plan), then navigate.
- Because the web's `client.search(query, { limit })` sends no `types`, `searchAll` is `true`, so communities are included automatically once the backend returns them.

## Data flow

```
web SearchDialog
  └─ client.search(q)  →  GET /api/v1/search?q=…
       └─ api-gateway  →  search-service Search RPC
            ├─ SearchUsers          (parallel, errgroup)
            ├─ SearchChannelMessages
            ├─ SearchDMMessages
            └─ SearchCommunities   ← NEW (is_public=true, +member_count, +joined)
       ← SearchResponse { users, messages, dm_messages, communities }
  ← mapped to SDK SearchResults (now incl. communities)
  ← rendered in Communities group
```

## Permissions

- Discovery requires authentication (matches existing search: `middleware.UserIDFromContext` must be non-empty, else `Unauthenticated`).
- Only `is_public = true` communities are returned.
- `member_count` and `joined` are computed for the caller.

## Testing

- **search-service repo test** (`repository_test.go` or equivalent): with two communities (one public matching the query, one private matching, one public non-matching) and the caller a member of one — assert (a) only the public matching community is returned, (b) `member_count` is correct, (c) `joined` reflects the caller's membership.
- **End-to-end smoke** after rebuild: `GET /api/v1/search?q=<existing-community-name>` returns a non-empty `communities` array; a private community is absent.

## Out of scope (future)

- UI for owners to toggle `is_public` (CreateCommunity / UpdateCommunity forms). Until then, communities default public and stay public.
- Three-tier visibility (public/unlisted/private).
- Discovery pagination / trending / categories.
- Search across community *channels* by name (separate from community discovery).
