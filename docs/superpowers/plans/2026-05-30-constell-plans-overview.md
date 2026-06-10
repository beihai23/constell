# Constell — 实施计划总览

基于架构设计规格 `docs/superpowers/specs/2026-05-29-constell-architecture-design.md`，拆分为 6 个阶段计划。

每个计划产出可独立运行、测试、演示的软件。后续计划依赖前序计划的产出。

---

## Plan 1: 基础设施 + 核心服务 + API Gateway

**目标：** 通过 REST API 完成注册、登录、创建 Server/Channel、发送 DM 和群消息。

**依赖：** 无（全新项目）

**涉及服务：** Auth Svc, User Svc, Community Svc, API Gateway

**产出物：**
- Proto 定义 (Buf 管理)
- Go Workspace 骨架 + 共享库 (jwt, redis, postgres, nats, groupcache, middleware)
- Docker Compose 开发环境 (PG, Redis, NATS)
- DB migrations (users, dm_messages, servers, channels, members, roles, channel_messages)
- Auth Service: 邮箱注册/登录, JWT 签发
- User Service: profile CRUD, 好友/黑名单, DM 收发 (有状态 groupcache)
- Community Service: Server/Channel CRUD, 成员管理, 角色权限, 群消息收发 (有状态 groupcache)
- API Gateway: REST → Connect-RPC 路由, JWT 鉴权中间件
- 集成测试: 完整的用户注册→登录→创建Server→发消息→读历史链路

**验证方式：** `curl` / Postman 调用 REST API 完成所有操作

**预估 Tasks：** 25-35 个

---

## Plan 2: 服务治理

**目标：** 统一的服务发现、配置管理、健康检查和可观测性。

**前置：** Plan 1

**涉及包：** pkg/registry, pkg/config, pkg/health, pkg/otel, pkg/logging, pkg/metrics

**产出物：**
- Registry 接口 + StaticRegistry（docker-compose）+ K8sRegistry（build-tagged）
- 统一配置加载器（环境变量 > yaml > defaults）
- 健康检查端点（healthz / readyz）
- OTel 可观测性（slog + Prometheus metrics + 分布式追踪 → OpenObserve）
- services.yaml 服务实例配置
- docker-compose 更新（healthcheck + openobserve + OTel）
- 所有现有服务迁移到治理包

**验证方式：** Docker Compose 启动所有服务，OpenObserve UI 可查看日志/指标/追踪

**预估 Tasks：** 16 个

---

## Plan 3: WS Gateway — 实时通信

**目标：** WebSocket 实时消息推送，完整的 IM 体验。

**前置：** Plan 2

**涉及服务：** WS Gateway

**产出物：**
- Protobuf WebSocket 协议定义 (gateway/v1)
- WS Gateway: WebSocket 升级, 连接认证, 心跳保活
- 连接状态管理: 本地 conn map + Redis uid→gw_id 注册表
- 消息路由: 根据 Protobuf 消息类型分流 DM→User Svc / 频道→Community Svc
- NATS 推送: 订阅 gw.push.* topics, 按本地 conn map 推送
- 扇出: 支持 WS GW 多实例, NATS 精确投递到目标实例
- 重连机制: 客户端断线重连 + 消息补发
- Docker Compose 中 WS Gateway × 2 实例, 验证多实例扇出

**验证方式：** 两个浏览器 WebSocket 客户端实时收发消息

**预估 Tasks：** 15-20 个

---

## Plan 4: File Service + Search Service + Notify Service

**目标：** 文件上传/下载、全文搜索、推送通知和未读计数。

**前置：** Plan 3

**涉及服务：** File Svc, Search Svc, Notify Svc

**产出物：**
- File Service: 文件上传/下载 (S3/MinIO), 图片缩略图生成
- 消息附件: Community Svc / User Svc 的消息支持附件关联
- Search Service: 统一搜索入口, 用户搜索 (→User Svc PG tsvector), 消息搜索 (→Community Svc PG tsvector), 频道搜索
- Notify Service: Web Push 推送, 未读计数管理 (Redis INCR), 离线消息聚合
- Docker Compose 中加入 MinIO

**验证方式：** 上传图片发送消息, 搜索关键字返回结果, 离线用户收到推送

**预估 Tasks：** 15-20 个

---

## Plan 5: Web 客户端

**目标：** 可用的 Web IM 应用。

**前置：** Plan 4

**涉及服务：** Web 前端, SDK-JS

**产出物：**
- SDK-JS: WebSocket 连接管理, Protobuf 序列化, 重连, 消息队列
- React 应用: 登录/注册页, Server/Channel 侧边栏, 消息列表, DM 视图
- 状态管理: 用户/Server/Channel/消息状态
- 文件上传 UI, 搜索 UI, 未读标记
- Docker Compose 中加入 Nginx 反代前端

**验证方式：** 浏览器打开完整 IM 应用, 两个用户实时聊天

**预估 Tasks：** 20-25 个

---

## Plan 6: SDK (Go + KMP)

**目标：** 提供 Go SDK 和 Kotlin Multiplatform SDK，支持 Bot 和移动端接入。

**前置：** Plan 5

**涉及服务：** SDK-Go, SDK-KMP

**产出物：**
- SDK-Go: gRPC/Connect-RPC 客户端, Bot 框架示例
- SDK-KMP: Kotlin Multiplatform (Android + iOS), WebSocket + Protobuf
- Android 示例应用壳

**验证方式：** Go Bot 发消息, Android 登录收消息

**预估 Tasks：** 15-20 个

---

## 执行顺序与依赖关系

```
Plan 1 (基础设施+核心)
  │
  ▼
Plan 2 (服务治理)
  │
  ▼
Plan 3 (WS Gateway)
  │
  ▼
Plan 4 (File+Search+Notify)
  │
  ▼
Plan 5 (Web 客户端)
  │
  ▼
Plan 6 (SDK)
```

每个 Plan 完成后：
1. 确认所有测试通过
2. Docker Compose 一键运行验证
3. 提交并打 tag (e.g. `v0.1.0`, `v0.2.0`, ...)
4. 再制定下一个 Plan 的详细任务
