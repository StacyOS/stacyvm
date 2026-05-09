package worker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
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
	var rpcServer *RPCServer
	serverErr := make(chan error, 1)
	if r.ListenAddr != "" {
		rpcServer = &RPCServer{
			WorkerID:     r.Client.WorkerID,
			Token:        r.Client.Token,
			Registry:     r.Registry,
			LeaseRenewer: r.Client,
		}
		server = NewHTTPServer(r.ListenAddr, rpcServer.Handler())
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
	if err := r.heartbeat(ctx, isDraining(rpcServer)); err != nil {
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
			if err := r.heartbeat(ctx, isDraining(rpcServer)); err != nil {
				r.Logger.Warn().Err(err).Msg("worker heartbeat failed")
			}
		}
	}
}

func isDraining(server *RPCServer) bool {
	return server != nil && server.Draining()
}

func (r Runtime) RunOnce(ctx context.Context) error {
	return r.heartbeat(ctx, false)
}

func (r Runtime) heartbeat(ctx context.Context, draining bool) error {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = r.Client.WorkerID
	}
	if hostname == "" {
		return fmt.Errorf("worker hostname is empty")
	}
	status := "online"
	if draining {
		status = "draining"
	}
	params := workerproto.HeartbeatParams{
		Hostname:     hostname,
		Status:       status,
		Providers:    r.Providers,
		Capabilities: []string{"remote_worker", "heartbeat"},
		Capacity:     r.Capacity,
	}
	if r.ListenAddr != "" {
		if params.Capacity == nil {
			params.Capacity = map[string]interface{}{}
		}
		rpcURL := r.ListenAddr
		if !strings.HasPrefix(rpcURL, "http://") && !strings.HasPrefix(rpcURL, "https://") {
			rpcURL = "http://" + rpcURL
		}
		params.Capacity["rpc_url"] = rpcURL
	}
	return r.Client.Heartbeat(ctx, params)
}
