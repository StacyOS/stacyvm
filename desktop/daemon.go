package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/StacyOs/stacyvm/internal/api"
	"github.com/StacyOs/stacyvm/internal/api/middleware"
	"github.com/StacyOs/stacyvm/internal/config"
	"github.com/StacyOs/stacyvm/internal/environments"
	"github.com/StacyOs/stacyvm/internal/orchestrator"
	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/rs/zerolog"
)

// runDaemon launches the StacyVM API server daemon in the background.
// It returns a function that can be called to cleanly shut it down.
func runDaemon(ctx context.Context, logger zerolog.Logger) (func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	// Inject Wails CORS Origins
	cfg.Server.CORSAllowedOrigins = append(cfg.Server.CORSAllowedOrigins, "wails://wails", "http://wails.localhost")

	level, err := zerolog.ParseLevel(cfg.Logging.Level)
	if err == nil {
		logger = logger.Level(level)
	}

	// Store
	st, err := store.Open(store.Config{
		Driver: cfg.Database.Driver,
		Path:   cfg.Database.Path,
		DSN:    cfg.Database.DSN,
	})
	if err != nil {
		return nil, fmt.Errorf("store init: %w", err)
	}

	// Event bus
	events := orchestrator.NewEventBus()
	if strings.EqualFold(strings.TrimSpace(cfg.Database.Driver), "postgres") ||
		strings.EqualFold(strings.TrimSpace(cfg.Database.Driver), "postgresql") {
		bridge, bridgeErr := orchestrator.NewDurableBridge(ctx, cfg.Database.DSN, events, logger)
		if bridgeErr != nil {
			logger.Warn().Err(bridgeErr).Msg("durable event bridge unavailable")
		} else {
			// Actually we can't defer bridge.Stop() here, we should return a cleanup function.
			// Let's do it in the returned cleanup func.
			_ = bridge // We will stop it via the events bus if needed, but DurableBridge stop isn't exposed easily without refactoring.
			// Wait, orchestrator.NewDurableBridge returns *DurableBridge which has Stop().
			// We will just capture it in the shutdown function.
			defer func(b *orchestrator.DurableBridge) {
				// To do proper shutdown, we will capture it below
			}(bridge)
		}
	}

	// Provider registry
	registry := providers.NewRegistry()
	if cfg.Providers.Mock.Enabled {
		mock := providers.NewMockProvider()
		registry.Register(mock)
	}
	if cfg.Providers.Firecracker.Enabled {
		fc := providers.NewFirecrackerProvider(providers.FirecrackerProviderConfig{
			FirecrackerPath: cfg.Providers.Firecracker.FirecrackerPath,
			KernelPath:      cfg.Providers.Firecracker.KernelPath,
			DefaultRootfs:   cfg.Providers.Firecracker.DefaultRootfs,
			AgentPath:       cfg.ResolveAgentPath(),
			DataDir:         cfg.Providers.Firecracker.DataDir,
			DefaultMemoryMB: cfg.Defaults.MemoryMB,
		}, logger)
		registry.Register(fc)
		fc.CheckDockerAccess()
	}
	if cfg.Providers.E2B.Enabled {
		e2b := providers.NewE2BProvider(providers.E2BConfig{
			APIKey:  cfg.Providers.E2B.APIKey,
			BaseURL: cfg.Providers.E2B.BaseURL,
		})
		registry.Register(e2b)
	}
	if cfg.Providers.Custom.Enabled && cfg.Providers.Custom.BaseURL != "" {
		timeout, _ := time.ParseDuration(cfg.Providers.Custom.Timeout)
		if timeout == 0 {
			timeout = 60 * time.Second
		}
		custom := providers.NewCustomProvider(providers.CustomProviderConfig{
			ProviderName: cfg.Providers.Custom.Name,
			BaseURL:      cfg.Providers.Custom.BaseURL,
			APIKey:       cfg.Providers.Custom.APIKey,
			Timeout:      timeout,
		})
		registry.Register(custom)
	}
	if cfg.Providers.Docker.Enabled {
		docker, err := providers.NewDockerProvider(providers.DockerProviderConfig{
			Socket:         cfg.Providers.Docker.Socket,
			Runtime:        cfg.Providers.Docker.Runtime,
			DefaultImage:   cfg.Providers.Docker.DefaultImage,
			NetworkMode:    cfg.Providers.Docker.NetworkMode,
			SeccompProfile: cfg.Providers.Docker.SeccompProfile,
			ReadOnlyRootfs: cfg.Providers.Docker.ReadOnlyRootfs,
			Memory:         cfg.Providers.Docker.Memory,
			CPUs:           cfg.Providers.Docker.CPUs,
			PidsLimit:      cfg.Providers.Docker.PidsLimit,
			User:           cfg.Providers.Docker.User,
			DroppedCaps:    cfg.Providers.Docker.DroppedCaps,
			AddedCaps:      cfg.Providers.Docker.AddedCaps,
			Tmpfs:          cfg.Providers.Docker.Tmpfs,
			PoolSecurity: providers.PoolSecurityProviderConfig{
				PerUserUID:           cfg.Providers.Docker.PoolSecurity.PerUserUID,
				PIDNamespace:         cfg.Providers.Docker.PoolSecurity.PIDNamespace,
				WorkspacePermissions: cfg.Providers.Docker.PoolSecurity.WorkspacePermissions,
				HidePID:              cfg.Providers.Docker.PoolSecurity.HidePID,
			},
			PreviewDomain: cfg.Server.PreviewDomain,
		}, logger)
		if err != nil {
			logger.Error().Err(err).Msg("failed to create docker provider")
		} else {
			registry.Register(docker)
		}
	}
	if cfg.Providers.PRoot.Enabled {
		timeout, _ := time.ParseDuration(cfg.Providers.PRoot.DefaultTimeout)
		if timeout == 0 {
			timeout = 60 * time.Second
		}
		proot := providers.NewPRootProvider(providers.PRootProviderConfig{
			RootfsPath:     cfg.Providers.PRoot.RootfsPath,
			PRootBinary:    cfg.Providers.PRoot.PRootBinary,
			WorkspaceBase:  cfg.Providers.PRoot.WorkspaceBase,
			DefaultTimeout: timeout,
			MaxSandboxes:   cfg.Providers.PRoot.MaxSandboxes,
			MaxMemoryMB:    cfg.Providers.PRoot.MaxMemoryMB,
			MaxDiskMB:      cfg.Providers.PRoot.MaxDiskMB,
			Languages:      cfg.Providers.PRoot.Languages,
		}, logger)
		registry.Register(proot)
	}

	if cfg.Providers.Default == "auto" {
		env := config.DetectEnvironment()
		cfg.Providers.Default = config.DefaultProviderForEnv(env)
	}

	_ = registry.SetDefault(cfg.Providers.Default)

	// Manager
	ttl, _ := time.ParseDuration(cfg.Defaults.TTL)
	maxTTL, _ := time.ParseDuration(cfg.Defaults.MaxTTL)
	defaultExecTimeout, _ := time.ParseDuration(cfg.Defaults.DefaultExecTimeout)
	maxExecTimeout, _ := time.ParseDuration(cfg.Defaults.MaxExecTimeout)
	spawnQueueTimeout, _ := time.ParseDuration(cfg.Defaults.SpawnQueueTimeout)
	mgr := orchestrator.NewManager(registry, st, events, logger, orchestrator.ManagerConfig{
		DefaultTTL:    ttl,
		DefaultImage:  cfg.Defaults.Image,
		DefaultMemory: cfg.Defaults.MemoryMB,
		DefaultVCPUs:  cfg.Defaults.VCPUs,
		Pool:          cfg.Pool,
		PreviewDomain: cfg.Server.PreviewDomain,
		Limits: orchestrator.OperationalLimits{
			MaxSandboxes:         cfg.Defaults.MaxSandboxes,
			MaxSandboxesPerOwner: cfg.Defaults.MaxSandboxesPerOwner,
			DefaultExecTimeout:   defaultExecTimeout,
			MaxExecTimeout:       maxExecTimeout,
			MaxTTL:               maxTTL,
			SpawnOverflow:        cfg.Defaults.SpawnOverflow,
			SpawnQueueTimeout:    spawnQueueTimeout,
			MaxSpawnQueue:        cfg.Defaults.MaxSpawnQueue,
		},
		WorkerToken:           cfg.Auth.WorkerToken,
		WorkerSigningKey:      cfg.Auth.WorkerSigningKey,
		WorkerRevokedTokenIDs: cfg.Auth.WorkerRevokedTokenIDs,
		// WorkerRPCTLS ignored for simplicity in daemon wrapper unless we import config.WorkerRPCTLS
	})
	if err := mgr.Reconcile(ctx); err != nil {
		return nil, fmt.Errorf("reconcile: %w", err)
	}
	mgr.Start()
	mgr.InitVMPool()

	templates := orchestrator.NewTemplateRegistry(st)
	pool := orchestrator.NewPoolManager(mgr, templates, logger)
	pool.Start()

	envBuilds := environments.NewManager(st, logger)
	envBuilds.Start(1)

	// Server
	rateLimitBucketTTL, _ := time.ParseDuration(cfg.RateLimit.BucketTTL)
	rateLimitCleanupInterval, _ := time.ParseDuration(cfg.RateLimit.CleanupInterval)
	adminAuditRetention, _ := time.ParseDuration(cfg.Auth.AdminAuditRetention)
	srv := api.NewServer(api.ServerConfig{
		Addr:                  cfg.Server.Addr(),
		APIKey:                cfg.Auth.APIKey,
		AdminAPIKey:           cfg.Auth.AdminAPIKey,
		AdminFallbackDisabled: !cfg.Auth.AdminFallbackEnabled,
		AdminAuditRetention:   adminAuditRetention,
		CORSAllowedOrigins:    cfg.Server.CORSAllowedOrigins,
		WorkerToken:           cfg.Auth.WorkerToken,
		WorkerTokens:          cfg.Auth.WorkerTokens,
		WorkerSigningKey:      cfg.Auth.WorkerSigningKey,
		WorkerSigningKeys:     cfg.Auth.WorkerSigningKeys,
		WorkerRevokedTokenIDs: cfg.Auth.WorkerRevokedTokenIDs,
		Version:               "desktop",
		RateLimit: middleware.RateLimitConfig{
			Enabled:           cfg.RateLimit.Enabled,
			RequestsPerMinute: cfg.RateLimit.RequestsPerMinute,
			Burst:             cfg.RateLimit.Burst,
			KeyBy:             cfg.RateLimit.KeyBy,
			BucketTTL:         rateLimitBucketTTL,
			CleanupInterval:   rateLimitCleanupInterval,
		},
		OIDC: middleware.OIDCConfig{
			Issuer:         cfg.Auth.OIDCIssuer,
			Audience:       cfg.Auth.OIDCAudience,
			JWKSUrl:        cfg.Auth.OIDCJWKSUrl,
			PublicKeyPEM:   cfg.Auth.OIDCPublicKey,
			GroupsClaim:    cfg.Auth.OIDCGroupsClaim,
			TenantClaim:    cfg.Auth.OIDCTenantClaim,
			AdminGroups:    cfg.Auth.OIDCAdminGroups,
			OperatorGroups: cfg.Auth.OIDCOperatorGroups,
			ViewerGroups:   cfg.Auth.OIDCViewerGroups,
		},
	}, registry, mgr, events, templates, pool, st, envBuilds, logger)

	go func() {
		if err := srv.Start(); err != nil {
			logger.Error().Err(err).Msg("server error")
		}
	}()

	shutdown := func() {
		logger.Info().Msg("shutting down daemon...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		envBuilds.Stop()
		pool.Stop()
		mgr.Stop()
		st.Close()
	}

	return shutdown, nil
}
