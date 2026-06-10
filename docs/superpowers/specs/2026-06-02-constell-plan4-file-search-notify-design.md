# Constell — Plan 4: File Service + Search Service + Notify Service 设计规格

> **阶段定位：** Plan 4 在 Plan 3 (WS Gateway) 之后，为系统添加文件存储、全文搜索、未读通知三大能力。
> 本文档是 Plan 4 实施的唯一上下文源。

## 决策摘要

| 决策点 | 结论 |
|--------|------|
| Notify 范围 | In-app via WS Gateway + Redis 未读计数（Pointer 方案），不做 Web Push |
| File 范围 | 完整版：上传/下载 + 缩略图 + 分块上传 + 消息附件 + 头像/图标上传 |
| Search 范围 | 频道消息 + DM + 用户搜索，DM 限自己参与的会话 |
| 未读粒度 | 每 DM 会话 + 每频道 |
| 未读方案 | Pointer：维护频道/会话消息总数 + 用户已读指针，差值 = 未读数 |
| 附件模型 | 独立 attachments 表，消息 1:N 附件 |
| Search 架构 | Search Service 直接查 PG（方案 B），MVP 优先简单 |
| file_id 生成 | 客户端生成 UUIDv4 |
| TODO 优化 | 消息读取时 file_id → URL 解析，减少客户端对 File Service 的额外请求 |

## 服务一览

| 服务 | 端口 | 状态 | 依赖 | 职责 |
|------|------|------|------|------|
| File Service | 9084 | 无状态 | MinIO, PG | 文件上传/下载/缩略图/分块上传 |
| Search Service | 9085 | 无状态 | PG (tsvector) | 统一搜索，权限过滤 |
| Notify Service | 9086 | 无状态 | Redis, NATS, PG | 未读计数 (Pointer) + 实时通知推送 |

---

## 1. File Service

### 1.1 上传流程

**普通上传**（< 5MB）：

```
Client 生成 file_id (UUIDv4)
→ File Service.Upload(file_id, filename, content_type, data)
  → PG: INSERT file_metadata (id=file_id, ...)
  → MinIO: PutObject(object_key = originals/{file_id})
  → 如果是图片: 缩略图 → PutObject(thumbnails/{file_id})
← {file_id, url, thumbnail_url}
```

**分块上传**（≥ 5MB，最大 25MB）：

```
1. Client 生成 file_id (UUIDv4)
2. File Service.InitMultipartUpload(file_id, filename, content_type)
   → MinIO: CreateMultipartUpload(bucket, originals/{file_id})
   → PG: INSERT file_metadata (id=file_id, status='uploading')
   ← {upload_id}

3. Client → File Service.UploadPart(file_id, upload_id, part_number, data)  [多次]
   → MinIO: UploadPart(...)
   ← {etag}

4. Client → File Service.CompleteMultipartUpload(file_id, upload_id, parts[{part_number, etag}])
   → MinIO: CompleteMultipartUpload(...)
   → PG: UPDATE file_metadata SET status='ready'
   → 如果是图片: 生成缩略图
   ← {file_id, url, thumbnail_url}
```

服务端在 `file_metadata` 表主键使用客户端传来的 UUID。INSERT 时如果冲突返回 `AlreadyExists`。

### 1.2 下载

- `GetFilePresignedURL(file_id)` → MinIO 生成 15 分钟有效期的 presigned URL
- 客户端拿到 URL 直接从 MinIO 下载，不经过 File Service 代理

### 1.3 缩略图

- 上传图片时自动生成缩略图（固定宽度 256px，保持比例）
- 使用 Go 标准库 `image` + `image/jpeg` + `image/png` 做缩放
- 非图片文件跳过缩略图
- 存储路径：原图 `originals/{file_id}`，缩略图 `thumbnails/{file_id}`

### 1.4 头像/图标上传

- 复用 `UploadFile` RPC，返回 file_id + URL
- 调用方（User Svc `UpdateProfile`、Community Svc `UpdateServer`）在各自的 RPC 中更新 avatar_url / icon_url
- File Service 不需要专门的 "set as avatar" RPC

### 1.5 数据模型

```sql
-- 010_file_metadata.up.sql
CREATE TABLE file_metadata (
    id            UUID PRIMARY KEY,  -- 客户端生成的 UUIDv4
    uploader_id   UUID NOT NULL REFERENCES users(id),
    filename      VARCHAR(255) NOT NULL,
    content_type  VARCHAR(128) NOT NULL,
    size          BIGINT NOT NULL,
    status        VARCHAR(16) NOT NULL DEFAULT 'uploading',  -- 'uploading' | 'ready'
    bucket        VARCHAR(64) NOT NULL DEFAULT 'constell',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_file_metadata_uploader ON file_metadata(uploader_id, created_at DESC);
```

object_key 由服务端按规则生成：`originals/{file_id}`（原图）、`thumbnails/{file_id}`（缩略图）。不需要存入数据库，file_id 即可推导。

### 1.6 Proto 定义

```protobuf
// file/v1/file.proto
syntax = "proto3";
package file.v1;
option go_package = "github.com/constell/constell/backend/pkg/proto/filev1";

service FileService {
  // 普通上传
  rpc UploadFile(UploadFileRequest) returns (UploadFileResponse);
  // 分块上传
  rpc InitMultipartUpload(InitMultipartUploadRequest) returns (InitMultipartUploadResponse);
  rpc UploadPart(UploadPartRequest) returns (UploadPartResponse);
  rpc CompleteMultipartUpload(CompleteMultipartUploadRequest) returns (CompleteMultipartUploadResponse);
  // 下载
  rpc GetFilePresignedURL(GetFilePresignedURLRequest) returns (GetFilePresignedURLResponse);
  // 删除
  rpc DeleteFile(DeleteFileRequest) returns (DeleteFileResponse);
}

// --- Upload ---
message UploadFileRequest {
  string file_id = 1;       // 客户端生成 UUIDv4
  string filename = 2;
  string content_type = 3;
  bytes data = 4;
}
message UploadFileResponse {
  FileInfo file = 1;
}

// --- Multipart Upload ---
message InitMultipartUploadRequest {
  string file_id = 1;
  string filename = 2;
  string content_type = 3;
}
message InitMultipartUploadResponse {
  string upload_id = 1;
}

message UploadPartRequest {
  string file_id = 1;
  string upload_id = 2;
  int32 part_number = 3;
  bytes data = 4;
}
message UploadPartResponse {
  string etag = 1;
}

message CompletedPart {
  int32 part_number = 1;
  string etag = 2;
}
message CompleteMultipartUploadRequest {
  string file_id = 1;
  string upload_id = 2;
  repeated CompletedPart parts = 3;
}
message CompleteMultipartUploadResponse {
  FileInfo file = 1;
}

// --- Download ---
message GetFilePresignedURLRequest {
  string file_id = 1;
}
message GetFilePresignedURLResponse {
  string url = 1;
  int64 expires_at = 2;
}

// --- Delete ---
message DeleteFileRequest {
  string file_id = 1;
}
message DeleteFileResponse {}

// --- Shared ---
message FileInfo {
  string id = 1;
  string filename = 2;
  string content_type = 3;
  int64 size = 4;
  string url = 5;
  string thumbnail_url = 6;
  int64 created_at = 7;
}
```

### 1.7 TODO 优化：file_id → URL 解析

当前：消息附件存 `file_id`，客户端再调 File Service 拿下载 URL。

优化方向：Community/User Service 读消息时 JOIN `file_metadata`，把 `file_id` 替换成直接可用的 URL 返回给客户端。

后续加 CDN 时只需改 URL 模板：`https://cdn.constell.im/files/{file_id}`。

---

## 2. Search Service

### 2.1 架构

Search Service 直接查询 PostgreSQL 的 tsvector 索引（方案 B）。MVP 优先简单，不经过其他服务 RPC。

### 2.2 搜索范围

| 实体 | 来源表 | tsvector 列 | 权限过滤 |
|------|--------|-------------|----------|
| 用户 | `users` | `search_vector` (nickname) | 无限制 |
| 频道消息 | `channel_messages` | `search_vector` (content) | 必须是该频道所在 Server 的成员 |
| DM 消息 | `dm_messages` | `search_vector` (content) | 必须是该 DM 会话的参与者 |

### 2.3 数据库变更

使用 `GENERATED ALWAYS ... STORED` 列 + GIN 索引，PG 自动维护，不需要触发器。

```sql
-- 011_search_vectors.up.sql

-- users: 搜索昵称
ALTER TABLE users ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', coalesce(nickname, ''))) STORED;
CREATE INDEX idx_users_search ON users USING GIN(search_vector);

-- channel_messages: 搜索内容
ALTER TABLE channel_messages ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', coalesce(content, ''))) STORED;
CREATE INDEX idx_channel_messages_search ON channel_messages USING GIN(search_vector);

-- dm_messages: 搜索内容
ALTER TABLE dm_messages ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', coalesce(content, ''))) STORED;
CREATE INDEX idx_dm_messages_search ON dm_messages USING GIN(search_vector);
```

使用 `simple` 分词器：IM 消息包含大量中文、混合语言、网络用语，`simple` 不分词（整词匹配），对中文更友好。后续可换 `pg_jieba` 中文分词插件。

### 2.4 搜索查询

```sql
-- 用户搜索 (无权限过滤)
SELECT id, nickname, avatar_url
FROM users
WHERE search_vector @@ plainto_tsquery('simple', $1)
ORDER BY ts_rank(search_vector, plainto_tsquery('simple', $1)) DESC
LIMIT $2;

-- 频道消息搜索 (需为 Server 成员)
SELECT cm.id, cm.content, cm.author_id, cm.channel_id, c.server_id, cm.created_at
FROM channel_messages cm
JOIN channels c ON c.id = cm.channel_id
JOIN server_members sm ON sm.server_id = c.server_id AND sm.user_id = $current_user_id
WHERE cm.search_vector @@ plainto_tsquery('simple', $query)
ORDER BY ts_rank(cm.search_vector, plainto_tsquery('simple', $query)) DESC
LIMIT $limit;

-- DM 消息搜索 (需为会话参与者)
SELECT dm.id, dm.content, dm.sender_id, dm.conversation_id, dm.created_at
FROM dm_messages dm
JOIN dm_conversations dc ON dc.id = dm.conversation_id
WHERE dm.search_vector @@ plainto_tsquery('simple', $query)
  AND (dc.user_a_id = $current_user_id OR dc.user_b_id = $current_user_id)
ORDER BY ts_rank(dm.search_vector, plainto_tsquery('simple', $query)) DESC
LIMIT $limit;
```

### 2.5 并行查询

三个搜索（用户、频道消息、DM）使用 Go `errgroup` 并行执行，取各自 top-N 结果合并返回。整体延迟 = 最慢的一个查询。

### 2.6 Proto 定义

```protobuf
// search/v1/search.proto
syntax = "proto3";
package search.v1;
option go_package = "github.com/constell/constell/backend/pkg/proto/searchv1";

service SearchService {
  rpc Search(SearchRequest) returns (SearchResponse);
}

enum SearchType {
  SEARCH_TYPE_UNSPECIFIED = 0;
  SEARCH_TYPE_USERS = 1;
  SEARCH_TYPE_MESSAGES = 2;
  SEARCH_TYPE_DM_MESSAGES = 3;
}

message SearchRequest {
  string query = 1;
  repeated SearchType types = 2;  // 空 = 搜全部
  int32 limit = 3;                 // 每类结果数，默认 10
}

message SearchResponse {
  repeated UserResult users = 1;
  repeated MessageResult messages = 2;
  repeated DMMessageResult dm_messages = 3;
}

message UserResult {
  string id = 1;
  string nickname = 2;
  string avatar_url = 3;
  double relevance = 4;
}

message MessageResult {
  string id = 1;
  string channel_id = 2;
  string server_id = 3;
  string author_id = 4;
  string content = 5;
  int64 created_at = 6;
  double relevance = 7;
}

message DMMessageResult {
  string id = 1;
  string conversation_id = 2;
  string peer_id = 3;
  string content = 4;
  int64 created_at = 5;
  double relevance = 6;
}
```

---

## 3. Notify Service

### 3.1 架构

Notify Service 是 NATS 消费者 + RPC 服务的复合体：
- 订阅 `constell.dm.created` 和 `constell.message.created` 事件
- 使用 Pointer 方案维护未读计数（Redis）
- 构造通知事件通过 `gw.push.{gw_id}` 推送到 WS Gateway
- 提供 RPC 给客户端拉取未读 / 标记已读

### 3.2 Pointer 方案：未读计数

**原理：** 维护每个频道/会话的消息总数 + 每个用户的已读指针，差值 = 未读数。

**Redis Key 设计：**

```
频道消息总数：    channel_msg_count:{channel_id}        → integer (INCR 自增)
DM 会话消息总数：  dm_msg_count:{conversation_id}        → integer (INCR 自增)

用户已读指针：    read_ptr:ch:{user_id}:{channel_id}    → integer
                read_ptr:dm:{user_id}:{conv_id}         → integer

用户频道集合：    user:channels:{user_id}                → SET {channel_id, ...}
用户 DM 集合：    user:conversations:{user_id}           → SET {conv_id, ...}
```

**发消息时（写路径 — O(1)）：**

```
频道消息到达：
  1. INCR channel_msg_count:{channel_id}   ← 只 1 次，与成员数无关

DM 消息到达：
  1. INCR dm_msg_count:{conversation_id}   ← 只 1 次
```

**拉取未读（读路径）：**

```
拉取频道未读：
  1. SMEMBERS user:channels:{user_id}               ← 获取用户所在频道
  2. MGET channel_msg_count:{ch1} {ch2} ...          ← 批量获取总数
  3. MGET read_ptr:ch:{user_id}:{ch1} ...            ← 批量获取指针
  4. 内存计算差值 = channel_msg_count - read_ptr
  5. 差值 > 0 的即为有未读的频道

拉取 DM 未读：
  同上，用 user:conversations + dm_msg_count + read_ptr:dm
```

实际场景：用户通常在 50-200 个频道，两次 MGET + 内存计算 < 5ms。

**标记已读：**

```
标记频道已读：
  1. GET channel_msg_count:{channel_id} → current_count
  2. SET read_ptr:ch:{user_id}:{channel_id} = current_count

标记 DM 已读：
  同上
```

### 3.3 消息删除的偏差

如果消息被删除，`channel_msg_count` 不会减少，导致未读数偏高 1-2。对 IM 红点标记来说完全可接受——红点只告诉用户"有新东西"，精确到个位数没有实际意义。

### 3.4 集合维护

用户加入/离开 Server 或发起新 DM 时，维护 Redis 集合：

```
加入 Server → SADD user:channels:{user_id} {该 Server 所有 channel_id}
离开 Server → SREM user:channels:{user_id} {该 Server 所有 channel_id}
新 DM 会话  → SADD user:conversations:{user_id} {conv_id}
```

Notify Service 订阅 `constell.member.joined` / `constell.member.left` / `constell.dm.created` 事件来维护这些集合。

成员加入 Server 时，Notify 需要查询该 Server 有哪些 channels。两种方式：
- 方案 A：`constell.member.joined` 事件载荷带上 `channel_ids[]`
- 方案 B：Notify 调 Community Svc `ListChannels` RPC

优先方案 A，减少 RPC 调用。离开 Server 时同理，带上 `channel_ids[]`。

### 3.5 NATS 事件消费与通知推送

**DM 消息事件处理：**

```
收到 constell.dm.created {sender_id, receiver_id, conversation_id, content, created_at}:
  1. INCR dm_msg_count:{conversation_id}
  2. 确保 SADD user:conversations:{sender_id} {conversation_id}
  3. 确保 SADD user:conversations:{receiver_id} {conversation_id}
  4. 查 Redis GET gw:{receiver_id} → gw_id
  5. 如果在线 → Publish gw.push.{gw_id} {NotificationEvent type=dm, ...}
```

**频道消息事件处理：**

```
收到 constell.message.created {message_id, channel_id, server_id, sender_id, content, member_ids[], created_at}:
  1. INCR channel_msg_count:{channel_id}
  2. 查 Redis MGET gw:{member_id} for all members → 在线成员 gw_ids
  3. 按 gw_id 分组
  4. Publish gw.push.{gw_id} {NotificationEvent type=channel_message, ...} ×M
```

注意：频道消息事件**不需要给每个成员做 Redis 写操作**（Pointer 方案的核心优势）。只需 INCR 一次频道计数器 + 查在线成员推送通知。

### 3.6 NATS 事件载荷扩展

`constell.message.created` 事件需要新增 `member_ids` 字段，避免 Notify Service 额外调用 Community Svc：

```json
{
  "message_id": "...",
  "channel_id": "...",
  "server_id": "...",
  "sender_id": "...",
  "content": "...",
  "member_ids": ["...", "..."],
  "created_at": 1234567890
}
```

如果未来成员列表很大（>1000），改为只带 `server_id`，Notify 自己查 Community Svc。

### 3.7 WS Gateway 事件扩展

在 `gateway.proto` 新增通知事件类型：

```protobuf
enum ServerEventType {
  ...existing types...
  SERVER_EVENT_TYPE_NOTIFICATION = 8;
}

message NotificationEvent {
  string notification_type = 1;  // "dm" | "channel_message"
  string source_id = 2;          // conversation_id 或 channel_id
  string server_id = 3;          // 仅频道消息有
  string sender_id = 4;
  string sender_nickname = 5;
  string preview = 6;            // 消息内容预览 (截断到 50 字符)
  int64 created_at = 7;
}

// ServerEvent 新增字段
message ServerEvent {
  ...existing fields...
  NotificationEvent notification_event = 15;
}
```

### 3.8 Proto 定义

```protobuf
// notify/v1/notify.proto
syntax = "proto3";
package notify.v1;
option go_package = "github.com/constell/constell/backend/pkg/proto/notifyv1";

service NotifyService {
  // 获取所有未读计数 (客户端登录/重连时调用)
  rpc GetUnreadCounts(GetUnreadCountsRequest) returns (GetUnreadCountsResponse);
  // 标记 DM 已读
  rpc MarkDMRead(MarkDMReadRequest) returns (MarkDMReadResponse);
  // 标记频道已读
  rpc MarkChannelRead(MarkChannelReadRequest) returns (MarkChannelReadResponse);
}

message GetUnreadCountsRequest {}

message UnreadDMConversation {
  string conversation_id = 1;
  string peer_id = 2;
  int32 count = 3;
}

message UnreadChannel {
  string channel_id = 1;
  string server_id = 2;
  int32 count = 3;
}

message GetUnreadCountsResponse {
  int32 dm_total = 1;
  repeated UnreadDMConversation dm_conversations = 2;
  int32 channel_total = 3;
  repeated UnreadChannel channels = 4;
}

message MarkDMReadRequest {
  string conversation_id = 1;
}
message MarkDMReadResponse {}

message MarkChannelReadRequest {
  string channel_id = 1;
}
message MarkChannelReadResponse {}
```

---

## 4. 消息附件

### 4.1 数据模型

```sql
-- 012_attachments.up.sql
CREATE TABLE attachments (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_type  VARCHAR(16) NOT NULL,  -- 'channel' | 'dm'
    message_id    UUID NOT NULL,          -- channel_messages.id 或 dm_messages.id
    file_id       UUID NOT NULL REFERENCES file_metadata(id),
    filename      VARCHAR(255) NOT NULL,
    content_type  VARCHAR(128) NOT NULL,
    size          BIGINT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_attachments_message ON attachments(message_type, message_id);
CREATE INDEX idx_attachments_file ON attachments(file_id);
```

使用 `message_type` 区分 DM 和频道消息附件，因为两者 message_id 来自不同的表（都用 UUID，可能冲突）。

### 4.2 发送带附件的消息流程

```
1. Client 生成 file_id → 上传文件到 File Service
2. Client 发送消息:
   - WS: SendChannelMessageRequest {content, file_ids: [file_id1, file_id2]}
   - WS: SendDMRequest {content, file_ids: [file_id1]}
3. Community/User Service:
   - INSERT message → PG
   - INSERT attachments (file_id, message_id) → PG
4. 返回消息 + 附件列表给客户端
```

### 4.3 Proto 变更

`common/v1/common.proto` 新增共享 Attachment 类型：
```protobuf
message Attachment {
  string id = 1;
  string file_id = 2;
  string filename = 3;
  string content_type = 4;
  int64 size = 5;
  string url = 6;           // TODO: 目前为空，优化后填充 presigned URL
  string thumbnail_url = 7; // 图片缩略图 URL
}
```

`community/v1/community.proto`:
```protobuf
message ChannelMessage {
  ...existing fields...
  repeated common.v1.Attachment attachments = 7;
}
message SendMessageRequest {
  string channel_id = 1;
  string content = 2;
  repeated string file_ids = 3;  // 附件 file_id 列表
}
```

`user/v1/user.proto`:
```protobuf
message DMMessage {
  ...existing fields...
  repeated common.v1.Attachment attachments = 5;
}
message SendDMRequest {
  string target_user_id = 1;
  string content = 2;
  repeated string file_ids = 3;
}
```

`gateway/v1/gateway.proto`:
```protobuf
message SendChannelMessageRequest {
  string channel_id = 1;
  string content = 2;
  repeated string file_ids = 3;
}
message SendDMRequest {
  string receiver_id = 1;
  string content = 2;
  repeated string file_ids = 3;
}
message ChannelMessageReceivedEvent {
  ...existing fields...
  repeated common.v1.Attachment attachments = 7;
}
message DMReceivedEvent {
  ...existing fields...
  repeated common.v1.Attachment attachments = 6;
}
```

---

## 5. 跨系统变更

### 5.1 Migrations 清单

| 编号 | 文件 | 内容 |
|------|------|------|
| 010 | `010_file_metadata.up.sql` | file_metadata 表 |
| 011 | `011_search_vectors.up.sql` | users、channel_messages、dm_messages 加 tsvector 列 + GIN 索引 |
| 012 | `012_attachments.up.sql` | attachments 表 |

### 5.2 Proto 新增

| 包 | 内容 |
|----|------|
| `file/v1/file.proto` | FileService 完整定义 |
| `search/v1/search.proto` | SearchService 定义 |
| `notify/v1/notify.proto` | NotifyService 定义 |
| `community/v1/community.proto` | SendMessageRequest 加 file_ids，ChannelMessage 加 attachments |
| `user/v1/user.proto` | SendDMRequest 加 file_ids，DMMessage 加 attachments |
| `gateway/v1/gateway.proto` | SendDMRequest/SendChannelMessageRequest 加 file_ids，NotificationEvent，事件加 attachments |

### 5.3 Docker Compose 新增

```yaml
file-service:
  environment:
    PORT: "9084"
    DATABASE_URL: "postgres://constell:constell_dev@postgres:5432/constell?sslmode=disable"
    MINIO_ENDPOINT: "minio:9000"
    MINIO_ACCESS_KEY: "minioadmin"
    MINIO_SECRET_KEY: "minioadmin"
    MINIO_BUCKET: "constell"
    MINIO_USE_SSL: "false"
  ports: ["9084:9084"]

search-service:
  environment:
    PORT: "9085"
    DATABASE_URL: "postgres://constell:constell_dev@postgres:5432/constell?sslmode=disable"
  ports: ["9085:9085"]

notify-service:
  environment:
    PORT: "9086"
    DATABASE_URL: "postgres://constell:constell_dev@postgres:5432/constell?sslmode=disable"
    REDIS_URL: "redis:6379"
    NATS_URL: "nats://nats:4222"
  ports: ["9086:9086"]
```

三个服务都加上治理包环境变量（REGISTRY_TYPE, SERVICES_CONFIG_PATH, OTEL_EXPORTER_OTLP_ENDPOINT）和 healthcheck。

### 5.4 API Gateway 路由新增

```
POST   /api/v1/files/upload              → File Svc UploadFile
POST   /api/v1/files/multipart/init      → File Svc InitMultipartUpload
POST   /api/v1/files/multipart/part      → File Svc UploadPart
POST   /api/v1/files/multipart/complete  → File Svc CompleteMultipartUpload
GET    /api/v1/files/:id/url             → File Svc GetFilePresignedURL
DELETE /api/v1/files/:id                 → File Svc DeleteFile
GET    /api/v1/search?q=...&types=...    → Search Svc Search
GET    /api/v1/notify/unread             → Notify Svc GetUnreadCounts
POST   /api/v1/notify/dm/:conv_id/read   → Notify Svc MarkDMRead
POST   /api/v1/notify/channel/:ch_id/read → Notify Svc MarkChannelRead
```

### 5.5 services.yaml 更新

在 `deploy/configs/services.yaml` 中添加三个新服务的地址配置。

### 5.6 dev.yaml 更新

```yaml
services:
  ...existing...
  file_service:
    addr: :9084
  search_service:
    addr: :9085
  notify_service:
    addr: :9086
```

---

## 6. 验证方式

Docker Compose 一键启动后：

1. **File Service**: `curl` 上传图片 → 获取 presigned URL → 浏览器打开下载；分块上传 20MB 文件
2. **Search Service**: 发若干消息后，`curl` 搜索关键字返回匹配结果；验证非成员搜不到频道消息
3. **Notify Service**: 用户 A 发消息 → 用户 B 通过 WS Gateway 收到 NotificationEvent；调 GetUnreadCounts 返回正确计数；MarkRead 后计数归零
4. **附件**: 上传文件 → 发消息带 file_ids → GetMessages 返回附件列表

---

## 7. 后续演进

- **file_id → URL 解析**: 消息读取时自动解析 file_id 为下载 URL，减少客户端请求
- **Web Push**: Notify Service 添加 Web Push (VAPID) 支持离线通知
- **中文分词**: 替换 `simple` 为 `pg_jieba` 提升中文搜索质量
- **Elasticsearch**: Search Service 底层替换为 ES，API 不变
- **CDN**: MinIO 前加 CDN，URL 模板改为 `https://cdn.constell.im/files/{file_id}`
- **通知历史**: Notify Service 持久化通知记录，支持通知列表和标记已读
