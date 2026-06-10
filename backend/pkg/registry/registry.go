package registry

import (
	"context"
)

// Instance represents a service instance.
type Instance struct {
	ServiceName string
	Addr        string
	Metadata    map[string]string
}

// Registry is the service registration and discovery interface.
type Registry interface {
	Register(ctx context.Context, inst Instance) error
	Deregister(ctx context.Context) error
	Discover(ctx context.Context, serviceName string) ([]Instance, error)
	Watch(ctx context.Context, serviceName string) (<-chan []Instance, error)
}

// Config holds configuration for creating a Registry.
type Config struct {
	Type   string       // "static" or "k8s"
	Static StaticConfig
}

// StaticConfig is configuration for StaticRegistry.
type StaticConfig struct {
	ConfigPath  string // path to services.yaml
	ServiceName string // current service name
}

// NewRegistry creates a Registry implementation based on config.
func NewRegistry(ctx context.Context, cfg Config) (Registry, error) {
	switch cfg.Type {
	case "k8s":
		return newK8sRegistry(ctx, cfg)
	default:
		return NewStaticRegistry(cfg.Static)
	}
}
