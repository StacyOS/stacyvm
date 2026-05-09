package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/StacyOs/stacyvm/internal/config"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and validate StacyVM configuration",
	}
	cmd.AddCommand(newConfigLintCmd())
	return cmd
}

func newConfigLintCmd() *cobra.Command {
	var file string
	var production bool
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Lint configuration for operational safety",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadLintConfig(file)
			if err != nil {
				return err
			}
			checks := lintConfig(cfg, production)
			failed := printDoctorChecks(checks)
			if failed > 0 {
				return fmt.Errorf("config lint found %d failing check(s)", failed)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "config file to lint; defaults to normal StacyVM config lookup")
	cmd.Flags().BoolVar(&production, "production", false, "treat production hardening warnings as failures")
	return cmd
}

func loadLintConfig(file string) (*config.Config, error) {
	if strings.TrimSpace(file) == "" {
		return config.Load()
	}
	return config.LoadFile(file)
}

func lintConfig(cfg *config.Config, production bool) []doctorCheck {
	checks := []doctorCheck{
		{Name: "config", Status: doctorPass, Message: "syntax and schema validation passed"},
	}
	checks = append(checks, lintAuthConfig(cfg, production)...)
	checks = append(checks, lintRateLimitConfig(cfg, production)...)
	checks = append(checks, lintDatabaseConfig(cfg, production)...)
	checks = append(checks, lintRuntimeLimits(cfg, production)...)
	checks = append(checks, lintLoggingConfig(cfg, production)...)
	checks = append(checks, lintWorkerRPCConfig(cfg, production)...)
	checks = append(checks, lintProviderConfig(cfg, production)...)
	return checks
}

func lintAuthConfig(cfg *config.Config, production bool) []doctorCheck {
	var checks []doctorCheck
	if !cfg.Auth.Enabled {
		checks = append(checks, doctorCheck{Name: "auth.enabled", Status: severityForProduction(production), Message: "authentication is disabled", Remediation: "Set auth.enabled=true before exposing StacyVM beyond a trusted local network."})
	} else {
		checks = append(checks, doctorCheck{Name: "auth.enabled", Status: doctorPass, Message: "enabled"})
	}

	checks = append(checks, lintSecret("auth.api_key", cfg.Auth.APIKey, production, "Set STACYVM_AUTH_API_KEY or auth.api_key to a random 32+ byte secret."))
	checks = append(checks, lintSecret("auth.admin_api_key", cfg.Auth.AdminAPIKey, production, "Set STACYVM_AUTH_ADMIN_API_KEY or auth.admin_api_key to a separate random 32+ byte secret."))
	if cfg.Auth.WorkerSigningKey != "" {
		checks = append(checks, lintSecret("auth.worker_signing_key", cfg.Auth.WorkerSigningKey, production, "Set STACYVM_AUTH_WORKER_SIGNING_KEY or auth.worker_signing_key to a random 32+ byte secret used to verify signed worker tokens."))
		checks = append(checks, lintWorkerSigningKeyRotation(cfg.Auth.WorkerSigningKey, cfg.Auth.WorkerSigningKeys)...)
		if len(cfg.Auth.WorkerRevokedTokenIDs) > 0 {
			checks = append(checks, doctorCheck{Name: "auth.worker_revoked_token_ids", Status: doctorPass, Message: fmt.Sprintf("%d revoked worker token id(s) configured", len(cfg.Auth.WorkerRevokedTokenIDs))})
		}
		if cfg.Auth.WorkerToken != "" {
			checks = append(checks, doctorCheck{Name: "auth.worker_token", Status: doctorWarn, Message: "shared worker token still configured with signed worker tokens", Remediation: "Remove auth.worker_token after workers and worker RPC clients use short-lived signed worker tokens."})
		}
		if len(cfg.Auth.WorkerTokens) > 0 {
			checks = append(checks, doctorCheck{Name: "auth.worker_tokens", Status: doctorWarn, Message: fmt.Sprintf("%d static per-worker token(s) still configured", len(cfg.Auth.WorkerTokens)), Remediation: "Prefer short-lived signed worker tokens for production workers; keep static worker tokens only during migration."})
		}
	} else if len(cfg.Auth.WorkerSigningKeys) > 0 {
		checks = append(checks, doctorCheck{Name: "auth.worker_signing_key", Status: severityForProduction(production), Message: "additional verification keys configured without a primary signing key", Remediation: "Set auth.worker_signing_key to the active signing key and keep old keys in auth.worker_signing_keys only during rotation."})
		checks = append(checks, lintWorkerRevokedTokenIDsWithoutSigningKey(cfg.Auth.WorkerRevokedTokenIDs)...)
	} else if len(cfg.Auth.WorkerTokens) > 0 {
		checks = append(checks, lintWorkerRevokedTokenIDsWithoutSigningKey(cfg.Auth.WorkerRevokedTokenIDs)...)
		checks = append(checks, doctorCheck{Name: "auth.worker_tokens", Status: doctorPass, Message: fmt.Sprintf("%d per-worker token(s) configured", len(cfg.Auth.WorkerTokens))})
	} else if cfg.Auth.WorkerToken != "" {
		checks = append(checks, lintWorkerRevokedTokenIDsWithoutSigningKey(cfg.Auth.WorkerRevokedTokenIDs)...)
		checks = append(checks, doctorCheck{Name: "auth.worker_tokens", Status: doctorWarn, Message: "using shared worker token", Remediation: "Configure auth.worker_tokens for production workers so each worker has an individually rotatable credential."})
	} else {
		checks = append(checks, lintWorkerRevokedTokenIDsWithoutSigningKey(cfg.Auth.WorkerRevokedTokenIDs)...)
		checks = append(checks, doctorCheck{Name: "auth.worker_tokens", Status: doctorWarn, Message: "no worker credentials configured", Remediation: "Set auth.worker_token for staging, auth.worker_tokens for migration, or auth.worker_signing_key for production signed worker identity."})
	}
	if cfg.Auth.APIKey != "" && cfg.Auth.APIKey == cfg.Auth.AdminAPIKey {
		checks = append(checks, doctorCheck{Name: "auth.key_separation", Status: severityForProduction(production), Message: "regular and admin API keys match", Remediation: "Use separate keys so admin endpoints can be rotated and restricted independently."})
	} else {
		checks = append(checks, doctorCheck{Name: "auth.key_separation", Status: doctorPass, Message: "regular and admin keys are separate"})
	}
	if cfg.Auth.AdminFallbackEnabled {
		checks = append(checks, doctorCheck{Name: "auth.admin_fallback_enabled", Status: severityForProduction(production), Message: "admin fallback is enabled", Remediation: "Set auth.admin_fallback_enabled=false in production."})
	} else {
		checks = append(checks, doctorCheck{Name: "auth.admin_fallback_enabled", Status: doctorPass, Message: "disabled"})
	}
	retention, _ := time.ParseDuration(cfg.Auth.AdminAuditRetention)
	if retention <= 0 {
		checks = append(checks, doctorCheck{Name: "auth.admin_audit_retention", Status: severityForProduction(production), Message: "admin audit retention is disabled", Remediation: "Set auth.admin_audit_retention to a positive retention window such as 2160h."})
	} else {
		checks = append(checks, doctorCheck{Name: "auth.admin_audit_retention", Status: doctorPass, Message: retention.String()})
	}
	return checks
}

func lintWorkerRevokedTokenIDsWithoutSigningKey(tokenIDs []string) []doctorCheck {
	if len(tokenIDs) == 0 {
		return nil
	}
	return []doctorCheck{{
		Name:        "auth.worker_revoked_token_ids",
		Status:      doctorWarn,
		Message:     fmt.Sprintf("%d revoked worker token id(s) configured without signed worker tokens", len(tokenIDs)),
		Remediation: "Keep auth.worker_revoked_token_ids only alongside auth.worker_signing_key during an incident-response window.",
	}}
}

func lintWorkerSigningKeyRotation(primary string, keys []string) []doctorCheck {
	if len(keys) == 0 {
		return nil
	}
	primary = strings.TrimSpace(primary)
	seen := map[string]struct{}{}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if key == primary {
			return []doctorCheck{{Name: "auth.worker_signing_keys", Status: doctorWarn, Message: "rotation keys include the active worker signing key", Remediation: "Keep only previous verification keys in auth.worker_signing_keys; auth.worker_signing_key is already accepted."}}
		}
		if _, ok := seen[key]; ok {
			return []doctorCheck{{Name: "auth.worker_signing_keys", Status: doctorWarn, Message: "duplicate worker signing rotation key configured", Remediation: "Remove duplicate entries from auth.worker_signing_keys so rotation state is unambiguous."}}
		}
		seen[key] = struct{}{}
	}
	return []doctorCheck{{Name: "auth.worker_signing_keys", Status: doctorPass, Message: fmt.Sprintf("%d additional verification key(s) configured", len(seen))}}
}

func lintSecret(name, value string, production bool, remediation string) doctorCheck {
	if strings.TrimSpace(value) == "" {
		return doctorCheck{Name: name, Status: severityForProduction(production), Message: "missing secret", Remediation: remediation}
	}
	lower := strings.ToLower(value)
	if strings.Contains(lower, "change-me") || strings.Contains(lower, "changeme") || strings.Contains(lower, "replace-me") {
		return doctorCheck{Name: name, Status: severityForProduction(production), Message: "placeholder secret is still configured", Remediation: remediation}
	}
	if len(value) < 32 {
		return doctorCheck{Name: name, Status: severityForProduction(production), Message: "secret is shorter than 32 bytes", Remediation: remediation}
	}
	return doctorCheck{Name: name, Status: doctorPass, Message: "configured"}
}

func lintRateLimitConfig(cfg *config.Config, production bool) []doctorCheck {
	var checks []doctorCheck
	if !cfg.RateLimit.Enabled {
		checks = append(checks, doctorCheck{Name: "rate_limit.enabled", Status: severityForProduction(production), Message: "rate limiting is disabled", Remediation: "Set rate_limit.enabled=true and choose bounded request limits."})
	} else {
		checks = append(checks, doctorCheck{Name: "rate_limit.enabled", Status: doctorPass, Message: "enabled"})
	}
	if cfg.RateLimit.RequestsPerMinute <= 0 || cfg.RateLimit.Burst <= 0 {
		checks = append(checks, doctorCheck{Name: "rate_limit.capacity", Status: severityForProduction(production), Message: "requests_per_minute and burst must be positive", Remediation: "Set positive rate_limit.requests_per_minute and rate_limit.burst values."})
	} else {
		checks = append(checks, doctorCheck{Name: "rate_limit.capacity", Status: doctorPass, Message: fmt.Sprintf("%d rpm, burst %d", cfg.RateLimit.RequestsPerMinute, cfg.RateLimit.Burst)})
	}
	if cfg.RateLimit.KeyBy == "ip" {
		checks = append(checks, doctorCheck{Name: "rate_limit.key_by", Status: doctorWarn, Message: "IP-based limiting can collapse unrelated users behind NAT", Remediation: "Prefer rate_limit.key_by=api_key or owner for authenticated production deployments."})
	} else {
		checks = append(checks, doctorCheck{Name: "rate_limit.key_by", Status: doctorPass, Message: cfg.RateLimit.KeyBy})
	}
	return checks
}

func lintDatabaseConfig(cfg *config.Config, production bool) []doctorCheck {
	driver := strings.ToLower(strings.TrimSpace(cfg.Database.Driver))
	if driver == "" {
		driver = "sqlite"
	}
	if driver == "postgres" || driver == "postgresql" {
		if cfg.Database.DSN == "" {
			return []doctorCheck{{Name: "database.dsn", Status: doctorFail, Message: "postgres DSN is required"}}
		}
		return []doctorCheck{{Name: "database.driver", Status: doctorPass, Message: "postgres"}}
	}
	if filepath.IsAbs(cfg.Database.Path) {
		return []doctorCheck{{Name: "database.path", Status: doctorPass, Message: cfg.Database.Path}}
	}
	return []doctorCheck{{Name: "database.path", Status: severityForProduction(production), Message: "database path is relative", Remediation: "Use an absolute path on durable storage, for example /var/lib/stacyvm/stacyvm.db."}}
}

func lintRuntimeLimits(cfg *config.Config, production bool) []doctorCheck {
	var checks []doctorCheck
	if cfg.Defaults.MaxSandboxes <= 0 || cfg.Defaults.MaxSandboxesPerOwner <= 0 {
		checks = append(checks, doctorCheck{Name: "defaults.sandbox_caps", Status: severityForProduction(production), Message: "global and per-owner sandbox caps must be positive", Remediation: "Set defaults.max_sandboxes and defaults.max_sandboxes_per_owner to bounded production values."})
	} else {
		checks = append(checks, doctorCheck{Name: "defaults.sandbox_caps", Status: doctorPass, Message: fmt.Sprintf("global %d, per-owner %d", cfg.Defaults.MaxSandboxes, cfg.Defaults.MaxSandboxesPerOwner)})
	}
	if cfg.Defaults.MaxSpawnQueue <= 0 {
		checks = append(checks, doctorCheck{Name: "defaults.max_spawn_queue", Status: severityForProduction(production), Message: "spawn queue is unbounded or disabled by capacity", Remediation: "Set defaults.max_spawn_queue to a positive bounded value."})
	} else {
		checks = append(checks, doctorCheck{Name: "defaults.max_spawn_queue", Status: doctorPass, Message: fmt.Sprintf("%d", cfg.Defaults.MaxSpawnQueue)})
	}
	defaultExec, _ := time.ParseDuration(cfg.Defaults.DefaultExecTimeout)
	maxExec, _ := time.ParseDuration(cfg.Defaults.MaxExecTimeout)
	if defaultExec <= 0 || maxExec <= 0 {
		checks = append(checks, doctorCheck{Name: "defaults.exec_timeouts", Status: severityForProduction(production), Message: "exec timeouts should be positive", Remediation: "Set defaults.default_exec_timeout and defaults.max_exec_timeout to positive durations."})
	} else {
		checks = append(checks, doctorCheck{Name: "defaults.exec_timeouts", Status: doctorPass, Message: fmt.Sprintf("default %s, max %s", defaultExec, maxExec)})
	}
	maxTTL, _ := time.ParseDuration(cfg.Defaults.MaxTTL)
	if maxTTL <= 0 {
		checks = append(checks, doctorCheck{Name: "defaults.max_ttl", Status: severityForProduction(production), Message: "max TTL should be positive", Remediation: "Set defaults.max_ttl to a positive duration so sandboxes cannot run forever."})
	} else {
		checks = append(checks, doctorCheck{Name: "defaults.max_ttl", Status: doctorPass, Message: maxTTL.String()})
	}
	return checks
}

func lintLoggingConfig(cfg *config.Config, production bool) []doctorCheck {
	if cfg.Logging.Format != "json" {
		return []doctorCheck{{Name: "logging.format", Status: severityForProduction(production), Message: "logs are not JSON formatted", Remediation: "Set logging.format=json so production log collectors can parse records reliably."}}
	}
	return []doctorCheck{{Name: "logging.format", Status: doctorPass, Message: "json"}}
}

func lintWorkerRPCConfig(cfg *config.Config, production bool) []doctorCheck {
	rpcTLS := cfg.Worker.RPCTLS
	if !rpcTLS.Enabled {
		return []doctorCheck{{Name: "worker.rpc_tls.enabled", Status: doctorWarn, Message: "worker RPC TLS is disabled", Remediation: "Enable worker.rpc_tls.enabled with server, client, and CA certificates before exposing worker RPC across a network."}}
	}
	var checks []doctorCheck
	checks = append(checks, doctorCheck{Name: "worker.rpc_tls.enabled", Status: doctorPass, Message: "enabled"})
	if rpcTLS.ServerCertFile == "" || rpcTLS.ServerKeyFile == "" {
		checks = append(checks, doctorCheck{Name: "worker.rpc_tls.server_cert", Status: severityForProduction(production), Message: "server cert and key are required", Remediation: "Set worker.rpc_tls.server_cert_file and worker.rpc_tls.server_key_file on worker nodes."})
	} else {
		checks = append(checks, doctorCheck{Name: "worker.rpc_tls.server_cert", Status: doctorPass, Message: "configured"})
	}
	if rpcTLS.ClientCAFile == "" {
		checks = append(checks, doctorCheck{Name: "worker.rpc_tls.client_ca", Status: severityForProduction(production), Message: "client CA is not configured", Remediation: "Set worker.rpc_tls.client_ca_file so worker RPC servers require trusted control-plane client certificates."})
	} else {
		checks = append(checks, doctorCheck{Name: "worker.rpc_tls.client_ca", Status: doctorPass, Message: "configured"})
	}
	if rpcTLS.CAFile == "" {
		checks = append(checks, doctorCheck{Name: "worker.rpc_tls.ca", Status: severityForProduction(production), Message: "server CA is not configured", Remediation: "Set worker.rpc_tls.ca_file so control planes verify worker RPC server certificates."})
	} else {
		checks = append(checks, doctorCheck{Name: "worker.rpc_tls.ca", Status: doctorPass, Message: "configured"})
	}
	if rpcTLS.ClientCertFile == "" || rpcTLS.ClientKeyFile == "" {
		checks = append(checks, doctorCheck{Name: "worker.rpc_tls.client_cert", Status: severityForProduction(production), Message: "client cert and key are required", Remediation: "Set worker.rpc_tls.client_cert_file and worker.rpc_tls.client_key_file on control-plane nodes."})
	} else {
		checks = append(checks, doctorCheck{Name: "worker.rpc_tls.client_cert", Status: doctorPass, Message: "configured"})
	}
	if rpcTLS.InsecureSkipVerify {
		checks = append(checks, doctorCheck{Name: "worker.rpc_tls.insecure_skip_verify", Status: severityForProduction(production), Message: "TLS certificate verification is disabled", Remediation: "Set worker.rpc_tls.insecure_skip_verify=false outside throwaway local tests."})
	} else {
		checks = append(checks, doctorCheck{Name: "worker.rpc_tls.insecure_skip_verify", Status: doctorPass, Message: "disabled"})
	}
	return checks
}

func lintProviderConfig(cfg *config.Config, production bool) []doctorCheck {
	if cfg.Providers.Default != "docker" || !cfg.Providers.Docker.Enabled {
		return []doctorCheck{{Name: "providers.default", Status: doctorWarn, Message: fmt.Sprintf("default provider is %q", cfg.Providers.Default), Remediation: "For single-node broad compatibility, validate non-Docker providers with runtime certification before production."}}
	}

	var checks []doctorCheck
	docker := cfg.Providers.Docker
	if docker.Runtime == "" {
		checks = append(checks, doctorCheck{Name: "docker.runtime", Status: severityForProduction(production), Message: "runtime is empty", Remediation: "Set providers.docker.runtime explicitly, such as runc, runsc, or kata."})
	} else {
		checks = append(checks, doctorCheck{Name: "docker.runtime", Status: doctorPass, Message: docker.Runtime})
	}
	if docker.NetworkMode == "" {
		checks = append(checks, doctorCheck{Name: "docker.network_mode", Status: severityForProduction(production), Message: "network mode is empty", Remediation: "Set providers.docker.network_mode explicitly."})
	} else {
		checks = append(checks, doctorCheck{Name: "docker.network_mode", Status: doctorPass, Message: docker.NetworkMode})
	}
	if docker.SeccompProfile == "" || docker.SeccompProfile == "unconfined" {
		checks = append(checks, doctorCheck{Name: "docker.seccomp_profile", Status: severityForProduction(production), Message: "seccomp is not enforced", Remediation: "Use Docker's default seccomp profile or a custom restrictive profile."})
	} else {
		checks = append(checks, doctorCheck{Name: "docker.seccomp_profile", Status: doctorPass, Message: docker.SeccompProfile})
	}
	if len(docker.AddedCaps) > 0 {
		checks = append(checks, doctorCheck{Name: "docker.added_caps", Status: severityForProduction(production), Message: "extra Linux capabilities are configured", Remediation: "Remove providers.docker.added_caps unless a certified workload requires them."})
	} else {
		checks = append(checks, doctorCheck{Name: "docker.added_caps", Status: doctorPass, Message: "none"})
	}
	if !containsString(docker.DroppedCaps, "ALL") {
		checks = append(checks, doctorCheck{Name: "docker.dropped_caps", Status: severityForProduction(production), Message: "ALL capabilities are not dropped by default", Remediation: "Set providers.docker.dropped_caps=[\"ALL\"] and add back only certified capabilities."})
	} else {
		checks = append(checks, doctorCheck{Name: "docker.dropped_caps", Status: doctorPass, Message: strings.Join(docker.DroppedCaps, ",")})
	}
	if docker.Memory == "" || docker.CPUs == "" || docker.PidsLimit <= 0 {
		checks = append(checks, doctorCheck{Name: "docker.resource_limits", Status: severityForProduction(production), Message: "memory, CPU, and pids limits must be configured", Remediation: "Set providers.docker.memory, providers.docker.cpus, and providers.docker.pids_limit."})
	} else {
		checks = append(checks, doctorCheck{Name: "docker.resource_limits", Status: doctorPass, Message: fmt.Sprintf("memory %s, cpus %s, pids %d", docker.Memory, docker.CPUs, docker.PidsLimit)})
	}
	if docker.User == "" {
		checks = append(checks, doctorCheck{Name: "docker.user", Status: severityForProduction(production), Message: "containers may run as image default user", Remediation: "Set providers.docker.user to a non-root UID/GID such as 1000:1000 when images support it."})
	} else {
		checks = append(checks, doctorCheck{Name: "docker.user", Status: doctorPass, Message: docker.User})
	}
	return checks
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
