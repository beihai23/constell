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

	select {
	case instances := <-ch:
		if len(instances) != 2 {
			t.Fatalf("Watch: expected 2 instances, got %d", len(instances))
		}
	default:
		t.Fatal("Watch: expected immediate value in channel")
	}

	select {
	case <-ch:
		t.Fatal("Watch: unexpected second value in channel")
	default:
		// correct: channel is empty
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
