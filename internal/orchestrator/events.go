package orchestrator

import (
	"encoding/json"
	"strconv"
	"sync"
	"time"
)

type EventType string

const (
	EventSandboxCreated   EventType = "sandbox.created"
	EventSandboxRunning   EventType = "sandbox.running"
	EventSandboxDestroyed EventType = "sandbox.destroyed"
	EventSandboxError     EventType = "sandbox.error"
	EventExecStarted      EventType = "exec.started"
	EventExecCompleted    EventType = "exec.completed"
	EventExecFailed       EventType = "exec.failed"
	EventExecTimeout      EventType = "exec.timeout"
	EventFileWritten      EventType = "file.written"
	EventFileRead         EventType = "file.read"
	EventOperationFailed  EventType = "operation.failed"
	EventResourceLimit    EventType = "resource.limit"
	EventProviderFailed   EventType = "provider.failed"
	EventReconcileAction  EventType = "reconcile.action"
)

type Event struct {
	ID        string          `json:"id"`
	Type      EventType       `json:"type"`
	SandboxID string          `json:"sandbox_id,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data,omitempty"`
}

const (
	defaultHistorySize   = 1000
	defaultSubscriberBuf = 64
)

// EventBus is an in-process pub/sub system with ring buffer history.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string]chan Event
	history     []Event
	historySize int
	nextID      int
}

type EventBusStats struct {
	Subscribers int `json:"subscribers"`
	HistorySize int `json:"history_size"`
	EventsTotal int `json:"events_total"`
}

func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string]chan Event),
		history:     make([]Event, 0, defaultHistorySize),
		historySize: defaultHistorySize,
	}
}

// Publish sends an event to all subscribers (non-blocking).
func (eb *EventBus) Publish(evt Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}
	eb.nextID++
	if evt.ID == "" {
		evt.ID = stringID(eb.nextID)
	}

	// Ring buffer: append or overwrite oldest
	if len(eb.history) < eb.historySize {
		eb.history = append(eb.history, evt)
	} else {
		eb.history[eb.nextID%eb.historySize] = evt
	}

	for _, ch := range eb.subscribers {
		select {
		case ch <- evt:
		default:
			// Drop if subscriber is slow — non-blocking
		}
	}
}

func stringID(id int) string {
	return "evt-" + strconv.Itoa(id)
}

// Subscribe creates a new subscription and returns a channel + unsubscribe key.
func (eb *EventBus) Subscribe(id string) <-chan Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	ch := make(chan Event, defaultSubscriberBuf)
	eb.subscribers[id] = ch
	return ch
}

// Unsubscribe removes a subscription.
func (eb *EventBus) Unsubscribe(id string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if ch, ok := eb.subscribers[id]; ok {
		close(ch)
		delete(eb.subscribers, id)
	}
}

// History returns the most recent events up to limit.
func (eb *EventBus) History(limit int) []Event {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	if limit <= 0 || limit > len(eb.history) {
		limit = len(eb.history)
	}

	start := len(eb.history) - limit
	if start < 0 {
		start = 0
	}
	result := make([]Event, limit)
	copy(result, eb.history[start:])
	return result
}

func (eb *EventBus) Stats() EventBusStats {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	return EventBusStats{
		Subscribers: len(eb.subscribers),
		HistorySize: len(eb.history),
		EventsTotal: eb.nextID,
	}
}
