# Constell — Plan 3: WS Gateway 设计规格

> **阶段定位：** Plan 3 为系统添加 WebSocket 实时通信能力。WS Gateway 是唯一持有客户端连接的有状态网关，负责连接管理、心跳保活、消息路由和多实例扇出。
> 本文档描述 WS Gateway 的设计，作为已实现系统的 spec 记录。

## 服务概览

| 属性 | 值 |
|------|-----|
| 服务名 | WS Gateway |
| 端口 | 8081 |
| 状态 | 有状态（conn map + Redis 注册表） |
| 依赖 | Redis, NATS, User Service (Connect-RPC), Community Service (Connect-RPC) |
| 连接入口 | `ws://{host}:8081/ws?token={jwt}` |

## 组件架构

```
WS Gateway
├── Authenticator      JWT 认证（WebSocket 升级时）
├── ConnManager        本地连接管理（user_id → ConnEntry）
├── Registry           Redis 注册表（user_id → gw_id）
├── Router             消息路由（ClientMessage → Service RPC）
├── PushSubscriber     NATS 推送订阅（gw.push.{gw_id}）
├── HeartbeatHandler   心跳保活
└── Protocol           二进制帧编解码（length-prefixed protobuf）
```

---

## 1. 连接生命周期

### 1.1 建立连接

```
Client → GET /ws?token={jwt}
  → Authenticator.AuthenticateUpgrade: 解析 JWT → user_id
  → WebSocket Upgrade (gorilla/websocket)
  → ConnManager.Register: 存入本地 conn map
  → Registry.RegisterConnection: Redis SET ws:uid:{user_id} = {gw_id} (TTL 5min)
  → NATS Publish "constell.user.online" {user_id, gw_id}
  → 启动 readPump goroutine
```

### 1.2 连接期间

- 每条消息（包括心跳）重置 WebSocket 读超时（heartbeat interval + deadline）
- 每次心跳刷新 Redis 注册表 TTL
- readPump 循环读取客户端消息，路由到对应 handler

### 1.3 断开连接

```
readPump 退出（读错误 / 连接关闭）
  → cleanupDisconnect:
    → ConnManager.Unregister: 删除本地 conn map + 关闭 WebSocket
    → Registry.UnregisterConnection: Redis DEL ws:uid:{user_id}
    → NATS Publish "constell.user.offline" {user_id}
```

---

## 2. 认证

### 2.1 机制

JWT token 通过 WebSocket 升级请求的 query parameter 传递：

```
ws://localhost:8081/ws?token=eyJhbGciOiJIUzI1NiIs...
```

- 算法：HS256
- 服务端使用 `pkg/jwt.ParseToken(secret, token)` 验证
- 返回 user_id 作为连接身份
- 认证失败返回 HTTP 401，不升级 WebSocket

### 2.2 单用户单连接

`ConnManager.Register` 对同一 user_id 直接覆盖旧连接。这意味着：
- 用户在同一台 GW 上重连时，旧连接被替换
- 不支持同一用户多设备同时在线（后续可扩展）

---

## 3. 本地连接管理 (ConnManager)

### 3.1 数据结构

```go
type ConnEntry struct {
    UserID             string
    Conn               *websocket.Conn
    ConnectedAt        time.Time
    SubscribedChannels map[string]bool    // 用户已订阅的频道集合
}

type ConnManager struct {
    mu    sync.RWMutex
    conns map[string]*ConnEntry           // user_id → ConnEntry
}
```

### 3.2 操作

| 操作 | 方法 | 说明 |
|------|------|------|
| 注册 | `Register(userID, conn)` | 写锁，覆盖已有连接 |
| 注销 | `Unregister(userID)` | 写锁，关闭 WebSocket 并删除 |
| 查询 | `Get(userID)` | 读锁，返回 ConnEntry |
| 全量 | `GetAll()` | 读锁，返回浅拷贝 |
| 计数 | `Count()` | 读锁 |
| 订阅频道 | `AddSubscribedChannel(userID, channelID)` | 写锁 |
| 取消订阅 | `RemoveSubscribedChannel(userID, channelID)` | 写锁 |

线程安全：`sync.RWMutex` 保护。读多写少场景，RWMutex 比 Mutex 更优。

---

## 4. Redis 注册表 (Registry)

### 4.1 数据模型

```
Key:    ws:uid:{user_id}
Value:  {gw_id}          // 例如 "ws-gateway-1"
TTL:    5 分钟            // 心跳刷新
```

### 4.2 操作

| 操作 | Redis 命令 | 说明 |
|------|-----------|------|
| 注册连接 | `SET ws:uid:{uid} {gw_id} EX 300` | 5 分钟 TTL |
| 注销连接 | `DEL ws:uid:{uid}` | |
| 查询单个 | `GET ws:uid:{uid}` | 返回 gw_id 或 Nil |
| 批量查询 | `MGET ws:uid:{uid1} ws:uid:{uid2} ...` | 用于扇出时查在线成员 |

### 4.3 用途

其他服务（User Svc、Community Svc、Notify Svc）通过 Redis 注册表判断：
1. 用户是否在线（key 是否存在）
2. 用户在哪台 GW 实例上（value = gw_id）
3. 向哪台 GW 发 NATS 推送消息（`gw.push.{gw_id}`）

---

## 5. 消息路由 (Router)

### 5.1 路由规则

| ClientMessageType | 目标服务 | RPC | 说明 |
|-------------------|----------|-----|------|
| `SEND_DM` | User Service | `SendDM(sender_id, receiver_id, content)` | 私聊消息 |
| `SEND_CHANNEL_MESSAGE` | Community Service | `SendMessage(sender_id, channel_id, content)` | 频道消息 |
| `SUBSCRIBE_CHANNEL` | 本地 ConnManager | `AddSubscribedChannel` | 订阅频道事件 |
| `UNSUBSCRIBE_CHANNEL` | 本地 ConnManager | `RemoveSubscribedChannel` | 取消订阅 |
| `HEARTBEAT` | 本地 HeartbeatHandler | 返回 HEARTBEAT_ACK | 心跳 |

### 5.2 路由流程

```
readPump 收到 ClientMessage
  → 如果是 HEARTBEAT → HeartbeatHandler 处理 + 刷新 deadline + 刷新 Redis TTL
  → 否则 → Router.Route(ctx, userID, msg)
    → 根据 msg.Type 分发到对应 handler
    → handler 调用 Connect-RPC 或本地操作
    → 成功 → 返回 ACK ServerEvent
    → 失败 → 返回 ERROR ServerEvent (附带 request_id)
```

### 5.3 ACK 响应

所有成功处理的消息返回 `SERVER_EVENT_TYPE_ACK`，携带原始 `request_id`：

```protobuf
ServerEvent {
  type = ACK
  request_id = "客户端原始 request_id"
}
```

### 5.4 错误响应

路由失败返回 `SERVER_EVENT_TYPE_ERROR`：

```protobuf
ServerEvent {
  type = ERROR
  request_id = "客户端原始 request_id"
  error_event = { code: "ROUTE_ERROR", message: "描述" }
}
```

---

## 6. NATS 推送 (PushSubscriber)

### 6.1 订阅机制

每台 WS Gateway 实例启动时订阅自己的 NATS topic：

```
Subject: gw.push.{gw_id}
例如: gw.push.ws-gateway-1
```

其他服务通过 Redis 查到目标用户的 gw_id 后，Publish 到对应的 topic。

### 6.2 推送载荷格式

```json
{
  "targets": ["user_id_1", "user_id_2"],
  "event_type": "DM_RECEIVED | CHANNEL_MESSAGE_RECEIVED | USER_ONLINE | USER_OFFLINE",
  "payload": {
    "message_id": "...",
    "sender_id": "...",
    "sender_nickname": "...",
    "content": "...",
    "created_at": 1234567890
  }
}
```

载荷使用 JSON（不是 Protobuf），便于各服务构造。

### 6.3 支持的事件类型

| event_type | ServerEvent Type | Payload 字段 |
|------------|-----------------|-------------|
| `DM_RECEIVED` | `SERVER_EVENT_TYPE_DM_RECEIVED` | message_id, sender_id, sender_nickname, content, created_at |
| `CHANNEL_MESSAGE_RECEIVED` | `SERVER_EVENT_TYPE_CHANNEL_MESSAGE_RECEIVED` | message_id, channel_id, sender_id, sender_nickname, content, created_at |
| `USER_ONLINE` | `SERVER_EVENT_TYPE_USER_ONLINE` | user_id |
| `USER_OFFLINE` | `SERVER_EVENT_TYPE_USER_OFFLINE` | user_id |

### 6.4 投递流程

```
NATS 消息到达 → handleNATSMessage
  → JSON 解析为 PushPayload
  → buildServerEvent: 根据 event_type 构造对应 Protobuf ServerEvent
  → DeliverToLocal:
    → 遍历 targets
    → ConnManager.Get(targetUserID) 查本地连接
    → 找到 → WriteMessage 写入 WebSocket
```

### 6.5 扇出

同一台 GW 上的多个目标用户合并在一条 NATS 消息的 `targets` 数组中。例如：

```json
{
  "targets": ["user_b", "user_e"],
  "event_type": "CHANNEL_MESSAGE_RECEIVED",
  "payload": { ... }
}
```

GW 收到后遍历 targets，只投递给本地有连接的用户。不在本地的 target 被静默跳过。

---

## 7. 心跳机制 (HeartbeatHandler)

### 7.1 参数

| 参数 | 值 | 说明 |
|------|-----|------|
| 心跳间隔 | 30 秒 | 客户端应每 30 秒发送 HEARTBEAT |
| 读超时 | interval × 1 | 每次收到消息/心跳后重置 `SetReadDeadline` |
| 注册表 TTL | 5 分钟 | 每次心跳刷新 Redis TTL |

### 7.2 流程

```
Client → HEARTBEAT {request_id: "hb-123"}
  → IsHeartbeatMessage: 检查 type == CLIENT_MESSAGE_TYPE_HEARTBEAT
  → HandleHeartbeat: 构造 HEARTBEAT_ACK
  → WriteMessage: 发送 HEARTBEAT_ACK
  → ResetDeadline: conn.SetReadDeadline(now + interval)
  → Registry.RegisterConnection: 刷新 Redis TTL
```

### 7.3 超时断开

如果超过心跳间隔没有收到任何消息（包括心跳），WebSocket 读操作超时 → readPump 退出 → cleanupDisconnect 触发断开清理。

---

## 8. 二进制协议 (Protocol)

### 8.1 帧格式

```
[4 bytes: big-endian payload length][protobuf payload]
```

所有 WebSocket 消息使用 `BinaryMessage` 类型（不是 TextMessage）。

### 8.2 编解码

| 方向 | Protobuf 类型 | 函数 |
|------|-------------|------|
| Client → Server | `ClientMessage` | `EncodeClientFrame` / `DecodeClientFrame` |
| Server → Client | `ServerEvent` | `EncodeFrame` / `DecodeFrame` |

### 8.3 读写封装

```go
func WriteMessage(conn *websocket.Conn, msg *ServerEvent) error   // 编码 + 写入
func ReadMessage(conn *websocket.Conn) (*ClientMessage, error)    // 读取 + 解码
```

---

## 9. 客户端 Proto 协议

### 9.1 客户端消息类型

```protobuf
enum ClientMessageType {
  CLIENT_MESSAGE_TYPE_UNSPECIFIED = 0;
  CLIENT_MESSAGE_TYPE_SEND_DM = 1;
  CLIENT_MESSAGE_TYPE_SEND_CHANNEL_MESSAGE = 2;
  CLIENT_MESSAGE_TYPE_SUBSCRIBE_CHANNEL = 3;
  CLIENT_MESSAGE_TYPE_UNSUBSCRIBE_CHANNEL = 4;
  CLIENT_MESSAGE_TYPE_HEARTBEAT = 5;
}
```

### 9.2 客户端消息结构

```protobuf
message ClientMessage {
  ClientMessageType type = 1;
  string request_id = 2;                // 用于匹配 ACK 响应
  SendDMRequest send_dm_request = 10;
  SendChannelMessageRequest send_channel_message_request = 11;
  SubscribeChannelRequest subscribe_channel_request = 12;
  UnsubscribeChannelRequest unsubscribe_channel_request = 13;
}
```

### 9.3 服务端事件类型

```protobuf
enum ServerEventType {
  SERVER_EVENT_TYPE_UNSPECIFIED = 0;
  SERVER_EVENT_TYPE_DM_RECEIVED = 1;
  SERVER_EVENT_TYPE_CHANNEL_MESSAGE_RECEIVED = 2;
  SERVER_EVENT_TYPE_USER_ONLINE = 3;
  SERVER_EVENT_TYPE_USER_OFFLINE = 4;
  SERVER_EVENT_TYPE_ERROR = 5;
  SERVER_EVENT_TYPE_HEARTBEAT_ACK = 6;
  SERVER_EVENT_TYPE_ACK = 7;
}
```

### 9.4 服务端事件结构

```protobuf
message ServerEvent {
  ServerEventType type = 1;
  string request_id = 2;                // ACK/ERROR 时回显客户端 request_id
  DMReceivedEvent dm_received_event = 10;
  ChannelMessageReceivedEvent channel_message_event = 11;
  UserOnlineEvent user_online_event = 12;
  UserOfflineEvent user_offline_event = 13;
  ErrorEvent error_event = 14;
}
```

---

## 10. 多实例部署

### 10.1 Docker Compose 配置

两台 WS Gateway 实例，不同 `GATEWAY_ID`：

```yaml
ws-gateway-1:
  environment:
    GATEWAY_ID: "ws-gateway-1"
    LISTEN_ADDR: ":8081"
  ports: ["8081:8081"]

ws-gateway-2:
  environment:
    GATEWAY_ID: "ws-gateway-2"
    LISTEN_ADDR: ":8081"
  ports: ["8082:8081"]
```

### 10.2 跨实例消息投递

```
用户 A 连接 ws-gateway-1，用户 B 连接 ws-gateway-2

A 发频道消息:
  1. ws-gateway-1 路由 → Community Svc
  2. Community Svc 处理消息
  3. Community Svc 查 Redis MGET ws:uid:{members} → 获取各成员 gw_id
  4. 按 gw_id 分组
  5. NATS Publish gw.push.ws-gateway-1 {targets: [A, ...]}
  6. NATS Publish gw.push.ws-gateway-2 {targets: [B, ...]}
  7. 各 GW 收到后只投递本地连接的用户
```

### 10.3 负载均衡

当前使用 L4 层随机分配（客户端随机连某一台 GW 实例）。未来可改为 UID-based 一致性哈希，减少实例增减时的连接迁移。

---

## 11. NATS 事件

### 11.1 WS Gateway 发布的事件

| Subject | 触发时机 | 载荷 |
|---------|----------|------|
| `constell.user.online` | 用户连接建立 | `{"user_id": "...", "gw_id": "..."}` |
| `constell.user.offline` | 用户断开连接 | `{"user_id": "..."}` |

### 11.2 WS Gateway 订阅的 Subject

| Subject | 说明 |
|---------|------|
| `gw.push.{gw_id}` | 该实例专属的推送 topic |

---

## 12. 服务配置

### 12.1 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `GATEWAY_ID` | `gw-001` | 实例唯一标识，用于 Redis 注册和 NATS 订阅 |
| `JWT_SECRET` | `constell-dev-secret` | JWT 签名密钥 |
| `LISTEN_ADDR` | `:8081` | 监听地址 |
| `REDIS_ADDR` | `localhost:6379` | Redis 地址 |
| `NATS_URL` | `nats://localhost:4222` | NATS 地址 |
| `USER_SERVICE_ADDR` | `http://localhost:9082` | User Service 地址 |
| `COMMUNITY_SERVICE_ADDR` | `http://localhost:9083` | Community Service 地址 |
| `REGISTRY_TYPE` | `static` | 服务发现类型 |
| `SERVICES_CONFIG_PATH` | `deploy/configs/services.yaml` | 静态服务配置路径 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://localhost:5080/api/default/v1/otlp` | OTel 端点 |

### 12.2 HTTP 路由

| 路径 | 方法 | 说明 |
|------|------|------|
| `/ws` | GET | WebSocket 升级入口 |
| `/healthz` | GET | 存活检查（依赖 Redis + NATS） |
| `/readyz` | GET | 就绪检查 |

---

## 13. 文件结构

```
backend/services/ws-gateway/
├── main.go           入口：配置加载、基础设施连接、组件组装、HTTP 路由、优雅关闭
├── server.go         Server 结构体：HandleUpgrade、readPump、cleanupDisconnect、广播
├── auth.go           Authenticator：JWT 认证
├── connmgr.go        ConnManager：本地连接管理
├── registry.go       Registry：Redis uid→gw_id 注册表
├── router.go         Router：消息路由（DM/频道/订阅）
├── push.go           PushSubscriber：NATS 推送订阅 + 本地投递
├── heartbeat.go      HeartbeatHandler：心跳检测 + deadline 管理
├── protocol.go       二进制帧编解码（length-prefixed protobuf）
├── Dockerfile        Docker 构建
├── go.mod / go.sum   Go 模块
└── *_test.go         各组件测试
```

---

## 14. 后续演进

- **多设备支持**：ConnManager 改为 `user_id → []*ConnEntry`，同一用户可多个连接
- **UID-based 负载均衡**：L4 层一致性哈希，减少实例增减时连接迁移
- **断线重连 + 消息补发**：客户端重连后拉取离线期间的消息（依赖 Notify Service）
- **频道订阅优化**：当前订阅存在本地内存，重连需重新订阅。可持久化到 Redis
- **连接限流**：单 GW 最大连接数限制，防止过载
- **WebSocket 压缩**：启用 permessage-deflate 减少带宽
