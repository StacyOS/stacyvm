package providers

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestFirecrackerProviderName(t *testing.T) {
	p := NewFirecrackerProvider(FirecrackerProviderConfig{}, zerolog.Nop())
	if p.Name() != "firecracker" {
		t.Errorf("Name() = %q, want %q", p.Name(), "firecracker")
	}
}

func TestSandboxIDFormat(t *testing.T) {
	for range 100 {
		id := generateSandboxID()
		if !strings.HasPrefix(id, "sb-") {
			t.Errorf("sandbox ID %q does not have sb- prefix", id)
		}
		// sb- + 8 hex chars = 11 total
		if len(id) != 11 {
			t.Errorf("sandbox ID %q has length %d, want 11", id, len(id))
		}
	}
}

func TestSandboxIDUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for range 1000 {
		id := generateSandboxID()
		if seen[id] {
			t.Errorf("duplicate sandbox ID: %s", id)
		}
		seen[id] = true
	}
}

func TestCIDAllocation(t *testing.T) {
	p := NewFirecrackerProvider(FirecrackerProviderConfig{}, zerolog.Nop())

	// CIDs should start at 3 and increment.
	cid1 := p.nextCID.Add(1) - 1
	cid2 := p.nextCID.Add(1) - 1
	cid3 := p.nextCID.Add(1) - 1

	if cid1 != 3 {
		t.Errorf("first CID = %d, want 3", cid1)
	}
	if cid2 != 4 {
		t.Errorf("second CID = %d, want 4", cid2)
	}
	if cid3 != 5 {
		t.Errorf("third CID = %d, want 5", cid3)
	}
}

func TestRequestIDUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for range 1000 {
		id := generateRequestID()
		if seen[id] {
			t.Errorf("duplicate request ID: %s", id)
		}
		seen[id] = true
		// 8 bytes = 16 hex chars
		if len(id) != 16 {
			t.Errorf("request ID %q has length %d, want 16", id, len(id))
		}
	}
}

func TestHealthyMissingBinaries(t *testing.T) {
	p := NewFirecrackerProvider(FirecrackerProviderConfig{
		FirecrackerPath: "/nonexistent/firecracker",
		KernelPath:      "/nonexistent/vmlinux.bin",
	}, zerolog.Nop())

	ctx := t.Context()
	if p.Healthy(ctx) {
		t.Error("Healthy() = true with missing binaries, want false")
	}
}

func TestGetVMNotFound(t *testing.T) {
	p := NewFirecrackerProvider(FirecrackerProviderConfig{}, zerolog.Nop())
	_, err := p.getVM("sb-nonexistent")
	if err == nil {
		t.Error("getVM should return error for nonexistent sandbox")
	}
}

func TestDestroyNotFound(t *testing.T) {
	p := NewFirecrackerProvider(FirecrackerProviderConfig{}, zerolog.Nop())
	ctx := t.Context()
	err := p.Destroy(ctx, "sb-nonexistent")
	if err == nil {
		t.Error("Destroy should return error for nonexistent sandbox")
	}
}

func TestStatusNotFound(t *testing.T) {
	p := NewFirecrackerProvider(FirecrackerProviderConfig{}, zerolog.Nop())
	ctx := t.Context()
	_, err := p.Status(ctx, "sb-nonexistent")
	if err == nil {
		t.Error("Status should return error for nonexistent sandbox")
	}
}

func TestFirecrackerProvider_Integration_Conformance(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("firecracker conformance requires Linux")
	}
	if _, err := os.Stat("/dev/kvm"); err != nil {
		t.Skip("firecracker conformance requires /dev/kvm")
	}

	firecrackerPath := os.Getenv("STACYVM_FIRECRACKER_PATH")
	if firecrackerPath == "" {
		var err error
		firecrackerPath, err = exec.LookPath("firecracker")
		if err != nil {
			t.Skip("firecracker binary not found; set STACYVM_FIRECRACKER_PATH")
		}
	}
	kernelPath := os.Getenv("STACYVM_KERNEL_PATH")
	if kernelPath == "" {
		t.Skip("set STACYVM_KERNEL_PATH to run firecracker conformance")
	}
	rootfsPath := os.Getenv("STACYVM_ROOTFS_PATH")
	if rootfsPath == "" {
		t.Skip("set STACYVM_ROOTFS_PATH to run firecracker conformance")
	}
	agentPath := os.Getenv("STACYVM_AGENT_PATH")
	if agentPath == "" {
		agentPath = "./bin/stacyvm-agent"
	}
	for name, path := range map[string]string{
		"kernel": kernelPath,
		"rootfs": rootfsPath,
		"agent":  agentPath,
	} {
		if _, err := os.Stat(path); err != nil {
			t.Skipf("%s path %q is unavailable: %v", name, path, err)
		}
	}

	runProviderConformance(t, func(t *testing.T) Provider {
		t.Helper()
		return NewFirecrackerProvider(FirecrackerProviderConfig{
			FirecrackerPath: firecrackerPath,
			KernelPath:      kernelPath,
			DefaultRootfs:   rootfsPath,
			AgentPath:       agentPath,
			DataDir:         t.TempDir(),
			DefaultMemoryMB: 256,
		}, zerolog.Nop())
	})
}
