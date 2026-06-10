# Constell — 服务治理设计

## 定位

本设计为 Constell 新增一个治理阶段（Plan 2），作为原 Plan 2（WS Gateway）的前置依赖。原 Plan 2-5 顺延为 Plan 3-6。

治理阶段的目标：为所有微服务提供统一的服务发现、配置管理、健康检查和可观测性基础设施，同时支持 docker-compose 单机部署和 K8s 集群部署。

## 设计原则

1. **抽象层 + 双实现**：定义 Go 接口屏蔽部署差异，docker-compose 和 K8s 各提供一套实现
2. **共享库，不引入独立服务**：所有治理能力封装在 `backend/pkg/` 下的共享包中，各服务 import 使用
3. **渐进增强**：docker-compose 下提供基础功能（静态发现、手动配置），K8s 下利用平台能力获得动态特性
4. **弹性能力后置**：熔断、重试、限流不纳入本阶段，待有实际流量后按需添加

## 模块概览

| 模块 | 包路径 | 职责 |
|------|--------|------|
| 服务注册发现 | `pkg/registry/` | 服务实例注册、发现、变更通知 |
| 配置管理 | `pkg/config/` | 统一配置加载，多层级配置源 |
| 健康检查 | `pkg/health/` | healthz / readyz 端点 |
| 可观测性 | `pkg/otel/` | OTel SDK 初始化（日志 + 指标 + 追踪 → OpenObserve） |
| 可观测性-日志 | `pkg/logging/` | slog 封装，统一格式和上下文传递 |
| 部署配置 | `deploy/` | services.yaml + docker-compose 更新 |

---

## 1. 服务注册与发现

### 接口定义

```go
// pkg/registry/registry.go

// Instance 表示一个服务实例
type Instance struct {
    ServiceName string // e.g. "user-service"
    Addr        string // e.g. "user-service-1:9082"
    Metadata    map[string]string // 可选: version, zone 等
}

// Registry 服务注册与发现接口
type Registry interface {
    // Register 注册当前实例
    Register(ctx context.Context, inst Instance) error
    // Deregister 注销当前实例
    Deregister(ctx context.Context) error
    // Discover 发现指定服务的所有实例
    Discover(ctx context.Context, serviceName string) ([]Instance, error)
    // Watch 监听指定服务的实例列表变化，返回变更通道
    // 用于 groupcache peer 列表动态更新
    Watch(ctx context.Context, serviceName string) (<-chan []Instance, error)
}
```

### 实现：StaticRegistry（docker-compose）

docker-compose 下实例列表固定在 YAML 配置文件中，启动时一次性加载，运行期间不变。

```go
// pkg/registry/static.go

type StaticRegistry struct {
    services map[string][]Instance // serviceName → instances
    self     Instance
}

// 从配置文件加载:
// Discover: 直接返回内存中的实例列表
// Watch: 返回一个通道，启动时写入一次后不再变更
// Register/Deregister: 无操作（实例已在配置中定义）
```

**配置格式：**

```yaml
# deploy/configs/services.yaml
services:
  api-gateway:
    instances:
      - addr: "api-gateway:8080"

  ws-gateway:
    instances:
      - addr: "ws-gateway-1:8081"
      - addr: "ws-gateway-2:8081"

  auth-service:
    instances:
      - addr: "auth-service:9081"

  user-service:
    instances:
      - addr: "user-service-1:9082"
      - addr: "user-service-2:9082"

  community-service:
    instances:
      - addr: "community-service-1:9083"
      - addr: "community-service-2:9083"
```

扩缩容流程：修改 `services.yaml` → `docker-compose up -d` → 新实例启动时读取最新配置。

### 实现：K8sRegistry（Kubernetes）

K8s 下利用平台 API 实现动态服务发现。

```go
// pkg/registry/k8s.go

type K8sRegistry struct {
    clientset *kubernetes.Clientset
    namespace string
    self      Instance
}

// Register: 无操作（K8s Pod 自动注册到 Service）
// Deregister: 无操作（Pod 终止时自动从 Endpoints 移除）
// Discover: 查询 K8s Endpoints 资源
// Watch: 通过 K8s Informer / Watch API 监听 Endpoints 变更，实时推送到通道
```

**依赖：** `k8s.io/client-go`（仅在 K8s 环境下编译/初始化）

**groupcache peer 更新流程（K8s）：**
1. K8sRegistry.Watch("user-service") 返回变更通道
2. user-service 启动时订阅 Watch
3. K8s 扩缩容 → Endpoints 变更 → Watch 通道收到新的实例列表
4. groupcache 更新一致性哈希环的 peer 列表

### 运行时选择

通过环境变量 `REGISTRY_TYPE` 选择实现：

| 环境变量值 | 使用实现 | 场景 |
|-----------|---------|------|
| `static`（默认） | StaticRegistry | docker-compose / 本地开发 |
| `k8s` | K8sRegistry | K8s 集群 |

```go
// pkg/registry/factory.go

func NewRegistry(ctx context.Context, cfg Config) (Registry, error) {
    switch cfg.Type {
    case "k8s":
        return NewK8sRegistry(cfg.K8s)
    default:
        return NewStaticRegistry(cfg.Static)
    }
}
```

### 对 groupcache 的改造

现有 `pkg/groupcache/` 需要集成 Registry：

```
当前: 启动时传入固定的 peer 列表 []string
改为: 启动时传入 Registry 实例，通过 Watch 动态更新 peer 列表

groupcache 初始化:
  1. 调用 Registry.Discover("user-service") 获取初始 peer 列表
  2. 调用 Registry.Watch("user-service") 订阅变更
  3. 收到变更时更新一致性哈希环
  4. StaticRegistry: 只收到一次，之后不变
  5. K8sRegistry: 实时接收扩缩容事件
```

### 对 WS Gateway 的改造

WS Gateway 已经使用 Redis 存储 `uid → gw_id` 映射。Registry 主要影响：

- groupcache peer 列表（如果有本地缓存）
- 下游服务地址发现（User Svc、Community Svc 的 Connect-RPC 客户端）

当前 Connect-RPC 客户端使用固定地址：
```go
userSvc := userv1connect.NewUserServiceClient(http.DefaultClient, "http://localhost:9082")
```

改为通过 Registry 获取地址后创建客户端：
```go
instances, _ := registry.Discover(ctx, "user-service")
// 创建连接池或使用负载均衡策略选择实例
```

**注意：** Connect-RPC 客户端本身不支持自动切换地址。本阶段采用简单策略：启动时从 Registry 获取地址列表，创建多个客户端实例，轮询选择。动态负载均衡和自动 failover 留给后续弹性能力阶段。

**有状态服务的路由特殊性：** 上述轮询策略只适用于无状态调用（如 API Gateway → Auth Service）。有状态服务（User Service、Community Service）的请求路由由 groupcache 内部处理——groupcache 根据一致性哈希确定 owning node，通过 peer fill 机制请求目标实例。因此有状态服务的 Connect-RPC 客户端地址选择需要与 groupcache peer 列表保持一致。

---

## 2. 配置管理

### 现状问题

各服务在 `main.go` 中通过 `os.Getenv("KEY")` 读取配置，散落在各处，没有统一管理。

### 改进方案

#### 配置加载器

```go
// pkg/config/config.go

// Loader 配置加载器
type Loader struct {
    env         string    // dev, staging, prod
    configDir   string    // 配置文件目录
    serviceName string    // 当前服务名
}

// Load 加载配置到目标结构体
// 优先级: 环境变量 > {env}.yaml > defaults tag
func (l *Loader) Load(target interface{}) error
```

#### 配置层级

```
优先级（高→低）：
1. 环境变量          — docker-compose env / K8s Container env
2. 环境特定配置文件   — configs/{env}.yaml
3. 代码默认值         — struct tag `default:"value"`
```

#### 配置结构

每个服务定义自己的配置结构体：

```go
// 以 user-service 为例
type Config struct {
    // 基础
    Env      string `default:"dev"`
    HTTPAddr string `default:":9082" env:"HTTP_ADDR"`

    // 基础设施
    PostgresDSN string `default:"postgres://constell:constell_dev@localhost:5432/constell" env:"POSTGRES_DSN"`
    RedisAddr   string `default:"localhost:6379" env:"REDIS_ADDR"`
    NATSURL     string `default:"nats://localhost:4222" env:"NATS_URL"`

    // 服务发现
    RegistryType string `default:"static" env:"REGISTRY_TYPE"`

    // OTel
    OTelEndpoint string `default:"http://localhost:5080/api/default/v1/otlp" env:"OTEL_EXPORTER_OTLP_ENDPOINT"`

    // groupcache（有状态服务特有）
    CachePeers []string `env:"CACHE_PEERS"` // 仅 static 模式需要，K8s 模式由 Registry 动态获取
}
```

#### 配置文件结构

```
deploy/configs/
├── dev.yaml          # 开发环境配置（默认值）
├── staging.yaml      # 预发布环境（未来）
├── prod.yaml         # 生产环境（未来）
└── services.yaml     # 服务发现实例列表（StaticRegistry 使用）
```

`dev.yaml` 保留当前的配置格式，`services.yaml` 专用于服务发现。

#### 本阶段不做

- 配置热更新（改配置需重启服务）
- 配置版本管理（配置中心）
- K8s ConfigMap watch

---

## 3. 健康检查

### 端点定义

每个服务暴露两个 HTTP 端点：

| 路径 | 用途 | 检查内容 | 响应 |
|------|------|---------|------|
| `GET /healthz` | 存活探针 (Liveness) | 进程在运行 | `200 OK {"status": "ok"}` |
| `GET /readyz` | 就绪探针 (Readiness) | 依赖服务（PG、Redis、NATS）连接正常 | `200 OK {"status": "ready"}` 或 `503` |

### 实现

```go
// pkg/health/health.go

type Checker struct {
    checks map[string]CheckFunc
}

type CheckFunc func(ctx context.Context) error

// NewChecker 创建健康检查器
func NewChecker() *Checker

// RegisterCheck 注册就绪检查项
// e.g. RegisterCheck("postgres", pgCheck)
//      RegisterCheck("redis", redisCheck)
//      RegisterCheck("nats", natsCheck)
func (c *Checker) RegisterCheck(name string, fn CheckFunc)

// HealthzHandler 存活检查 handler
func (c *Checker) HealthzHandler() http.HandlerFunc

// ReadyHandler 就绪检查 handler（执行所有注册的 CheckFunc）
func (c *Checker) ReadyHandler() http.HandlerFunc
```

### 各服务注册的检查项

| 服务 | 检查项 |
|------|--------|
| 所有服务 | /healthz — 进程存活 |
| Auth Service | postgres |
| User Service | postgres, redis, nats |
| Community Service | postgres, redis, nats |
| WS Gateway | redis, nats |
| API Gateway | (无依赖，readyz 检查 downstream 可达) |

### 集成方式

各服务在 `main.go` 中：

```go
health := health.NewChecker()
health.RegisterCheck("postgres", pgInstance.Ping)
health.RegisterCheck("redis", redisInstance.Ping)
health.RegisterCheck("nats", natsInstance.Ping)

mux := http.NewServeMux()
mux.HandleFunc("/healthz", health.HealthzHandler())
mux.HandleFunc("/readyz", health.ReadyHandler())
// ... 注册 Connect-RPC handlers
```

### docker-compose 集成

```yaml
# docker-compose.yml 中每个服务加:
healthcheck:
  test: ["CMD", "wget", "-q", "--spider", "http://localhost:9082/healthz"]
  interval: 10s
  timeout: 5s
  retries: 3
```

### K8s 集成（预留，本阶段不写 Helm）

```yaml
# K8s 部署时配置（示意）:
livenessProbe:
  httpGet: { path: /healthz, port: 9082 }
  periodSeconds: 10
readinessProbe:
  httpGet: { path: /readyz, port: 9082 }
  periodSeconds: 5
```

---

## 4. 可观测性

### 架构

```
应用服务 (OTel SDK)
    │
    ├─ 日志 (slog → OTel Log Bridge) ──→ OpenObserve
    ├─ 指标 (OTel Metrics SDK) ─────────→ OpenObserve
    └─ 追踪 (OTel Trace SDK) ───────────→ OpenObserve

docker-compose: 1 个 openobserve 容器，监听 OTLP (HTTP :5080)
K8s: OpenObserve 部署为独立服务（或使用外部托管服务）
```

### 包结构

#### pkg/otel/ — OTel 初始化

```go
// pkg/otel/otel.go

type Config struct {
    ServiceName string
    Environment string // dev, staging, prod
    Endpoint    string // OpenObserve OTLP endpoint
    Insecure    bool   // 是否使用明文传输（docker-compose 下用）
}

// Init 初始化 OTel Provider（Trace + Metric + Log）
// 返回 shutdown 函数，供 graceful shutdown 时调用
func Init(ctx context.Context, cfg Config) (shutdown func(ctx context.Context) error, err error)
```

初始化内容：
1. **TracerProvider** — 创建 OTLP Trace Exporter，指向 OpenObserve
2. **MeterProvider** — 创建 OTLP Metric Exporter，指向 OpenObserve
3. **LoggerProvider** — 创建 OTLP Log Exporter，桥接 slog

#### pkg/logging/ — 结构化日志

```go
// pkg/logging/logging.go

// Init 初始化 slog logger
// - JSON 格式输出
// - 注入 service_name, environment, instance_id 等公共字段
// - 桥接到 OTel Log Provider（日志同时输出到 stdout 和 OpenObserve）
func Init(serviceName, env string) *slog.Logger

// FromContext 从 context 中提取关联了 trace_id 的 logger
func FromContext(ctx context.Context) *slog.Logger

// NewContext 创建携带 logger 的 context
func NewContext(ctx context.Context, logger *slog.Logger) context.Context
```

**日志格式示例：**
```json
{
  "time": "2026-06-02T10:30:00.000Z",
  "level": "INFO",
  "msg": "message sent",
  "service": "user-service",
  "env": "dev",
  "trace_id": "abc123",
  "span_id": "def456",
  "channel_id": "42",
  "sender_id": "7"
}
```

#### pkg/metrics/ — 指标中间件

```go
// pkg/metrics/metrics.go

// HTTPMiddleware 记录 HTTP 请求的 QPS / 延迟 / 错误率
func HTTPMiddleware(handler http.Handler) http.Handler

// ConnectRPCInterceptor 记录 Connect-RPC 调用的 QPS / 延迟 / 错误率
func ConnectRPCInterceptor() connect.UnaryInterceptorFunc
```

**核心指标：**

| 指标名 | 类型 | 标签 | 适用服务 |
|--------|------|------|---------|
| `http_request_total` | Counter | method, path, status | 所有 |
| `http_request_duration_seconds` | Histogram | method, path | 所有 |
| `connect_rpc_call_total` | Counter | service, method, code | 所有 |
| `connect_rpc_call_duration_seconds` | Histogram | service, method | 所有 |
| `ws_connections_active` | Gauge | — | WS Gateway |
| `groupcache_hit_total` | Counter | cache, operation (hit/miss/peer_fill) | User/Community Svc |
| `groupcache_fill_duration_seconds` | Histogram | cache, source (local/peer/db) | User/Community Svc |

### Connect-RPC 集成

OTel 自动注入 trace context 到 Connect-RPC 调用：

```go
import (
    "connectrpc.com/otelconnect"
)

// 客户端拦截器 — 注入 trace context
clientOpts := []connect.ClientOption{
    connect.WithInterceptors(otelconnect.NewClientInterceptor()),
}

// 服务端拦截器 — 提取 trace context，记录 RPC 指标
serverOpts := []connect.HandlerOption{
    connect.WithInterceptors(
        otelconnect.NewServerInterceptor(),
        metrics.ConnectRPCInterceptor(),
    ),
}
```

`otelconnect` 是 `connectrpc.com/otelconnect` 包，官方维护，无需自己实现传播。

### docker-compose 部署

```yaml
# docker-compose.yml 新增:
openobserve:
  image: public.ecr.aws/zinclabs/openobserve:latest
  ports:
    - "5080:5080"   # Web UI + OTLP HTTP
  environment:
    ZO_ROOT_USER_EMAIL: "admin@constell.local"
    ZO_ROOT_USER_PASSWORD: "admin123"
  volumes:
    - oo-data:/data
```

各服务环境变量新增：
```yaml
OTEL_EXPORTER_OTLP_ENDPOINT: "http://openobserve:5080/api/default/v1/otlp"
OTEL_EXPORTER_OTLP_INSECURE: "true"
```

---

## 5. 部署配置更新

### services.yaml

新增 `deploy/configs/services.yaml`，定义所有服务实例列表：

```yaml
registry:
  type: static  # static | k8s

services:
  api-gateway:
    instances:
      - addr: "api-gateway:8080"

  ws-gateway:
    instances:
      - addr: "ws-gateway-1:8081"
      - addr: "ws-gateway-2:8081"

  auth-service:
    instances:
      - addr: "auth-service:9081"

  user-service:
    instances:
      - addr: "user-service-1:9082"
      - addr: "user-service-2:9082"

  community-service:
    instances:
      - addr: "community-service-1:9083"
      - addr: "community-service-2:9083"
```

### docker-compose.yml 更新

1. 所有服务加 `healthcheck` 配置
2. 新增 `openobserve` 容器
3. 所有服务加 OTel 环境变量
4. 依赖声明加 `condition: service_healthy`

---

## 6. 对现有服务的影响

### 各服务 main.go 改造

统一启动流程：

```go
func main() {
    // 1. 加载配置
    var cfg Config
    config.MustLoad(&cfg)

    // 2. 初始化 OTel
    shutdown, err := otel.Init(context.Background(), otel.Config{
        ServiceName: "user-service",
        Environment: cfg.Env,
        Endpoint:    cfg.OTelEndpoint,
        Insecure:    true,
    })
    defer shutdown(context.Background())

    // 3. 初始化日志
    logger := logging.Init("user-service", cfg.Env)
    slog.SetDefault(logger)

    // 4. 初始化服务发现
    reg := registry.MustNew(cfg.RegistryType, cfg.RegistryConfig)

    // 5. 健康检查
    health := health.NewChecker()
    // ... 注册检查项

    // 6. 启动 HTTP server
    mux := http.NewServeMux()
    mux.HandleFunc("/healthz", health.HealthzHandler())
    mux.HandleFunc("/readyz", health.ReadyHandler())
    // ... 注册 Connect-RPC handlers（带 OTel interceptor）
}
```

### 对现有包的改造

| 包 | 改造内容 |
|----|---------|
| `pkg/groupcache/` | 接受 Registry 实例，通过 Watch 动态更新 peer 列表 |
| `pkg/middleware/` | 集成 OTel interceptor |
| `services/*/main.go` | 统一使用 config.Load + otel.Init + health checker |

### 不改动的部分

- Proto 定义 — 不变
- 业务逻辑 — 不变
- 数据库 schema — 不变
- NATS 事件定义 — 不变

---

## 7. 新增依赖

| 依赖 | 用途 | 引入位置 |
|------|------|---------|
| `go.opentelemetry.io/otel` | OTel SDK 核心 | `pkg/otel/` |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` | Trace OTLP Exporter | `pkg/otel/` |
| `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp` | Metric OTLP Exporter | `pkg/otel/` |
| `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp` | Log OTLP Exporter | `pkg/otel/` |
| `go.opentelemetry.io/otel/bridge/opentelemetry/log/slog` | slog → OTel 桥接 | `pkg/logging/` |
| `connectrpc.com/otelconnect` | Connect-RPC OTel 拦截器 | 各服务 |
| `k8s.io/client-go` | K8s API 客户端 | `pkg/registry/` (仅 K8s 实现) |
| `gopkg.in/yaml.v3` | YAML 配置解析 | `pkg/config/`, `pkg/registry/` |

### 编译优化

K8s 客户端（`k8s.io/client-go`）只在 K8s 环境下使用，通过 Go build tag 控制编译：

```
pkg/registry/
├── registry.go       # 接口定义 + Factory
├── static.go         # StaticRegistry（始终编译）
├── k8s.go            # K8sRegistry（build tag: //go:build k8s）
└── k8s_test.go       # K8s 测试（build tag: //go:build k8s）
```

docker-compose 构建时不编译 K8s 实现，避免引入不必要的依赖。

---

## 8. 本阶段不包含

以下能力明确不纳入本阶段，留给后续：

| 能力 | 原因 | 预计引入时机 |
|------|------|-------------|
| 熔断（Circuit Breaker） | 无实际流量，策略设计容易过度 | Plan 3+ 有 WS Gateway 后 |
| 重试策略 | 同上 | Plan 3+ |
| 限流（Rate Limiting） | 同上 | Plan 3+ |
| 配置热更新 | docker-compose 下不需要，K8s ConfigMap watch 可后补 | 有需要时 |
| K8s Helm Charts | 本阶段确保代码支持，部署配置另做 | 独立部署阶段 |
| 服务网格（Istio/Linkerd） | 应用层治理已足够，不需要 sidecar | 可能永远不需要 |
| Alerting 规则 | 先能看指标，告警规则后配 | 生产化阶段 |
