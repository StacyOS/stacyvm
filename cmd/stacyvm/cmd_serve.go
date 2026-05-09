package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/StacyOs/stacyvm/internal/api"
	"github.com/StacyOs/stacyvm/internal/api/middleware"
	"github.com/StacyOs/stacyvm/internal/config"
	"github.com/StacyOs/stacyvm/internal/environments"
	"github.com/StacyOs/stacyvm/internal/orchestrator"
	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/store"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the StacyVM API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe()
		},
	}
}

func runServe() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Logger
	var logger zerolog.Logger
	if cfg.Logging.Format == "pretty" {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
	} else {
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}
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
		return err
	}
	defer st.Close()

	// Event bus
	events := orchestrator.NewEventBus()

	// Provider registry
	registry := providers.NewRegistry()
	if cfg.Providers.Mock.Enabled {
		mock := providers.NewMockProvider()
		registry.Register(mock)
		logger.Info().Msg("mock provider registered")
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
		logger.Info().Str("path", cfg.Providers.Firecracker.FirecrackerPath).Msg("firecracker provider registered")
	}
	if cfg.Providers.E2B.Enabled {
		e2b := providers.NewE2BProvider(providers.E2BConfig{
			APIKey:  cfg.Providers.E2B.APIKey,
			BaseURL: cfg.Providers.E2B.BaseURL,
		})
		registry.Register(e2b)
		logger.Info().Msg("e2b provider registered")
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
		logger.Info().Str("name", cfg.Providers.Custom.Name).Str("url", cfg.Providers.Custom.BaseURL).Msg("custom provider registered")
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
			logger.Info().
				Str("runtime", cfg.Providers.Docker.Runtime).
				Str("network", cfg.Providers.Docker.NetworkMode).
				Msg("docker provider registered")
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
		logger.Info().
			Str("rootfs", cfg.Providers.PRoot.RootfsPath).
			Str("binary", cfg.Providers.PRoot.PRootBinary).
			Msg("proot provider registered")
	}

	// Auto-detect provider if set to "auto"
	if cfg.Providers.Default == "auto" {
		env := config.DetectEnvironment()
		cfg.Providers.Default = config.DefaultProviderForEnv(env)
		logger.Info().
			Str("environment", env.String()).
			Str("provider", cfg.Providers.Default).
			Msg("auto-detected default provider")
	}

	if err := registry.SetDefault(cfg.Providers.Default); err != nil {
		logger.Warn().Err(err).Msg("setting default provider")
	}

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
		WorkerToken: cfg.Auth.WorkerToken,
	})
	if err := mgr.Reconcile(context.Background()); err != nil {
		return err
	}
	mgr.Start()
	mgr.InitVMPool()
	defer mgr.Stop()

	// Template registry
	templates := orchestrator.NewTemplateRegistry(st)

	// Pool manager
	pool := orchestrator.NewPoolManager(mgr, templates, logger)
	pool.Start()
	defer pool.Stop()

	// Environment build manager
	envBuilds := environments.NewManager(st, logger)
	envBuilds.Start(1)
	defer envBuilds.Stop()

	// Auth warning
	if cfg.Auth.APIKey == "" {
		logger.Warn().Msg("WARNING: no API key configured — all endpoints are unauthenticated. Set auth.api_key in config or STACYVM_AUTH_API_KEY env var")
	}

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
		WorkerToken:           cfg.Auth.WorkerToken,
		WorkerTokens:          cfg.Auth.WorkerTokens,
		Version:               version,
		RateLimit: middleware.RateLimitConfig{
			Enabled:           cfg.RateLimit.Enabled,
			RequestsPerMinute: cfg.RateLimit.RequestsPerMinute,
			Burst:             cfg.RateLimit.Burst,
			KeyBy:             cfg.RateLimit.KeyBy,
			BucketTTL:         rateLimitBucketTTL,
			CleanupInterval:   rateLimitCleanupInterval,
		},
	}, registry, mgr, events, templates, pool, st, envBuilds, logger)

	// Graceful shutdown
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		logger.Info().Msg("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	case err := <-errCh:
		return err
	}
}
