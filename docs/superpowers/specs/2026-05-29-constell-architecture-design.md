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
