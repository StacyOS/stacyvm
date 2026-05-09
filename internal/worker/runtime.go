package worker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/StacyOs/stacyvm/internal/providers"
	"github.com/StacyOs/stacyvm/internal/workerproto"
	"github.com/rs/zerolog"
)

type Runtime struct {
	Client            Client
	ListenAddr        string
	HeartbeatInterval time.Duration
	Logger            zerolog.Logger
	Providers         []string
	Capacity          map[string]interface{}
	Registry          *providers.Registry
}

func (r Runtime) Run(ctx context.Context) error {
	var server *http.Server
	serverErr := make(chan error, 1)
	if r.ListenAddr != "" {
		server = NewHTTPServer(r.ListenAddr, RPCServer{
			WorkerID:     r.Client.WorkerID,
			Token:        r.Client.Token,
			Registry:     r.Registry,
			LeaseRenewer: r.Client,
		}.Handler())
		go func() {
			r.Logger.Info().Str("addr", r.ListenAddr).Msg("starting worker RPC server")
			err := server.ListenAndServe()
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			serverErr <- err
		}()
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
		}()
	}
	if err := r.heartbeat(ctx); err != nil {
		return err
	}
	interval := r.HeartbeatInterval
	if interval == 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-serverErr:
			return err
		case <-ticker.C:
			if err := r.heartbeat(ctx); err != nil {
				r.Logger.Warn().Err(err).Msg("worker heartbeat failed")
			}
		}
	}
}

func (r Runtime) RunOnce(ctx context.Context) error {
	return r.heartbeat(ctx)
}

func (r Runtime) heartbeat(ctx context.Context) error {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = r.Client.WorkerID
	}
	if hostname == "" {
		return fmt.Errorf("worker hostname is empty")
	}
	params := workerproto.HeartbeatParams{
		Hostname:     hostname,
		Status:       "online",
		Providers:    r.Providers,
		Capabilities: []string{"remote_worker", "heartbeat"},
		Capacity:     r.Capacity,
	}
	return r.Client.Heartbeat(ctx, params)
}
