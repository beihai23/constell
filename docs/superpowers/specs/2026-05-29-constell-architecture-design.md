# Constell — IM 系统架构设计

## 项目定位

Constell（星座）是一个面向志趣相投人群的开源社群型 IM 系统，类似 Discord 模式，以 Server/Guild、Channel、成员管理为核心。

## 技术选型

| 层面 | 选择 | 理由 |
|------|------|------|
| 后端语言 | Go | 高并发天然优势，IM 领域主流 |
| 传输协议 | WebSocket + Protobuf | 性能最优，移动端省流量 |
| 服务间通信 | Connect-RPC | Buf 生态原生，HTTP 友好，兼容 gRPC |
| 异步事件 | NATS | Go 原生，轻量高性能，JetStream 持久化 |
| 业务存储 | PostgreSQL | 消息、用户、频道元数据 |
| 缓存/状态 | Redis | 连接路由表、未读计数、session |
| 文件存储 | Object Storage (S3 兼容) | 图片、文件 |
| 全文搜索 | PostgreSQL tsvector/tsquery | 初期不引入独立搜索引擎，Search Service 底层调各服务的 PG 全文搜索 |
| Protobuf 管理 | Buf | proto 文件的 lint、生成、版本管理 |
| 部署 | Docker Compose | 初期轻量部署，架构预留 K8s 迁移空间 |
| 认证 | 自建 + OAuth2 | 邮箱/手机注册 + Google/GitHub/Apple 登录 |

## 系统架构

### 服务拆分

```
                         ┌─────────────┐
                         │   Clients    │
                         │  Web / SDK   │
                         └──────┬───────┘
                                │ WebSocket + Protobuf
                     ┌──────────┴──────────┐
                     │                     │
              ┌──────┴──────┐      ┌───────┴──────┐
              │  API GW ×N  │      │  WS GW  ×N   │
              │  (REST)     │      │ (连接管理)    │
              └──────┬──────┘      └───────┬───────┘
                     │                     │
                     │   Connect-RPC       │ Connect-RPC
                     │◄───────────────────►│
                     │                     │
    ┌────────────────┼─────────────────────┼────────────────┐
    │                │                     │                │
    │  消息类型分流:  │                     │                │
    │  DM ──────────►│ User Svc            │                │
    │  频道消息 ────►│ Community Svc       │                │
    │                │                     │                │
    ┌────┴────┐  ┌───┴────┐  ┌──────┐  ┌──┴──┐  ┌──────┐
    │Auth Svc │  │User Svc│  │Comm. │  │File │  │Search│
    │  ×N     │  │  ×N    │  │Svc×N │  │Svc×N│  │ Svc  │
    │无状态   │  │有状态  │  │有状态│  │无状态│  │无状态│
    └─────────┘  └────────┘  └──────┘  └─────┘  └──────┘
                                    │
                    ┌───────────────┼───────────────┐
                    │               │               │
              ┌─────┴─────┐  ┌─────┴─────┐  ┌─────┴─────┐
              │ PostgreSQL │  │   Redis   │  │   NATS    │
              └───────────┘  └───────────┘  └───────────┘
```

### 服务职责

| 服务 | 状态 | 分区键 | 职责 |
|------|------|--------|------|
| **API Gateway** | 无状态 | — | REST API 入口，认证鉴权，限流，Connect-RPC 路由 |
| **WS Gateway** | 有状态 (conn map + Redis) | — | WebSocket 连接管理，心跳保活，消息路由。对业务透明，升级不中断连接 |
| **Auth Service** | 无状态 | — | 注册/登录，JWT 签发，OAuth2 接入 |
| **User Service** | 有状态 (groupcache) | user_id | 用户 Profile，好友关系/黑名单，**DM 对话和消息**，在线状态查询 |
| **Community Service** | 有状态 (groupcache) | community_id | Server/Guild CRUD，Channel CRUD，成员管理，角色/权限模型，**群聊消息存储和历史**，已读状态/Reaction/Pin |
| **File Service** | 无状态 | — | 文件上传/下载，图片缩略图，对象存储管理 |
| **Search Service** | 无状态 | — | 统一搜索入口，聚合用户搜索 (→User Svc) 和消息搜索 (→Community Svc)。底层使用 PG tsvector，后续可替换为 Elasticsearch |
| **Notify Service** | 无状态 | — | 推送通知 (Web Push/APNs/FCM)，未读计数管理 |

### 有状态服务设计 — Groupcache 模式

User Service 和 Community Service 采用相同的 groupcache 思想：

1. **一致性哈希分区**：每个实例"拥有"一部分数据
   - User Service：按 user_id 分区，owning node 缓存该用户的 profile 和关系
   - Community Service：按 community_id 分区，owning node 缓存该 Server 下所有频道元数据、成员列表、角色/权限定义

   **为什么 Community Service 按 community_id 而非 channel_id 分区：**
   权限计算需要同时访问 Server 级别数据（角色定义、成员角色绑定）和 Channel 级别数据（Permission Overwrite）。
   按 community_id 分区保证同一个 Server 的所有 Channel、成员、角色都在同一个 owning instance 上，
   权限计算和成员查询全部本地内存完成，无需跨实例 peer fill。

2. **Peer-to-Peer Fill**：非 owning node 收到请求后，通过 Connect-RPC 向 owning node 请求数据

3. **Singleflight 防惊群**：同一 key 的并发请求只穿透 DB 一次

4. **写操作天然一致**：写请求路由到 owning node → 写 DB → 更新本地缓存。每个 key 只缓存在一个实例，无需失效/广播机制

5. **Failover**：实例宕机后，哈希环自动将数据迁移到相邻节点，首次请求从 DB 填充

**不缓存的数据**：消息内容（数据量大、读写均衡）留在 PostgreSQL，不在进程内缓存。

## WS Gateway 集群设计

WS Gateway 持有连接状态，不是无状态服务：

1. **连接注册表**：Redis 存储 `uid → gw_id` 映射，任何服务可查询用户在哪个 WS GW 实例上
2. **本地 conn map**：每台 WS GW 维护本地 `uid → websocket connection` 映射
3. **消息推送流程**：
   - 业务服务查 Redis 获取目标用户的 gw_id
   - 通过 NATS topic `gw.push.{gw_id}` 精确投递到目标 GW 实例
   - WS GW 收到后查本地 conn map 推送
4. **负载均衡**：L4 层用一致性哈希 (UID-based)，实例增减时最小化连接迁移
5. **连接生命周期**：
   - 建立：认证 JWT → 写本地 conn map → 写 Redis `uid→gw_id` → NATS 广播 user_online
   - 断开：移除本地 conn map → 删除 Redis key → NATS 广播 user_offline

## 数据模型

### users 表

```sql
CREATE TABLE users (
    id          BIGSERIAL PRIMARY KEY,
    username    VARCHAR(32) NOT NULL UNIQUE,
    email       VARCHAR(255) NOT NULL UNIQUE,
    password    VARCHAR(255) NOT NULL,       -- bcrypt hash
    nickname    VARCHAR(64) NOT NULL DEFAULT '',
    avatar_url  TEXT NOT NULL DEFAULT '',
    status_msg  VARCHAR(128) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_email ON users(email);
-- 全文搜索
ALTER TABLE users ADD COLUMN search_vector tsvector;
CREATE INDEX idx_users_search ON users USING GIN(search_vector);
-- 触发器: 昵称变更时自动更新 search_vector
```

### user_relations 表

```sql
CREATE TABLE user_relations (
    user_id       BIGINT NOT NULL REFERENCES users(id),
    target_id     BIGINT NOT NULL REFERENCES users(id),
    relation_type VARCHAR(16) NOT NULL,  -- 'friend' | 'blocked'
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, target_id)
);
CREATE INDEX idx_user_relations_target ON user_relations(target_id, relation_type);
```

### dm_conversations 表

```sql
CREATE TABLE dm_conversations (
    id              BIGSERIAL PRIMARY KEY,
    user_a_id       BIGINT NOT NULL REFERENCES users(id),
    user_b_id       BIGINT NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_a_id, user_b_id),
    CONSTRAINT user_a_less_than_b CHECK (user_a_id < user_b_id)
);
CREATE INDEX idx_dm_conversations_user_a ON dm_conversations(user_a_id);
CREATE INDEX idx_dm_conversations_user_b ON dm_conversations(user_b_id);
```

**conversation_id 生成规则**：两个 user_id 排序后较小的为 user_a，较大的为 user_b。DM 消息通过 (user_a_id, user_b_id) 唯一定位一个会话。

### dm_messages 表

```sql
CREATE TABLE dm_messages (
    id              BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES dm_conversations(id),
    sender_id       BIGINT NOT NULL REFERENCES users(id),
    content_type    VARCHAR(16) NOT NULL DEFAULT 'text',  -- 'text' | 'image' | 'file'
    content         TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_dm_messages_conv_time ON dm_messages(conversation_id, created_at DESC);
```

### servers 表

```sql
CREATE TABLE servers (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(100) NOT NULL,
    icon_url    TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    owner_id    BIGINT NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### channels 表

```sql
CREATE TABLE channels (
    id          BIGSERIAL PRIMARY KEY,
    server_id   BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name        VARCHAR(100) NOT NULL,
    topic       VARCHAR(256) NOT NULL DEFAULT '',
    position    INT NOT NULL DEFAULT 0,
    type        VARCHAR(16) NOT NULL DEFAULT 'text',  -- 'text' | 'announcement'
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_channels_server ON channels(server_id, position);
```

### server_members 表

```sql
CREATE TABLE server_members (
    server_id   BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    user_id     BIGINT NOT NULL REFERENCES users(id),
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (server_id, user_id)
);
CREATE INDEX idx_server_members_user ON server_members(user_id);
```

### roles 表

```sql
CREATE TABLE roles (
    id          BIGSERIAL PRIMARY KEY,
    server_id   BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name        VARCHAR(64) NOT NULL,
    color       INT NOT NULL DEFAULT 0,
    permissions BIGINT NOT NULL DEFAULT 0,    -- 位掩码
    position    INT NOT NULL DEFAULT 0,
    is_default  BOOLEAN NOT NULL DEFAULT false -- 新成员自动获得
);
CREATE INDEX idx_roles_server ON roles(server_id, position);

-- 关联表: 成员 ↔ 角色 (多对多)
CREATE TABLE member_roles (
    role_id   BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    user_id   BIGINT NOT NULL,
    server_id BIGINT NOT NULL,
    PRIMARY KEY (role_id, user_id, server_id),
    FOREIGN KEY (server_id, user_id) REFERENCES server_members(server_id, user_id) ON DELETE CASCADE
);
```

### channel_messages 表

```sql
CREATE TABLE channel_messages (
    id          BIGSERIAL PRIMARY KEY,
    channel_id  BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    sender_id   BIGINT NOT NULL REFERENCES users(id),
    content_type VARCHAR(16) NOT NULL DEFAULT 'text',
    content     TEXT NOT NULL,
    mentions    BIGINT[] NOT NULL DEFAULT '{}',  -- 被 @提及的用户 ID 列表
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_channel_messages_channel_time ON channel_messages(channel_id, created_at DESC);
-- 全文搜索
ALTER TABLE channel_messages ADD COLUMN search_vector tsvector;
CREATE INDEX idx_channel_messages_search ON channel_messages USING GIN(search_vector);
```

## 权限模型

采用 Discord 风格的位掩码权限系统。

### 权限位定义

```
ADMINISTRATOR       = 1 << 0   // 绕过所有权限检查
MANAGE_SERVER       = 1 << 1   // 修改服务器设置
MANAGE_CHANNELS     = 1 << 2   // 创建/编辑/删除频道
MANAGE_ROLES        = 1 << 3   // 管理角色
KICK_MEMBERS        = 1 << 4   // 踢出成员
BAN_MEMBERS         = 1 << 5   // 封禁成员
MANAGE_MESSAGES     = 1 << 6   // 删除他人消息
SEND_MESSAGES       = 1 << 7   // 发送消息
READ_MESSAGES       = 1 << 8   // 查看频道消息
MENTION_EVERYONE    = 1 << 9   // @everyone
ATTACH_FILES        = 1 << 10  // 上传附件
```

### 权限计算规则

```
1. 获取用户在 Server 中的所有 Role
2. 合并所有 Role 的 permissions (位或 OR)
3. 如果拥有 ADMINISTRATOR 位 → 允许一切
4. 检查频道级别的 Permission Overwrite:
   - @everyone 角色 overwrite → 先应用
   - 用户所在 Role overwrite → 再应用
   - 用户特定 overwrite → 最后应用
   - 每个 overwrite: deny 先清除位，allow 再设置位
5. 最终结果 = 计算后的权限位掩码
```

### Permission Overwrite (未来实现，MVP 简化)

MVP 阶段：成员 = 有 READ_MESSAGES + SEND_MESSAGES，非成员 = 无权限。频道级别的 overwrite 在后续版本添加。

## 服务层 Proto 接口定义

### Auth Service (auth/v1/auth.proto)

```protobuf
service AuthService {
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc Login(LoginRequest) returns (LoginResponse);
  rpc RefreshToken(RefreshTokenRequest) returns (RefreshTokenResponse);
}

message RegisterRequest { string username = 1; string email = 2; string password = 3; }
message RegisterResponse { string user_id = 1; string access_token = 2; string refresh_token = 3; }
message LoginRequest { string email = 1; string password = 2; }
message LoginResponse { string user_id = 1; string access_token = 2; string refresh_token = 3; }
message RefreshTokenRequest { string refresh_token = 1; }
message RefreshTokenResponse { string access_token = 1; string refresh_token = 2; }
```

### User Service (user/v1/user.proto)

```protobuf
service UserService {
  // Profile
  rpc GetUser(GetUserRequest) returns (GetUserResponse);
  rpc UpdateProfile(UpdateProfileRequest) returns (UpdateProfileResponse);

  // Relations
  rpc GetRelation(GetRelationRequest) returns (GetRelationResponse);
  rpc BlockUser(BlockUserRequest) returns (BlockUserResponse);
  rpc UnblockUser(UnblockUserRequest) returns (UnblockUserResponse);
  rpc ListFriends(ListFriendsRequest) returns (ListFriendsResponse);

  // DM
  rpc SendDM(SendDMRequest) returns (SendDMResponse);
  rpc GetDMHistory(GetDMHistoryRequest) returns (GetDMHistoryResponse);
  rpc GetDMConversations(GetDMConversationsRequest) returns (GetDMConversationsResponse);

  // Internal: peer fill for groupcache
  rpc GetLocalUser(GetLocalUserRequest) returns (GetLocalUserResponse);
  rpc GetLocalRelation(GetLocalRelationRequest) returns (GetLocalRelationResponse);
}

message GetUserRequest { string user_id = 1; }
message GetUserResponse { string user_id = 1; string username = 2; string nickname = 3; string avatar_url = 4; string status_msg = 5; }
message UpdateProfileRequest { string user_id = 1; optional string nickname = 2; optional string avatar_url = 3; optional string status_msg = 4; }
message UpdateProfileResponse { GetUserResponse user = 1; }

message GetRelationRequest { string user_id = 1; string target_id = 2; }
message GetRelationResponse { bool is_friend = 1; bool is_blocked = 2; }
message BlockUserRequest { string user_id = 1; string target_id = 2; }
message BlockUserResponse {}
message UnblockUserRequest { string user_id = 1; string target_id = 2; }
message UnblockUserResponse {}
message ListFriendsRequest { string user_id = 1; int32 limit = 2; string cursor = 3; }
message ListFriendsResponse { repeated GetUserResponse friends = 1; string next_cursor = 2; }

message SendDMRequest { string sender_id = 1; string receiver_id = 2; string content_type = 3; string content = 4; }
message SendDMResponse { string message_id = 1; string created_at = 2; }
message GetDMHistoryRequest { string user_id = 1; string peer_id = 2; int32 limit = 3; string cursor = 4; }
message GetDMHistoryResponse { repeated DMMessage messages = 1; string next_cursor = 2; }
message GetDMConversationsRequest { string user_id = 1; int32 limit = 2; string cursor = 3; }
message GetDMConversationsResponse { repeated DMConversation conversations = 1; string next_cursor = 2; }

message DMMessage { string id = 1; string sender_id = 2; string content_type = 3; string content = 4; string created_at = 5; }
message DMConversation { string peer_id = 1; GetUserResponse peer = 2; string last_message = 3; string last_message_at = 4; }

// Internal groupcache peer fill
message GetLocalUserRequest { string user_id = 1; }
message GetLocalUserResponse { GetUserResponse user = 1; }
message GetLocalRelationRequest { string user_id = 1; string target_id = 2; }
message GetLocalRelationResponse { bool is_friend = 1; bool is_blocked = 2; }
```

### Community Service (community/v1/community.proto)

```protobuf
service CommunityService {
  // Server
  rpc CreateServer(CreateServerRequest) returns (CreateServerResponse);
  rpc GetServer(GetServerRequest) returns (GetServerResponse);
  rpc UpdateServer(UpdateServerRequest) returns (UpdateServerResponse);
  rpc ListUserServers(ListUserServersRequest) returns (ListUserServersResponse);

  // Channel
  rpc CreateChannel(CreateChannelRequest) returns (CreateChannelResponse);
  rpc GetChannels(GetChannelsRequest) returns (GetChannelsResponse);
  rpc UpdateChannel(UpdateChannelRequest) returns (UpdateChannelResponse);

  // Members
  rpc AddMember(AddMemberRequest) returns (AddMemberResponse);
  rpc RemoveMember(RemoveMemberRequest) returns (RemoveMemberResponse);
  rpc ListMembers(ListMembersRequest) returns (ListMembersResponse);

  // Roles
  rpc CreateRole(CreateRoleRequest) returns (CreateRoleResponse);
  rpc AssignRole(AssignRoleRequest) returns (AssignRoleResponse);

  // Messages
  rpc SendMessage(SendMessageRequest) returns (SendMessageResponse);
  rpc GetHistory(GetHistoryRequest) returns (GetHistoryResponse);

  // Internal: peer fill for groupcache
  rpc GetLocalServer(GetLocalServerRequest) returns (GetLocalServerResponse);
  rpc GetLocalMembers(GetLocalMembersRequest) returns (GetLocalMembersResponse);
  rpc GetLocalRoles(GetLocalRolesRequest) returns (GetLocalRolesResponse);
}

// Server messages
message Server { string id = 1; string name = 2; string icon_url = 3; string description = 4; string owner_id = 5; string created_at = 6; }
message CreateServerRequest { string name = 1; string owner_id = 2; }
message CreateServerResponse { Server server = 1; }
message GetServerRequest { string server_id = 1; }
message GetServerResponse { Server server = 1; }
message UpdateServerRequest { string server_id = 1; string name = 2; string icon_url = 3; string description = 4; }
message UpdateServerResponse { Server server = 1; }
message ListUserServersRequest { string user_id = 1; }
message ListUserServersResponse { repeated Server servers = 1; }

// Channel messages
message Channel { string id = 1; string server_id = 2; string name = 3; string topic = 4; int32 position = 5; string type = 6; }
message CreateChannelRequest { string server_id = 1; string name = 2; string type = 3; }
message CreateChannelResponse { Channel channel = 1; }
message GetChannelsRequest { string server_id = 1; }
message GetChannelsResponse { repeated Channel channels = 1; }
message UpdateChannelRequest { string channel_id = 1; string name = 2; string topic = 3; int32 position = 4; }
message UpdateChannelResponse { Channel channel = 1; }

// Member messages
message ServerMember { string user_id = 1; string server_id = 2; string joined_at = 3; repeated string role_ids = 4; }
message AddMemberRequest { string server_id = 1; string user_id = 2; }
message AddMemberResponse { ServerMember member = 1; }
message RemoveMemberRequest { string server_id = 1; string user_id = 2; }
message RemoveMemberResponse {}
message ListMembersRequest { string server_id = 1; int32 limit = 2; string cursor = 3; }
message ListMembersResponse { repeated ServerMember members = 1; string next_cursor = 2; }

// Role messages
message Role { string id = 1; string server_id = 2; string name = 3; int32 color = 4; int64 permissions = 5; int32 position = 6; bool is_default = 7; }
message CreateRoleRequest { string server_id = 1; string name = 2; int64 permissions = 3; }
message CreateRoleResponse { Role role = 1; }
message AssignRoleRequest { string server_id = 1; string user_id = 2; string role_id = 3; }
message AssignRoleResponse {}

// Channel messages (content)
message ChannelMessage { string id = 1; string channel_id = 2; string sender_id = 3; string content_type = 4; string content = 5; repeated string mentions = 6; string created_at = 7; }
message SendMessageRequest { string channel_id = 1; string sender_id = 2; string content_type = 3; string content = 4; repeated string mentions = 5; }
message SendMessageResponse { string message_id = 1; string created_at = 2; }
message GetHistoryRequest { string channel_id = 1; int32 limit = 2; string cursor = 3; }
message GetHistoryResponse { repeated ChannelMessage messages = 1; string next_cursor = 2; }

// Internal groupcache peer fill
message GetLocalServerRequest { string server_id = 1; }
message GetLocalServerResponse { Server server = 1; repeated Channel channels = 2; }
message GetLocalMembersRequest { string server_id = 1; }
message GetLocalMembersResponse { repeated ServerMember members = 1; }
message GetLocalRolesRequest { string server_id = 1; }
message GetLocalRolesResponse { repeated Role roles = 1; }
```

## NATS 事件定义

所有事件使用 JetStream 持久化，stream 名 `constell`，subject 前缀 `constell.>`。

| Subject | 发布者 | 载荷 | 消费者 |
|---------|--------|------|--------|
| `constell.dm.created` | User Service | `{conversation_id, user_a_id, user_b_id, message_id, content, created_at}` | Search Svc, Notify Svc |
| `constell.message.created` | Community Service | `{message_id, channel_id, server_id, sender_id, content, mentions[], created_at}` | Search Svc, Notify Svc, WS GW (via gw.push.*) |
| `constell.member.joined` | Community Service | `{server_id, user_id}` | User Svc (更新 joined_servers 缓存) |
| `constell.member.left` | Community Service | `{server_id, user_id}` | User Svc |
| `constell.user.online` | WS Gateway | `{user_id, gw_id}` | User Svc (更新在线状态) |
| `constell.user.offline` | WS Gateway | `{user_id}` | User Svc |
| `gw.push.{gw_id}` | User Svc / Community Svc | `{targets: [uid], payload: {...}}` | WS GW (指定实例) |

## JWT 规范

- **算法**: HS256
- **Claims**: `sub` = user_id (string), `iat` = issued at, `exp` = expiry
- **Access Token**: 有效期 15 分钟
- **Refresh Token**: 有效期 7 天，存储在 Redis `refresh:{token_hash} → user_id`
- **Header**: `Authorization: Bearer <access_token>`
- **签发**: Auth Service
- **验证**: API Gateway / WS Gateway 中间件

## 错误码规范

服务间使用 Connect-RPC 标准错误码：

| 场景 | Connect Code | 说明 |
|------|-------------|------|
| 未认证 | CodeUnauthenticated | JWT 缺失或过期 |
| 权限不足 | CodePermissionDenied | 无 SEND_MESSAGES 等权限 |
| 资源不存在 | CodeNotFound | 用户/频道/Server 不存在 |
| 参数错误 | CodeInvalidArgument | 字段校验失败 |
| 已存在 | CodeAlreadyExists | 用户名/邮箱已注册 |
| 被拉黑 | CodePermissionDenied | DM 对方已拉黑发送者 |
| 内部错误 | CodeInternal | 未预期错误 |

## 服务端口分配

| 服务 | 端口 | 说明 |
|------|------|------|
| API Gateway | 8080 | HTTP REST 入口 |
| WS Gateway | 8081 | WebSocket 入口 |
| Auth Service | 9081 | Connect-RPC (内部) |
| User Service | 9082 | Connect-RPC (内部) |
| Community Service | 9083 | Connect-RPC (内部) |
| File Service | 9084 | Connect-RPC (内部) |
| Search Service | 9085 | Connect-RPC (内部) |
| Notify Service | 9086 | Connect-RPC (内部) |

## 领域边界

### User Service — 用户间的一切

- **用户 Profile**：昵称、头像、状态消息、偏好设置
- **用户关系**：好友列表、黑名单
- **DM 对话**：两个用户之间的直接消息，包括消息存储和历史查询
- **在线状态**：读取 Redis 中的 `uid→gw_id` 判断用户是否在线

DM 是用户关系的延伸，不属于 Community 领域。DM 消息存储在独立的 `dm_messages` 表，按 conversation_id（由两个 user_id 派生）组织。

### Community Service — 社区里的一切

- **Server/Guild**：创建、配置、图标、描述
- **Channel**：创建、分类、排序、频道类型（文本/公告/...）
- **成员管理**：加入/离开/踢出/Ban
- **角色和权限**：角色定义、分配、Permission Overwrite
- **群聊消息**：频道内消息的存储、历史查询
- **消息交互**：已读状态、Reaction、Pin

## 协议分层与 Proto 定位

系统存在两层独立的 Protobuf 协议，分别服务于不同的通信场景：

### 服务层 Proto (Connect-RPC)

定义在 `proto/auth/`、`proto/user/`、`proto/community/` 中，是后端微服务之间的内部接口。

- 传输方式：Connect-RPC over HTTP（服务间同步调用）
- 消费者：API Gateway、WS Gateway、其他后端服务
- 不直接暴露给客户端
- 包含完整的业务 RPC：`SendMessage`、`GetHistory`、`CreateServer` 等

```
                    Connect-RPC (内部)
API Gateway ──────────────────────→ User Service
               ──────────────────────→ Community Service
WS Gateway   ──────────────────────→ Auth Service
               ──────────────────────→ ...
```

### 客户端层 Proto (WebSocket)

定义在 `proto/gateway/` 中，是客户端与 WS Gateway 之间的协议。

- 传输方式：WebSocket + Protobuf 二进制帧
- 消费者：Web 客户端、移动端 SDK
- 包含客户端操作：`send_dm`、`send_channel_message`、`subscribe_channel` 等
- WS Gateway 负责将客户端协议翻译为服务层 Connect-RPC 调用

```
客户端 ←── WebSocket + Protobuf ──→ WS Gateway ──Connect-RPC──→ 后端服务
         (gateway/v1 proto)              (协议翻译)
```

### 为什么分成两层

1. **关注点不同**：服务层关注业务语义（权限、存储），客户端层关注交互体验（消息类型、事件推送、订阅管理）
2. **独立演进**：客户端协议可以增加便捷操作（如批量拉取、游标订阅）而不影响服务接口
3. **推送机制**：客户端层有服务端主动推送的事件（`message.created`、`user.online`），这是单向的，不属于 RPC 模式
4. **Plan 1 只实现服务层**：API Gateway 暴露 HTTP 端点调用服务层 RPC，用于验证后端逻辑。客户端层 Proto 和 WS Gateway 在 Plan 2 实现。

## 核心场景时序

### 私聊发消息

```
User A → WS GW → User Svc:
  ③ 查黑名单 (owning node 内存, ~100ns)
  ④ INSERT dm_message → PostgreSQL
  ⑤ GET user_b → gw_id → Redis
  ⑥ Publish "gw.push.gw2" → NATS
  ← ack → WS GW → User A
  NATS → WS GW#2 → User B

热路径: 零跨服务调用
```

### 群聊发消息

```
User A → WS GW → Community Svc:
  ③ 验证频道 + 计算权限 (owning node 内存)
  ④ GetUser(user_b) → User Svc [仅@mention, owning node 内存]
  ⑤ INSERT message → PostgreSQL
  ⑥ 读成员列表 (owning node 内存)
  ⑦ MGET members → gw_ids → Redis
  ⑧ 按 GW 分组 → Publish "gw.push.*" × M → NATS
  ← ack → WS GW → User A
  NATS → WS GW#2, #3 → 在线成员
  NATS → Notify Svc → 离线成员推送通知

热路径: 最多 1 次跨服务调用 (@mention)
```

### 扇出优化

同一 WS GW 实例上的多个目标用户合并成一条 NATS 消息。例如 gw2 上有 user_b 和 user_e，只发一条 Publish "gw.push.gw2" {targets: [b, e]}。WS GW 收到后遍历本地 conn map 逐一推送。

## Monorepo 目录结构

```
constell/
├── proto/                        # 共享 Protobuf 定义 (Buf 管理)
│   ├── auth/v1/
│   ├── user/v1/
│   ├── community/v1/
│   ├── file/v1/
│   ├── search/v1/
│   ├── notify/v1/
│   └── gateway/v1/               # WS 协议定义
│
├── backend/                      # Go Workspace (go.work)
│   ├── pkg/                      # 共享库
│   │   ├── jwt/
│   │   ├── redis/
│   │   ├── postgres/
│   │   ├── nats/
│   │   ├── groupcache/           # 有状态服务的通用 groupcache 封装
│   │   ├── proto/                # 生成的 Go protobuf 代码
│   │   └── middleware/           # Connect-RPC 中间件
│   │
│   ├── services/
│   │   ├── api-gateway/
│   │   ├── ws-gateway/
│   │   ├── auth-service/
│   │   ├── user-service/
│   │   ├── community-service/
│   │   ├── file-service/
│   │   ├── search-service/
│   │   └── notify-service/
│   │
│   └── tools/
│       ├── migrate/
│       └── seed/
│
├── sdk/                          # 客户端 SDK
│   ├── sdk-go/
│   ├── sdk-js/
│   └── sdk-kmp/
│
├── clients/                      # 客户端应用
│   └── web/
│
├── deploy/
│   ├── docker/
│   │   ├── docker-compose.yml
│   │   └── Dockerfile.*
│   └── configs/
│       ├── dev.yaml
│       └── prod.yaml
│
├── docs/
├── scripts/
├── Makefile
└── buf.yaml
```

## MVP 功能范围

一期包含：

- [x] 用户系统：注册/登录、Profile、OAuth2、在线状态
- [x] 聊天核心：DM、Server/Channel 管理、群聊消息、已读状态
- [x] 文件媒体：图片/文件上传、缩略图
- [x] 搜索功能：消息搜索、用户搜索、频道搜索

一期客户端仅 Web，后续扩展 Android/iOS/Desktop。

## 部署方案

Docker Compose 一键启动：

- api-gateway, ws-gateway × 2, auth-service × 2, user-service × 2, community-service × 2
- file-service × 2, search-service, notify-service
- PostgreSQL, Redis, NATS, MinIO (S3 兼容)

每个有状态/连接密集型服务至少 2 实例，验证水平扩展。

## 后续演进

- Web 客户端 → Android/iOS (SDK-KMP)
- Docker Compose → Kubernetes (Helm Charts)
- 单 PostgreSQL → 读写分离 → 分库分表（按 server_id 或 channel_id）
- PG 全文搜索 → Meilisearch/Elasticsearch（Search Service 底层替换，客户端 API 不变）
- 添加语音/视频通话
- Bot/集成框架
