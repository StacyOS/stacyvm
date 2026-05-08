package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/StacyOs/stacyvm/internal/config"
	"github.com/spf13/cobra"
)

type doctorStatus string

const (
	doctorPass doctorStatus = "PASS"
	doctorWarn doctorStatus = "WARN"
	doctorFail doctorStatus = "FAIL"
)

type doctorCheck struct {
	Name    string
	Status  doctorStatus
	Message string
}

func newDoctorCmd() *cobra.Command {
	var production bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run local production-readiness diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			checks := runDoctor(cmd.Context(), production)
			failed := printDoctorChecks(checks)
			if failed > 0 {
				return fmt.Errorf("doctor found %d failing check(s)", failed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&production, "production", false, "treat production hardening warnings as failures")
	return cmd
}

func runDoctor(ctx context.Context, production bool) []doctorCheck {
	var checks []doctorCheck

	cfg, err := config.Load()
	if err != nil {
		return []doctorCheck{{
			Name:    "config",
			Status:  doctorFail,
			Message: err.Error(),
		}}
	}

	checks = append(checks,
		checkConfig(cfg, production)...,
	)
	checks = append(checks,
		checkDocker(ctx, cfg)...,
	)
	checks = append(checks,
		checkFirecracker(cfg)...,
	)
	checks = append(checks,
		checkPRoot(cfg)...,
	)
	return checks
}

func checkConfig(cfg *config.Config, production bool) []doctorCheck {
	checks := []doctorCheck{
		{Name: "config", Status: doctorPass, Message: "loaded successfully"},
	}

	if cfg.Auth.APIKey == "" {
		checks = append(checks, doctorCheck{Name: "auth.api_key", Status: severityForProduction(production), Message: "missing API key; endpoints are unauthenticated"})
	} else if len(cfg.Auth.APIKey) < 32 {
		checks = append(checks, doctorCheck{Name: "auth.api_key", Status: severityForProduction(production), Message: "API key is shorter than the recommended 32 bytes"})
	} else {
		checks = append(checks, doctorCheck{Name: "auth.api_key", Status: doctorPass, Message: "configured"})
	}

	if cfg.Auth.AdminAPIKey == "" {
		checks = append(checks, doctorCheck{Name: "auth.admin_api_key", Status: severityForProduction(production), Message: "missing dedicated admin API key"})
	} else if cfg.Auth.AdminAPIKey == cfg.Auth.APIKey {
		checks = append(checks, doctorCheck{Name: "auth.admin_api_key", Status: severityForProduction(production), Message: "admin API key matches regular API key"})
	} else {
		checks = append(checks, doctorCheck{Name: "auth.admin_api_key", Status: doctorPass, Message: "configured separately"})
	}

	if cfg.Auth.AdminFallbackEnabled {
		checks = append(checks, doctorCheck{Name: "auth.admin_fallback_enabled", Status: severityForProduction(production), Message: "admin fallback is enabled; production should require a dedicated admin key"})
	} else {
		checks = append(checks, doctorCheck{Name: "auth.admin_fallback_enabled", Status: doctorPass, Message: "disabled"})
	}

	dbDir := filepath.Dir(cfg.Database.Path)
	if dbDir == "." || dbDir == "" {
		checks = append(checks, doctorCheck{Name: "database.path", Status: doctorWarn, Message: "database path is relative; production should use persistent storage"})
	} else if info, err := os.Stat(dbDir); err != nil {
		checks = append(checks, doctorCheck{Name: "database.path", Status: severityForProduction(production), Message: fmt.Sprintf("database directory unavailable: %v", err)})
	} else if !info.IsDir() {
		checks = append(checks, doctorCheck{Name: "database.path", Status: doctorFail, Message: "database parent path is not a directory"})
	} else {
		checks = append(checks, doctorCheck{Name: "database.path", Status: doctorPass, Message: dbDir})
	}

	return checks
}

func checkDocker(ctx context.Context, cfg *config.Config) []doctorCheck {
	if !cfg.Providers.Docker.Enabled && cfg.Providers.Default != "docker" {
		return []doctorCheck{{Name: "docker", Status: doctorWarn, Message: "Docker provider is not enabled/default"}}
	}

	var checks []doctorCheck
	if _, err := exec.LookPath("docker"); err != nil {
		return []doctorCheck{{Name: "docker.cli", Status: doctorFail, Message: "docker CLI not found in PATH"}}
	}
	checks = append(checks, doctorCheck{Name: "docker.cli", Status: doctorPass, Message: "found"})

	if out, err := runDoctorCommand(ctx, 3*time.Second, "docker", "info", "--format", "{{.ServerVersion}}"); err != nil {
		checks = append(checks, doctorCheck{Name: "docker.daemon", Status: doctorFail, Message: strings.TrimSpace(err.Error() + " " + out)})
	} else {
		checks = append(checks, doctorCheck{Name: "docker.daemon", Status: doctorPass, Message: "server " + strings.TrimSpace(out)})
	}

	if cfg.Providers.Docker.NetworkMode == "" {
		checks = append(checks, doctorCheck{Name: "docker.network_mode", Status: doctorWarn, Message: "empty network mode; explicit mode is recommended"})
	} else {
		checks = append(checks, doctorCheck{Name: "docker.network_mode", Status: doctorPass, Message: cfg.Providers.Docker.NetworkMode})
	}
	if len(cfg.Providers.Docker.DroppedCaps) == 0 {
		checks = append(checks, doctorCheck{Name: "docker.dropped_caps", Status: doctorWarn, Message: "no dropped capabilities configured"})
	} else {
		checks = append(checks, doctorCheck{Name: "docker.dropped_caps", Status: doctorPass, Message: strings.Join(cfg.Providers.Docker.DroppedCaps, ",")})
	}
	return checks
}

func checkFirecracker(cfg *config.Config) []doctorCheck {
	if !cfg.Providers.Firecracker.Enabled && cfg.Providers.Default != "firecracker" {
		return []doctorCheck{{Name: "firecracker", Status: doctorWarn, Message: "Firecracker provider is not enabled/default"}}
	}

	checks := []doctorCheck{}
	if _, err := exec.LookPath(filepath.Base(cfg.Providers.Firecracker.FirecrackerPath)); err != nil {
		if _, statErr := os.Stat(cfg.Providers.Firecracker.FirecrackerPath); statErr != nil {
			checks = append(checks, doctorCheck{Name: "firecracker.binary", Status: doctorFail, Message: "Firecracker binary unavailable"})
		} else {
			checks = append(checks, doctorCheck{Name: "firecracker.binary", Status: doctorPass, Message: cfg.Providers.Firecracker.FirecrackerPath})
		}
	} else {
		checks = append(checks, doctorCheck{Name: "firecracker.binary", Status: doctorPass, Message: "found in PATH"})
	}

	checks = append(checks, fileCheck("firecracker.kvm", "/dev/kvm", false))
	checks = append(checks, fileCheck("firecracker.kernel", cfg.Providers.Firecracker.KernelPath, false))
	checks = append(checks, fileCheck("firecracker.agent", cfg.Providers.Firecracker.AgentPath, false))
	return checks
}

func checkPRoot(cfg *config.Config) []doctorCheck {
	if !cfg.Providers.PRoot.Enabled && cfg.Providers.Default != "proot" {
		return []doctorCheck{{Name: "proot", Status: doctorWarn, Message: "PRoot provider is not enabled/default"}}
	}

	var checks []doctorCheck
	if _, err := exec.LookPath(cfg.Providers.PRoot.PRootBinary); err != nil {
		checks = append(checks, doctorCheck{Name: "proot.binary", Status: doctorFail, Message: "proot binary unavailable"})
	} else {
		checks = append(checks, doctorCheck{Name: "proot.binary", Status: doctorPass, Message: cfg.Providers.PRoot.PRootBinary})
	}
	checks = append(checks, fileCheck("proot.rootfs", cfg.Providers.PRoot.RootfsPath, true))
	checks = append(checks, fileCheck("proot.workspace_base", cfg.Providers.PRoot.WorkspaceBase, true))
	return checks
}

func fileCheck(name, path string, wantDir bool) doctorCheck {
	if strings.TrimSpace(path) == "" {
		return doctorCheck{Name: name, Status: doctorFail, Message: "path is empty"}
	}
	info, err := os.Stat(path)
	if err != nil {
		return doctorCheck{Name: name, Status: doctorFail, Message: err.Error()}
	}
	if wantDir && !info.IsDir() {
		return doctorCheck{Name: name, Status: doctorFail, Message: "path is not a directory"}
	}
	return doctorCheck{Name: name, Status: doctorPass, Message: path}
}

func severityForProduction(production bool) doctorStatus {
	if production {
		return doctorFail
	}
	return doctorWarn
}

func runDoctorCommand(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := exec.CommandContext(cmdCtx, name, args...).CombinedOutput()
	return string(out), err
}

func printDoctorChecks(checks []doctorCheck) int {
	failed := 0
	for _, check := range checks {
		if check.Status == doctorFail {
			failed++
		}
		fmt.Printf("[%s] %s: %s\n", check.Status, check.Name, check.Message)
	}
	return failed
}
