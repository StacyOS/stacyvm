package tui

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestDashboardRenders verifies the Mission Control dashboard renders the full
// chrome + modules with seed data and without panicking at the reference size.
func TestDashboardRenders(t *testing.T) {
	m := NewModel("http://localhost:7423", "")
	m.width, m.height = 198, 52
	m.booting = false
	m.activeTab = tabDashboard
	exp := time.Now().Add(24 * time.Minute)
	m.sandboxes = []sandboxData{
		{ID: "sb-7f3a91", State: "running", Provider: "docker", Image: "python:3.12", ExpiresAt: exp},
		{ID: "sb-9d11ba", State: "creating", Provider: "firecracker", Image: "alpine:latest", ExpiresAt: exp},
		{ID: "sb-1e90cf", State: "idle", Provider: "docker", Image: "rust:1.78", ExpiresAt: exp},
	}
	m.providerList = []providerData{
		{Name: "docker", IsDefault: true, Healthy: true, LatencyMS: 12},
		{Name: "firecracker", Healthy: false, LatencyMS: 8},
	}
	m.host = hostSnapshot{cpuPct: 34, memPct: 61, diskPct: 22, netRxBps: 600000, netTxBps: 600000, load1: 0.9, ok: true}
	m.sbStats["sb-7f3a91"] = sandboxStat{cpuPct: 71, memBytes: 100, memLimit: 1000, supported: true}
	m.health = &healthData{Status: "ok", Version: "0.9.2", Uptime: "4h2m"}

	out := m.View()
	if path := os.Getenv("STACY_SNAPSHOT"); path != "" {
		_ = os.WriteFile(path, []byte(out), 0o644)
	}
	for _, want := range []string{
		"STACYVM", "DASH", "SANDBOXES", "ACTIVE SANDBOXES",
		"HOST TELEMETRY", "PROVIDERS", "EVENT STREAM",
		"sb-7f3a91", "ONLINE",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dashboard render missing %q", want)
		}
	}
	if strings.Count(out, "\n") < 20 {
		t.Errorf("dashboard render unexpectedly short:\n%s", out)
	}
}
