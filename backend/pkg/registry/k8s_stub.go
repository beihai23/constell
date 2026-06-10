//go:build !k8s

package registry

import (
	"context"
	"fmt"
)

// K8sConfig is unavailable in non-k8s builds.
type K8sConfig struct{}

func newK8sRegistry(_ context.Context, _ Config) (Registry, error) {
	return nil, fmt.Errorf("registry: k8s support not compiled; rebuild with -tags k8s")
}
