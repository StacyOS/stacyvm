package orchestrator

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

const pgChannel = "stacyvm_events"

// bridgeEnvelope wraps an Event with the source instance ID so the publishing
// instance can ignore its own reflected NOTIFY and avoid double-delivery.
type bridgeEnvelope struct {
	SourceID string `json:"_src"`
	Event
}

// DurableBridge connects an EventBus to Postgres LISTEN/NOTIFY so that events
// published on one control-plane replica reach subscribers on all other replicas.
//
// Each bridge instance stamps published events with a unique instanceID. When
// its own NOTIFY is reflected back by Postgres, the listener skips it —
// preventing double-delivery to local subscribers.
//
// Usage:
//
//	bus := orchestrator.NewEventBus()
//	bridge, err := orchestrator.NewDurableBridge(ctx, dsn, bus, logger)
//	if err != nil { ... }
//	defer bridge.Stop()
//
// The bridge is a no-op for SQLite deployments — callers should only attach it
// when the store driver is Postgres.
type DurableBridge struct {
	bus        *EventBus
	instanceID string
	conn       *pgx.Conn
	cancel     context.CancelFunc
	done       chan struct{}
	logger     zerolog.Logger
}

// NewDurableBridge opens a dedicated Postgres connection for LISTEN/NOTIFY,
// attaches the onPublish hook to the given EventBus, and starts the listener
// goroutine. The returned bridge must be stopped with Stop() on shutdown.
func NewDurableBridge(ctx context.Context, dsn string, bus *EventBus, logger zerolog.Logger) (*DurableBridge, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(ctx, "LISTEN "+pgChannel); err != nil {
		conn.Close(ctx)
		return nil, err
	}

	listenCtx, cancel := context.WithCancel(context.Background())
	b := &DurableBridge{
		bus:        bus,
		instanceID: uuid.New().String(),
		conn:       conn,
		cancel:     cancel,
		done:       make(chan struct{}),
		logger:     logger,
	}

	// Wire the publish hook: NOTIFY Postgres when a local event is published.
	bus.AttachDurableBridge(func(evt Event) {
		b.notify(evt)
	})

	go b.listen(listenCtx)
	return b, nil
}

// Stop cancels the LISTEN goroutine and closes the connection.
func (b *DurableBridge) Stop() {
	b.cancel()
	<-b.done
	b.conn.Close(context.Background())
}

func (b *DurableBridge) notify(evt Event) {
	env := bridgeEnvelope{SourceID: b.instanceID, Event: evt}
	payload, err := json.Marshal(env)
	if err != nil {
		b.logger.Warn().Err(err).Msg("durable bridge: failed to marshal event for NOTIFY")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// pg_notify payload limit is 8000 bytes; strip Event.Data for large payloads.
	if len(payload) > 7900 {
		env.Data = nil
		payload, _ = json.Marshal(env)
	}
	if _, err := b.conn.Exec(ctx, "SELECT pg_notify($1, $2)", pgChannel, string(payload)); err != nil {
		b.logger.Warn().Err(err).Msg("durable bridge: pg_notify failed")
	}
}

func (b *DurableBridge) listen(ctx context.Context) {
	defer close(b.done)
	for {
		notification, err := b.conn.WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // clean shutdown
			}
			b.logger.Warn().Err(err).Msg("durable bridge: lost LISTEN connection; events may be missed until reconnect")
			// Attempt reconnect with exponential backoff (up to 20 s per attempt).
			for attempt := 1; attempt <= 10; attempt++ {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Duration(attempt) * 2 * time.Second):
				}
				newConn, connErr := pgx.Connect(ctx, b.conn.Config().ConnString())
				if connErr != nil {
					b.logger.Warn().Err(connErr).Int("attempt", attempt).Msg("durable bridge: reconnect failed")
					continue
				}
				if _, execErr := newConn.Exec(ctx, "LISTEN "+pgChannel); execErr != nil {
					newConn.Close(ctx)
					b.logger.Warn().Err(execErr).Int("attempt", attempt).Msg("durable bridge: LISTEN after reconnect failed")
					continue
				}
				b.conn = newConn
				b.logger.Info().Int("attempt", attempt).Msg("durable bridge: reconnected")
				break
			}
			continue
		}

		var env bridgeEnvelope
		if err := json.Unmarshal([]byte(notification.Payload), &env); err != nil {
			b.logger.Warn().Err(err).Msg("durable bridge: failed to unmarshal notification payload")
			continue
		}
		// Skip our own reflected NOTIFY — local subscribers already received
		// this event via the in-process bus (double-delivery prevention).
		if env.SourceID == b.instanceID {
			continue
		}
		// publishLocal avoids re-notifying Postgres (prevents infinite loop).
		b.bus.publishLocal(env.Event)
	}
}
