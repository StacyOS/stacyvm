package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/StacyOs/stacyvm/internal/config"
	"github.com/StacyOs/stacyvm/internal/worker"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

func newWorkerCmd() *cobra.Command {
	var id string
	var controlPlaneURL string
	var token string
	var heartbeatInterval string
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
			interval, err := time.ParseDuration(heartbeatInterval)
			if err != nil {
				return fmt.Errorf("worker heartbeat interval: %w", err)
			}
			logger := newCommandLogger(cfg)
			rt := worker.Runtime{
				Client: worker.Client{
					BaseURL:  strings.TrimRight(controlPlaneURL, "/"),
					WorkerID: id,
					Token:    token,
				},
				HeartbeatInterval: interval,
				Logger:            logger,
				Providers:         enabledProviderNames(cfg),
				Capacity: map[string]interface{}{
					"max_sandboxes":           cfg.Defaults.MaxSandboxes,
					"max_sandboxes_per_owner": cfg.Defaults.MaxSandboxesPerOwner,
				},
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
	cmd.Flags().BoolVar(&once, "once", false, "send one heartbeat and exit")
	return cmd
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
