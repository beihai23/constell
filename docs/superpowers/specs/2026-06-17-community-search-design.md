# 社区搜索（发现模式）—— 设计文档

**日期：** 2026-06-17
**状态：** 已批准（设计）
**作者：** brainstorming 会话产出

## 目标

让已登录用户能够**按名称 / 描述搜索社区**，并发现**尚未加入**的社区 —— 类似 Discord 的服务器发现。当前搜索契约（`proto/search/v1`）只覆盖用户、频道消息和 DM 消息，社区完全不可搜索。

## 关键决策

**发现范围：** 所有公开社区，包括调用者尚未加入的。

**可见性模型：** 在 `communities` 上加一个布尔字段 `is_public`，**默认 `true`**。公开社区出现在发现结果里；私有社区被排除。所有现有社区立即可被发现（在这个 dev/开源环境下可接受；所有者之后可以改为私有）。

## 方案

**由 search-service 直接查询 `communities` 表。** 这与该服务现有做法一致：它的 repository 已经直接读 `users`、`channel_messages`、`dm_messages`，并且在消息权限过滤时已经 JOIN 了 `community_members`。新增一个 `SearchCommunities` repository 方法来查询 `communities` + `community_members`，保持一致，维持单一统一的 `SearchResponse`，不引入新的 RPC，延迟最低。

**考虑过但未采用的替代方案：**
- 在 community-service 上新增 `SearchCommunities` RPC —— 服务边界更干净，但 search-service 已经越过了这条边界（直接读 `community_members`），对社区用 RPC 却对消息用直连 DB 反而前后不一致，而且更慢。
- community-service 通过独立端点承载发现功能 —— 把搜索体验拆成两次调用，迫使前端合并结果，破坏统一契约。

## 组件

### 1. 迁移 —— `deploy/migrations/014_community_visibility.up.sql`（+ `.down.sql`）

参照 `011_search_vectors.up.sql` 里的 `search_vector` 模式。

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

### 2. Proto —— `proto/search/v1/search.proto`

- 在 `SearchType` 枚举中新增：`SEARCH_TYPE_COMMUNITIES = 4;`
- 在 `SearchResponse` 中新增：`repeated CommunityResult communities = 4;`
- 新增 message：

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

执行 `make proto-gen` 重新生成 Go（`backend/pkg/proto`）和 SDK 代码。

### 3. search-service（`backend/services/search-service/`）

**`repository.go`：**
- 新增结构体 `CommunitySearchResult { ID, Name, IconURL, Description string; MemberCount int64; Joined bool; Relevance float64 }`。
- 在 `SearchRepository` 接口及其 `Repository` 实现中新增 `SearchCommunities(ctx, query, userID string, limit int) ([]CommunitySearchResult, error)`：

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
（`$1` = 查询词，`$2` = 调用者 userID，`$3` = limit。`ts_rank(c.search_vector,…)` 函数依赖于已分组的 PK `c.id`，Postgres 接受。）

**`service.go`：**
- 新增 `searchCommunities := searchAll || containsType(types, SEARCH_TYPE_COMMUNITIES)`。
- 新增 `CommunityResult` 累加器和一个 `errgroup` goroutine 调用 `s.repo.SearchCommunities`。
- 新增 `toPBCommunityResults`，并把 `Communities:` 接入 `SearchResponse`。

### 4. api-gateway

纯 proto 透传 —— 无代码改动。重新生成的 search proto 会通过现有的 `/api/v1/search` handler 自动带上新的 `communities` 字段。

### 5. SDK（`sdk/sdk-js`）

- `src/types.ts`：新增 `CommunitySearchResult` 接口，并在 `SearchResults` 中加一个 `communities: CommunitySearchResult[]` 字段。
- `src/client.ts`（`search()` 内，约 line 600）：新增一个 `communities` 映射块，与现有的 `users` / `messages` / `dmMessages` 映射并列，读取 `raw.communities` 并把 snake_case 映射成 camelCase（`icon_url→iconUrl`、`member_count→memberCount`）。

### 6. Web（`clients/web/src/components/search/SearchDialog.tsx`）

- 在结果中新增一个 **Communities** `CommandGroup`，每条结果渲染为：图标 + 名称 + 成员数 + `Joined`/`Join` 入口。
- 选中时：
  - 若 `joined` → `navigate('/' + communityId)`；
  - 否则 → 通过现有的 `JoinCommunity` 能力加入（api-gateway 已暴露；若 SDK 已有封装方法则用它，否则加一层薄 REST 封装 —— 具体在 plan 里确认），加入后跳转。
- 因为 web 的 `client.search(query, { limit })` 不传 `types`，`searchAll` 为 `true`，所以后端一旦返回社区，结果会自动包含。

## 数据流

```
web SearchDialog
  └─ client.search(q)  →  GET /api/v1/search?q=…
       └─ api-gateway  →  search-service Search RPC
            ├─ SearchUsers          (并行，errgroup)
            ├─ SearchChannelMessages
            ├─ SearchDMMessages
            └─ SearchCommunities   ← 新增 (is_public=true, +member_count, +joined)
       ← SearchResponse { users, messages, dm_messages, communities }
  ← 映射成 SDK SearchResults（现包含 communities）
  ← 在 Communities 分组里渲染
```

## 权限

- 发现功能需要登录（与现有 search 一致：`middleware.UserIDFromContext` 必须非空，否则 `Unauthenticated`）。
- 只返回 `is_public = true` 的社区。
- `member_count` 和 `joined` 按调用者计算。

## 测试

- **search-service repository 测试**（`repository_test.go` 或等价文件）：准备三个社区（一个匹配查询的公开、一个匹配查询的私有、一个不匹配的公开），且调用者已是其中一个的成员 —— 断言 (a) 只返回那个匹配的公开社区，(b) `member_count` 正确，(c) `joined` 反映调用者的成员关系。
- **重建后的端到端冒烟**：`GET /api/v1/search?q=<已有社区名>` 返回非空 `communities` 数组；私有社区不出现。

## 范围之外（未来）

- 所有者切换 `is_public` 的 UI（CreateCommunity / UpdateCommunity 表单）。在此之前，社区默认公开并保持公开。
- 三级可见性（公开 / 不列出 / 私有）。
- 发现功能的分页 / 热门 / 分类。
- 按名称搜索社区下的*频道*（与社区发现是两回事）。
