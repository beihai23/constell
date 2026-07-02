# Constell

> 一个开源的、以社群为核心的实时消息平台 —— 可自托管、微服务架构，体验对标 Discord。

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg)](https://go.dev)
[![Status](https://img.shields.io/badge/status-alpha-orange.svg)](docs/PROJECT_STATUS.md)
[![Protocol](https://img.shields.io/badge/RPC-Connect--RPC-blue.svg)](https://connect.build)

Constell 是一个社群型 IM 系统：用户可以创建社区（servers），每个社区下有文本频道和私信，并进行实时聊天。后端是一组 Go 微服务，服务间通过 **Connect-RPC** 与 **NATS JetStream** 通信；前端是基于 React + Vite 的单页应用，通过一个有状态的 WebSocket 网关获取实时消息推送。

---

## 功能特性

- **社区与频道** —— 创建社区、文本频道、角色，以及细粒度权限控制。
- **实时消息** —— 基于 WebSocket 的频道消息和私信，带连接注册表，网关可多实例水平扩展。
- **消息生命周期** —— 编辑、删除，配合乐观 UI 与回滚对账。
- **未读状态** —— 按频道 / 按私信的未读计数，落库 Postgres，锚定到单调递增的消息 seq（reload 不漂移）。
- **文件附件** —— 基于 MinIO（S3 兼容）的上传 / 下载 / 缩略图 / 分块传输。
- **全文搜索** —— 基于 `tsvector` 的社区级与消息级搜索。
- **社区发现** —— 公开社区目录，可搜索。
- **认证** —— 注册、登录、JWT access/refresh token。
- **可观测性** —— OpenTelemetry 链路追踪 + OpenObserve 后端。

## 架构

```
                        ┌──────────────┐
      浏览器    ────────►│   Web :3000  │  React + Vite（nginx）
     (@constell/sdk-js) └──────┬───────┘
                        REST   │   WebSocket
              ┌────────────────┴────────────────┐
              ▼                                   ▼
     ┌─────────────────┐                ┌──────────────────┐
     │  API Gateway    │                │   WS Gateway     │
     │     :8080       │                │  :8081 / :8082   │  （有状态，可多实例）
     └────────┬────────┘                └────────┬─────────┘
       Connect-RPC              NATS JetStream（扇出）· Redis（presence）
              │                            │
   ┌──────────┼──────────┬────────┬────────┴────────┬──────────┐
   ▼          ▼          ▼        ▼                 ▼          ▼
 auth      user     community    file             search     notify
 :9081     :9082     :9083       :9084            :9085      :9086
   │          │          │         │                 │          │
   └──────────┴──────────┴─────────┴─────────────────┴──────────┘
                              │
            PostgreSQL :15432 · MinIO :9000 · OpenObserve :5080
```

| 服务 | 端口 | 职责 | 状态 |
|---------|------|------|------|
| `api-gateway` | 8080 | 无状态 REST 入口，REST → Connect-RPC 转换 | — |
| `ws-gateway` | 8081（+8082…） | 有状态 WebSocket 网关；连接表、Redis presence、NATS 订阅 | 连接 |
| `auth-service` | 9081 | 注册 / 登录 / token 刷新 | 无状态 |
| `user-service` | 9082 | 用户资料、私信、关系 | groupcache |
| `community-service` | 9083 | 社区、频道、角色、权限、消息 | groupcache |
| `file-service` | 9084 | 上传 / 下载 / 缩略图 / 分块 | MinIO |
| `search-service` | 9085 | 全文搜索（`tsvector`） | — |
| `notify-service` | 9086 | 未读 read-state（Postgres）+ NATS 推送 | Postgres |

**关键设计决策**

- 全部服务间调用使用 **Connect-RPC**（而非 gRPC-gateway）。生成代码位于 `backend/pkg/proto/`。
- **自研 groupcache**（`backend/pkg/groupcache/`）—— 一致性哈希分片、peer-to-peer 填充、singleflight 去重、自动故障转移。供有状态缓存使用。
- **WS Gateway 是唯一的有状态网关** —— 持有连接表、在 Redis 注册 presence、订阅 NATS 推送主题。多实例可水平扩展。
- **NATS JetStream** 承载异步事件（新消息扇出、通知）；**Redis** 仅用于 WS presence 注册表。
- **未读计数** 计算方式为 `count(messages WHERE seq > last_read_seq)`，mark-read 时 upsert `GREATEST(existing, max(seq))` —— 原子、幂等、始终可对账。

## 技术栈

| 层级 | 技术 |
|-------|-----------|
| 语言 | Go 1.25（workspace）、TypeScript |
| RPC | Connect-RPC + Buf（protobuf） |
| 存储 | PostgreSQL 16 |
| 缓存 / presence | Redis 7 |
| 消息队列 | NATS 2（JetStream） |
| 对象存储 | MinIO（S3 兼容） |
| 可观测性 | OpenTelemetry、OpenObserve |
| Web 客户端 | React、Vite、Tailwind |
| 实时 SDK | `@constell/sdk-js` |
| E2E 测试 | Playwright |

## 快速开始

最快的方式是用 Docker 全量启动（基础设施 + 全部服务 + Web）：

```bash
git clone https://github.com/beihai23/constell.git
cd constell

make docker-up        # 构建并启动全部（postgres、redis、nats、minio、openobserve、
                      # 全部 8 个服务，以及 Web 客户端）
make migrate-up       # 执行数据库迁移
```

然后打开 **<http://localhost:3000>** 注册账号即可。

> 基础设施端口已重映射以避免宿主机冲突：Postgres `15432`、Redis `16379`、
> NATS `4222`、MinIO `9000`（控制台 `9001`）、OpenObserve `5080`。

常用命令：

```bash
make docker-down      # 停止全量栈
make docker-build     # 显式（重新）构建全部镜像
make infra-up         # 仅启动基础设施（postgres/redis/nats/minio/openobserve）
                       # —— 本地开发运行各服务时使用
docker compose -f deploy/docker/docker-compose.yml logs -f   # 查看日志
```

## 开发

### 常用命令

```bash
make proto-gen        # 通过 buf 从 proto/ 重新生成 Go 代码
make lint             # lint proto 文件
make build            # 构建全部服务 + 共享包
make build/ws-gateway # 构建单个二进制到 bin/

make migrate-up       # 执行迁移（deploy/migrations/）
make migrate-down     # 回滚最近一次迁移

make test             # Go 单元测试（short）
make test/all         # Go 测试，verbose，-count=1
make test/ws-gateway  # ws-gateway 测试，verbose
make test/integration # 跨服务集成测试
```

### 本地运行单个服务

用 `make infra-up` 起好依赖后，即可按 dev 配置本地运行任意服务：

```bash
make run/auth-service
make run/user-service
make run/community-service
make run/file-service
make run/search-service
make run/notify-service
make run/api-gateway
make run/ws-gateway
```

运行单个 Go 测试：

```bash
cd backend && go test -v -run TestFuncName ./services/ws-gateway/...
```

### Web 客户端与 SDK

```bash
cd clients/web && npm install && npm run dev     # Vite dev server
cd clients/web && npm run test:e2e               # Playwright e2e 套件
cd sdk/sdk-js   && npm install && npm test        # SDK 单元测试
```

## 项目结构

```
proto/                          Protobuf 定义（Buf module 根）
  auth/v1 · user/v1 · community/v1 · gateway/v1 · common/v1
  notify/v1 · file/v1 · search/v1
backend/
  go.work                       Go workspace —— 服务 + pkg + tools + tests
  pkg/                          共享包（单个 Go module）
    proto/                      生成的 protobuf + Connect-RPC 代码（勿手改）
    groupcache/                 LRU 缓存：一致性哈希 + singleflight
    jwt/ · middleware/ · nats/ · postgres/ · redis/
  services/
    api-gateway/(:8080)         REST 入口 → Connect-RPC
    ws-gateway/(:8081)          有状态 WebSocket 网关
    auth-service/(:9081)
    user-service/(:9082)        groupcache
    community-service/(:9083)   groupcache
    file-service/(:9084)
    search-service/(:9085)
    notify-service/(:9086)
  tools/migrate/                迁移执行器
  tests/integration/            跨服务集成测试
clients/web/                    React + Vite Web 客户端（Playwright e2e）
sdk/sdk-js/                     @constell/sdk-js —— WS + REST 客户端 SDK
deploy/
  configs/dev.yaml              Dev 配置（DB、Redis、NATS、MinIO、服务地址）
  configs/services.yaml         服务发现 / 地址
  docker/docker-compose.yml     本地全量栈
  migrations/                   SQL 迁移（001–015）
docs/                           架构 spec、计划、项目状态
```

## 数据库

PostgreSQL，迁移为成对的 up/down SQL 文件，位于 `deploy/migrations/`（目前到 `015_read_state`）。迁移执行器是 `backend/tools/migrate`。

```bash
make migrate-up      # 执行
make migrate-down    # 回滚
```

Dev 连接串：`postgres://constell:constell_dev@localhost:15432/constell`
（Docker 容器内端口为 `5432`）。

## 配置

所有服务从 **环境变量** 读取配置，默认值来源于 `deploy/configs/dev.yaml`
（服务发现用 `services.yaml`）。本地运行或在容器中运行时，可通过环境变量覆盖任意值。

## 状态与路线图

Constell 正在积极开发中（alpha）。六个规划阶段已完成五个（基础设施、服务治理、
WS 网关、File/Search/Notify、Web 客户端）；SDK 阶段为下一步。完整进度见
[`docs/PROJECT_STATUS.md`](docs/PROJECT_STATUS.md)，架构 spec 与各阶段计划见
`docs/superpowers/`。

## License

[MIT](LICENSE) © 2026 beihai23
