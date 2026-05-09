package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/StacyOs/stacyvm/internal/api/middleware"
	"github.com/StacyOs/stacyvm/internal/config"
	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/worker"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

func newWorkerCmd() *cobra.Command {
	var id string
	var controlPlaneURL string
	var token string
	var heartbeatInterval string
	var listenAddr string
	var previewDomain string
	var once bool
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Start a StacyVM remote worker process",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if id == "" {
				id = cfg.Worker.ID
			}
			if id == "" {
				hostname, _ := os.Hostname()
				id = hostname
			}
			if controlPlaneURL == "" {
				controlPlaneURL = cfg.Worker.ControlPlaneURL
			}
			if token == "" {
				token = cfg.Auth.WorkerToken
			}
			if heartbeatInterval == "" {
				heartbeatInterval = cfg.Worker.HeartbeatInterval
			}
			if listenAddr == "" {
				listenAddr = cfg.Worker.ListenAddr
			}
			if previewDomain == "" {
				previewDomain = cfg.Worker.PreviewDomain
			}
			if previewDomain == "" {
				previewDomain = cfg.Server.PreviewDomain
			}
			interval, err := time.ParseDuration(heartbeatInterval)
			if err != nil {
				return fmt.Errorf("worker heartbeat interval: %w", err)
			}
			logger := newCommandLogger(cfg)
			registry := buildWorkerRegistry(cfg, logger, previewDomain)
			tokenFunc := signedWorkerTokenFunc(id, cfg.Auth.WorkerSigningKey)
			if token != "" {
				tokenFunc = nil
			}
			rt := worker.Runtime{
				Client: worker.Client{
					BaseURL:   strings.TrimRight(controlPlaneURL, "/"),
					WorkerID:  id,
					Token:     token,
					TokenFunc: tokenFunc,
				},
				HeartbeatInterval: interval,
				ListenAddr:        listenAddr,
				Logger:            logger,
				Providers:         enabledProviderNames(cfg),
				Capacity: map[string]interface{}{
					"max_sandboxes":           cfg.Defaults.MaxSandboxes,
					"max_sandboxes_per_owner": cfg.Defaults.MaxSandboxesPerOwner,
					"preview_domain":          previewDomain,
				},
				Registry:    registry,
				RPCTLS:      workerTLSConfig(cfg.Worker.RPCTLS),
				SigningKey:  cfg.Auth.WorkerSigningKey,
				SigningKeys: cfg.Auth.WorkerSigningKeys,
			}
			if once {
				return rt.RunOnce(cmd.Context())
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			logger.Info().Str("worker_id", id).Str("control_plane", controlPlaneURL).Msg("starting StacyVM worker")
			return rt.Run(ctx)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "worker ID; defaults to worker.id or hostname")
	cmd.Flags().StringVar(&controlPlaneURL, "control-plane", "", "control plane URL; defaults to worker.control_plane_url")
	cmd.Flags().StringVar(&token, "worker-token", os.Getenv("STACYVM_AUTH_WORKER_TOKEN"), "worker bearer token; defaults to auth.worker_token")
	cmd.Flags().StringVar(&heartbeatInterval, "heartbeat-interval", "", "worker heartbeat interval")
	cmd.Flags().StringVar(&listenAddr, "listen", "", "worker RPC listen address; defaults to worker.listen_addr")
	cmd.Flags().StringVar(&previewDomain, "preview-domain", "", "worker preview domain; defaults to worker.preview_domain or server.preview_domain")
	cmd.Flags().BoolVar(&once, "once", false, "send one heartbeat and exit")
	cmd.AddCommand(newWorkerTokenCmd())
	return cmd
}

func newWorkerTokenCmd() *cobra.Command {
	var signingKey string
	var ttl string
	var scopes []string
	var audience string
	cmd := &cobra.Command{
		Use:   "token <worker-id>",
		Short: "Issue a signed worker token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if signingKey == "" {
				signingKey = cfg.Auth.WorkerSigningKey
			}
			token, err := issueWorkerToken(workerTokenIssueOptions{
				WorkerID:   args[0],
				SigningKey: signingKey,
				TTL:        ttl,
				Scopes:     scopes,
				Audience:   audience,
				Now:        time.Now,
			})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), token)
			return err
		},
	}
	cmd.Flags().StringVar(&signingKey, "signing-key", os.Getenv("STACYVM_AUTH_WORKER_SIGNING_KEY"), "worker signing key; defaults to auth.worker_signing_key")
	cmd.Flags().StringVar(&ttl, "ttl", "5m", "token lifetime")
	cmd.Flags().StringVar(&audience, "audience", middleware.WorkerTokenAudienceControlPlane, "token audience: worker:control-plane or worker:rpc")
	cmd.Flags().StringArrayVar(&scopes, "scope", nil, "worker scope to include; repeatable, defaults to all worker scopes")
	return cmd
}

type workerTokenIssueOptions struct {
	WorkerID   string
	SigningKey string
	TTL        string
	Scopes     []string
	Audience   string
	Now        func() time.Time
}

func issueWorkerToken(opts workerTokenIssueOptions) (string, error) {
	workerID := strings.TrimSpace(opts.WorkerID)
	signingKey := strings.TrimSpace(opts.SigningKey)
	if workerID == "" {
		return "", fmt.Errorf("worker id is required")
	}
	if signingKey == "" {
		return "", fmt.Errorf("worker signing key is required")
	}
	ttl, err := time.ParseDuration(opts.TTL)
	if err != nil {
		return "", fmt.Errorf("worker token ttl: %w", err)
	}
	if ttl <= 0 {
		return "", fmt.Errorf("worker token ttl must be positive")
	}
	audience := strings.TrimSpace(opts.Audience)
	if audience == "" {
		audience = middleware.WorkerTokenAudienceControlPlane
	}
	if audience != middleware.WorkerTokenAudienceControlPlane && audience != middleware.WorkerTokenAudienceRPC {
		return "", fmt.Errorf("worker token audience must be %q or %q", middleware.WorkerTokenAudienceControlPlane, middleware.WorkerTokenAudienceRPC)
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	issuedAt := now().UTC()
	return middleware.SignWorkerToken(signingKey, middleware.WorkerTokenClaims{
		WorkerID:  workerID,
		Audience:  audience,
		Scopes:    opts.Scopes,
		IssuedAt:  issuedAt.Unix(),
		ExpiresAt: issuedAt.Add(ttl).Unix(),
	})
}

func signedWorkerTokenFunc(workerID, signingKey string) func() (string, error) {
	signingKey = strings.TrimSpace(signingKey)
	workerID = strings.TrimSpace(workerID)
	if signingKey == "" || workerID == "" {
		return nil
	}
	return func() (string, error) {
		now := time.Now().UTC()
		return middleware.SignWorkerToken(signingKey, middleware.WorkerTokenClaims{
			WorkerID:  workerID,
			Audience:  middleware.WorkerTokenAudienceControlPlane,
			IssuedAt:  now.Unix(),
			ExpiresAt: now.Add(5 * time.Minute).Unix(),
		})
	}
}

func workerTLSConfig(cfg config.WorkerRPCTLSConfig) worker.TLSConfig {
	return worker.TLSConfig{
		Enabled:            cfg.Enabled,
		ServerCertFile:     cfg.ServerCertFile,
		ServerKeyFile:      cfg.ServerKeyFile,
		ClientCAFile:       cfg.ClientCAFile,
		CAFile:             cfg.CAFile,
		ClientCertFile:     cfg.ClientCertFile,
		ClientKeyFile:      cfg.ClientKeyFile,
		ServerName:         cfg.ServerName,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}
}

func newCommandLogger(cfg *config.Config) zerolog.Logger {
	var logger zerolog.Logger
	if cfg.Logging.Format == "pretty" {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
	} else {
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}
	if level, err := zerolog.ParseLevel(cfg.Logging.Level); err == nil {
		logger = logger.Level(level)
	}
	return logger
}

func enabledProviderNames(cfg *config.Config) []string {
	var providers []string
	if cfg.Providers.Mock.Enabled {
		providers = append(providers, "mock")
	}
	if cfg.Providers.Firecracker.Enabled {
		providers = append(providers, "firecracker")
	}
	if cfg.Providers.Docker.Enabled {
		providers = append(providers, "docker")
	}
	if cfg.Providers.E2B.Enabled {
		providers = append(providers, "e2b")
	}
	if cfg.Providers.Custom.Enabled {
		providers = append(providers, cfg.Providers.Custom.Name)
	}
	if cfg.Providers.PRoot.Enabled {
		providers = append(providers, "proot")
	}
	return providers
}

func buildWorkerRegistry(cfg *config.Config, logger zerolog.Logger, previewDomain string) *providers.Registry {
	registry := providers.NewRegistry()
	if cfg.Providers.Mock.Enabled {
		registry.Register(providers.NewMockProvider())
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
			PreviewDomain: previewDomain,
		}, logger)
		if err != nil {
			logger.Error().Err(err).Msg("failed to create worker docker provider")
		} else {
			registry.Register(docker)
		}
	}
	if len(registry.List()) > 0 {
		if err := registry.SetDefault(cfg.Providers.Default); err != nil {
			_ = registry.SetDefault(registry.List()[0])
		}
	}
	return registry
}
