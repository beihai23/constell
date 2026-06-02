package registry

import (
	"context"
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// StaticRegistry loads a fixed list of service instances from a YAML config file.
type StaticRegistry struct {
	mu       sync.RWMutex
	services map[string][]Instance
	self     *Instance
}

type staticRegistryConfig struct {
	Services map[string]struct {
		Instances []struct {
			Addr     string            `yaml:"addr"`
			Metadata map[string]string `yaml:"metadata,omitempty"`
		} `yaml:"instances"`
	} `yaml:"services"`
}

// NewStaticRegistry loads service instances from a YAML file.
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

// Register is a no-op for static registry.
func (r *StaticRegistry) Register(_ context.Context, inst Instance) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.self = &inst
	return nil
}

// Deregister is a no-op for static registry.
func (r *StaticRegistry) Deregister(_ context.Context) error {
	return nil
}

// Discover returns the instance list for a service.
func (r *StaticRegistry) Discover(_ context.Context, serviceName string) ([]Instance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instances, ok := r.services[serviceName]
	if !ok {
		return nil, fmt.Errorf("registry: service %q not found", serviceName)
	}
	result := make([]Instance, len(instances))
	copy(result, instances)
	return result, nil
}

// Watch returns a channel that emits the instance list once, then never changes.
func (r *StaticRegistry) Watch(ctx context.Context, serviceName string) (<-chan []Instance, error) {
	instances, err := r.Discover(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	ch := make(chan []Instance, 1)
	ch <- instances
	return ch, nil
}

// SelfAddr returns the address of the current instance (set via Register).
func (r *StaticRegistry) SelfAddr() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.self == nil {
		return ""
	}
	return r.self.Addr
}
