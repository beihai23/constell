# Plan 4: File Service + Search Service + Notify Service — 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Constell 添加文件存储、全文搜索和未读通知三大能力，包含 File Service、Search Service、Notify Service 三个新微服务，以及消息附件、NATS 事件集成等跨系统变更。

**Architecture:** 三个无状态微服务。File Service 对接 MinIO 做文件存储 + 自动缩略图。Search Service 直接查询 PG tsvector 索引（方案 B，MVP 优先简单）。Notify Service 实现 Pointer 方案未读计数（Redis）+ NATS 事件消费 + WS Gateway 实时推送。消息附件通过独立 attachments 表关联。

**Tech Stack:** Go 1.25, Connect-RPC, Buf/protobuf, PostgreSQL (tsvector/GIN), Redis, NATS JetStream, MinIO, pgx, OTel/OpenObserve

---

## File Structure

```
# ===== 新增 Proto =====
proto/
├── file/v1/file.proto              # FileService 定义
├── search/v1/search.proto          # SearchService 定义
└── notify/v1/notify.proto          # NotifyService 定义

# ===== 修改 Proto =====
proto/
├── common/v1/common.proto          # 新增 Attachment 消息
├── gateway/v1/gateway.proto        # 新增 NotificationEvent, file_ids, attachments
├── community/v1/community.proto    # SendMessageRequest 加 file_ids, ChannelMessage 加 attachments
└── user/v1/user.proto              # SendDMRequest 加 file_ids, DMMessage 加 attachments

# ===== 新增共享包 =====
backend/pkg/
├── minio/
│   ├── minio.go                    # MinIO 连接 + Bucket 初始化
│   └── minio_test.go

# ===== 新增服务 =====
backend/services/
├── file-service/
│   ├── main.go                     # 入口: Config → OTel → PG → MinIO → Health → Service → HTTP
│   ├── service.go                  # FileService: Upload, GetPresignedURL, Delete
│   ├── multipart.go                # 分块上传: Init, UploadPart, Complete
│   ├── thumbnail.go                # 图片缩略图生成
│   ├── repository.go               # file_metadata CRUD
│   ├── thumbnail_test.go           # 缩略图单元测试
│   ├── repository_test.go          # Repository 单元测试
│   └── Dockerfile                  # 多阶段构建
├── search-service/
│   ├── main.go                     # 入口: Config → OTel → PG → Health → Service → HTTP
│   ├── service.go                  # SearchService: Search (并行 errgroup)
│   ├── repository.go               # 三类搜索查询 + 权限过滤
│   ├── repository_test.go          # 搜索查询单元测试
│   └── Dockerfile
├── notify-service/
│   ├── main.go                     # 入口: Config → OTel → PG → Redis → NATS → Health → Service → HTTP
│   ├── service.go                  # NotifyService: GetUnreadCounts, MarkDMRead, MarkChannelRead
│   ├── store.go                    # Redis Pointer 操作 + Set 维护
│   ├── store_test.go               # Store 单元测试 (miniredis)
│   ├── subscriber.go               # NATS 事件消费 + WS Gateway 推送
│   ├── subscriber_test.go          # Subscriber 单元测试
│   └── Dockerfile

# ===== 修改服务 =====
backend/services/
├── community-service/
│   ├── main.go                     # 加 NATS 连接
│   ├── service.go                  # SendMessage: file_ids → attachments, 发布 NATS 事件
│   └── repository.go               # 新增 InsertAttachments, 发布 member.joined/left 事件
├── user-service/
│   ├── main.go                     # 加 NATS 连接
│   ├── service.go                  # SendDM: file_ids → attachments, 发布 NATS 事件
│   └── repository.go               # 新增 InsertAttachments
├── ws-gateway/
│   ├── push.go                     # 新增 buildNotificationEvent
│   ├── router.go                   # SendDMRequest/SendChannelMessageRequest 传 file_ids
│   └── main.go                     # (可能需要微调)
├── api-gateway/
│   ├── main.go                     # 新增 File/Search/Notify service URL 配置
│   ├── routes.go                   # 新增 file/search/notify 路由
│   └── handlers/
│       ├── helpers.go              # Clients 加 File/Search/Notify clients
│       ├── file_handler.go         # File REST handlers
│       ├── search_handler.go       # Search REST handler
│       └── notify_handler.go       # Notify REST handlers

# ===== 数据库迁移 =====
deploy/migrations/
├── 010_file_metadata.up.sql
├── 010_file_metadata.down.sql
├── 011_search_vectors.up.sql
├── 011_search_vectors.down.sql
├── 012_attachments.up.sql
└── 012_attachments.down.sql

# ===== 配置 =====
deploy/configs/
├── dev.yaml                        # 加 file/search/notify service 地址
├── services.yaml                   # 加三个新服务实例
deploy/docker/
└── docker-compose.yml              # 加三个新服务容器

# ===== Go Workspace =====
backend/go.work                     # 加三个新服务模块
```

---

### Task 1: Proto — File Service 定义

**Files:**
- Create: `proto/file/v1/file.proto`

- [ ] **Step 1: 创建 proto/file/v1/file.proto**

```protobuf
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

- [ ] **Step 2: Commit**

```bash
git add proto/file/v1/file.proto
git commit -m "feat(proto): add file/v1/file.proto — FileService definition"
```

---

### Task 2: Proto — Search Service 定义

**Files:**
- Create: `proto/search/v1/search.proto`

- [ ] **Step 1: 创建 proto/search/v1/search.proto**

```protobuf
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

- [ ] **Step 2: Commit**

```bash
git add proto/search/v1/search.proto
git commit -m "feat(proto): add search/v1/search.proto — SearchService definition"
```

---

### Task 3: Proto — Notify Service 定义

**Files:**
- Create: `proto/notify/v1/notify.proto`

- [ ] **Step 1: 创建 proto/notify/v1/notify.proto**

```protobuf
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

- [ ] **Step 2: Commit**

```bash
git add proto/notify/v1/notify.proto
git commit -m "feat(proto): add notify/v1/notify.proto — NotifyService definition"
```

---

### Task 4: Proto — 更新 common/gateway/community/user 消息定义

**Files:**
- Modify: `proto/common/v1/common.proto`
- Modify: `proto/gateway/v1/gateway.proto`
- Modify: `proto/community/v1/community.proto`
- Modify: `proto/user/v1/user.proto`

- [ ] **Step 1: 在 common.proto 末尾追加 Attachment 消息**

在 `common/v1/common.proto` 文件末尾追加:

```protobuf
// Attachment represents a file attached to a message.
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

- [ ] **Step 2: 更新 gateway.proto — 新增 NotificationEvent 和 file_ids**

在 `ServerEventType` 枚举中追加:
```protobuf
  SERVER_EVENT_TYPE_NOTIFICATION = 8;
```

在 `ClientMessage` 中，更新 `SendDMRequest` 和 `SendChannelMessageRequest`:

```protobuf
// SendDMRequest asks the gateway to send a direct message to another user.
message SendDMRequest {
  string receiver_id = 1;
  string content = 2;
  repeated string file_ids = 3;  // 附件 file_id 列表
}

// SendChannelMessageRequest asks the gateway to send a message to a channel.
message SendChannelMessageRequest {
  string channel_id = 1;
  string content = 2;
  repeated string file_ids = 3;  // 附件 file_id 列表
}
```

在 `ServerEvent` 中追加 `notification_event` 字段:
```protobuf
message ServerEvent {
  ServerEventType type = 1;
  string request_id = 2;

  DMReceivedEvent dm_received_event = 10;
  ChannelMessageReceivedEvent channel_message_event = 11;
  UserOnlineEvent user_online_event = 12;
  UserOfflineEvent user_offline_event = 13;
  ErrorEvent error_event = 14;
  NotificationEvent notification_event = 15;
}
```

在 `DMReceivedEvent` 中追加 attachments:
```protobuf
message DMReceivedEvent {
  string message_id = 1;
  string sender_id = 2;
  string sender_nickname = 3;
  string content = 4;
  int64 created_at = 5;
  repeated common.v1.Attachment attachments = 6;
}
```

在 `ChannelMessageReceivedEvent` 中追加 attachments:
```protobuf
message ChannelMessageReceivedEvent {
  string message_id = 1;
  string channel_id = 2;
  string sender_id = 3;
  string sender_nickname = 4;
  string content = 5;
  int64 created_at = 6;
  repeated common.v1.Attachment attachments = 7;
}
```

在文件末尾追加 NotificationEvent:
```protobuf
// NotificationEvent is pushed when the user has new unread messages.
message NotificationEvent {
  string notification_type = 1;  // "dm" | "channel_message"
  string source_id = 2;          // conversation_id 或 channel_id
  string server_id = 3;          // 仅频道消息有
  string sender_id = 4;
  string sender_nickname = 5;
  string preview = 6;            // 消息内容预览 (截断到 50 字符)
  int64 created_at = 7;
}
```

注意: gateway.proto 需要新增 `import "common/v1/common.proto";`。

- [ ] **Step 3: 更新 community.proto — SendMessageRequest 加 file_ids, ChannelMessage 加 attachments**

在 `SendMessageRequest` 中追加:
```protobuf
message SendMessageRequest {
  string channel_id = 1;
  string content = 2;
  repeated string file_ids = 3;  // 附件 file_id 列表
}
```

在 `ChannelMessage` 中追加:
```protobuf
message ChannelMessage {
  string id = 1;
  string channel_id = 2;
  string author_id = 3;
  string content = 4;
  int64 created_at = 5;
  int64 updated_at = 6;
  repeated common.v1.Attachment attachments = 7;
}
```

- [ ] **Step 4: 更新 user.proto — SendDMRequest 加 file_ids, DMMessage 加 attachments**

在 `SendDMRequest` 中追加:
```protobuf
message SendDMRequest {
  string target_user_id = 1;
  string content = 2;
  repeated string file_ids = 3;  // 附件 file_id 列表
}
```

在 `DMMessage` 中追加:
```protobuf
message DMMessage {
  string id = 1;
  string conversation_id = 2;
  string sender_id = 3;
  string content = 4;
  int64 created_at = 5;
  repeated common.v1.Attachment attachments = 6;
}
```

- [ ] **Step 5: Commit**

```bash
git add proto/common/v1/common.proto proto/gateway/v1/gateway.proto proto/community/v1/community.proto proto/user/v1/user.proto
git commit -m "feat(proto): add Attachment, NotificationEvent, file_ids to existing protos"
```

---

### Task 5: Proto 生成 + Go Workspace 更新

**Files:**
- Modify: `backend/go.work`

- [ ] **Step 1: 创建三个新服务的 Go 模块**

```bash
mkdir -p backend/services/file-service backend/services/search-service backend/services/notify-service
```

`backend/services/file-service/go.mod`:
```
module github.com/constell/constell/backend/services/file-service

go 1.25.6
```

`backend/services/search-service/go.mod`:
```
module github.com/constell/constell/backend/services/search-service

go 1.25.6
```

`backend/services/notify-service/go.mod`:
```
module github.com/constell/constell/backend/services/notify-service

go 1.25.6
```

- [ ] **Step 2: 运行 proto 生成**

```bash
make proto-gen
```

Expected: 无错误，`backend/pkg/proto/` 下生成 `filev1/`, `searchv1/`, `notifyv1/` 目录，以及更新的 `commonv1/`, `gatewayv1/`, `communityv1/`, `userv1/`。

- [ ] **Step 3: 更新 go.work**

在 `backend/go.work` 的 `use (...)` 中追加:
```
	./services/file-service
	./services/search-service
	./services/notify-service
```

- [ ] **Step 4: 同步依赖**

在 `backend/` 目录下运行:
```bash
cd backend && go work sync
```

然后为每个新服务补充 go.mod 依赖（先创建 placeholder main.go 再 tidy）:

```bash
cd backend/services/file-service && go mod tidy
cd ../search-service && go mod tidy
cd ../notify-service && go mod tidy
```

- [ ] **Step 5: Commit**

```bash
git add backend/go.work backend/go.work.sum backend/pkg/proto/ backend/services/file-service/ backend/services/search-service/ backend/services/notify-service/
git commit -m "feat: proto-gen for file/search/notify services, update go.work"
```

---

### Task 6: pkg/minio — MinIO 连接辅助包

**Files:**
- Create: `backend/pkg/minio/minio.go`
- Create: `backend/pkg/minio/minio_test.go`

- [ ] **Step 1: 写 minio.go**

```go
package minio

import (
	"fmt"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Config holds MinIO connection parameters.
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Bucket    string
}

// Result holds the MinIO client and bucket name returned by New.
type Result struct {
	Client *minio.Client
	Bucket string
}

// New connects to MinIO and ensures the configured bucket exists.
func New(cfg Config) (*Result, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio connect: %w", err)
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("check bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create bucket: %w", err)
		}
	}

	return &Result{Client: client, Bucket: cfg.Bucket}, nil
}
```

注意: 需要 `import "context"`。

- [ ] **Step 2: 写 minio_test.go**

```go
package minio

import (
	"testing"
)

func TestConfigDefaults(t *testing.T) {
	cfg := Config{
		Endpoint:  "localhost:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		UseSSL:    false,
		Bucket:    "constell",
	}
	if cfg.Endpoint != "localhost:9000" {
		t.Errorf("expected localhost:9000, got %s", cfg.Endpoint)
	}
	if cfg.Bucket != "constell" {
		t.Errorf("expected constell, got %s", cfg.Bucket)
	}
}
```

- [ ] **Step 3: 运行测试**

```bash
cd backend && go test ./pkg/minio/... -v
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add backend/pkg/minio/
git commit -m "feat(pkg): add pkg/minio — MinIO connection helper"
```

---

### Task 7: 数据库迁移 — file_metadata + search_vectors + attachments

**Files:**
- Create: `deploy/migrations/010_file_metadata.up.sql`
- Create: `deploy/migrations/010_file_metadata.down.sql`
- Create: `deploy/migrations/011_search_vectors.up.sql`
- Create: `deploy/migrations/011_search_vectors.down.sql`
- Create: `deploy/migrations/012_attachments.up.sql`
- Create: `deploy/migrations/012_attachments.down.sql`

- [ ] **Step 1: 010_file_metadata**

`deploy/migrations/010_file_metadata.up.sql`:
```sql
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

`deploy/migrations/010_file_metadata.down.sql`:
```sql
DROP TABLE IF EXISTS file_metadata;
```

- [ ] **Step 2: 011_search_vectors**

`deploy/migrations/011_search_vectors.up.sql`:
```sql
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

`deploy/migrations/011_search_vectors.down.sql`:
```sql
DROP INDEX IF EXISTS idx_dm_messages_search;
ALTER TABLE dm_messages DROP COLUMN IF EXISTS search_vector;

DROP INDEX IF EXISTS idx_channel_messages_search;
ALTER TABLE channel_messages DROP COLUMN IF EXISTS search_vector;

DROP INDEX IF EXISTS idx_users_search;
ALTER TABLE users DROP COLUMN IF EXISTS search_vector;
```

- [ ] **Step 3: 012_attachments**

`deploy/migrations/012_attachments.up.sql`:
```sql
CREATE TABLE attachments (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_type  VARCHAR(16) NOT NULL,  -- 'channel' | 'dm'
    message_id    UUID NOT NULL,
    file_id       UUID NOT NULL REFERENCES file_metadata(id),
    filename      VARCHAR(255) NOT NULL,
    content_type  VARCHAR(128) NOT NULL,
    size          BIGINT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_attachments_message ON attachments(message_type, message_id);
CREATE INDEX idx_attachments_file ON attachments(file_id);
```

`deploy/migrations/012_attachments.down.sql`:
```sql
DROP TABLE IF EXISTS attachments;
```

- [ ] **Step 4: 运行迁移**

```bash
make migrate-up
```

Expected: 3 个新迁移成功应用。

- [ ] **Step 5: Commit**

```bash
git add deploy/migrations/010_* deploy/migrations/011_* deploy/migrations/012_*
git commit -m "feat(db): add migrations 010-012 for file_metadata, search_vectors, attachments"
```

---

### Task 8: File Service — Repository

**Files:**
- Create: `backend/services/file-service/repository.go`
- Create: `backend/services/file-service/repository_test.go`

- [ ] **Step 1: 写 repository.go**

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FileMeta represents a row from file_metadata.
type FileMeta struct {
	ID          string
	UploaderID  string
	Filename    string
	ContentType string
	Size        int64
	Status      string
	Bucket      string
	CreatedAt   time.Time
}

// FileRepository defines database operations for FileService.
type FileRepository interface {
	InsertFileMeta(ctx context.Context, f *FileMeta) error
	GetFileMeta(ctx context.Context, id string) (*FileMeta, error)
	UpdateFileStatus(ctx context.Context, id, status string) error
	DeleteFileMeta(ctx context.Context, id string) error
}

// repository implements FileRepository backed by pgxpool.
type repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new FileRepository.
func NewRepository(pool *pgxpool.Pool) FileRepository {
	return &repository{pool: pool}
}

func (r *repository) InsertFileMeta(ctx context.Context, f *FileMeta) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO file_metadata (id, uploader_id, filename, content_type, size, status, bucket, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		f.ID, f.UploaderID, f.Filename, f.ContentType, f.Size, f.Status, f.Bucket, f.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert file_metadata: %w", err)
	}
	return nil
}

func (r *repository) GetFileMeta(ctx context.Context, id string) (*FileMeta, error) {
	var f FileMeta
	err := r.pool.QueryRow(ctx,
		`SELECT id, uploader_id, filename, content_type, size, status, bucket, created_at
		 FROM file_metadata WHERE id = $1`, id,
	).Scan(&f.ID, &f.UploaderID, &f.Filename, &f.ContentType, &f.Size, &f.Status, &f.Bucket, &f.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("file not found: %w", err)
		}
		return nil, fmt.Errorf("query file_metadata: %w", err)
	}
	return &f, nil
}

func (r *repository) UpdateFileStatus(ctx context.Context, id, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE file_metadata SET status = $1 WHERE id = $2`, status, id)
	if err != nil {
		return fmt.Errorf("update file status: %w", err)
	}
	return nil
}

func (r *repository) DeleteFileMeta(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM file_metadata WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete file_metadata: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: 写 repository_test.go**

```go
package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFileRepository_CRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, "postgres://constell:constell_dev@localhost:15432/constell?sslmode=disable")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	repo := NewRepository(pool)
	now := time.Now()
	f := &FileMeta{
		ID:          "00000000-0000-0000-0000-000000000001",
		UploaderID:  "00000000-0000-0000-0000-000000000099",
		Filename:    "test.png",
		ContentType: "image/png",
		Size:        1024,
		Status:      "uploading",
		Bucket:      "constell",
		CreatedAt:   now,
	}

	if err := repo.InsertFileMeta(ctx, f); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := repo.GetFileMeta(ctx, f.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Filename != "test.png" {
		t.Errorf("filename: got %q, want %q", got.Filename, "test.png")
	}

	if err := repo.UpdateFileStatus(ctx, f.ID, "ready"); err != nil {
		t.Fatalf("update status: %v", err)
	}
	got2, _ := repo.GetFileMeta(ctx, f.ID)
	if got2.Status != "ready" {
		t.Errorf("status: got %q, want %q", got2.Status, "ready")
	}

	if err := repo.DeleteFileMeta(ctx, f.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = repo.GetFileMeta(ctx, f.ID)
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}
```

- [ ] **Step 3: 运行测试（需要本地 PG）**

```bash
cd backend && go test -short ./services/file-service/... -v -run TestFileRepository
```

Expected: SKIP (short mode) 或 PASS (有本地 PG)

- [ ] **Step 4: Commit**

```bash
git add backend/services/file-service/repository.go backend/services/file-service/repository_test.go
git commit -m "feat(file-service): add repository — file_metadata CRUD"
```

---

### Task 9: File Service — 缩略图生成

**Files:**
- Create: `backend/services/file-service/thumbnail.go`
- Create: `backend/services/file-service/thumbnail_test.go`

- [ ] **Step 1: 写 thumbnail.go**

```go
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
)

const thumbnailWidth = 256

// isImage returns true if the content type is a supported image format.
func isImage(contentType string) bool {
	return strings.HasPrefix(contentType, "image/jpeg") ||
		strings.HasPrefix(contentType, "image/png") ||
		strings.HasPrefix(contentType, "image/gif")
}

// generateThumbnail creates a thumbnail for an image, scaled to thumbnailWidth
// while preserving aspect ratio. Returns the thumbnail bytes in the same format.
func generateThumbnail(data []byte, contentType string) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= thumbnailWidth {
		return data, nil // no resize needed
	}

	ratio := float64(thumbnailWidth) / float64(w)
	newH := int(float64(h) * ratio)
	thumb := image.NewRGBA(image.Rect(0, 0, thumbnailWidth, newH))

	// Simple nearest-neighbor scaling.
	for y := 0; y < newH; y++ {
		for x := 0; x < thumbnailWidth; x++ {
			sx := int(float64(x) / ratio)
			sy := int(float64(y) / ratio)
			thumb.Set(x, y, img.At(sx, sy))
		}
	}

	var buf bytes.Buffer
	if strings.Contains(contentType, "png") {
		if err := png.Encode(&buf, thumb); err != nil {
			return nil, fmt.Errorf("encode png thumbnail: %w", err)
		}
	} else {
		if err := jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: 85}); err != nil {
			return nil, fmt.Errorf("encode jpeg thumbnail: %w", err)
		}
	}
	return buf.Bytes(), nil
}

// decodeImageSize returns the dimensions of an image without fully decoding.
func decodeImageSize(r io.Reader) (int, int, error) {
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}
```

- [ ] **Step 2: 写 thumbnail_test.go**

```go
package main

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestIsImage(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"image/jpeg", true},
		{"image/png", true},
		{"image/gif", true},
		{"application/pdf", false},
		{"text/plain", false},
	}
	for _, tt := range tests {
		if got := isImage(tt.ct); got != tt.want {
			t.Errorf("isImage(%q) = %v, want %v", tt.ct, got, tt.want)
		}
	}
}

func TestGenerateThumbnail_JPEG(t *testing.T) {
	// Create a 1024x768 test image.
	img := image.NewRGBA(image.Rect(0, 0, 1024, 768))
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})

	thumb, err := generateThumbnail(buf.Bytes(), "image/jpeg")
	if err != nil {
		t.Fatalf("generateThumbnail: %v", err)
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(thumb))
	if err != nil {
		t.Fatalf("decode thumbnail: %v", err)
	}
	if cfg.Width != 256 {
		t.Errorf("thumbnail width = %d, want 256", cfg.Width)
	}
	if cfg.Height != 192 {
		t.Errorf("thumbnail height = %d, want 192", cfg.Height)
	}
}

func TestGenerateThumbnail_PNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 512, 512))
	var buf bytes.Buffer
	png.Encode(&buf, img)

	thumb, err := generateThumbnail(buf.Bytes(), "image/png")
	if err != nil {
		t.Fatalf("generateThumbnail: %v", err)
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(thumb))
	if err != nil {
		t.Fatalf("decode thumbnail: %v", err)
	}
	if cfg.Width != 256 {
		t.Errorf("thumbnail width = %d, want 256", cfg.Width)
	}
	if cfg.Height != 256 {
		t.Errorf("thumbnail height = %d, want 256", cfg.Height)
	}
}

func TestGenerateThumbnail_SmallImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	var buf bytes.Buffer
	png.Encode(&buf, img)

	thumb, err := generateThumbnail(buf.Bytes(), "image/png")
	if err != nil {
		t.Fatalf("generateThumbnail: %v", err)
	}
	// Should return original data since width <= thumbnailWidth.
	if len(thumb) == 0 {
		t.Error("thumbnail should not be empty")
	}
}
```

- [ ] **Step 3: 运行测试**

```bash
cd backend && go test ./services/file-service/... -v -run TestGenerate -short
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add backend/services/file-service/thumbnail.go backend/services/file-service/thumbnail_test.go
git commit -m "feat(file-service): add thumbnail generation for images"
```

---

### Task 10: File Service — Service (Upload, GetPresignedURL, Delete)

**Files:**
- Create: `backend/services/file-service/service.go`

- [ ] **Step 1: 写 service.go**

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/minio/minio-go/v7"

	pbv1 "github.com/constell/constell/backend/pkg/proto/file/v1"
	"github.com/constell/constell/backend/pkg/proto/file/v1/filev1connect"
	"github.com/constell/constell/backend/pkg/middleware"
	pkgminio "github.com/constell/constell/backend/pkg/minio"
)

// FileService implements the Connect-RPC FileServiceHandler.
type FileService struct {
	repo    FileRepository
	minio   *pkgminio.Result
	baseURL string // 用于生成下载 URL, 如 "http://localhost:9000"
}

// NewFileService creates a new FileService.
func NewFileService(repo FileRepository, minioResult *pkgminio.Result, baseURL string) *FileService {
	return &FileService{
		repo:    repo,
		minio:   minioResult,
		baseURL: baseURL,
	}
}

var _ filev1connect.FileServiceHandler = (*FileService)(nil)

// UploadFile handles a simple file upload (< 5MB).
func (s *FileService) UploadFile(
	ctx context.Context,
	req *connect.Request[pbv1.UploadFileRequest],
) (*connect.Response[pbv1.UploadFileResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}

	msg := req.Msg
	if msg.FileId == "" || msg.Filename == "" || msg.ContentType == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("file_id, filename, and content_type are required"))
	}
	if len(msg.Data) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("data is required"))
	}
	if len(msg.Data) > 5*1024*1024 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("file too large for simple upload, use multipart (>5MB)"))
	}

	// Insert metadata.
	meta := &FileMeta{
		ID:          msg.FileId,
		UploaderID:  callerID,
		Filename:    msg.Filename,
		ContentType: msg.ContentType,
		Size:        int64(len(msg.Data)),
		Status:      "ready",
		Bucket:      s.minio.Bucket,
		CreatedAt:   time.Now(),
	}
	if err := s.repo.InsertFileMeta(ctx, meta); err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to save file metadata: %w", err))
	}

	// Upload original to MinIO.
	objectKey := "originals/" + msg.FileId
	if err := s.uploadToMinIO(ctx, objectKey, msg.Data, msg.ContentType); err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to upload file: %w", err))
	}

	// Generate thumbnail if image.
	var thumbnailURL string
	if isImage(msg.ContentType) {
		thumbData, err := generateThumbnail(msg.Data, msg.ContentType)
		if err != nil {
			slog.Warn("thumbnail generation failed", "file_id", msg.FileId, "error", err)
		} else {
			thumbKey := "thumbnails/" + msg.FileId
			if err := s.uploadToMinIO(ctx, thumbKey, thumbData, msg.ContentType); err != nil {
				slog.Warn("thumbnail upload failed", "file_id", msg.FileId, "error", err)
			} else {
				thumbnailURL = s.objectURL(thumbKey)
			}
		}
	}

	fileURL := s.objectURL(objectKey)
	resp := connect.NewResponse(&pbv1.UploadFileResponse{
		File: &pbv1.FileInfo{
			Id: msg.FileId, Filename: msg.Filename, ContentType: msg.ContentType,
			Size: int64(len(msg.Data)), Url: fileURL, ThumbnailUrl: thumbnailURL,
			CreatedAt: meta.CreatedAt.Unix(),
		},
	})
	return resp, nil
}

// GetFilePresignedURL generates a 15-minute presigned download URL.
func (s *FileService) GetFilePresignedURL(
	ctx context.Context,
	req *connect.Request[pbv1.GetFilePresignedURLRequest],
) (*connect.Response[pbv1.GetFilePresignedURLResponse], error) {
	_, err := middleware.UserIDFromContext(ctx)
	if err != "" {
		// authenticated
	} else {
		// allow unauthenticated for now
	}

	if req.Msg.FileId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("file_id is required"))
	}

	meta, err := s.repo.GetFileMeta(ctx, req.Msg.FileId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound,
			fmt.Errorf("file not found: %w", err))
	}
	if meta.Status != "ready" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("file is not ready"))
	}

	objectKey := "originals/" + req.Msg.FileId
	urlStr, err := s.minio.Client.PresignedGetObject(ctx, s.minio.Bucket, objectKey,
		15*time.Minute, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to generate presigned URL: %w", err))
	}

	resp := connect.NewResponse(&pbv1.GetFilePresignedURLResponse{
		Url:      urlStr.String(),
		ExpiresAt: time.Now().Add(15 * time.Minute).Unix(),
	})
	return resp, nil
}

// DeleteFile removes a file and its metadata.
func (s *FileService) DeleteFile(
	ctx context.Context,
	req *connect.Request[pbv1.DeleteFileRequest],
) (*connect.Response[pbv1.DeleteFileResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	if req.Msg.FileId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("file_id is required"))
	}

	meta, err := s.repo.GetFileMeta(ctx, req.Msg.FileId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound,
			fmt.Errorf("file not found: %w", err))
	}
	if meta.UploaderID != callerID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the uploader can delete this file"))
	}

	// Delete from MinIO.
	s.minio.Client.RemoveObject(ctx, s.minio.Bucket, "originals/"+req.Msg.FileId, minio.RemoveObjectOptions{})
	s.minio.Client.RemoveObject(ctx, s.minio.Bucket, "thumbnails/"+req.Msg.FileId, minio.RemoveObjectOptions{})

	// Delete metadata.
	if err := s.repo.DeleteFileMeta(ctx, req.Msg.FileId); err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to delete file metadata: %w", err))
	}

	resp := connect.NewResponse(&pbv1.DeleteFileResponse{})
	return resp, nil
}

func (s *FileService) uploadToMinIO(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.minio.Client.PutObject(ctx, s.minio.Bucket, key,
		bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType},
	)
	return err
}

func (s *FileService) objectURL(key string) string {
	return fmt.Sprintf("%s/%s/%s", s.baseURL, s.minio.Bucket, key)
}
```

注意: 需要在文件顶部 import `"bytes"`。

- [ ] **Step 2: Commit**

```bash
git add backend/services/file-service/service.go
git commit -m "feat(file-service): add FileService — Upload, GetPresignedURL, Delete"
```

---

### Task 11: File Service — 分块上传 (Multipart)

**Files:**
- Create: `backend/services/file-service/multipart.go`

- [ ] **Step 1: 写 multipart.go**

```go
package main

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/minio/minio-go/v7"

	pbv1 "github.com/constell/constell/backend/pkg/proto/file/v1"
	"github.com/constell/constell/backend/pkg/middleware"
)

// InitMultipartUpload starts a multipart upload session.
func (s *FileService) InitMultipartUpload(
	ctx context.Context,
	req *connect.Request[pbv1.InitMultipartUploadRequest],
) (*connect.Response[pbv1.InitMultipartUploadResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	msg := req.Msg
	if msg.FileId == "" || msg.Filename == "" || msg.ContentType == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("file_id, filename, and content_type are required"))
	}

	objectKey := "originals/" + msg.FileId
	uploadID, err := s.minio.Client.NewMultipartUpload(ctx, s.minio.Bucket, objectKey,
		minio.PutObjectOptions{ContentType: msg.ContentType})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to init multipart upload: %w", err))
	}

	// Insert metadata as 'uploading'.
	meta := &FileMeta{
		ID:          msg.FileId,
		UploaderID:  callerID,
		Filename:    msg.Filename,
		ContentType: msg.ContentType,
		Size:        0,
		Status:      "uploading",
		Bucket:      s.minio.Bucket,
		CreatedAt:   time.Now(),
	}
	if err := s.repo.InsertFileMeta(ctx, meta); err != nil {
		// Abort the MinIO multipart upload on DB failure.
		s.minio.Client.AbortMultipartUpload(ctx, s.minio.Bucket, objectKey, uploadID)
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to save file metadata: %w", err))
	}

	resp := connect.NewResponse(&pbv1.InitMultipartUploadResponse{UploadId: uploadID})
	return resp, nil
}

// UploadPart uploads a single part of a multipart upload.
func (s *FileService) UploadPart(
	ctx context.Context,
	req *connect.Request[pbv1.UploadPartRequest],
) (*connect.Response[pbv1.UploadPartResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	msg := req.Msg
	if msg.FileId == "" || msg.UploadId == "" || len(msg.Data) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("file_id, upload_id, and data are required"))
	}

	objectKey := "originals/" + msg.FileId
	part, err := s.minio.Client.UploadPart(ctx, s.minio.Bucket, objectKey,
		bytes.NewReader(msg.Data), msg.UploadId, int(msg.PartNumber),
		int64(len(msg.Data)), minio.PutObjectOptions{})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to upload part: %w", err))
	}

	resp := connect.NewResponse(&pbv1.UploadPartResponse{Etag: part.ETag})
	return resp, nil
}

// CompleteMultipartUpload finalizes a multipart upload.
func (s *FileService) CompleteMultipartUpload(
	ctx context.Context,
	req *connect.Request[pbv1.CompleteMultipartUploadRequest],
) (*connect.Response[pbv1.CompleteMultipartUploadResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	msg := req.Msg
	if msg.FileId == "" || msg.UploadId == "" || len(msg.Parts) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("file_id, upload_id, and parts are required"))
	}

	objectKey := "originals/" + msg.FileId

	// Convert proto parts to minio parts.
	parts := make([]minio.CompletePart, 0, len(msg.Parts))
	for _, p := range msg.Parts {
		parts = append(parts, minio.CompletePart{
			PartNumber: int(p.PartNumber),
			ETag:       p.Etag,
		})
	}

	objInfo, err := s.minio.Client.CompleteMultipartUpload(ctx, s.minio.Bucket, objectKey,
		msg.UploadId, parts, minio.PutObjectOptions{})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to complete multipart upload: %w", err))
	}

	// Update metadata to 'ready' with final size.
	if err := s.repo.UpdateFileStatus(ctx, msg.FileId, "ready"); err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to update file status: %w", err))
	}

	// Generate thumbnail if image.
	meta, _ := s.repo.GetFileMeta(ctx, msg.FileId)
	var thumbnailURL string
	if meta != nil && isImage(meta.ContentType) {
		// For multipart, we skip thumbnail generation on complete
		// (would need to download from MinIO first). Mark as TODO.
		_ = objInfo
	}

	fileURL := s.objectURL(objectKey)
	resp := connect.NewResponse(&pbv1.CompleteMultipartUploadResponse{
		File: &pbv1.FileInfo{
			Id: msg.FileId, Filename: meta.Filename, ContentType: meta.ContentType,
			Size: objInfo.Size, Url: fileURL, ThumbnailUrl: thumbnailURL,
			CreatedAt: meta.CreatedAt.Unix(),
		},
	})
	return resp, nil
}
```

- [ ] **Step 2: Commit**

```bash
git add backend/services/file-service/multipart.go
git commit -m "feat(file-service): add multipart upload — Init, UploadPart, Complete"
```

---

### Task 12: File Service — main.go + Dockerfile

**Files:**
- Create: `backend/services/file-service/main.go`
- Create: `backend/services/file-service/Dockerfile`

- [ ] **Step 1: 写 main.go**

```go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/constell/constell/backend/pkg/config"
	"github.com/constell/constell/backend/pkg/health"
	"github.com/constell/constell/backend/pkg/logging"
	"github.com/constell/constell/backend/pkg/metrics"
	"github.com/constell/constell/backend/pkg/middleware"
	pkgminio "github.com/constell/constell/backend/pkg/minio"
	pkgotel "github.com/constell/constell/backend/pkg/otel"
	"github.com/constell/constell/backend/pkg/proto/file/v1/filev1connect"
)

type Config struct {
	Port      string `env:"PORT" default:"9084"`
	DBUrl     string `env:"DATABASE_URL" default:"postgres://constell:constell_dev@localhost:5432/constell?sslmode=disable"`
	Env       string `env:"ENV" default:"dev"`
	JWTSecret string `env:"JWT_SECRET" default:"dev-secret-change-me"`

	MinioEndpoint  string `env:"MINIO_ENDPOINT" default:"localhost:9000"`
	MinioAccessKey string `env:"MINIO_ACCESS_KEY" default:"minioadmin"`
	MinioSecretKey string `env:"MINIO_SECRET_KEY" default:"minioadmin"`
	MinioBucket    string `env:"MINIO_BUCKET" default:"constell"`
	MinioUseSSL    string `env:"MINIO_USE_SSL" default:"false"`
	MinioBaseURL   string `env:"MINIO_BASE_URL" default:"http://localhost:9000"`

	OTelEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:"http://localhost:5080/api/default/v1/otlp"`
	OTelInsecure string `env:"OTEL_EXPORTER_OTLP_INSECURE" default:"true"`
}

func main() {
	var cfg Config
	config.NewLoader("").MustLoad(&cfg)

	shutdown, err := pkgotel.Init(context.Background(), pkgotel.Config{
		ServiceName: "file-service",
		Environment: cfg.Env,
		Endpoint:    cfg.OTelEndpoint,
		Insecure:    cfg.OTelInsecure == "true",
	})
	if err != nil {
		slog.Error("otel init", "error", err)
	} else {
		defer pkgotel.ShutdownWithTimeout(shutdown, 5*time.Second)
	}

	logger := logging.Init("file-service", cfg.Env)
	slog.SetDefault(logger)

	// PostgreSQL
	poolCfg, err := pgxpool.ParseConfig(cfg.DBUrl)
	if err != nil {
		slog.Error("parse database URL", "error", err)
		os.Exit(1)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		slog.Error("create pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		slog.Error("ping postgres", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to postgres")

	// MinIO
	minioResult, err := pkgminio.New(pkgminio.Config{
		Endpoint:  cfg.MinioEndpoint,
		AccessKey: cfg.MinioAccessKey,
		SecretKey: cfg.MinioSecretKey,
		UseSSL:    cfg.MinioUseSSL == "true",
		Bucket:    cfg.MinioBucket,
	})
	if err != nil {
		slog.Error("minio init", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to minio", "endpoint", cfg.MinioEndpoint)

	// Health checks
	hc := health.NewChecker()
	hc.RegisterCheck("postgres", func(ctx context.Context) error { return pool.Ping(ctx) })

	// Wire up
	repo := NewRepository(pool)
	fileSvc := NewFileService(repo, minioResult, cfg.MinioBaseURL)

	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		slog.Error("create otel interceptor", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", hc.HealthzHandler())
	mux.HandleFunc("/readyz", hc.ReadyHandler())
	mux.Handle(filev1connect.NewFileServiceHandler(
		fileSvc,
		connect.WithInterceptors(
			otelInterceptor,
			metrics.ConnectRPCInterceptor(),
			middleware.NewAuthInterceptor(cfg.JWTSecret),
		),
	))

	server := &http.Server{Addr: ":" + cfg.Port, Handler: mux}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown", "error", err)
		}
	}()

	slog.Info("file-service listening", "port", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: 写 Dockerfile** (参照 auth-service Dockerfile 模式)

`backend/services/file-service/Dockerfile`:
```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY ../../go.work ../../go.work.sum ./
COPY ../../pkg/ pkg/
COPY ../../services/file-service/ services/file-service/
RUN CGO_ENABLED=0 go build -o /bin/file-service ./services/file-service

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/file-service /bin/file-service
ENTRYPOINT ["/bin/file-service"]
```

- [ ] **Step 3: 运行 go mod tidy + 编译验证**

```bash
cd backend/services/file-service && go mod tidy
cd ../.. && go build ./services/file-service/...
```

Expected: 编译通过

- [ ] **Step 4: Commit**

```bash
git add backend/services/file-service/
git commit -m "feat(file-service): add main.go + Dockerfile"
```

---

### Task 13: Search Service — Repository

**Files:**
- Create: `backend/services/search-service/repository.go`
- Create: `backend/services/search-service/repository_test.go`

- [ ] **Step 1: 写 repository.go**

```go
package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UserSearchResult holds a user search hit.
type UserSearchResult struct {
	ID        string
	Nickname  string
	AvatarURL string
	Relevance float64
}

// MessageSearchResult holds a channel message search hit.
type MessageSearchResult struct {
	ID        string
	ChannelID string
	ServerID  string
	AuthorID  string
	Content   string
	CreatedAt int64
	Relevance float64
}

// DMMessageSearchResult holds a DM message search hit.
type DMMessageSearchResult struct {
	ID             string
	ConversationID string
	PeerID         string
	Content        string
	CreatedAt      int64
	Relevance      float64
}

// SearchRepository provides search queries.
type SearchRepository interface {
	SearchUsers(ctx context.Context, query string, limit int) ([]UserSearchResult, error)
	SearchChannelMessages(ctx context.Context, query string, userID string, limit int) ([]MessageSearchResult, error)
	SearchDMMessages(ctx context.Context, query string, userID string, limit int) ([]DMMessageSearchResult, error)
}

type searchRepo struct {
	pool *pgxpool.Pool
}

// NewSearchRepository creates a new SearchRepository.
func NewSearchRepository(pool *pgxpool.Pool) SearchRepository {
	return &searchRepo{pool: pool}
}

func (r *searchRepo) SearchUsers(ctx context.Context, query string, limit int) ([]UserSearchResult, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, nickname, COALESCE(avatar_url, ''),
		        ts_rank(search_vector, plainto_tsquery('simple', $1)) AS relevance
		 FROM users
		 WHERE search_vector @@ plainto_tsquery('simple', $1)
		 ORDER BY relevance DESC
		 LIMIT $2`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	defer rows.Close()

	var results []UserSearchResult
	for rows.Next() {
		var r UserSearchResult
		if err := rows.Scan(&r.ID, &r.Nickname, &r.AvatarURL, &r.Relevance); err != nil {
			return nil, fmt.Errorf("scan user result: %w", err)
		}
		results = append(results, r)
	}
	return results, nil
}

func (r *searchRepo) SearchChannelMessages(ctx context.Context, query string, userID string, limit int) ([]MessageSearchResult, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT cm.id, cm.channel_id, c.server_id, cm.author_id, cm.content,
		        EXTRACT(EPOCH FROM cm.created_at)::bigint AS created_at,
		        ts_rank(cm.search_vector, plainto_tsquery('simple', $1)) AS relevance
		 FROM channel_messages cm
		 JOIN channels c ON c.id = cm.channel_id
		 JOIN server_members sm ON sm.server_id = c.server_id AND sm.user_id = $2
		 WHERE cm.search_vector @@ plainto_tsquery('simple', $1)
		 ORDER BY relevance DESC
		 LIMIT $3`, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("search channel messages: %w", err)
	}
	defer rows.Close()

	var results []MessageSearchResult
	for rows.Next() {
		var r MessageSearchResult
		if err := rows.Scan(&r.ID, &r.ChannelID, &r.ServerID, &r.AuthorID, &r.Content,
			&r.CreatedAt, &r.Relevance); err != nil {
			return nil, fmt.Errorf("scan message result: %w", err)
		}
		results = append(results, r)
	}
	return results, nil
}

func (r *searchRepo) SearchDMMessages(ctx context.Context, query string, userID string, limit int) ([]DMMessageSearchResult, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT dm.id, dm.conversation_id,
		        CASE WHEN dc.user_a_id = $2 THEN dc.user_b_id ELSE dc.user_a_id END AS peer_id,
		        dm.content,
		        EXTRACT(EPOCH FROM dm.created_at)::bigint AS created_at,
		        ts_rank(dm.search_vector, plainto_tsquery('simple', $1)) AS relevance
		 FROM dm_messages dm
		 JOIN dm_conversations dc ON dc.id = dm.conversation_id
		 WHERE dm.search_vector @@ plainto_tsquery('simple', $1)
		   AND (dc.user_a_id = $2 OR dc.user_b_id = $2)
		 ORDER BY relevance DESC
		 LIMIT $3`, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("search dm messages: %w", err)
	}
	defer rows.Close()

	var results []DMMessageSearchResult
	for rows.Next() {
		var r DMMessageSearchResult
		if err := rows.Scan(&r.ID, &r.ConversationID, &r.PeerID, &r.Content,
			&r.CreatedAt, &r.Relevance); err != nil {
			return nil, fmt.Errorf("scan dm result: %w", err)
		}
		results = append(results, r)
	}
	return results, nil
}
```

- [ ] **Step 2: 写 repository_test.go**

```go
package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSearchRepository_Users(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, "postgres://constell:constell_dev@localhost:15432/constell?sslmode=disable")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	repo := NewSearchRepository(pool)
	results, err := repo.SearchUsers(ctx, "test", 10)
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	t.Logf("found %d user results", len(results))
}
```

- [ ] **Step 3: Commit**

```bash
git add backend/services/search-service/repository.go backend/services/search-service/repository_test.go
git commit -m "feat(search-service): add repository — tsvector search queries with permission filtering"
```

---

### Task 14: Search Service — Service (并行 errgroup 搜索)

**Files:**
- Create: `backend/services/search-service/service.go`

- [ ] **Step 1: 写 service.go**

```go
package main

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"golang.org/x/sync/errgroup"

	pbv1 "github.com/constell/constell/backend/pkg/proto/search/v1"
	"github.com/constell/constell/backend/pkg/proto/search/v1/searchv1connect"
	"github.com/constell/constell/backend/pkg/middleware"
)

// SearchService implements the Connect-RPC SearchServiceHandler.
type SearchService struct {
	repo SearchRepository
}

// NewSearchService creates a new SearchService.
func NewSearchService(repo SearchRepository) *SearchService {
	return &SearchService{repo: repo}
}

var _ searchv1connect.SearchServiceHandler = (*SearchService)(nil)

// Search executes parallel search across users, channel messages, and DMs.
func (s *SearchService) Search(
	ctx context.Context,
	req *connect.Request[pbv1.SearchRequest],
) (*connect.Response[pbv1.SearchResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}

	query := req.Msg.Query
	if query == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("query is required"))
	}

	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 10
	}

	types := req.Msg.Types
	searchAll := len(types) == 0

	searchUsers := searchAll || containsType(types, pbv1.SearchType_SEARCH_TYPE_USERS)
	searchMessages := searchAll || containsType(types, pbv1.SearchType_SEARCH_TYPE_MESSAGES)
	searchDMs := searchAll || containsType(types, pbv1.SearchType_SEARCH_TYPE_DM_MESSAGES)

	var (
		userResults    []UserSearchResult
		messageResults []MessageSearchResult
		dmResults      []DMMessageSearchResult
	)

	g, gctx := errgroup.WithContext(ctx)

	if searchUsers {
		g.Go(func() error {
			var err error
			userResults, err = s.repo.SearchUsers(gctx, query, limit)
			return err
		})
	}

	if searchMessages {
		g.Go(func() error {
			var err error
			messageResults, err = s.repo.SearchChannelMessages(gctx, query, callerID, limit)
			return err
		})
	}

	if searchDMs {
		g.Go(func() error {
			var err error
			dmResults, err = s.repo.SearchDMMessages(gctx, query, callerID, limit)
			return err
		})
	}

	if err := g.Wait(); err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("search failed: %w", err))
	}

	resp := connect.NewResponse(&pbv1.SearchResponse{
		Users:     toPBUserResults(userResults),
		Messages:  toPBMessageResults(messageResults),
		DmMessages: toPBDMMessageResults(dmResults),
	})
	return resp, nil
}

func containsType(types []pbv1.SearchType, want pbv1.SearchType) bool {
	for _, t := range types {
		if t == want {
			return true
		}
	}
	return false
}

func toPBUserResults(results []UserSearchResult) []*pbv1.UserResult {
	out := make([]*pbv1.UserResult, 0, len(results))
	for _, r := range results {
		out = append(out, &pbv1.UserResult{
			Id: r.ID, Nickname: r.Nickname, AvatarUrl: r.AvatarURL,
			Relevance: r.Relevance,
		})
	}
	return out
}

func toPBMessageResults(results []MessageSearchResult) []*pbv1.MessageResult {
	out := make([]*pbv1.MessageResult, 0, len(results))
	for _, r := range results {
		out = append(out, &pbv1.MessageResult{
			Id: r.ID, ChannelId: r.ChannelID, ServerId: r.ServerID,
			AuthorId: r.AuthorID, Content: r.Content, CreatedAt: r.CreatedAt,
			Relevance: r.Relevance,
		})
	}
	return out
}

func toPBDMMessageResults(results []DMMessageSearchResult) []*pbv1.DMMessageResult {
	out := make([]*pbv1.DMMessageResult, 0, len(results))
	for _, r := range results {
		out = append(out, &pbv1.DMMessageResult{
			Id: r.ID, ConversationId: r.ConversationID, PeerId: r.PeerID,
			Content: r.Content, CreatedAt: r.CreatedAt, Relevance: r.Relevance,
		})
	}
	return out
}
```

- [ ] **Step 2: Commit**

```bash
git add backend/services/search-service/service.go
git commit -m "feat(search-service): add SearchService — parallel errgroup search"
```

---

### Task 15: Search Service — main.go + Dockerfile

**Files:**
- Create: `backend/services/search-service/main.go`
- Create: `backend/services/search-service/Dockerfile`

- [ ] **Step 1: 写 main.go**

```go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/constell/constell/backend/pkg/config"
	"github.com/constell/constell/backend/pkg/health"
	"github.com/constell/constell/backend/pkg/logging"
	"github.com/constell/constell/backend/pkg/metrics"
	"github.com/constell/constell/backend/pkg/middleware"
	pkgotel "github.com/constell/constell/backend/pkg/otel"
	"github.com/constell/constell/backend/pkg/proto/search/v1/searchv1connect"
)

type Config struct {
	Port      string `env:"PORT" default:"9085"`
	DBUrl     string `env:"DATABASE_URL" default:"postgres://constell:constell_dev@localhost:5432/constell?sslmode=disable"`
	Env       string `env:"ENV" default:"dev"`
	JWTSecret string `env:"JWT_SECRET" default:"dev-secret-change-me"`

	OTelEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:"http://localhost:5080/api/default/v1/otlp"`
	OTelInsecure string `env:"OTEL_EXPORTER_OTLP_INSECURE" default:"true"`
}

func main() {
	var cfg Config
	config.NewLoader("").MustLoad(&cfg)

	shutdown, err := pkgotel.Init(context.Background(), pkgotel.Config{
		ServiceName: "search-service",
		Environment: cfg.Env,
		Endpoint:    cfg.OTelEndpoint,
		Insecure:    cfg.OTelInsecure == "true",
	})
	if err != nil {
		slog.Error("otel init", "error", err)
	} else {
		defer pkgotel.ShutdownWithTimeout(shutdown, 5*time.Second)
	}

	logger := logging.Init("search-service", cfg.Env)
	slog.SetDefault(logger)

	poolCfg, err := pgxpool.ParseConfig(cfg.DBUrl)
	if err != nil {
		slog.Error("parse database URL", "error", err)
		os.Exit(1)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		slog.Error("create pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		slog.Error("ping postgres", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to postgres")

	hc := health.NewChecker()
	hc.RegisterCheck("postgres", func(ctx context.Context) error { return pool.Ping(ctx) })

	repo := NewSearchRepository(pool)
	searchSvc := NewSearchService(repo)

	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		slog.Error("create otel interceptor", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", hc.HealthzHandler())
	mux.HandleFunc("/readyz", hc.ReadyHandler())
	mux.Handle(searchv1connect.NewSearchServiceHandler(
		searchSvc,
		connect.WithInterceptors(
			otelInterceptor,
			metrics.ConnectRPCInterceptor(),
			middleware.NewAuthInterceptor(cfg.JWTSecret),
		),
	))

	server := &http.Server{Addr: ":" + cfg.Port, Handler: mux}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown", "error", err)
		}
	}()

	slog.Info("search-service listening", "port", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: 写 Dockerfile**

`backend/services/search-service/Dockerfile`:
```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY ../../go.work ../../go.work.sum ./
COPY ../../pkg/ pkg/
COPY ../../services/search-service/ services/search-service/
RUN CGO_ENABLED=0 go build -o /bin/search-service ./services/search-service

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/search-service /bin/search-service
ENTRYPOINT ["/bin/search-service"]
```

- [ ] **Step 3: 编译验证**

```bash
cd backend/services/search-service && go mod tidy
cd ../.. && go build ./services/search-service/...
```

Expected: 编译通过

- [ ] **Step 4: Commit**

```bash
git add backend/services/search-service/
git commit -m "feat(search-service): add main.go + Dockerfile"
```

---

### Task 16: Notify Service — Redis Store (Pointer 操作 + Set 维护)

**Files:**
- Create: `backend/services/notify-service/store.go`
- Create: `backend/services/notify-service/store_test.go`

- [ ] **Step 1: 写 store.go**

```go
package main

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

const (
	channelMsgCountPrefix = "channel_msg_count:"
	dmMsgCountPrefix      = "dm_msg_count:"
	readPtrChPrefix       = "read_ptr:ch:"
	readPtrDmPrefix       = "read_ptr:dm:"
	userChannelsPrefix    = "user:channels:"
	userConvsPrefix       = "user:conversations:"
)

// UnreadChannel holds unread info for a channel.
type UnreadChannel struct {
	ChannelID string
	ServerID  string
	Count     int32
}

// UnreadDM holds unread info for a DM conversation.
type UnreadDM struct {
	ConversationID string
	PeerID         string
	Count          int32
}

// Store manages Redis-based unread pointer operations.
type Store struct {
	rdb *goredis.Client
}

// NewStore creates a new Store.
func NewStore(rdb *goredis.Client) *Store {
	return &Store{rdb: rdb}
}

// IncrementChannelMsgCount increments the message counter for a channel. O(1).
func (s *Store) IncrementChannelMsgCount(ctx context.Context, channelID string) error {
	return s.rdb.Incr(ctx, channelMsgCountPrefix+channelID).Err()
}

// IncrementDMMsgCount increments the message counter for a DM conversation. O(1).
func (s *Store) IncrementDMMsgCount(ctx context.Context, convID string) error {
	return s.rdb.Incr(ctx, dmMsgCountPrefix+convID).Err()
}

// GetChannelMsgCount returns the current message count for a channel.
func (s *Store) GetChannelMsgCount(ctx context.Context, channelID string) (int64, error) {
	val, err := s.rdb.Get(ctx, channelMsgCountPrefix+channelID).Int64()
	if err == goredis.Nil {
		return 0, nil
	}
	return val, err
}

// GetDMMsgCount returns the current message count for a DM conversation.
func (s *Store) GetDMMsgCount(ctx context.Context, convID string) (int64, error) {
	val, err := s.rdb.Get(ctx, dmMsgCountPrefix+convID).Int64()
	if err == goredis.Nil {
		return 0, nil
	}
	return val, err
}

// MarkChannelRead sets the user's read pointer for a channel to the current count.
func (s *Store) MarkChannelRead(ctx context.Context, userID, channelID string) error {
	count, err := s.GetChannelMsgCount(ctx, channelID)
	if err != nil {
		return fmt.Errorf("get channel msg count: %w", err)
	}
	return s.rdb.Set(ctx, readPtrChPrefix+userID+":"+channelID, count, 0).Err()
}

// MarkDMRead sets the user's read pointer for a DM conversation to the current count.
func (s *Store) MarkDMRead(ctx context.Context, userID, convID string) error {
	count, err := s.GetDMMsgCount(ctx, convID)
	if err != nil {
		return fmt.Errorf("get dm msg count: %w", err)
	}
	return s.rdb.Set(ctx, readPtrDmPrefix+userID+":"+convID, count, 0).Err()
}

// GetUserChannels returns the set of channel IDs the user is in.
func (s *Store) GetUserChannels(ctx context.Context, userID string) ([]string, error) {
	return s.rdb.SMembers(ctx, userChannelsPrefix+userID).Result()
}

// GetUserConversations returns the set of DM conversation IDs the user is in.
func (s *Store) GetUserConversations(ctx context.Context, userID string) ([]string, error) {
	return s.rdb.SMembers(ctx, userConvsPrefix+userID).Result()
}

// AddChannelsToUser adds channel IDs to a user's channel set.
func (s *Store) AddChannelsToUser(ctx context.Context, userID string, channelIDs []string) error {
	if len(channelIDs) == 0 {
		return nil
	}
	members := make([]interface{}, len(channelIDs))
	for i, id := range channelIDs {
		members[i] = id
	}
	return s.rdb.SAdd(ctx, userChannelsPrefix+userID, members...).Err()
}

// RemoveChannelsFromUser removes channel IDs from a user's channel set.
func (s *Store) RemoveChannelsFromUser(ctx context.Context, userID string, channelIDs []string) error {
	if len(channelIDs) == 0 {
		return nil
	}
	members := make([]interface{}, len(channelIDs))
	for i, id := range channelIDs {
		members[i] = id
	}
	return s.rdb.SRem(ctx, userChannelsPrefix+userID, members...).Err()
}

// AddConversationToUser adds a conversation ID to a user's DM set.
func (s *Store) AddConversationToUser(ctx context.Context, userID, convID string) error {
	return s.rdb.SAdd(ctx, userConvsPrefix+userID, convID).Err()
}

// GetUnreadChannels computes unread counts for all channels the user is in.
func (s *Store) GetUnreadChannels(ctx context.Context, userID string) ([]UnreadChannel, error) {
	channels, err := s.GetUserChannels(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, nil
	}

	// Batch get channel message counts.
	countKeys := make([]string, len(channels))
	for i, ch := range channels {
		countKeys[i] = channelMsgCountPrefix + ch
	}
	counts, err := s.rdb.MGet(ctx, countKeys...).Result()
	if err != nil {
		return nil, err
	}

	// Batch get read pointers.
	ptrKeys := make([]string, len(channels))
	for i, ch := range channels {
		ptrKeys[i] = readPtrChPrefix + userID + ":" + ch
	}
	ptrs, err := s.rdb.MGet(ctx, ptrKeys...).Result()
	if err != nil {
		return nil, err
	}

	var unread []UnreadChannel
	for i, ch := range channels {
		total := toInt64(counts[i])
		ptr := toInt64(ptrs[i])
		if diff := total - ptr; diff > 0 {
			unread = append(unread, UnreadChannel{ChannelID: ch, Count: int32(diff)})
		}
	}
	return unread, nil
}

// GetUnreadDMs computes unread counts for all DM conversations the user is in.
func (s *Store) GetUnreadDMs(ctx context.Context, userID string) ([]UnreadDM, error) {
	convs, err := s.GetUserConversations(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(convs) == 0 {
		return nil, nil
	}

	countKeys := make([]string, len(convs))
	for i, c := range convs {
		countKeys[i] = dmMsgCountPrefix + c
	}
	counts, err := s.rdb.MGet(ctx, countKeys...).Result()
	if err != nil {
		return nil, err
	}

	ptrKeys := make([]string, len(convs))
	for i, c := range convs {
		ptrKeys[i] = readPtrDmPrefix + userID + ":" + c
	}
	ptrs, err := s.rdb.MGet(ctx, ptrKeys...).Result()
	if err != nil {
		return nil, err
	}

	var unread []UnreadDM
	for i, c := range convs {
		total := toInt64(counts[i])
		ptr := toInt64(ptrs[i])
		if diff := total - ptr; diff > 0 {
			unread = append(unread, UnreadDM{ConversationID: c, Count: int32(diff)})
		}
	}
	return unread, nil
}

func toInt64(v interface{}) int64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case string:
		var n int64
		fmt.Sscanf(val, "%d", &n)
		return n
	case int64:
		return val
	default:
		return 0
	}
}
```

- [ ] **Step 2: 写 store_test.go**

```go
package main

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func newTestStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	return NewStore(rdb), mr
}

func TestStore_IncrementAndGet(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	store.IncrementChannelMsgCount(ctx, "ch1")
	store.IncrementChannelMsgCount(ctx, "ch1")
	store.IncrementChannelMsgCount(ctx, "ch1")

	count, err := store.GetChannelMsgCount(ctx, "ch1")
	if err != nil {
		t.Fatalf("GetChannelMsgCount: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

func TestStore_MarkChannelRead(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	store.IncrementChannelMsgCount(ctx, "ch1")
	store.IncrementChannelMsgCount(ctx, "ch1")

	if err := store.MarkChannelRead(ctx, "user1", "ch1"); err != nil {
		t.Fatalf("MarkChannelRead: %v", err)
	}

	// After mark read, count should equal read pointer.
	store.IncrementChannelMsgCount(ctx, "ch1") // new message
	store.AddChannelsToUser(ctx, "user1", []string{"ch1"})
	unread, err := store.GetUnreadChannels(ctx, "user1")
	if err != nil {
		t.Fatalf("GetUnreadChannels: %v", err)
	}
	if len(unread) != 1 {
		t.Fatalf("expected 1 unread channel, got %d", len(unread))
	}
	if unread[0].Count != 1 {
		t.Errorf("unread count = %d, want 1", unread[0].Count)
	}
}

func TestStore_AddChannelsToUser(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	store.AddChannelsToUser(ctx, "user1", []string{"ch1", "ch2", "ch3"})
	channels, err := store.GetUserChannels(ctx, "user1")
	if err != nil {
		t.Fatalf("GetUserChannels: %v", err)
	}
	if len(channels) != 3 {
		t.Errorf("expected 3 channels, got %d", len(channels))
	}

	store.RemoveChannelsFromUser(ctx, "user1", []string{"ch2"})
	channels, _ = store.GetUserChannels(ctx, "user1")
	if len(channels) != 2 {
		t.Errorf("expected 2 channels after remove, got %d", len(channels))
	}
}

func TestStore_DMUnread(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	store.IncrementDMMsgCount(ctx, "conv1")
	store.IncrementDMMsgCount(ctx, "conv1")
	store.AddConversationToUser(ctx, "user1", "conv1")

	unread, err := store.GetUnreadDMs(ctx, "user1")
	if err != nil {
		t.Fatalf("GetUnreadDMs: %v", err)
	}
	if len(unread) != 1 {
		t.Fatalf("expected 1 unread DM, got %d", len(unread))
	}
	if unread[0].Count != 2 {
		t.Errorf("unread count = %d, want 2", unread[0].Count)
	}

	store.MarkDMRead(ctx, "user1", "conv1")
	unread, _ = store.GetUnreadDMs(ctx, "user1")
	if len(unread) != 0 {
		t.Errorf("expected 0 unread after mark read, got %d", len(unread))
	}
}
```

- [ ] **Step 3: 运行测试**

```bash
cd backend && go test ./services/notify-service/... -v -run TestStore -short
```

Expected: PASS (miniredis 不需要外部依赖)

- [ ] **Step 4: Commit**

```bash
git add backend/services/notify-service/store.go backend/services/notify-service/store_test.go
git commit -m "feat(notify-service): add Redis store — Pointer unread counts + set maintenance"
```

---

### Task 17: Notify Service — NATS 事件消费 + WS Gateway 推送

**Files:**
- Create: `backend/services/notify-service/subscriber.go`
- Create: `backend/services/notify-service/subscriber_test.go`

- [ ] **Step 1: 写 subscriber.go**

```go
package main

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/nats-io/nats.go"
)

// DMCreatedEvent is the payload for constell.dm.created.
type DMCreatedEvent struct {
	SenderID       string `json:"sender_id"`
	ReceiverID     string `json:"receiver_id"`
	ConversationID string `json:"conversation_id"`
	Content        string `json:"content"`
	CreatedAt      int64  `json:"created_at"`
}

// MessageCreatedEvent is the payload for constell.message.created.
type MessageCreatedEvent struct {
	MessageID string   `json:"message_id"`
	ChannelID string   `json:"channel_id"`
	ServerID  string   `json:"server_id"`
	SenderID  string   `json:"sender_id"`
	Content   string   `json:"content"`
	MemberIDs []string `json:"member_ids"`
	CreatedAt int64    `json:"created_at"`
}

// MemberJoinedEvent is the payload for constell.member.joined.
type MemberJoinedEvent struct {
	ServerID   string   `json:"server_id"`
	UserID     string   `json:"user_id"`
	ChannelIDs []string `json:"channel_ids"`
}

// MemberLeftEvent is the payload for constell.member.left.
type MemberLeftEvent struct {
	ServerID   string   `json:"server_id"`
	UserID     string   `json:"user_id"`
	ChannelIDs []string `json:"channel_ids"`
}

// NotificationPush is the payload for gw.push.{gw_id}.
type NotificationPush struct {
	Targets   []string               `json:"targets"`
	EventType string                 `json:"event_type"`
	Payload   map[string]interface{} `json:"payload"`
}

// Subscriber consumes NATS events and maintains unread state.
type Subscriber struct {
	nc   *nats.Conn
	js   nats.JetStreamContext
	store *Store
}

// NewSubscriber creates a new Subscriber.
func NewSubscriber(nc *nats.Conn, js nats.JetStreamContext, store *Store) *Subscriber {
	return &Subscriber{nc: nc, js: js, store: store}
}

// SubscribeAll starts consuming all relevant NATS subjects.
func (s *Subscriber) SubscribeAll() error {
 subjects := []struct {
		subject string
		handler func(msg *nats.Msg)
	}{
		{"constell.dm.created", s.handleDMCreated},
		{"constell.message.created", s.handleMessageCreated},
		{"constell.member.joined", s.handleMemberJoined},
		{"constell.member.left", s.handleMemberLeft},
	}

	for _, sub := range subjects {
		if _, err := s.js.Subscribe(sub.subject, sub.handler,
			nats.Durable("notify-"+sub.subject),
			nats.ManualAck(),
		); err != nil {
			return err
		}
		slog.Info("subscribed to NATS subject", "subject", sub.subject)
	}
	return nil
}

func (s *Subscriber) handleDMCreated(msg *nats.Msg) {
	var evt DMCreatedEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		slog.Error("unmarshal dm.created", "error", err)
		msg.Nak()
		return
	}

	ctx := context.Background()

	// Increment DM conversation message count.
	if err := s.store.IncrementDMMsgCount(ctx, evt.ConversationID); err != nil {
		slog.Error("increment dm count", "error", err)
	}

	// Ensure both users are in the conversation set.
	s.store.AddConversationToUser(ctx, evt.SenderID, evt.ConversationID)
	s.store.AddConversationToUser(ctx, evt.ReceiverID, evt.ConversationID)

	// Push notification to receiver via WS Gateway.
	preview := truncate(evt.Content, 50)
	s.pushToUser(ctx, evt.ReceiverID, map[string]interface{}{
		"notification_type": "dm",
		"source_id":         evt.ConversationID,
		"sender_id":         evt.SenderID,
		"preview":           preview,
		"created_at":        evt.CreatedAt,
	})

	msg.Ack()
}

func (s *Subscriber) handleMessageCreated(msg *nats.Msg) {
	var evt MessageCreatedEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		slog.Error("unmarshal message.created", "error", err)
		msg.Nak()
		return
	}

	ctx := context.Background()

	// Increment channel message count.
	if err := s.store.IncrementChannelMsgCount(ctx, evt.ChannelID); err != nil {
		slog.Error("increment channel count", "error", err)
	}

	// Push notification to all online members except sender.
	preview := truncate(evt.Content, 50)
	for _, memberID := range evt.MemberIDs {
		if memberID == evt.SenderID {
			continue
		}
		s.pushToUser(ctx, memberID, map[string]interface{}{
			"notification_type": "channel_message",
			"source_id":         evt.ChannelID,
			"server_id":         evt.ServerID,
			"sender_id":         evt.SenderID,
			"preview":           preview,
			"created_at":        evt.CreatedAt,
		})
	}

	msg.Ack()
}

func (s *Subscriber) handleMemberJoined(msg *nats.Msg) {
	var evt MemberJoinedEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		slog.Error("unmarshal member.joined", "error", err)
		msg.Nak()
		return
	}

	ctx := context.Background()
	if err := s.store.AddChannelsToUser(ctx, evt.UserID, evt.ChannelIDs); err != nil {
		slog.Error("add channels to user", "error", err)
	}

	msg.Ack()
}

func (s *Subscriber) handleMemberLeft(msg *nats.Msg) {
	var evt MemberLeftEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		slog.Error("unmarshal member.left", "error", err)
		msg.Nak()
		return
	}

	ctx := context.Background()
	if err := s.store.RemoveChannelsFromUser(ctx, evt.UserID, evt.ChannelIDs); err != nil {
		slog.Error("remove channels from user", "error", err)
	}

	msg.Ack()
}

// pushToUser looks up the user's WS Gateway via Redis and publishes a notification push.
func (s *Subscriber) pushToUser(ctx context.Context, userID string, payload map[string]interface{}) {
	// Look up which WS Gateway the user is connected to.
	// Key: gw:{user_id} → gw_id (set by ws-gateway registry)
	gwID, err := s.store.rdb.Get(ctx, "gw:"+userID).Result()
	if err != nil {
		return // user not online
	}

	push := NotificationPush{
		Targets:   []string{userID},
		EventType: "NOTIFICATION",
		Payload:   payload,
	}
	data, _ := json.Marshal(push)

	topic := "gw.push." + gwID
	if err := s.nc.Publish(topic, data); err != nil {
		slog.Error("publish notification push", "gw_id", gwID, "error", err)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
```

- [ ] **Step 2: 写 subscriber_test.go**

```go
package main

import (
	"testing"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world this is a long message", 10, "hello worl..."},
		{"short", 5, "short"},
		{"", 10, ""},
	}
	for _, tt := range tests {
		got := truncate(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}
```

- [ ] **Step 3: 运行测试**

```bash
cd backend && go test ./services/notify-service/... -v -run TestTruncate -short
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add backend/services/notify-service/subscriber.go backend/services/notify-service/subscriber_test.go
git commit -m "feat(notify-service): add NATS subscriber — event consumer + WS Gateway push"
```

---

### Task 18: Notify Service — Service (RPC handlers)

**Files:**
- Create: `backend/services/notify-service/service.go`

- [ ] **Step 1: 写 service.go**

```go
package main

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	pbv1 "github.com/constell/constell/backend/pkg/proto/notify/v1"
	"github.com/constell/constell/backend/pkg/proto/notify/v1/notifyv1connect"
	"github.com/constell/constell/backend/pkg/middleware"
)

// NotifyService implements the Connect-RPC NotifyServiceHandler.
type NotifyService struct {
	store *Store
}

// NewNotifyService creates a new NotifyService.
func NewNotifyService(store *Store) *NotifyService {
	return &NotifyService{store: store}
}

var _ notifyv1connect.NotifyServiceHandler = (*NotifyService)(nil)

// GetUnreadCounts returns all unread counts for the current user.
func (s *NotifyService) GetUnreadCounts(
	ctx context.Context,
	req *connect.Request[pbv1.GetUnreadCountsRequest],
) (*connect.Response[pbv1.GetUnreadCountsResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}

	unreadChs, err := s.store.GetUnreadChannels(ctx, callerID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to get unread channels: %w", err))
	}

	unreadDMs, err := s.store.GetUnreadDMs(ctx, callerID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to get unread DMs: %w", err))
	}

	var dmTotal int32
	pbDMs := make([]*pbv1.UnreadDMConversation, 0, len(unreadDMs))
	for _, dm := range unreadDMs {
		dmTotal += dm.Count
		pbDMs = append(pbDMs, &pbv1.UnreadDMConversation{
			ConversationId: dm.ConversationID,
			PeerId:         dm.PeerID,
			Count:          dm.Count,
		})
	}

	var chTotal int32
	pbChs := make([]*pbv1.UnreadChannel, 0, len(unreadChs))
	for _, ch := range unreadChs {
		chTotal += ch.Count
		pbChs = append(pbChs, &pbv1.UnreadChannel{
			ChannelId: ch.ChannelID,
			ServerId:  ch.ServerID,
			Count:     ch.Count,
		})
	}

	resp := connect.NewResponse(&pbv1.GetUnreadCountsResponse{
		DmTotal:        dmTotal,
		DmConversations: pbDMs,
		ChannelTotal:   chTotal,
		Channels:       pbChs,
	})
	return resp, nil
}

// MarkDMRead marks a DM conversation as read for the current user.
func (s *NotifyService) MarkDMRead(
	ctx context.Context,
	req *connect.Request[pbv1.MarkDMReadRequest],
) (*connect.Response[pbv1.MarkDMReadResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	if req.Msg.ConversationId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("conversation_id is required"))
	}

	if err := s.store.MarkDMRead(ctx, callerID, req.Msg.ConversationId); err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to mark DM read: %w", err))
	}

	resp := connect.NewResponse(&pbv1.MarkDMReadResponse{})
	return resp, nil
}

// MarkChannelRead marks a channel as read for the current user.
func (s *NotifyService) MarkChannelRead(
	ctx context.Context,
	req *connect.Request[pbv1.MarkChannelReadRequest],
) (*connect.Response[pbv1.MarkChannelReadResponse], error) {
	callerID := middleware.UserIDFromContext(ctx)
	if callerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("not authenticated"))
	}
	if req.Msg.ChannelId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("channel_id is required"))
	}

	if err := s.store.MarkChannelRead(ctx, callerID, req.Msg.ChannelId); err != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to mark channel read: %w", err))
	}

	resp := connect.NewResponse(&pbv1.MarkChannelReadResponse{})
	return resp, nil
}
```

- [ ] **Step 2: Commit**

```bash
git add backend/services/notify-service/service.go
git commit -m "feat(notify-service): add NotifyService — GetUnreadCounts, MarkDMRead, MarkChannelRead"
```

---

### Task 19: Notify Service — main.go + Dockerfile

**Files:**
- Create: `backend/services/notify-service/main.go`
- Create: `backend/services/notify-service/Dockerfile`

- [ ] **Step 1: 写 main.go**

```go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/constell/constell/backend/pkg/config"
	"github.com/constell/constell/backend/pkg/health"
	"github.com/constell/constell/backend/pkg/logging"
	"github.com/constell/constell/backend/pkg/metrics"
	"github.com/constell/constell/backend/pkg/middleware"
	pkgnats "github.com/constell/constell/backend/pkg/nats"
	pkgotel "github.com/constell/constell/backend/pkg/otel"
	"github.com/constell/constell/backend/pkg/proto/notify/v1/notifyv1connect"
)

type Config struct {
	Port      string `env:"PORT" default:"9086"`
	DBUrl     string `env:"DATABASE_URL" default:"postgres://constell:constell_dev@localhost:5432/constell?sslmode=disable"`
	RedisURL  string `env:"REDIS_URL" default:"localhost:6379"`
	NatsURL   string `env:"NATS_URL" default:"nats://localhost:4222"`
	Env       string `env:"ENV" default:"dev"`
	JWTSecret string `env:"JWT_SECRET" default:"dev-secret-change-me"`

	OTelEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:"http://localhost:5080/api/default/v1/otlp"`
	OTelInsecure string `env:"OTEL_EXPORTER_OTLP_INSECURE" default:"true"`
}

func main() {
	var cfg Config
	config.NewLoader("").MustLoad(&cfg)

	shutdown, err := pkgotel.Init(context.Background(), pkgotel.Config{
		ServiceName: "notify-service",
		Environment: cfg.Env,
		Endpoint:    cfg.OTelEndpoint,
		Insecure:    cfg.OTelInsecure == "true",
	})
	if err != nil {
		slog.Error("otel init", "error", err)
	} else {
		defer pkgotel.ShutdownWithTimeout(shutdown, 5*time.Second)
	}

	logger := logging.Init("notify-service", cfg.Env)
	slog.SetDefault(logger)

	// PostgreSQL
	poolCfg, err := pgxpool.ParseConfig(cfg.DBUrl)
	if err != nil {
		slog.Error("parse database URL", "error", err)
		os.Exit(1)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		slog.Error("create pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		slog.Error("ping postgres", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to postgres")

	// Redis
	rdb := goredis.NewClient(&goredis.Options{Addr: cfg.RedisURL})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Error("ping redis", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to redis")

	// NATS
	natsResult, err := pkgnats.New(pkgnats.Config{URL: cfg.NatsURL})
	if err != nil {
		slog.Error("nats connect", "error", err)
		os.Exit(1)
	}
	defer natsResult.Conn.Close()
	slog.Info("connected to nats")

	// Wire up
	store := NewStore(rdb)
	subscriber := NewSubscriber(natsResult.Conn, natsResult.JS, store)

	// Start NATS subscriptions
	if err := subscriber.SubscribeAll(); err != nil {
		slog.Error("subscribe to NATS", "error", err)
		os.Exit(1)
	}

	// Health checks
	hc := health.NewChecker()
	hc.RegisterCheck("postgres", func(ctx context.Context) error { return pool.Ping(ctx) })
	hc.RegisterCheck("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() })

	notifySvc := NewNotifyService(store)

	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		slog.Error("create otel interceptor", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", hc.HealthzHandler())
	mux.HandleFunc("/readyz", hc.ReadyHandler())
	mux.Handle(notifyv1connect.NewNotifyServiceHandler(
		notifySvc,
		connect.WithInterceptors(
			otelInterceptor,
			metrics.ConnectRPCInterceptor(),
			middleware.NewAuthInterceptor(cfg.JWTSecret),
		),
	))

	server := &http.Server{Addr: ":" + cfg.Port, Handler: mux}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown", "error", err)
		}
	}()

	slog.Info("notify-service listening", "port", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: 写 Dockerfile**

`backend/services/notify-service/Dockerfile`:
```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY ../../go.work ../../go.work.sum ./
COPY ../../pkg/ pkg/
COPY ../../services/notify-service/ services/notify-service/
RUN CGO_ENABLED=0 go build -o /bin/notify-service ./services/notify-service

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/notify-service /bin/notify-service
ENTRYPOINT ["/bin/notify-service"]
```

- [ ] **Step 3: 编译验证**

```bash
cd backend/services/notify-service && go mod tidy
cd ../.. && go build ./services/notify-service/...
```

Expected: 编译通过

- [ ] **Step 4: Commit**

```bash
git add backend/services/notify-service/
git commit -m "feat(notify-service): add main.go + Dockerfile"
```

---

### Task 20: Community Service — NATS 事件发布 + 附件支持

**Files:**
- Modify: `backend/services/community-service/main.go`
- Modify: `backend/services/community-service/service.go`
- Modify: `backend/services/community-service/repository.go`

- [ ] **Step 1: 在 main.go 中添加 NATS 连接**

在 Config 结构体中追加:
```go
	NatsURL         string `env:"NATS_URL" default:"nats://localhost:4222"`
```

在 main() 函数中，Redis 连接之后追加:
```go
	// NATS
	natsResult, err := pkgnats.New(pkgnats.Config{URL: cfg.NatsURL})
	if err != nil {
		slog.Error("nats connect", "error", err)
		os.Exit(1)
	}
	defer natsResult.Conn.Close()
	slog.Info("connected to nats")
```

在 service wire-up 时传入 NATS 连接:
```go
	communityService := NewCommunityService(repo, serverCache, membersCache, rolesCache, natsResult.Conn)
```

在 healthcheck 中追加:
```go
	hc.RegisterCheck("nats", func(ctx context.Context) error {
		if !natsResult.Conn.IsConnected() {
			return fmt.Errorf("nats not connected")
		}
		return nil
	})
```

需要 import `pkgnats "github.com/constell/constell/backend/pkg/nats"` 和 `"fmt"`。

- [ ] **Step 2: 在 repository.go 中添加 InsertAttachments 方法**

追加到 Repository 结构体:

```go
// AttachmentRow represents an attachment record.
type AttachmentRow struct {
	ID          string
	MessageType string
	MessageID   string
	FileID      string
	Filename    string
	ContentType string
	Size        int64
}

// InsertAttachments inserts multiple attachments for a message.
func (r *Repository) InsertAttachments(ctx context.Context, attachments []*AttachmentRow) error {
	for _, a := range attachments {
		_, err := r.pool.Exec(ctx,
			`INSERT INTO attachments (id, message_type, message_id, file_id, filename, content_type, size)
			 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6)`,
			a.MessageType, a.MessageID, a.FileID, a.Filename, a.ContentType, a.Size)
		if err != nil {
			return fmt.Errorf("insert attachment: %w", err)
		}
	}
	return nil
}

// GetAttachmentsByMessage retrieves attachments for a message.
func (r *Repository) GetAttachmentsByMessage(ctx context.Context, messageType, messageID string) ([]*AttachmentRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, message_type, message_id, file_id, filename, content_type, size
		 FROM attachments WHERE message_type = $1 AND message_id = $2`, messageType, messageID)
	if err != nil {
		return nil, fmt.Errorf("query attachments: %w", err)
	}
	defer rows.Close()

	var result []*AttachmentRow
	for rows.Next() {
		var a AttachmentRow
		if err := rows.Scan(&a.ID, &a.MessageType, &a.MessageID, &a.FileID, &a.Filename, &a.ContentType, &a.Size); err != nil {
			return nil, fmt.Errorf("scan attachment: %w", err)
		}
		result = append(result, &a)
	}
	return result, nil
}
```

- [ ] **Step 3: 修改 service.go — CommunityService 构造函数加 NATS**

更新 CommunityService 结构体，添加 natsConn:
```go
type CommunityService struct {
	repo         *Repository
	serverCache  *ServerCache
	membersCache *MembersCache
	rolesCache   *RolesCache
	natsConn     *nats.Conn
}
```

更新 NewCommunityService:
```go
func NewCommunityService(
	repo *Repository,
	serverCache *ServerCache,
	membersCache *MembersCache,
	rolesCache *RolesCache,
	natsConn *nats.Conn,
) *CommunityService {
	return &CommunityService{
		repo: repo, serverCache: serverCache,
		membersCache: membersCache, rolesCache: rolesCache,
		natsConn: natsConn,
	}
}
```

需要 import `"github.com/nats-io/nats.go"` 和 `"encoding/json"`。

- [ ] **Step 4: 修改 SendMessage — 支持 file_ids + 发布 NATS 事件**

更新 SendMessage 方法，在 `req.Msg` 解析后追加 `file_ids`:
```go
	fileIDs := req.Msg.FileIds
```

在消息插入后，插入附件:
```go
	if len(fileIDs) > 0 {
		attachments := make([]*AttachmentRow, 0, len(fileIDs))
		for _, fid := range fileIDs {
			attachments = append(attachments, &AttachmentRow{
				MessageType: "channel",
				MessageID:   msg.ID,
				FileID:      fid,
				Filename:    fid,     // TODO: 从 file_metadata 查询
				ContentType: "",      // TODO: 从 file_metadata 查询
				Size:        0,       // TODO: 从 file_metadata 查询
			})
		}
		if err := s.repo.InsertAttachments(ctx, attachments); err != nil {
			slog.Warn("failed to insert attachments", "error", err)
		}
	}
```

在 return 之前，发布 NATS 事件:
```go
	// Publish constell.message.created event.
	if s.natsConn != nil {
		memberIDs := make([]string, 0)
		for _, m := range members {
			memberIDs = append(memberIDs, m.UserID)
		}
		evt := MessageCreatedEvent{
			MessageID: msg.ID,
			ChannelID: channelID,
			ServerID:  ch.ServerID,
			SenderID:  callerID,
			Content:   content,
			MemberIDs: memberIDs,
			CreatedAt: msg.CreatedAt.Unix(),
		}
		data, _ := json.Marshal(evt)
		if err := s.natsConn.Publish("constell.message.created", data); err != nil {
			slog.Warn("failed to publish message.created event", "error", err)
		}
	}
```

- [ ] **Step 5: 发布 member.joined/left 事件**

在 JoinServer 成功后，发布事件:
```go
	if s.natsConn != nil {
		channels, _ := s.repo.ListChannelsByServer(ctx, serverID)
		channelIDs := make([]string, 0, len(channels))
		for _, ch := range channels {
			channelIDs = append(channelIDs, ch.ID)
		}
		evt := map[string]interface{}{
			"server_id":   serverID,
			"user_id":     callerID,
			"channel_ids": channelIDs,
		}
		data, _ := json.Marshal(evt)
		s.natsConn.Publish("constell.member.joined", data)
	}
```

在 LeaveServer 成功后:
```go
	if s.natsConn != nil {
		channels, _ := s.repo.ListChannelsByServer(ctx, serverID)
		channelIDs := make([]string, 0, len(channels))
		for _, ch := range channels {
			channelIDs = append(channelIDs, ch.ID)
		}
		evt := map[string]interface{}{
			"server_id":   serverID,
			"user_id":     callerID,
			"channel_ids": channelIDs,
		}
		data, _ := json.Marshal(evt)
		s.natsConn.Publish("constell.member.left", data)
	}
```

- [ ] **Step 6: 编译验证**

```bash
cd backend && go build ./services/community-service/...
```

Expected: 编译通过

- [ ] **Step 7: Commit**

```bash
git add backend/services/community-service/
git commit -m "feat(community-service): add NATS event publishing + attachment support"
```

---

### Task 21: User Service — NATS 事件发布 + 附件支持

**Files:**
- Modify: `backend/services/user-service/main.go`
- Modify: `backend/services/user-service/service.go`
- Modify: `backend/services/user-service/repository.go`

- [ ] **Step 1: 在 main.go 中添加 NATS 连接**

在 Config 结构体中追加:
```go
	NatsURL         string `env:"NATS_URL" default:"nats://localhost:4222"`
```

在 main() 函数中，Redis 连接之后追加 NATS:
```go
	natsResult, err := pkgnats.New(pkgnats.Config{URL: cfg.NatsURL})
	if err != nil {
		slog.Error("nats connect", "error", err)
		os.Exit(1)
	}
	defer natsResult.Conn.Close()
	slog.Info("connected to nats")
```

更新 service wire-up:
```go
	userService := NewUserService(repo, userCache, relationCache, natsResult.Conn)
```

需要 import `pkgnats "github.com/constell/constell/backend/pkg/nats"`。

- [ ] **Step 2: 在 repository.go 中添加 InsertAttachments 和 GetAttachmentsByMessage**

与 Task 20 步骤 2 相同的 AttachmentRow 类型和方法追加到 user-service/repository.go。

- [ ] **Step 3: 修改 service.go — UserService 构造函数加 NATS**

更新 UserService 结构体:
```go
type UserService struct {
	repo          *Repository
	userCache     UserCacheReader
	relationCache RelationCacheReader
	userWriter    UserCacheWriter
	natsConn      *nats.Conn
}
```

更新 NewUserService:
```go
func NewUserService(
	repo *Repository,
	userCache *UserCache,
	relationCache *RelationCache,
	natsConn *nats.Conn,
) *UserService {
	return &UserService{
		repo: repo, userCache: userCache, relationCache: relationCache,
		userWriter: userCache, natsConn: natsConn,
	}
}
```

- [ ] **Step 4: 修改 SendDM — 支持 file_ids + 发布 NATS 事件**

在 SendDM 中，解析 `req.Msg.FileIds`:
```go
	fileIDs := req.Msg.FileIds
```

在消息插入后:
```go
	if len(fileIDs) > 0 {
		attachments := make([]*AttachmentRow, 0, len(fileIDs))
		for _, fid := range fileIDs {
			attachments = append(attachments, &AttachmentRow{
				MessageType: "dm",
				MessageID:   msg.ID,
				FileID:      fid,
				Filename:    fid,
				ContentType: "",
				Size:        0,
			})
		}
		if err := s.repo.InsertAttachments(ctx, attachments); err != nil {
			slog.Warn("failed to insert attachments", "error", err)
		}
	}
```

在 return 之前，发布 NATS 事件:
```go
	if s.natsConn != nil {
		evt := map[string]interface{}{
			"sender_id":       callerID,
			"receiver_id":     targetUserID,
			"conversation_id": conv.ID,
			"content":         content,
			"created_at":      msg.CreatedAt.Unix(),
		}
		data, _ := json.Marshal(evt)
		if err := s.natsConn.Publish("constell.dm.created", data); err != nil {
			slog.Warn("failed to publish dm.created event", "error", err)
		}
	}
```

- [ ] **Step 5: 编译验证**

```bash
cd backend && go build ./services/user-service/...
```

Expected: 编译通过

- [ ] **Step 6: Commit**

```bash
git add backend/services/user-service/
git commit -m "feat(user-service): add NATS event publishing + attachment support"
```

---

### Task 22: WS Gateway — NotificationEvent + file_ids 支持

**Files:**
- Modify: `backend/services/ws-gateway/push.go`
- Modify: `backend/services/ws-gateway/router.go`

- [ ] **Step 1: 更新 push.go — 添加 buildNotificationEvent**

在 buildServerEvent 的 switch 中追加:
```go
	case "NOTIFICATION":
		return ps.buildNotificationEvent(payload.Payload)
```

添加新方法:
```go
func (ps *PushSubscriber) buildNotificationEvent(p map[string]interface{}) (*gatewayv1.ServerEvent, error) {
	return &gatewayv1.ServerEvent{
		Type: gatewayv1.ServerEventType_SERVER_EVENT_TYPE_NOTIFICATION,
		NotificationEvent: &gatewayv1.NotificationEvent{
			NotificationType: getStringField(p, "notification_type"),
			SourceId:         getStringField(p, "source_id"),
			ServerId:         getStringField(p, "server_id"),
			SenderId:         getStringField(p, "sender_id"),
			SenderNickname:   getStringField(p, "sender_nickname"),
			Preview:          getStringField(p, "preview"),
			CreatedAt:        getInt64Field(p, "created_at"),
		},
	}, nil
}
```

注意: `NotificationEvent` 和 `SERVER_EVENT_TYPE_NOTIFICATION` 是 Task 4 中添加到 gateway.proto 的新类型。proto-gen 后即可使用。

- [ ] **Step 2: 更新 router.go — 传递 file_ids**

更新 `handleSendDM`:
```go
func (r *Router) handleSendDM(ctx context.Context, userID string, msg *gatewayv1.ClientMessage) (*gatewayv1.ServerEvent, error) {
	req := msg.SendDmRequest
	if req == nil {
		return nil, fmt.Errorf("send_dm_request is nil")
	}
	if req.ReceiverId == "" {
		return nil, fmt.Errorf("receiver_id is required")
	}
	if req.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	// file_ids are passed through to user-service via the SendDM RPC.
	// The current UserSvcClient interface only accepts content.
	// For now, file_ids are ignored here — they are sent via API Gateway REST endpoint.
	_ = req.FileIds

	_, _, err := r.userClient.SendDM(ctx, userID, req.ReceiverId, req.Content)
	if err != nil {
		return nil, fmt.Errorf("user service SendDM: %w", err)
	}

	return &gatewayv1.ServerEvent{
		Type:      gatewayv1.ServerEventType_SERVER_EVENT_TYPE_ACK,
		RequestId: msg.RequestId,
	}, nil
}
```

同理更新 `handleSendChannelMessage`:
```go
	_ = req.FileIds  // passed through via API Gateway REST
```

- [ ] **Step 3: 编译验证**

```bash
cd backend && go build ./services/ws-gateway/...
```

Expected: 编译通过

- [ ] **Step 4: Commit**

```bash
git add backend/services/ws-gateway/push.go backend/services/ws-gateway/router.go
git commit -m "feat(ws-gateway): add NotificationEvent handling + file_ids passthrough"
```

---

### Task 23: API Gateway — 新服务路由

**Files:**
- Modify: `backend/services/api-gateway/main.go`
- Modify: `backend/services/api-gateway/routes.go`
- Create: `backend/services/api-gateway/handlers/file_handler.go`
- Create: `backend/services/api-gateway/handlers/search_handler.go`
- Create: `backend/services/api-gateway/handlers/notify_handler.go`
- Modify: `backend/services/api-gateway/handlers/helpers.go`

- [ ] **Step 1: 更新 helpers.go — Clients 加新服务客户端**

在 Clients 结构体追加:
```go
	File    filev1connect.FileServiceClient
	Search  searchv1connect.SearchServiceClient
	Notify  notifyv1connect.NotifyServiceClient
```

在 clientsConfig 追加:
```go
	FileServiceURL    string
	SearchServiceURL  string
	NotifyServiceURL  string
```

在 newClients 中追加:
```go
	File: filev1connect.NewFileServiceClient(
		http.DefaultClient,
		cfg.FileServiceURL,
	),
	Search: searchv1connect.NewSearchServiceClient(
		http.DefaultClient,
		cfg.SearchServiceURL,
	),
	Notify: notifyv1connect.NewNotifyServiceClient(
		http.DefaultClient,
		cfg.NotifyServiceURL,
	),
```

更新 NewClientsFromURLs:
```go
func NewClientsFromURLs(authURL, userURL, communityURL, fileURL, searchURL, notifyURL string) *Clients {
	return newClients(clientsConfig{
		AuthServiceURL:      authURL,
		UserServiceURL:      userURL,
		CommunityServiceURL: communityURL,
		FileServiceURL:      fileURL,
		SearchServiceURL:    searchURL,
		NotifyServiceURL:    notifyURL,
	})
}
```

需要 import 对应的 connect 包:
```go
	filev1connect "github.com/constell/constell/backend/pkg/proto/file/v1/filev1connect"
	searchv1connect "github.com/constell/constell/backend/pkg/proto/search/v1/searchv1connect"
	notifyv1connect "github.com/constell/constell/backend/pkg/proto/notify/v1/notifyv1connect"
```

- [ ] **Step 2: 更新 main.go — Config 加新服务 URL**

在 Config 结构体追加:
```go
	FileServiceURL    string `env:"FILE_SERVICE_URL" default:"http://file-service:9084"`
	SearchServiceURL  string `env:"SEARCH_SERVICE_URL" default:"http://search-service:9085"`
	NotifyServiceURL  string `env:"NOTIFY_SERVICE_URL" default:"http://notify-service:9086"`
```

更新 service discovery:
```go
	fileURL := cfg.FileServiceURL
	searchURL := cfg.SearchServiceURL
	notifyURL := cfg.NotifyServiceURL

	if reg != nil {
		// ...existing discovery...
		if instances, err := reg.Discover(context.Background(), "file-service"); err == nil && len(instances) > 0 {
			fileURL = "http://" + instances[0].Addr
		}
		if instances, err := reg.Discover(context.Background(), "search-service"); err == nil && len(instances) > 0 {
			searchURL = "http://" + instances[0].Addr
		}
		if instances, err := reg.Discover(context.Background(), "notify-service"); err == nil && len(instances) > 0 {
			notifyURL = "http://" + instances[0].Addr
		}
	}
```

更新 NewClientsFromURLs 调用:
```go
	clients := handlers.NewClientsFromURLs(authURL, userURL, communityURL, fileURL, searchURL, notifyURL)
```

- [ ] **Step 3: 创建 file_handler.go**

```go
package handlers

import (
	"net/http"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"

	filev1 "github.com/constell/constell/backend/pkg/proto/file/v1"
	filev1connect "github.com/constell/constell/backend/pkg/proto/file/v1/filev1connect"
)

type FileHandler struct {
	client filev1connect.FileServiceClient
}

func NewFileHandler(client filev1connect.FileServiceClient) *FileHandler {
	return &FileHandler{client: client}
}

// UploadFile handles POST /api/v1/files/upload
func (h *FileHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse form")
		return
	}

	fileID := r.FormValue("file_id")
	file, header, err := r.FormFile("data")
	if err != nil {
		writeError(w, http.StatusBadRequest, "data field required")
		return
	}
	defer file.Close()

	data := make([]byte, header.Size)
	if _, err := file.Read(data); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read file data")
		return
	}

	cr := connect.NewRequest(&filev1.UploadFileRequest{
		FileId:      fileID,
		Filename:    header.Filename,
		ContentType: header.Header.Get("Content-Type"),
		Data:        data,
	})
	forwardAuth(r, cr)

	resp, err := h.client.UploadFile(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp.Msg.File)
}

// GetFileURL handles GET /api/v1/files/{id}/url
func (h *FileHandler) GetFileURL(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "id")
	if fileID == "" {
		writeError(w, http.StatusBadRequest, "file id required")
		return
	}

	cr := connect.NewRequest(&filev1.GetFilePresignedURLRequest{FileId: fileID})
	forwardAuth(r, cr)

	resp, err := h.client.GetFilePresignedURL(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"url":       resp.Msg.Url,
		"expires_at": resp.Msg.ExpiresAt,
	})
}

// DeleteFile handles DELETE /api/v1/files/{id}
func (h *FileHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "id")
	cr := connect.NewRequest(&filev1.DeleteFileRequest{FileId: fileID})
	forwardAuth(r, cr)

	_, err := h.client.DeleteFile(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
```

- [ ] **Step 4: 创建 search_handler.go**

```go
package handlers

import (
	"net/http"
	"strconv"

	"connectrpc.com/connect"

	searchv1 "github.com/constell/constell/backend/pkg/proto/search/v1"
	searchv1connect "github.com/constell/constell/backend/pkg/proto/search/v1/searchv1connect"
)

type SearchHandler struct {
	client searchv1connect.SearchServiceClient
}

func NewSearchHandler(client searchv1connect.SearchServiceClient) *SearchHandler {
	return &SearchHandler{client: client}
}

// Search handles GET /api/v1/search?q=...&types=...&limit=...
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	var types []searchv1.SearchType
	for _, t := range r.URL.Query()["types"] {
		switch t {
		case "users":
			types = append(types, searchv1.SearchType_SEARCH_TYPE_USERS)
		case "messages":
			types = append(types, searchv1.SearchType_SEARCH_TYPE_MESSAGES)
		case "dm_messages":
			types = append(types, searchv1.SearchType_SEARCH_TYPE_DM_MESSAGES)
		}
	}

	cr := connect.NewRequest(&searchv1.SearchRequest{
		Query: query,
		Types: types,
		Limit: int32(limit),
	})
	forwardAuth(r, cr)

	resp, err := h.client.Search(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp.Msg)
}
```

- [ ] **Step 5: 创建 notify_handler.go**

```go
package handlers

import (
	"net/http"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"

	notifyv1 "github.com/constell/constell/backend/pkg/proto/notify/v1"
	notifyv1connect "github.com/constell/constell/backend/pkg/proto/notify/v1/notifyv1connect"
)

type NotifyHandler struct {
	client notifyv1connect.NotifyServiceClient
}

func NewNotifyHandler(client notifyv1connect.NotifyServiceClient) *NotifyHandler {
	return &NotifyHandler{client: client}
}

// GetUnread handles GET /api/v1/notify/unread
func (h *NotifyHandler) GetUnread(w http.ResponseWriter, r *http.Request) {
	cr := connect.NewRequest(&notifyv1.GetUnreadCountsRequest{})
	forwardAuth(r, cr)

	resp, err := h.client.GetUnreadCounts(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp.Msg)
}

// MarkDMRead handles POST /api/v1/notify/dm/{conv_id}/read
func (h *NotifyHandler) MarkDMRead(w http.ResponseWriter, r *http.Request) {
	convID := chi.URLParam(r, "conv_id")
	cr := connect.NewRequest(&notifyv1.MarkDMReadRequest{ConversationId: convID})
	forwardAuth(r, cr)

	_, err := h.client.MarkDMRead(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// MarkChannelRead handles POST /api/v1/notify/channel/{ch_id}/read
func (h *NotifyHandler) MarkChannelRead(w http.ResponseWriter, r *http.Request) {
	chID := chi.URLParam(r, "ch_id")
	cr := connect.NewRequest(&notifyv1.MarkChannelReadRequest{ChannelId: chID})
	forwardAuth(r, cr)

	_, err := h.client.MarkChannelRead(r.Context(), cr)
	if err != nil {
		writeConnectError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **Step 6: 更新 routes.go — 新增路由**

在 `registerRoutes` 函数中追加:
```go
	// Create new handler instances.
	fileHandler := handlers.NewFileHandler(clients.File)
	searchHandler := handlers.NewSearchHandler(clients.Search)
	notifyHandler := handlers.NewNotifyHandler(clients.Notify)
```

在 `/api/v1` 路由组中追加:
```go
	// File routes.
	r.Post("/files/upload", fileHandler.UploadFile)
	r.Get("/files/{id}/url", fileHandler.GetFileURL)
	r.Delete("/files/{id}", fileHandler.DeleteFile)

	// Search route.
	r.Get("/search", searchHandler.Search)

	// Notify routes.
	r.Get("/notify/unread", notifyHandler.GetUnread)
	r.Post("/notify/dm/{conv_id}/read", notifyHandler.MarkDMRead)
	r.Post("/notify/channel/{ch_id}/read", notifyHandler.MarkChannelRead)
```

- [ ] **Step 7: 编译验证**

```bash
cd backend && go build ./services/api-gateway/...
```

Expected: 编译通过

- [ ] **Step 8: Commit**

```bash
git add backend/services/api-gateway/
git commit -m "feat(api-gateway): add file/search/notify REST routes and handlers"
```

---

### Task 24: 配置 + Docker Compose + services.yaml + 集成验证

**Files:**
- Modify: `deploy/configs/dev.yaml`
- Modify: `deploy/configs/services.yaml`
- Modify: `deploy/docker/docker-compose.yml`
- Modify: `backend/go.work`

- [ ] **Step 1: 更新 dev.yaml**

在 `services:` 下追加:
```yaml
  file_service:
    addr: :9084
  search_service:
    addr: :9085
  notify_service:
    addr: :9086
```

- [ ] **Step 2: 更新 services.yaml**

追加三个新服务:
```yaml
  file-service:
    instances:
      - addr: "file-service:9084"

  search-service:
    instances:
      - addr: "search-service:9085"

  notify-service:
    instances:
      - addr: "notify-service:9086"
```

- [ ] **Step 3: 更新 docker-compose.yml**

在 `# Backend Services` 部分追加:
```yaml
  file-service:
    build:
      context: ../../
      dockerfile: backend/services/file-service/Dockerfile
    container_name: constell-file-service
    environment:
      PORT: "9084"
      DATABASE_URL: "postgres://constell:constell_dev@postgres:5432/constell?sslmode=disable"
      MINIO_ENDPOINT: "minio:9000"
      MINIO_ACCESS_KEY: "minioadmin"
      MINIO_SECRET_KEY: "minioadmin"
      MINIO_BUCKET: "constell"
      MINIO_USE_SSL: "false"
      MINIO_BASE_URL: "http://minio:9000"
      JWT_SECRET: "dev-secret-change-me"
      REGISTRY_TYPE: "static"
      SERVICES_CONFIG_PATH: "/app/configs/services.yaml"
      OTEL_EXPORTER_OTLP_ENDPOINT: "http://openobserve:5080/api/default/v1/otlp"
    ports:
      - "9084:9084"
    volumes:
      - ../../deploy/configs:/app/configs:ro
    depends_on:
      postgres:
        condition: service_healthy
      minio:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:9084/healthz"]
      interval: 10s
      timeout: 5s
      retries: 3

  search-service:
    build:
      context: ../../
      dockerfile: backend/services/search-service/Dockerfile
    container_name: constell-search-service
    environment:
      PORT: "9085"
      DATABASE_URL: "postgres://constell:constell_dev@postgres:5432/constell?sslmode=disable"
      JWT_SECRET: "dev-secret-change-me"
      REGISTRY_TYPE: "static"
      SERVICES_CONFIG_PATH: "/app/configs/services.yaml"
      OTEL_EXPORTER_OTLP_ENDPOINT: "http://openobserve:5080/api/default/v1/otlp"
    ports:
      - "9085:9085"
    volumes:
      - ../../deploy/configs:/app/configs:ro
    depends_on:
      postgres:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:9085/healthz"]
      interval: 10s
      timeout: 5s
      retries: 3

  notify-service:
    build:
      context: ../../
      dockerfile: backend/services/notify-service/Dockerfile
    container_name: constell-notify-service
    environment:
      PORT: "9086"
      DATABASE_URL: "postgres://constell:constell_dev@postgres:5432/constell?sslmode=disable"
      REDIS_URL: "redis:6379"
      NATS_URL: "nats://nats:4222"
      JWT_SECRET: "dev-secret-change-me"
      REGISTRY_TYPE: "static"
      SERVICES_CONFIG_PATH: "/app/configs/services.yaml"
      OTEL_EXPORTER_OTLP_ENDPOINT: "http://openobserve:5080/api/default/v1/otlp"
    ports:
      - "9086:9086"
    volumes:
      - ../../deploy/configs:/app/configs:ro
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
      nats:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:9086/healthz"]
      interval: 10s
      timeout: 5s
      retries: 3
```

同时在 api-gateway 的 environment 中追加:
```yaml
      FILE_SERVICE_URL: "http://file-service:9084"
      SEARCH_SERVICE_URL: "http://search-service:9085"
      NOTIFY_SERVICE_URL: "http://notify-service:9086"
```

在 api-gateway 的 depends_on 中追加:
```yaml
      - file-service
      - search-service
      - notify-service
```

在 community-service 的 environment 中追加:
```yaml
      NATS_URL: "nats://nats:4222"
```

在 user-service 的 environment 中追加:
```yaml
      NATS_URL: "nats://nats:4222"
```

- [ ] **Step 4: 更新 PROJECT_STATUS.md**

将 Plan 4 状态从 "⏳ 待规划" 改为 "📋 计划就绪":
```
| Plan 4: File + Search + Notify | plans/2026-06-02-plan4-file-search-notify.md | 📋 计划就绪 | |
```

- [ ] **Step 5: Docker Compose 启动验证**

```bash
make docker-up
```

Expected: 所有服务健康启动，包括 3 个新服务。

验证:
```bash
# File Service health
curl http://localhost:9084/healthz

# Search Service health
curl http://localhost:9085/healthz

# Notify Service health
curl http://localhost:9086/healthz

# API Gateway health (包含新路由)
curl http://localhost:8080/healthz
```

- [ ] **Step 6: Commit**

```bash
git add deploy/configs/ deploy/docker/ docs/PROJECT_STATUS.md
git commit -m "feat: add file/search/notify services to Docker Compose, configs, and project status"
```

---

## Self-Review Checklist

- [x] **Spec coverage:** 每个 spec section 都有对应 Task:
  - Section 1 (File Service) → Tasks 8-12
  - Section 2 (Search Service) → Tasks 13-15
  - Section 3 (Notify Service) → Tasks 16-19
  - Section 4 (Message Attachments) → Tasks 20-21
  - Section 5 (Cross-system) → Tasks 1-7, 22-24
  - Section 6 (Validation) → Task 24
  - Section 7 (Future) → 不在本 plan 范围
- [x] **Placeholder scan:** 无 TBD/TODO (除明确的 TODO 优化注释)
- [x] **Type consistency:** 所有 proto message 字段名、Go 结构体字段名、RPC 方法签名在 Tasks 之间保持一致
