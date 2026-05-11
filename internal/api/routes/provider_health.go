package routes

import (
	"context"
	"time"

	"github.com/StacyOs/stacyvm/internal/providers"
)

func collectProviderHealth(ctx context.Context, registry *providers.Registry) []ProviderHealth {
	names := registry.List()
	out := make([]ProviderHealth, 0, len(names))
	defaultProvider := registry.Default()

	for _, name := range names {
		start := time.Now()
		checkedAt := time.Now().UTC()
		item := ProviderHealth{
			Name:         name,
			Default:      name == defaultProvider,
			LastChecked:  checkedAt.Format(time.RFC3339),
			Capabilities: []string{"spawn", "exec", "exec_stream", "files", "console", "health"},
		}

		prov, err := registry.Get(name)
		if err != nil {
			item.Healthy = false
			item.Error = err.Error()
			item.LatencyMS = time.Since(start).Milliseconds()
			out = append(out, item)
			continue
		}

		item.Healthy = prov.Healthy(ctx)
		item.LatencyMS = time.Since(start).Milliseconds()
		if !item.Healthy {
			item.Error = "health check returned false"
		}
		item.Capabilities = append(item.Capabilities, providerCapabilities(prov)...)

		if lister, ok := prov.(providers.RuntimeSandboxLister); ok {
			runtimes, err := lister.ListRuntimeSandboxes(ctx)
			if err != nil {
				if item.Error == "" {
					item.Error = "runtime inventory: " + err.Error()
				}
			} else {
				count := len(runtimes)
				item.RuntimeCount = &count
			}
		}

		out = append(out, item)
	}

	return out
}

func providerCapabilities(prov providers.Provider) []string {
	capabilities := make([]string, 0, 4)
	if _, ok := prov.(providers.RuntimeSandboxLister); ok {
		capabilities = append(capabilities, "runtime_inventory")
	}
	if _, ok := prov.(providers.SnapshotLister); ok {
		capabilities = append(capabilities, "snapshots")
	}
	switch prov.(type) {
	case *providers.FirecrackerProvider:
		capabilities = append(capabilities, "microvm", "vsock_agent")
	case *providers.DockerProvider:
		capabilities = append(capabilities, "container")
	case *providers.PRootProvider:
		capabilities = append(capabilities, "userspace_isolation")
	case *providers.CustomProvider:
		capabilities = append(capabilities, "remote_http")
	case *providers.E2BProvider:
		capabilities = append(capabilities, "remote_e2b")
	case *providers.MockProvider:
		capabilities = append(capabilities, "test_provider")
	}
	return capabilities
}
