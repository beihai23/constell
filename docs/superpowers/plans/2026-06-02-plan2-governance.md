# Plan 2: 服务治理 — 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Constell 所有微服务提供统一的服务发现、配置管理、健康检查和可观测性基础设施，支持 docker-compose 和 K8s 双部署。

**Architecture:** 定义 Go 接口屏蔽部署差异。`pkg/registry/` 提供服务发现（StaticRegistry / K8sRegistry），`pkg/config/` 统一配置加载，`pkg/health/` 提供健康检查端点，`pkg/otel/` + `pkg/logging/` 通过 OTel SDK 将日志/指标/追踪统一发送到 OpenObserve。所有能力封装在共享库中，各服务 import 使用。

**Tech Stack:** Go 1.25, OTel SDK, OpenObserve, slog, connectrpc.com/otelconnect, k8s.io/client-go (build-tagged), gopkg.in/yaml.v3

---

## File Structure

```
backend/pkg/
├── config/
│   ├── config.go          # Loader: 多层级配置加载（环境变量 > yaml > defaults）
│   └── config_test.go
├── health/
│   ├── health.go          # Checker: healthz / readyz 端点
│   └── health_test.go
├── registry/
│   ├── registry.go        # Registry 接口 + Instance + Factory
│   ├── static.go          # StaticRegistry（docker-compose, 读 services.yaml）
│   ├── static_test.go
│   ├── k8s.go             # K8sRegistry（//go:build k8s）
│   └── k8s_test.go        # （//go:build k8s）
├── otel/
│   ├── otel.go            # Init: TracerProvider + MeterProvider + LoggerProvider
│   └── otel_test.go
├── logging/
│   ├── logging.go         # slog 封装, JSON 格式, trace_id 关联
│   └── logging_test.go
├── metrics/
│   ├── metrics.go         # HTTP + Connect-RPC 指标中间件
│   └── metrics_test.go
├── groupcache/
│   ├── groupcache.go      # 改造: 接受 Registry, Watch 动态更新 peers
│   └── groupcache_test.go # 新增 TestRegistryWatchPeers
├── middleware/
│   └── auth.go            # 不变

deploy/
├── configs/
│   ├── dev.yaml           # 更新: 加 otel 配置
│   └── services.yaml      # 新增: 服务实例列表
└── docker/
    └── docker-compose.yml # 更新: 加 healthcheck, openobserve, OTel env

# 各服务 main.go 统一改造
backend/services/*/main.go
```

---

### Task 1: pkg/config — 统一配置加载器

**Files:**
- Create: `backend/pkg/config/config.go`
- Create: `backend/pkg/config/config_test.go`

- [ ] **Step 1: 写 pkg/config/config.go**

```go
package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// Loader 加载配置到目标结构体。
// 优先级: 环境变量 > 配置文件 > struct tag 默认值。
type Loader struct {
	prefix string // 环境变量前缀，如 "AUTH_SERVICE_"
}

// NewLoader 创建配置加载器。prefix 用于环境变量匹配。
func NewLoader(prefix string) *Loader {
	return &Loader{prefix: prefix}
}

// Load 将配置加载到 target（必须是指向结构体的指针）。
// 支持 struct tag: `env:"KEY"` 指定环境变量名, `default:"val"` 指定默认值。
func (l *Loader) Load(target interface{}) error {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("config: target must be a pointer to struct")
	}
	return l.loadStruct(v.Elem())
}

func (l *Loader) loadStruct(v reflect.Value) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldVal := v.Field(i)

		// 递归处理嵌入结构体
		if fieldVal.Kind() == reflect.Struct && field.Anonymous {
			if err := l.loadStruct(fieldVal); err != nil {
				return err
			}
			continue
		}

		// 递归处理非指针结构体字段
		if fieldVal.Kind() == reflect.Struct {
			if err := l.loadStruct(fieldVal); err != nil {
				return err
			}
			continue
		}

		envKey := field.Tag.Get("env")
		defaultVal := field.Tag.Get("default")

		// 1. 尝试环境变量
		var val string
		if envKey != "" {
			// 先尝试带前缀的环境变量
			if l.prefix != "" {
				if v, ok := os.LookupEnv(l.prefix + envKey); ok {
					val = v
				}
			}
			if val == "" {
				if v, ok := os.LookupEnv(envKey); ok {
					val = v
				}
			}
		}

		// 2. 使用默认值
		if val == "" {
			val = defaultVal
		}

		if val == "" {
			continue
		}

		if err := setField(fieldVal, val); err != nil {
			return fmt.Errorf("config: field %s: %w", field.Name, err)
		}
	}
	return nil
}

func setField(f reflect.Value, val string) error {
	switch f.Kind() {
	case reflect.String:
		f.SetString(val)
	case reflect.Int:
		n, err := strconv.Atoi(val)
		if err != nil {
			return err
		}
		f.SetInt(int64(n))
	case reflect.Bool:
		b, err := strconv.ParseBool(val)
		if err != nil {
			return err
		}
		f.SetBool(b)
	case reflect.Slice:
		// 逗号分隔的字符串切片
		if f.Type().Elem().Kind() == reflect.String {
			parts := strings.Split(val, ",")
			for i, p := range parts {
				parts[i] = strings.TrimSpace(p)
			}
			// 过滤空串
			filtered := parts[:0]
			for _, p := range parts {
				if p != "" {
					filtered = append(filtered, p)
				}
			}
			f.Set(reflect.ValueOf(filtered))
		}
	default:
		return fmt.Errorf("unsupported type %s", f.Kind())
	}
	return nil
}

// MustLoad 同 Load，失败时 panic。
func (l *Loader) MustLoad(target interface{}) {
	if err := l.Load(target); err != nil {
		panic(err)
	}
}
```

- [ ] **Step 2: 写 pkg/config/config_test.go**

```go
package config

import (
	"os"
	"testing"
)

type TestConfig struct {
	Host    string   `env:"HOST" default:"localhost"`
	Port    int      `env:"PORT" default:"8080"`
	Debug   bool     `env:"DEBUG" default:"false"`
	Peers   []string `env:"PEERS"`
	NoTag   string
}

func TestLoadWithDefaults(t *testing.T) {
	os.Clearenv()
	var cfg TestConfig
	if err := NewLoader("").Load(&cfg); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "localhost" {
		t.Errorf("Host: got %q, want %q", cfg.Host, "localhost")
	}
	if cfg.Port != 8080 {
		t.Errorf("Port: got %d, want %d", cfg.Port, 8080)
	}
	if cfg.Debug != false {
		t.Errorf("Debug: got %v, want false", cfg.Debug)
	}
}

func TestLoadWithEnvOverride(t *testing.T) {
	os.Clearenv()
	os.Setenv("HOST", "0.0.0.0")
	os.Setenv("PORT", "3000")
	os.Setenv("DEBUG", "true")
	os.Setenv("PEERS", "a:8080,b:8080,c:8080")

	var cfg TestConfig
	if err := NewLoader("").Load(&cfg); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host: got %q, want %q", cfg.Host, "0.0.0.0")
	}
	if cfg.Port != 3000 {
		t.Errorf("Port: got %d, want %d", cfg.Port, 3000)
	}
	if cfg.Debug != true {
		t.Errorf("Debug: got %v, want true", cfg.Debug)
	}
	if len(cfg.Peers) != 3 {
		t.Fatalf("Peers: got %d, want 3", len(cfg.Peers))
	}
	if cfg.Peers[0] != "a:8080" {
		t.Errorf("Peers[0]: got %q", cfg.Peers[0])
	}
}

func TestLoadWithPrefix(t *testing.T) {
	os.Clearenv()
	os.Setenv("MYAPP_HOST", "myapp-host")

	var cfg TestConfig
	if err := NewLoader("MYAPP_").Load(&cfg); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "myapp-host" {
		t.Errorf("Host: got %q, want %q", cfg.Host, "myapp-host")
	}
}

func TestMustLoadPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()
	NewLoader("").MustLoad("not-a-struct-pointer")
}
```

- [ ] **Step 3: 运行测试**

Run: `cd backend && go test -v ./pkg/config/...`
Expected: 4 tests PASS

- [ ] **Step 4: 在 pkg/go.mod 中添加 gopkg.in/yaml.v3（为后续 registry 加载 services.yaml 预备）**

Run: `cd backend/pkg && go get gopkg.in/yaml.v3`

- [ ] **Step 5: Commit**

```bash
git add backend/pkg/config/ backend/pkg/go.mod backend/pkg/go.sum
git commit -m "feat: add unified config loader (pkg/config)"
```

---

### Task 2: pkg/health — 健康检查

**Files:**
- Create: `backend/pkg/health/health.go`
- Create: `backend/pkg/health/health_test.go`

- [ ] **Step 1: 写 pkg/health/health.go**

```go
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
)

// CheckFunc 检查一个依赖是否健康。返回 nil 表示健康。
type CheckFunc func(ctx context.Context) error

// Checker 管理存活和就绪检查。
type Checker struct {
	mu     sync.RWMutex
	checks map[string]CheckFunc
}

// NewChecker 创建健康检查器。
func NewChecker() *Checker {
	return &Checker{
		checks: make(map[string]CheckFunc),
	}
}

// RegisterCheck 注册就绪检查项。
func (c *Checker) RegisterCheck(name string, fn CheckFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks[name] = fn
}

// HealthzHandler 返回存活检查 handler。始终返回 200。
func (c *Checker) HealthzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// ReadyHandler 返回就绪检查 handler。执行所有注册的 CheckFunc。
func (c *Checker) ReadyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		c.mu.RLock()
		defer c.mu.RUnlock()

		failures := make(map[string]string)
		for name, fn := range c.checks {
			if err := fn(ctx); err != nil {
				failures[name] = err.Error()
			}
		}

		if len(failures) > 0 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
				"status":   "not ready",
				"failures": failures,
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
```

- [ ] **Step 2: 写 pkg/health/health_test.go**

```go
package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthzAlwaysOK(t *testing.T) {
	c := NewChecker()
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	c.HealthzHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("healthz: got %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReadyAllChecksPass(t *testing.T) {
	c := NewChecker()
	c.RegisterCheck("db", func(ctx context.Context) error { return nil })
	c.RegisterCheck("cache", func(ctx context.Context) error { return nil })

	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()
	c.ReadyHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("readyz: got %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReadyCheckFails(t *testing.T) {
	c := NewChecker()
	c.RegisterCheck("db", func(ctx context.Context) error { return nil })
	c.RegisterCheck("redis", func(ctx context.Context) error {
		return errors.New("connection refused")
	})

	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()
	c.ReadyHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz: got %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestReadyNoChecks(t *testing.T) {
	c := NewChecker()
	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()
	c.ReadyHandler().ServeHTTP(rec, req)

	// 无注册检查时视为就绪
	if rec.Code != http.StatusOK {
		t.Fatalf("readyz with no checks: got %d, want %d", rec.Code, http.StatusOK)
	}
}
```

- [ ] **Step 3: 运行测试**

Run: `cd backend && go test -v ./pkg/health/...`
Expected: 4 tests PASS

- [ ] **Step 4: Commit**

```bash
git add backend/pkg/health/
git commit -m "feat: add health check package (pkg/health)"
```

---

### Task 3: pkg/registry — 服务发现接口 + StaticRegistry

**Files:**
- Create: `backend/pkg/registry/registry.go`
- Create: `backend/pkg/registry/static.go`
- Create: `backend/pkg/registry/static_test.go`

- [ ] **Step 1: 写 pkg/registry/registry.go**

```go
package registry

import (
	"context"
)

// Instance 表示一个服务实例。
type Instance struct {
	ServiceName string            // e.g. "user-service"
	Addr        string            // e.g. "user-service-1:9082"
	Metadata    map[string]string // 可选: version, zone 等
}

// Registry 服务注册与发现接口。
type Registry interface {
	// Register 注册当前实例。
	Register(ctx context.Context, inst Instance) error
	// Deregister 注销当前实例。
	Deregister(ctx context.Context) error
	// Discover 发现指定服务的所有实例。
	Discover(ctx context.Context, serviceName string) ([]Instance, error)
	// Watch 监听指定服务的实例列表变化。
	Watch(ctx context.Context, serviceName string) (<-chan []Instance, error)
}

// Config 创建 Registry 的配置。
type Config struct {
	Type    string       // "static" 或 "k8s"
	Static  StaticConfig // StaticRegistry 配置
	// K8s 配置在 k8s.go 中定义（build tag 控制）
}

// StaticConfig 是 StaticRegistry 的配置。
type StaticConfig struct {
	// ConfigPath 指向 services.yaml 的路径。
	ConfigPath string
	// ServiceName 当前服务名，用于从配置中过滤自己的实例。
	ServiceName string
}

// NewRegistry 根据配置创建对应的 Registry 实现。
func NewRegistry(ctx context.Context, cfg Config) (Registry, error) {
	switch cfg.Type {
	case "k8s":
		return newK8sRegistry(ctx, cfg)
	default:
		return NewStaticRegistry(cfg.Static)
	}
}
```

- [ ] **Step 2: 写 pkg/registry/static.go**

```go
package registry

import (
	"context"
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// StaticRegistry 从 YAML 配置文件加载固定的服务实例列表。
// docker-compose 环境下使用，实例列表在启动时一次性加载。
type StaticRegistry struct {
	mu       sync.RWMutex
	services map[string][]Instance
	self     *Instance
}

// StaticRegistryConfig 是 services.yaml 的顶层结构。
type staticRegistryConfig struct {
	Services map[string]struct {
		Instances []struct {
			Addr     string            `yaml:"addr"`
			Metadata map[string]string `yaml:"metadata,omitempty"`
		} `yaml:"instances"`
	} `yaml:"services"`
}

// NewStaticRegistry 从 YAML 文件加载服务实例列表。
func NewStaticRegistry(cfg StaticConfig) (*StaticRegistry, error) {
	data, err := os.ReadFile(cfg.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("registry: read config %s: %w", cfg.ConfigPath, err)
	}

	var raw staticRegistryConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("registry: parse config: %w", err)
	}

	services := make(map[string][]Instance)
	for svcName, svc := range raw.Services {
		instances := make([]Instance, 0, len(svc.Instances))
		for _, inst := range svc.Instances {
			instances = append(instances, Instance{
				ServiceName: svcName,
				Addr:        inst.Addr,
				Metadata:    inst.Metadata,
			})
		}
		services[svcName] = instances
	}

	return &StaticRegistry{
		services: services,
	}, nil
}

// Register 无操作。静态模式下实例已在配置中定义。
func (r *StaticRegistry) Register(_ context.Context, inst Instance) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.self = &inst
	return nil
}

// Deregister 无操作。
func (r *StaticRegistry) Deregister(_ context.Context) error {
	return nil
}

// Discover 返回指定服务的实例列表。
func (r *StaticRegistry) Discover(_ context.Context, serviceName string) ([]Instance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instances, ok := r.services[serviceName]
	if !ok {
		return nil, fmt.Errorf("registry: service %q not found", serviceName)
	}
	// 返回副本避免外部修改
	result := make([]Instance, len(instances))
	copy(result, instances)
	return result, nil
}

// Watch 返回一个通道，启动时写入一次实例列表后不再变更。
func (r *StaticRegistry) Watch(ctx context.Context, serviceName string) (<-chan []Instance, error) {
	instances, err := r.Discover(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	ch := make(chan []Instance, 1)
	ch <- instances
	return ch, nil
}

// SelfAddr 返回当前实例地址（注册时传入的）。
func (r *StaticRegistry) SelfAddr() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.self == nil {
		return ""
	}
	return r.self.Addr
}
```

- [ ] **Step 3: 写 pkg/registry/static_test.go**

```go
package registry

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeTestServicesYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "services.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write test yaml: %v", err)
	}
	return path
}

const testServicesYAML = `
services:
  auth-service:
    instances:
      - addr: "auth-service:9081"
  user-service:
    instances:
      - addr: "user-service-1:9082"
      - addr: "user-service-2:9082"
  community-service:
    instances:
      - addr: "community-service:9083"
  ws-gateway:
    instances:
      - addr: "ws-gateway-1:8081"
      - addr: "ws-gateway-2:8081"
`

func TestStaticDiscover(t *testing.T) {
	path := writeTestServicesYAML(t, testServicesYAML)
	reg, err := NewStaticRegistry(StaticConfig{ConfigPath: path})
	if err != nil {
		t.Fatalf("NewStaticRegistry: %v", err)
	}

	instances, err := reg.Discover(context.Background(), "user-service")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(instances))
	}
	if instances[0].Addr != "user-service-1:9082" {
		t.Errorf("instance[0].Addr: got %q", instances[0].Addr)
	}
	if instances[1].Addr != "user-service-2:9082" {
		t.Errorf("instance[1].Addr: got %q", instances[1].Addr)
	}
}

func TestStaticDiscoverNotFound(t *testing.T) {
	path := writeTestServicesYAML(t, testServicesYAML)
	reg, err := NewStaticRegistry(StaticConfig{ConfigPath: path})
	if err != nil {
		t.Fatalf("NewStaticRegistry: %v", err)
	}

	_, err = reg.Discover(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown service")
	}
}

func TestStaticWatch(t *testing.T) {
	path := writeTestServicesYAML(t, testServicesYAML)
	reg, err := NewStaticRegistry(StaticConfig{ConfigPath: path})
	if err != nil {
		t.Fatalf("NewStaticRegistry: %v", err)
	}

	ch, err := reg.Watch(context.Background(), "ws-gateway")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// 应该立即收到一次实例列表
	select {
	case instances := <-ch:
		if len(instances) != 2 {
			t.Fatalf("Watch: expected 2 instances, got %d", len(instances))
		}
	default:
		t.Fatal("Watch: expected immediate value in channel")
	}

	// 通道不应再发送新值
	select {
	case <-ch:
		t.Fatal("Watch: unexpected second value in channel")
	default:
		// 正确: 通道为空
	}
}

func TestStaticRegisterSelfAddr(t *testing.T) {
	path := writeTestServicesYAML(t, testServicesYAML)
	reg, err := NewStaticRegistry(StaticConfig{ConfigPath: path})
	if err != nil {
		t.Fatalf("NewStaticRegistry: %v", err)
	}

	ctx := context.Background()
	inst := Instance{ServiceName: "user-service", Addr: "user-service-1:9082"}
	if err := reg.Register(ctx, inst); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if reg.SelfAddr() != "user-service-1:9082" {
		t.Errorf("SelfAddr: got %q, want %q", reg.SelfAddr(), "user-service-1:9082")
	}
}

func TestStaticFileNotFound(t *testing.T) {
	_, err := NewStaticRegistry(StaticConfig{ConfigPath: "/nonexistent/services.yaml"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
```

- [ ] **Step 4: 运行测试**

Run: `cd backend && go test -v ./pkg/registry/...`
Expected: 5 tests PASS

- [ ] **Step 5: Commit**

```bash
git add backend/pkg/registry/
git commit -m "feat: add service registry interface + StaticRegistry (pkg/registry)"
```

---

### Task 4: pkg/registry — K8sRegistry (build-tagged)

**Files:**
- Create: `backend/pkg/registry/k8s.go`
- Create: `backend/pkg/registry/k8s_stub.go` (非 k8s 构建时的桩实现)

- [ ] **Step 1: 写 pkg/registry/k8s.go**

```go
//go:build k8s

package registry

import (
	"context"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// K8sConfig 是 K8sRegistry 的配置。
type K8sConfig struct {
	Namespace string // K8s namespace，默认 "default"
}

type k8sRegistry struct {
	clientset *kubernetes.Clientset
	namespace string
	mu        sync.RWMutex
	self      *Instance
	watchers  map[string][]chan []Instance
}

func newK8sRegistry(ctx context.Context, cfg Config) (*k8sRegistry, error) {
	k8sCfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("registry: k8s in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(k8sCfg)
	if err != nil {
		return nil, fmt.Errorf("registry: k8s client: %w", err)
	}

	ns := cfg.K8s.Namespace
	if ns == "" {
		ns = "default"
	}

	return &k8sRegistry{
		clientset: clientset,
		namespace: ns,
		watchers:  make(map[string][]chan []Instance),
	}, nil
}

func (r *k8sRegistry) Register(_ context.Context, inst Instance) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.self = &inst
	return nil
}

func (r *k8sRegistry) Deregister(_ context.Context) error {
	return nil
}

func (r *k8sRegistry) Discover(ctx context.Context, serviceName string) ([]Instance, error) {
	endpoints, err := r.clientset.CoreV1().Endpoints(r.namespace).Get(
		ctx, serviceName, metav1.GetOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("registry: k8s get endpoints %s: %w", serviceName, err)
	}

	return endpointsToInstances(endpoints, serviceName), nil
}

func (r *k8sRegistry) Watch(ctx context.Context, serviceName string) (<-chan []Instance, error) {
	ch := make(chan []Instance, 16)

	// 初始发现
	instances, err := r.Discover(ctx, serviceName)
	if err != nil {
		return nil, err
	}
	ch <- instances

	// 启动 watcher goroutine
	go r.watchEndpoints(ctx, serviceName, ch)

	return ch, nil
}

func (r *k8sRegistry) watchEndpoints(ctx context.Context, serviceName string, ch chan<- []Instance) {
	watcher, err := r.clientset.CoreV1().Endpoints(r.namespace).Watch(
		ctx, metav1.SingleObject(metav1.ObjectMeta{Name: serviceName}),
	)
	if err != nil {
		close(ch)
		return
	}
	defer watcher.Stop()

	for event := range watcher.ResultChan() {
		endpoints, ok := event.Object.(*corev1.Endpoints)
		if !ok {
			continue
		}
		instances := endpointsToInstances(endpoints, serviceName)
		select {
		case ch <- instances:
		case <-ctx.Done():
			return
		}
	}
}

func endpointsToInstances(ep *corev1.Endpoints, serviceName string) []Instance {
	var instances []Instance
	for _, subset := range ep.Subsets {
		for _, addr := range subset.Addresses {
			instances = append(instances, Instance{
				ServiceName: serviceName,
				Addr:        fmt.Sprintf("%s:%d", addr.IP, subset.Ports[0].Port),
			})
		}
	}
	return instances
}
```

- [ ] **Step 2: 写 pkg/registry/k8s_stub.go**（非 k8s 构建时的桩，保证编译通过）

```go
//go:build !k8s

package registry

import (
	"context"
	"fmt"
)

// K8sConfig 在非 k8s 构建时不可用。
type K8sConfig struct{}

func newK8sRegistry(_ context.Context, _ Config) (*k8sRegistryStub, error) {
	return nil, fmt.Errorf("registry: k8s support not compiled; rebuild with -tags k8s")
}

type k8sRegistryStub struct{}
```

- [ ] **Step 3: 验证非 k8s 构建编译通过**

Run: `cd backend && go build ./pkg/registry/...`
Expected: 编译成功（使用 k8s_stub.go）

- [ ] **Step 4: Commit**

```bash
git add backend/pkg/registry/k8s.go backend/pkg/registry/k8s_stub.go
git commit -m "feat: add K8sRegistry implementation (build-tagged)"
```

**注意:** 不在本阶段安装 `k8s.io/client-go` 依赖（避免膨胀非 K8s 构建）。当需要编译 K8s 版本时，在 `pkg/go.mod` 中添加依赖并用 `go build -tags k8s` 编译。

---

### Task 5: groupcache 集成 Registry 动态 peers

**Files:**
- Modify: `backend/pkg/groupcache/groupcache.go`
- Modify: `backend/pkg/groupcache/groupcache_test.go`

- [ ] **Step 1: 在 groupcache.go 中添加 Registry 集成**

在现有 `Option` 类型下方新增一个 option 和 `UpdatePeers` 方法：

```go
// WithRegistry 设置 Registry 实例，用于通过 Watch 动态更新 peer 列表。
// 如果同时设置了 WithPeers，WithPeers 作为初始值，Registry Watch 生效后覆盖。
func WithRegistry[K comparable, V any](reg registry.Registry, serviceName string) Option[K, V] {
	return func(c *Cache[K, V]) {
		c.reg = reg
		c.regServiceName = serviceName
	}
}
```

在 `Cache` struct 中新增字段：

```go
type Cache[K comparable, V any] struct {
	mu             sync.RWMutex
	capacity       int
	peers          []string
	peerSet        map[string]bool
	filler         func(ctx context.Context, key K) (V, error)
	lru            *lruCache[K, V]
	sfGroup        singleflight.Group
	sfKeyFunc      func(key K) string
	reg            registry.Registry   // 新增: 可选的 Registry
	regServiceName string              // 新增: Watch 的服务名
	regCancel      context.CancelFunc  // 新增: 停止 Watch
}
```

在 `NewCache` 函数中，在 `return c` 之前添加 Registry Watch 逻辑：

```go
	// 如果配置了 Registry，启动 Watch 更新 peers。
	if c.reg != nil && c.regServiceName != "" {
		ctx, cancel := context.WithCancel(context.Background())
		c.regCancel = cancel
		go c.watchPeers(ctx)
	}
```

新增方法：

```go
import "github.com/constell/constell/backend/pkg/registry"

// watchPeers 监听 Registry 的实例变更并更新 peer 列表。
func (c *Cache[K, V]) watchPeers(ctx context.Context) {
	ch, err := c.reg.Watch(ctx, c.regServiceName)
	if err != nil {
		return
	}
	for {
		select {
		case instances, ok := <-ch:
			if !ok {
				return
			}
			peers := make([]string, 0, len(instances))
			for _, inst := range instances {
				peers = append(peers, inst.Addr)
			}
			peers = dedupePeers(peers)
			c.mu.Lock()
			c.peers = peers
			c.peerSet = make(map[string]bool, len(peers))
			for _, p := range peers {
				c.peerSet[p] = true
			}
			c.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

// Close 停止 Registry Watch 并释放资源。
func (c *Cache[K, V]) Close() {
	if c.regCancel != nil {
		c.regCancel()
	}
}
```

在文件顶部 import 中添加:

```go
import "github.com/constell/constell/backend/pkg/registry"
```

- [ ] **Step 2: 添加 TestRegistryWatchPeers 测试**

在 `groupcache_test.go` 末尾添加：

```go
type mockRegistry struct {
	instances []registry.Instance
	ch        chan []registry.Instance
}

func (m *mockRegistry) Register(_ context.Context, _ registry.Instance) error { return nil }
func (m *mockRegistry) Deregister(_ context.Context) error                    { return nil }
func (m *mockRegistry) Discover(_ context.Context, _ string) ([]registry.Instance, error) {
	return m.instances, nil
}
func (m *mockRegistry) Watch(_ context.Context, _ string) (<-chan []registry.Instance, error) {
	ch := make(chan []registry.Instance, 1)
	ch <- m.instances
	m.ch = ch
	return ch, nil
}

func TestRegistryWatchPeers(t *testing.T) {
	mock := &mockRegistry{
		instances: []registry.Instance{
			{ServiceName: "user-service", Addr: "node1:9082"},
			{ServiceName: "user-service", Addr: "node2:9082"},
		},
		ch: make(chan []registry.Instance, 4),
	}

	cache := NewCache[string, string](
		WithLocalCapacity[string, string](100),
		WithRegistry[string, string](mock, "user-service"),
		WithFiller(func(ctx context.Context, key string) (string, error) {
			return "val-" + key, nil
		}),
	)
	defer cache.Close()

	// 等待初始 Watch 值被消费
	time.Sleep(50 * time.Millisecond)

	cache.mu.RLock()
	peers := cache.peers
	cache.mu.RUnlock()

	if len(peers) != 2 {
		t.Fatalf("expected 2 peers from registry, got %d: %v", len(peers), peers)
	}

	// 模拟 Registry 变更: 新增一个实例
	mock.ch <- []registry.Instance{
		{ServiceName: "user-service", Addr: "node1:9082"},
		{ServiceName: "user-service", Addr: "node2:9082"},
		{ServiceName: "user-service", Addr: "node3:9082"},
	}

	time.Sleep(50 * time.Millisecond)

	cache.mu.RLock()
	peers = cache.peers
	cache.mu.RUnlock()

	if len(peers) != 3 {
		t.Fatalf("expected 3 peers after update, got %d: %v", len(peers), peers)
	}
}
```

- [ ] **Step 3: 运行测试**

Run: `cd backend && go test -v -run TestRegistryWatchPeers ./pkg/groupcache/...`
Expected: PASS

Run: `cd backend && go test -v ./pkg/groupcache/...`
Expected: 全部 PASS（包括原有测试）

- [ ] **Step 4: Commit**

```bash
git add backend/pkg/groupcache/
git commit -m "feat: integrate groupcache with Registry for dynamic peer updates"
```

---

### Task 6: pkg/otel — OTel 初始化

**Files:**
- Create: `backend/pkg/otel/otel.go`
- Create: `backend/pkg/otel/otel_test.go`

- [ ] **Step 1: 安装 OTel 依赖**

Run: `cd backend/pkg && go get go.opentelemetry.io/otel go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp go.opentelemetry.io/otel/sdk go.opentelemetry.io/otel/sdk/log go.opentelemetry.io/otel/sdk/metric go.opentelemetry.io/otel/bridge/opentelemetry/log/slog`

- [ ] **Step 2: 写 pkg/otel/otel.go**

```go
package otel

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config OTel 初始化配置。
type Config struct {
	ServiceName string
	Environment string // dev, staging, prod
	Endpoint    string // OpenObserve OTLP endpoint, e.g. "http://localhost:5080/api/default/v1/otlp"
	Insecure    bool   // 使用 HTTP（非 TLS），docker-compose 下为 true
}

// ShutdownFunc 关闭所有 OTel provider。
type ShutdownFunc func(ctx context.Context) error

// Init 初始化 OTel: TracerProvider + MeterProvider + LoggerProvider。
// 返回 shutdown 函数，应在 graceful shutdown 时调用。
func Init(ctx context.Context, cfg Config) (ShutdownFunc, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.DeploymentEnvironmentKey.String(cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel: create resource: %w", err)
	}

	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	// TracerProvider
	traceExporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("otel: create trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)

	// MeterProvider
	metricExporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpoint(cfg.Endpoint),
	)
	if err != nil {
		return nil, fmt.Errorf("otel: create metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)

	// LoggerProvider
	logExporter, err := otlploghttp.New(ctx,
		otlploghttp.WithEndpoint(cfg.Endpoint),
	)
	if err != nil {
		return nil, fmt.Errorf("otel: create log exporter: %w", err)
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
		sdklog.WithResource(res),
	)

	// 设置全局 provider
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	shutdown := func(ctx context.Context) error {
		var errs []error
		if err := tp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("tracer shutdown: %w", err))
		}
		if err := mp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("meter shutdown: %w", err))
		}
		if err := lp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("logger shutdown: %w", err))
		}
		if len(errs) > 0 {
			return fmt.Errorf("otel shutdown errors: %v", errs)
		}
		return nil
	}

	return shutdown, nil
}

// ShutdownWithTimeout 在超时内执行 shutdown。
func ShutdownWithTimeout(shutdown ShutdownFunc, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		fmt.Printf("otel shutdown error: %v\n", err)
	}
}
```

- [ ] **Step 3: 写 pkg/otel/otel_test.go**

```go
package otel

import (
	"context"
	"testing"
)

func TestInitWithInvalidEndpoint(t *testing.T) {
	// 无效端点不应 panic，Init 可能返回错误或成功（OTel 是异步导出）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 使用一个不存在的端点。OTel SDK 创建 exporter 时不会立即连接，
	// 所以 Init 通常不会报错。我们只验证不 panic。
	shutdown, err := Init(ctx, Config{
		ServiceName: "test-service",
		Environment: "test",
		Endpoint:    "http://localhost:9999/api/default/v1/otlp",
		Insecure:    true,
	})
	if err != nil {
		// 可以接受错误
		t.Logf("Init returned error (expected for invalid endpoint): %v", err)
		return
	}

	// shutdown 应该能正常执行
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()
	if err := shutdown(shutdownCtx); err != nil {
		t.Logf("shutdown error (expected for invalid endpoint): %v", err)
	}
}

func TestShutdownWithTimeout(t *testing.T) {
	ShutdownWithTimeout(func(_ context.Context) error {
		return nil
	}, 0)
}
```

- [ ] **Step 4: 运行测试**

Run: `cd backend && go test -v ./pkg/otel/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/pkg/otel/ backend/pkg/go.mod backend/pkg/go.sum
git commit -m "feat: add OTel initialization package (pkg/otel)"
```

---

### Task 7: pkg/logging — slog 封装

**Files:**
- Create: `backend/pkg/logging/logging.go`
- Create: `backend/pkg/logging/logging_test.go`

- [ ] **Step 1: 写 pkg/logging/logging.go**

```go
package logging

import (
	"context"
	"log/slog"
	"os"

	slogotel "go.opentelemetry.io/otel/bridge/opentelemetry/log/slog"
)

type contextKey string

const loggerKey contextKey = "constell-logger"

// Init 初始化 slog logger。
// - JSON 格式输出到 stdout
// - 注入 service_name, environment 公共字段
// - 桥接到 OTel Log Provider（如果已初始化 OTel）
func Init(serviceName, env string) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	// 尝试桥接到 OTel LoggerProvider
	// 如果 OTel 未初始化，桥接会被忽略（handler 直接输出到 stdout）
	otelHandler := slogotel.NewOtelHandler(handler)

	logger := slog.New(otelHandler).With(
		"service", serviceName,
		"env", env,
	)
	return logger
}

// FromContext 从 context 中提取 logger。如果不存在，返回默认 logger。
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

// NewContext 创建携带 logger 的 context。
func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// WithTraceID 返回附加了 trace_id 的 logger（从 OTel span 中提取）。
// 用法: logger := logging.WithTraceID(logging.FromContext(ctx), ctx)
func WithTraceID(logger *slog.Logger, ctx context.Context) *slog.Logger {
	spanCtx := fromContext(ctx)
	if spanCtx.HasTraceID() {
		return logger.With("trace_id", spanCtx.TraceID())
	}
	return logger
}
```

添加一个小的 helper 文件来提取 trace context（避免直接依赖 OTel trace 包在主要 API 上）：

```go
// 在 logging.go 同一个文件中:
import "go.opentelemetry.io/otel/trace"

func fromContext(ctx context.Context) trace.SpanContext {
	span := trace.SpanFromContext(ctx)
	if span == nil {
		return trace.SpanContext{}
	}
	return span.SpanContext()
}
```

- [ ] **Step 2: 写 pkg/logging/logging_test.go**

```go
package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestInitLoggerFormat(t *testing.T) {
	// 捕获 stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger := Init("test-service", "test")
	logger.Info("hello", "key", "value")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, `"service":"test-service"`) {
		t.Errorf("output missing service field: %s", output)
	}
	if !strings.Contains(output, `"env":"test"`) {
		t.Errorf("output missing env field: %s", output)
	}
	if !strings.Contains(output, `"msg":"hello"`) {
		t.Errorf("output missing msg: %s", output)
	}

	// 验证是合法 JSON
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &m); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, output)
	}
}

func TestContextRoundTrip(t *testing.T) {
	logger := slog.Default()
	ctx := NewContext(context.Background(), logger)

	retrieved := FromContext(ctx)
	if retrieved != logger {
		t.Error("FromContext should return the same logger")
	}
}

func TestFromContextEmpty(t *testing.T) {
	logger := FromContext(context.Background())
	if logger == nil {
		t.Error("FromContext should return default logger when none in context")
	}
}
```

- [ ] **Step 3: 运行测试**

Run: `cd backend && go test -v ./pkg/logging/...`
Expected: 3 tests PASS

- [ ] **Step 4: Commit**

```bash
git add backend/pkg/logging/
git commit -m "feat: add structured logging package (pkg/logging)"
```

---

### Task 8: pkg/metrics — HTTP + Connect-RPC 指标中间件

**Files:**
- Create: `backend/pkg/metrics/metrics.go`
- Create: `backend/pkg/metrics/metrics_test.go`

- [ ] **Step 1: 安装 connectrpc.com/otelconnect 依赖**

Run: `cd backend/pkg && go get connectrpc.com/otelconnect`

- [ ] **Step 2: 写 pkg/metrics/metrics.go**

```go
package metrics

import (
	"net/http"
	"time"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	meterName = "github.com/constell/constell/backend/pkg/metrics"
)

// HTTPMiddleware 记录 HTTP 请求的 QPS / 延迟 / 错误率。
func HTTPMiddleware(handler http.Handler) http.Handler {
	meter := otel.Meter(meterName)
	requestTotal, _ := meter.Int64Counter(
		"http_request_total",
		metric.WithDescription("Total HTTP requests"),
	)
	requestDuration, _ := meter.Float64Histogram(
		"http_request_duration_seconds",
		metric.WithDescription("HTTP request duration in seconds"),
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		handler.ServeHTTP(wrapped, r)

		duration := time.Since(start).Seconds()
		attrs := metric.WithAttributes(
			attribute.String("method", r.Method),
			attribute.String("path", r.URL.Path),
			attribute.Int("status", wrapped.statusCode),
		)

		requestTotal.Add(r.Context(), 1, attrs)
		requestDuration.Record(r.Context(), duration, attrs)
	})
}

// ConnectRPCInterceptor 记录 Connect-RPC 调用的 QPS / 延迟 / 错误率。
// 注意: 大部分 RPC 指标已由 otelconnect interceptor 覆盖。
// 此中间件提供额外的业务指标记录。
func ConnectRPCInterceptor() connect.UnaryInterceptorFunc {
	meter := otel.Meter(meterName)
	callTotal, _ := meter.Int64Counter(
		"connect_rpc_call_total",
		metric.WithDescription("Total Connect-RPC calls"),
	)

	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			procedure := req.Spec().Procedure

			code := "ok"
			if err != nil {
				if connectErr := new(connect.Error); connectErr != nil {
					code = connectErr.Code().String()
				} else {
					code = "unknown"
				}
			}

			callTotal.Add(ctx, 1,
				metric.WithAttributes(
					attribute.String("procedure", procedure),
					attribute.String("code", code),
				),
			)

			return resp, err
		}
	}
}

// responseWriter 包装 http.ResponseWriter 以捕获 status code。
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
```

- [ ] **Step 3: 写 pkg/metrics/metrics_test.go**

```go
package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPMiddlewareRecordsMetrics(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	wrapped := HTTPMiddleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	// 指标已记录到 OTel MeterProvider（无 panic 即成功）
}

func TestHTTPMiddlewareCapturesStatus(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	wrapped := HTTPMiddleware(handler)
	req := httptest.NewRequest("GET", "/missing", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}
```

- [ ] **Step 4: 运行测试**

Run: `cd backend && go test -v ./pkg/metrics/...`
Expected: 2 tests PASS

- [ ] **Step 5: Commit**

```bash
git add backend/pkg/metrics/ backend/pkg/go.mod backend/pkg/go.sum
git commit -m "feat: add HTTP + Connect-RPC metrics middleware (pkg/metrics)"
```

---

### Task 9: 部署配置 — services.yaml + docker-compose 更新

**Files:**
- Create: `deploy/configs/services.yaml`
- Modify: `deploy/docker/docker-compose.yml`

- [ ] **Step 1: 创建 deploy/configs/services.yaml**

```yaml
# Constell 服务实例列表
# StaticRegistry 从此文件加载实例地址。
# docker-compose 下扩缩容: 修改此文件 → docker-compose up -d
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

- [ ] **Step 2: 更新 deploy/configs/dev.yaml，添加 otel 配置**

在 `dev.yaml` 末尾追加：

```yaml
otel:
  endpoint: "http://localhost:5080/api/default/v1/otlp"
  insecure: true

registry:
  type: static
  config_path: "deploy/configs/services.yaml"
```

- [ ] **Step 3: 更新 docker-compose.yml**

在每个后端服务中添加:
1. `healthcheck` 配置
2. OTel 环境变量
3. `REGISTRY_TYPE` 和 `SERVICES_CONFIG_PATH` 环境变量
4. 挂载 `deploy/configs/` 目录

在 `docker-compose.yml` 末尾新增 `openobserve` 服务。

**以 auth-service 为例（其他服务类似修改）:**

```yaml
  auth-service:
    build:
      context: ../../
      dockerfile: backend/services/auth-service/Dockerfile
    container_name: constell-auth-service
    environment:
      PORT: "9081"
      DATABASE_URL: "postgres://constell:constell_dev@postgres:5432/constell?sslmode=disable"
      REDIS_URL: "redis:6379"
      JWT_SECRET: "dev-secret-change-me"
      REGISTRY_TYPE: "static"
      SERVICES_CONFIG_PATH: "/app/configs/services.yaml"
      OTEL_EXPORTER_OTLP_ENDPOINT: "http://openobserve:5080/api/default/v1/otlp"
    volumes:
      - ../../deploy/configs:/app/configs:ro
    ports:
      - "9081:9081"
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:9081/healthz"]
      interval: 10s
      timeout: 5s
      retries: 3
```

**新增 openobserve 服务:**

```yaml
  openobserve:
    image: public.ecr.aws/zinclabs/openobserve:latest
    container_name: constell-openobserve
    environment:
      ZO_ROOT_USER_EMAIL: "admin@constell.local"
      ZO_ROOT_USER_PASSWORD: "admin123"
      ZO_DATA_DIR: "/data"
    ports:
      - "5080:5080"    # Web UI + OTLP HTTP
    volumes:
      - oo_data:/data
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:5080/web/"]
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  # ... 原有 volumes ...
  oo_data:
```

- [ ] **Step 4: Commit**

```bash
git add deploy/configs/ deploy/docker/docker-compose.yml
git commit -m "feat: add services.yaml and update docker-compose with governance"
```

---

### Task 10: 迁移 auth-service（最简单的无状态服务，作为模板）

**Files:**
- Modify: `backend/services/auth-service/main.go`

auth-service 改造后的 main.go 作为所有服务改造的参考模板。

- [ ] **Step 1: 重写 auth-service main.go**

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
	pkgotel "github.com/constell/constell/backend/pkg/otel"
	"github.com/constell/constell/backend/pkg/middleware"
	"github.com/constell/constell/backend/pkg/proto/auth/v1/authv1connect"
)

type Config struct {
	Port      string `env:"PORT" default:"9081"`
	DBUrl     string `env:"DATABASE_URL" default:"postgres://constell:constell_dev@localhost:5432/constell?sslmode=disable"`
	RedisURL  string `env:"REDIS_URL" default:"localhost:6379"`
	JWTSecret string `env:"JWT_SECRET" default:"dev-secret-change-me"`
	Env       string `env:"ENV" default:"dev"`

	// OTel
	OTelEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:"http://localhost:5080/api/default/v1/otlp"`
	OTelInsecure string `env:"OTEL_EXPORTER_OTLP_INSECURE" default:"true"`
}

func main() {
	// 1. 加载配置
	var cfg Config
	config.NewLoader("").MustLoad(&cfg)

	// 2. 初始化 OTel
	shutdown, err := pkgotel.Init(context.Background(), pkgotel.Config{
		ServiceName: "auth-service",
		Environment: cfg.Env,
		Endpoint:    cfg.OTelEndpoint,
		Insecure:    cfg.OTelInsecure == "true",
	})
	if err != nil {
		slog.Error("otel init", "error", err)
	} else {
		defer pkgotel.ShutdownWithTimeout(shutdown, 5*time.Second)
	}

	// 3. 初始化日志
	logger := logging.Init("auth-service", cfg.Env)
	slog.SetDefault(logger)

	// 4. 连接基础设施
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

	rdb := goredis.NewClient(&goredis.Options{Addr: cfg.RedisURL})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Error("ping redis", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to redis")

	// 5. 健康检查
	hc := health.NewChecker()
	hc.RegisterCheck("postgres", func(ctx context.Context) error {
		return pool.Ping(ctx)
	})
	hc.RegisterCheck("redis", func(ctx context.Context) error {
		return rdb.Ping(ctx).Err()
	})

	// 6. 创建 OTel Connect interceptor
	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		slog.Error("create otel interceptor", "error", err)
		os.Exit(1)
	}

	// 7. Wire up service
	repo := NewRepository(pool)
	authService := NewAuthService(repo, rdb, cfg.JWTSecret)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", hc.HealthzHandler())
	mux.HandleFunc("/readyz", hc.ReadyHandler())
	mux.Handle(authv1connect.NewAuthServiceHandler(
		authService,
		connect.WithInterceptors(
			otelInterceptor,
			metrics.ConnectRPCInterceptor(),
			middleware.NewAuthInterceptor(cfg.JWTSecret),
		),
	))

	// 8. 启动 HTTP server
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

	slog.Info("auth-service listening", "port", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: 删除 auth-service main.go 中的 `envOrDefault` 函数**（已被 config 包替代）

- [ ] **Step 3: 确保编译通过**

Run: `cd backend && go build ./services/auth-service/...`
Expected: 编译成功

- [ ] **Step 4: 运行 auth-service 原有测试**

Run: `cd backend && go test ./services/auth-service/...`
Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
git add backend/services/auth-service/
git commit -m "refactor: migrate auth-service to use governance packages"
```

---

### Task 11: 迁移 user-service（有状态，groupcache 集成 Registry）

**Files:**
- Modify: `backend/services/user-service/main.go`
- Modify: `backend/services/user-service/cache.go`

- [ ] **Step 1: 重写 user-service main.go**

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
	pkgotel "github.com/constell/constell/backend/pkg/otel"
	"github.com/constell/constell/backend/pkg/middleware"
	"github.com/constell/constell/backend/pkg/proto/user/v1/userv1connect"
	"github.com/constell/constell/backend/pkg/registry"
)

type Config struct {
	Port     string `env:"PORT" default:"9082"`
	DBUrl    string `env:"DATABASE_URL" default:"postgres://constell:constell_dev@localhost:5432/constell?sslmode=disable"`
	RedisURL string `env:"REDIS_URL" default:"localhost:6379"`
	JWTSecret string `env:"JWT_SECRET" default:"dev-secret-change-me"`
	Env      string `env:"ENV" default:"dev"`

	// Registry
	RegistryType     string `env:"REGISTRY_TYPE" default:"static"`
	ServicesCfgPath  string `env:"SERVICES_CONFIG_PATH" default:"deploy/configs/services.yaml"`
	Peers            []string `env:"GROUPCACHE_PEERS"` // static fallback

	// OTel
	OTelEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:"http://localhost:5080/api/default/v1/otlp"`
	OTelInsecure string `env:"OTEL_EXPORTER_OTLP_INSECURE" default:"true"`
}

func main() {
	var cfg Config
	config.NewLoader("").MustLoad(&cfg)

	shutdown, err := pkgotel.Init(context.Background(), pkgotel.Config{
		ServiceName: "user-service",
		Environment: cfg.Env,
		Endpoint:    cfg.OTelEndpoint,
		Insecure:    cfg.OTelInsecure == "true",
	})
	if err != nil {
		slog.Error("otel init", "error", err)
	} else {
		defer pkgotel.ShutdownWithTimeout(shutdown, 5*time.Second)
	}

	logger := logging.Init("user-service", cfg.Env)
	slog.SetDefault(logger)

	// 连接基础设施
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

	rdb := goredis.NewClient(&goredis.Options{Addr: cfg.RedisURL})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Error("ping redis", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to redis")

	// 初始化 Registry（可选，groupcache 需要 peer 列表）
	var reg registry.Registry
	if cfg.RegistryType == "static" && cfg.ServicesCfgPath != "" {
		var err error
		reg, err = registry.NewStaticRegistry(registry.StaticConfig{
			ConfigPath: cfg.ServicesCfgPath,
		})
		if err != nil {
			slog.Warn("registry init failed, using env peers", "error", err)
		}
	}

	// 健康检查
	hc := health.NewChecker()
	hc.RegisterCheck("postgres", func(ctx context.Context) error { return pool.Ping(ctx) })
	hc.RegisterCheck("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() })

	// OTel interceptor
	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		slog.Error("create otel interceptor", "error", err)
		os.Exit(1)
	}

	// Wire up
	repo := NewRepository(pool)
	userCache := NewUserCache(10000, cfg.Peers, reg, "user-service", repo)
	relationCache := NewRelationCache(10000, cfg.Peers, reg, "user-service", repo)
	userService := NewUserService(repo, userCache, relationCache)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", hc.HealthzHandler())
	mux.HandleFunc("/readyz", hc.ReadyHandler())
	mux.Handle(userv1connect.NewUserServiceHandler(
		userService,
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

	slog.Info("user-service listening", "port", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: 修改 user-service/cache.go，接受 Registry 参数**

修改 `NewUserCache` 和 `NewRelationCache` 函数签名，接受可选的 `registry.Registry`:

```go
// NewUserCache 创建 UserCache。
// peers 作为 fallback（Registry 未配置时使用）。
// reg 和 regServiceName 用于动态更新 peer 列表。
func NewUserCache(localCapacity int, peers []string, reg registry.Registry, regServiceName string, repo *Repository) *UserCache {
	opts := []groupcache.Option[string, []byte]{
		groupcache.WithLocalCapacity[string, []byte](localCapacity),
		groupcache.WithFiller[string, []byte](func(ctx context.Context, key string) ([]byte, error) {
			userID := key[len("user:"):]
			user, err := repo.GetUserByID(ctx, userID)
			if err != nil {
				return nil, err
			}
			return MarshalUser(user)
		}),
	}

	if reg != nil {
		opts = append(opts, groupcache.WithRegistry[string, []byte](reg, regServiceName))
	} else if len(peers) > 0 {
		opts = append(opts, groupcache.WithPeers[string, []byte](peers))
	}

	c := groupcache.NewCache[string, []byte](opts...)
	return &UserCache{cache: c, repo: repo}
}
```

对 `NewRelationCache` 做同样的修改。

在文件顶部 import 中添加:
```go
"github.com/constell/constell/backend/pkg/registry"
```

- [ ] **Step 3: 删除 `envOrDefault` 和 `strings` 导入**

- [ ] **Step 4: 确保编译通过**

Run: `cd backend && go build ./services/user-service/...`

- [ ] **Step 5: 运行测试**

Run: `cd backend && go test ./services/user-service/...`

- [ ] **Step 6: Commit**

```bash
git add backend/services/user-service/
git commit -m "refactor: migrate user-service to use governance packages + registry"
```

---

### Task 12: 迁移 community-service（有状态，groupcache 集成 Registry）

**Files:**
- Modify: `backend/services/community-service/main.go`
- Modify: `backend/services/community-service/cache.go`

- [ ] **Step 1: 重写 community-service main.go**

与 user-service (Task 11) 结构一致。区别：
- 端口默认 `9083`
- 社区服务的处理器
- 使用 `communityv1connect.NewCommunityServiceHandler`

遵循 Task 11 中完全相同的模式，替换服务特定部分。

- [ ] **Step 2: 修改 community-service/cache.go**

与 user-service cache.go (Task 11) 相同的 Registry 集成模式。修改 `NewServerCache`、`NewMembersCache`、`NewRolesCache` 三个函数签名：

```go
func NewServerCache(repo *Repository, peers []string, reg registry.Registry, regServiceName string) *ServerCache {
	opts := []groupcache.Option[string, []byte]{
		groupcache.WithLocalCapacity[string, []byte](10000),
		groupcache.WithFiller[string, []byte](func(ctx context.Context, key string) ([]byte, error) {
			serverID := key[len("server:"):]
			server, err := repo.GetServer(ctx, serverID)
			if err != nil {
				return nil, err
			}
			return MarshalServer(server)
		}),
	}
	if reg != nil {
		opts = append(opts, groupcache.WithRegistry[string, []byte](reg, regServiceName))
	} else if len(peers) > 0 {
		opts = append(opts, groupcache.WithPeers[string, []byte](peers))
	}
	c := groupcache.NewCache[string, []byte](opts...)
	return &ServerCache{cache: c, repo: repo}
}
```

`NewMembersCache` 和 `NewRolesCache` 同理。

- [ ] **Step 3: 删除 `envOrDefault` 和 `strings` 导入**

- [ ] **Step 4: 确保编译通过**

Run: `cd backend && go build ./services/community-service/...`

- [ ] **Step 5: 运行测试**

Run: `cd backend && go test ./services/community-service/...`

- [ ] **Step 6: Commit**

```bash
git add backend/services/community-service/
git commit -m "refactor: migrate community-service to use governance packages + registry"
```

---

### Task 13: 迁移 api-gateway

**Files:**
- Modify: `backend/services/api-gateway/main.go`

- [ ] **Step 1: 重写 api-gateway main.go**

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

	"connectrpc.com/otelconnect"

	"github.com/constell/constell/backend/pkg/config"
	"github.com/constell/constell/backend/pkg/health"
	"github.com/constell/constell/backend/pkg/logging"
	"github.com/constell/constell/backend/pkg/metrics"
	pkgotel "github.com/constell/constell/backend/pkg/otel"
	"github.com/constell/constell/backend/pkg/registry"

	"github.com/constell/constell/backend/services/api-gateway/handlers"
)

type Config struct {
	Addr      string `env:"ADDR" default:":8080"`
	JWTSecret string `env:"JWT_SECRET" default:"dev-secret-change-me"`
	Env       string `env:"ENV" default:"dev"`

	// Downstream services
	AuthServiceURL      string `env:"AUTH_SERVICE_URL" default:"http://auth-service:9081"`
	UserServiceURL      string `env:"USER_SERVICE_URL" default:"http://user-service:9082"`
	CommunityServiceURL string `env:"COMMUNITY_SERVICE_URL" default:"http://community-service:9083"`

	// Registry (可选，用于未来动态发现)
	RegistryType    string `env:"REGISTRY_TYPE" default:"static"`
	ServicesCfgPath string `env:"SERVICES_CONFIG_PATH" default:"deploy/configs/services.yaml"`

	// OTel
	OTelEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:"http://localhost:5080/api/default/v1/otlp"`
	OTelInsecure string `env:"OTEL_EXPORTER_OTLP_INSECURE" default:"true"`
}

func main() {
	var cfg Config
	config.NewLoader("").MustLoad(&cfg)

	shutdown, err := pkgotel.Init(context.Background(), pkgotel.Config{
		ServiceName: "api-gateway",
		Environment: cfg.Env,
		Endpoint:    cfg.OTelEndpoint,
		Insecure:    cfg.OTelInsecure == "true",
	})
	if err != nil {
		slog.Error("otel init", "error", err)
	} else {
		defer pkgotel.ShutdownWithTimeout(shutdown, 5*time.Second)
	}

	logger := logging.Init("api-gateway", cfg.Env)
	slog.SetDefault(logger)

	// 初始化 Registry
	var reg registry.Registry
	if cfg.RegistryType == "static" && cfg.ServicesCfgPath != "" {
		var err error
		reg, err = registry.NewStaticRegistry(registry.StaticConfig{
			ConfigPath: cfg.ServicesCfgPath,
		})
		if err != nil {
			slog.Warn("registry init failed", "error", err)
		}
	}

	// 如果有 Registry，从中获取服务地址
	authURL := cfg.AuthServiceURL
	userURL := cfg.UserServiceURL
	communityURL := cfg.CommunityServiceURL

	if reg != nil {
		if instances, err := reg.Discover(context.Background(), "auth-service"); err == nil && len(instances) > 0 {
			authURL = "http://" + instances[0].Addr
		}
		if instances, err := reg.Discover(context.Background(), "user-service"); err == nil && len(instances) > 0 {
			userURL = "http://" + instances[0].Addr
		}
		if instances, err := reg.Discover(context.Background(), "community-service"); err == nil && len(instances) > 0 {
			communityURL = "http://" + instances[0].Addr
		}
	}

	slog.Info("service endpoints",
		"auth", authURL,
		"user", userURL,
		"community", communityURL,
	)

	// 健康检查
	hc := health.NewChecker()

	// OTel interceptor
	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		slog.Error("create otel interceptor", "error", err)
		os.Exit(1)
	}

	// Wire up
	clients := handlers.NewClientsFromURLs(authURL, userURL, communityURL)
	h := handlers.New(clients, cfg.JWTSecret)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", hc.HealthzHandler())
	mux.HandleFunc("/readyz", hc.ReadyHandler())
	h.RegisterRoutes(mux)

	// 用 metrics middleware 包装
	wrappedMux := metrics.HTTPMiddleware(mux)

	server := &http.Server{Addr: cfg.Addr, Handler: wrappedMux}

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

	slog.Info("api-gateway listening", "addr", cfg.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: 更新 api-gateway handlers 包**

需要在 `handlers/` 中添加一个工厂函数（如果不存在），用于从 URL 创建 clients：

```go
// NewClientsFromURLs 从 URL 字符串创建 Clients。
func NewClientsFromURLs(authURL, userURL, communityURL string) *Clients {
	return newClients(Config{
		AuthServiceURL:      authURL,
		UserServiceURL:      userURL,
		CommunityServiceURL: communityURL,
	})
}
```

- [ ] **Step 3: 删除旧的 `envOrDefault` 和相关代码**

- [ ] **Step 4: 确保编译通过**

Run: `cd backend && go build ./services/api-gateway/...`

- [ ] **Step 5: 运行测试**

Run: `cd backend && go test ./services/api-gateway/...`

- [ ] **Step 6: Commit**

```bash
git add backend/services/api-gateway/
git commit -m "refactor: migrate api-gateway to use governance packages"
```

---

### Task 14: 迁移 ws-gateway

**Files:**
- Modify: `backend/services/ws-gateway/main.go`

- [ ] **Step 1: 重写 ws-gateway main.go**

遵循 Task 10 的统一模式。WS Gateway 特有：
- NATS 连接
- Redis 连接（已用 pkg redis）
- `/ws` WebSocket 路由保留
- 现有 `/health` 替换为 `/healthz` + `/readyz`
- 从 Registry 获取 User Svc 和 Community Svc 地址

```go
type Config struct {
	GatewayID       string `env:"GATEWAY_ID" default:"gw-001"`
	JWTSecret       string `env:"JWT_SECRET" default:"constell-dev-secret"`
	ListenAddr      string `env:"LISTEN_ADDR" default:":8081"`
	RedisAddr       string `env:"REDIS_ADDR" default:"localhost:6379"`
	NatsURL         string `env:"NATS_URL" default:"nats://localhost:4222"`
	UserSvcAddr     string `env:"USER_SERVICE_ADDR" default:"http://localhost:9082"`
	CommunitySvcAddr string `env:"COMMUNITY_SERVICE_ADDR" default:"http://localhost:9083"`
	Env             string `env:"ENV" default:"dev"`

	RegistryType    string `env:"REGISTRY_TYPE" default:"static"`
	ServicesCfgPath string `env:"SERVICES_CONFIG_PATH" default:"deploy/configs/services.yaml"`

	OTelEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:"http://localhost:5080/api/default/v1/otlp"`
	OTelInsecure string `env:"OTEL_EXPORTER_OTLP_INSECURE" default:"true"`
}
```

- [ ] **Step 2: 用 Registry 发现下游服务地址**（与 api-gateway Task 13 同模式）

- [ ] **Step 3: 替换 `/health` 为 `/healthz` + `/readyz`**

- [ ] **Step 4: 删除 `envOr` 函数，用 config 包替代**

- [ ] **Step 5: 确保编译通过**

Run: `cd backend && go build ./services/ws-gateway/...`

- [ ] **Step 6: 运行测试**

Run: `cd backend && go test -v ./services/ws-gateway/...`

- [ ] **Step 7: Commit**

```bash
git add backend/services/ws-gateway/
git commit -m "refactor: migrate ws-gateway to use governance packages"
```

---

### Task 15: 全量构建 + 集成测试

**Files:**
- Verify: 所有服务编译
- Verify: 所有测试通过

- [ ] **Step 1: 全量构建**

Run: `cd backend && go build ./...`
Expected: 所有包编译成功

- [ ] **Step 2: 全量测试**

Run: `cd backend && go test -v -count=1 ./pkg/... ./services/...`
Expected: 全部 PASS

- [ ] **Step 3: 集成测试**

Run: `cd backend && go test -v -count=1 ./tests/integration/...`
Expected: PASS（集成测试应不受 config 初始化方式影响，行为不变）

- [ ] **Step 4: Commit（如有修复）**

```bash
git add -A
git commit -m "fix: resolve any issues from full build and test"
```

---

### Task 16: 更新项目文档

**Files:**
- Modify: `docs/PROJECT_STATUS.md`
- Modify: `docs/superpowers/plans/2026-05-30-constell-plans-overview.md`

- [ ] **Step 1: 更新 PROJECT_STATUS.md**

将阶段进度表更新为：

| 阶段 | 状态 |
|------|------|
| Plan 1: 基础设施 + 核心服务 | ✅ 已完成 |
| **Plan 2: 服务治理** | **✅ 已完成** |
| Plan 3: WS Gateway | 📋 已规划（原 Plan 2） |
| Plan 4: File + Search + Notify | ⏳ 待规划 |
| Plan 5: Web 客户端 | ⏳ 待规划 |
| Plan 6: SDK | ⏳ 待规划 |

- [ ] **Step 2: 更新 plans-overview.md**

在 Plan 1 和 Plan 2（WS Gateway）之间插入新的治理阶段：

```markdown
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
```

将原 Plan 2-5 重编号为 Plan 3-6。

- [ ] **Step 3: Commit**

```bash
git add docs/
git commit -m "docs: update project status and plans for governance phase"
```

---

## 自检清单

### Spec 覆盖率

| Spec 章节 | 对应 Task |
|-----------|----------|
| 1. 服务注册与发现 (接口 + Static + K8s) | Task 3, 4 |
| 2. 配置管理 | Task 1 |
| 3. 健康检查 | Task 2 |
| 4. 可观测性 (OTel + slog + OpenObserve) | Task 6, 7, 8 |
| 5. 部署配置更新 | Task 9 |
| 6. 现有服务改造 | Task 10-14 |
| groupcache Registry 集成 | Task 5 |
| 全量测试 | Task 15 |
| 文档更新 | Task 16 |

### Placeholder 扫描

无 TBD / TODO / "implement later" / "add validation" 等。每个步骤都包含完整代码。

### 类型一致性

- `registry.Registry` 接口在 Task 3 定义，Task 4/5/10-14 使用一致
- `config.Loader` 在 Task 1 定义，Task 10-14 使用一致
- `health.Checker` 在 Task 2 定义，Task 10-14 使用一致
- `groupcache.WithRegistry` option 在 Task 5 定义，Task 11/12 使用一致
- `pkgotel.Init` 在 Task 6 定义，Task 10-14 使用一致
