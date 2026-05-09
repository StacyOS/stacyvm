package worker

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/StacyOs/stacyvm/internal/workerproto"
	"github.com/rs/zerolog"
)

type Runtime struct {
	Client            Client
	HeartbeatInterval time.Duration
	Logger            zerolog.Logger
	Providers         []string
	Capacity          map[string]interface{}
}

func (r Runtime) Run(ctx context.Context) error {
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
