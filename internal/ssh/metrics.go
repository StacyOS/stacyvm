package ssh

import (
	"fmt"
	"io"
	"sync/atomic"
)

// Metrics holds SSH gateway counters exported to Prometheus. All methods are
// safe to call on a nil *Metrics (a no-op), so the gateway can run without it.
type Metrics struct {
	sessionsActive int64
	sessionsTotal  int64
	authFailures   int64
	bytesIn        int64
	bytesOut       int64
}

func NewMetrics() *Metrics { return &Metrics{} }

func (m *Metrics) sessionStarted() {
	if m == nil {
		return
	}
	atomic.AddInt64(&m.sessionsActive, 1)
	atomic.AddInt64(&m.sessionsTotal, 1)
}

func (m *Metrics) sessionEnded() {
	if m == nil {
		return
	}
	atomic.AddInt64(&m.sessionsActive, -1)
}

func (m *Metrics) authFailure() {
	if m == nil {
		return
	}
	atomic.AddInt64(&m.authFailures, 1)
}

func (m *Metrics) addBytesIn(n int64) {
	if m == nil || n <= 0 {
		return
	}
	atomic.AddInt64(&m.bytesIn, n)
}

func (m *Metrics) addBytesOut(n int64) {
	if m == nil || n <= 0 {
		return
	}
	atomic.AddInt64(&m.bytesOut, n)
}

// WritePrometheus appends the gateway's metrics in Prometheus text format.
func (m *Metrics) WritePrometheus(w io.Writer) {
	if m == nil {
		return
	}
	fmt.Fprintf(w, "# HELP stacyvm_ssh_sessions_active Active SSH sessions.\n")
	fmt.Fprintf(w, "# TYPE stacyvm_ssh_sessions_active gauge\n")
	fmt.Fprintf(w, "stacyvm_ssh_sessions_active %d\n", atomic.LoadInt64(&m.sessionsActive))

	fmt.Fprintf(w, "# HELP stacyvm_ssh_sessions_total Total SSH sessions opened.\n")
	fmt.Fprintf(w, "# TYPE stacyvm_ssh_sessions_total counter\n")
	fmt.Fprintf(w, "stacyvm_ssh_sessions_total %d\n", atomic.LoadInt64(&m.sessionsTotal))

	fmt.Fprintf(w, "# HELP stacyvm_ssh_auth_failures_total Total SSH authentication failures.\n")
	fmt.Fprintf(w, "# TYPE stacyvm_ssh_auth_failures_total counter\n")
	fmt.Fprintf(w, "stacyvm_ssh_auth_failures_total %d\n", atomic.LoadInt64(&m.authFailures))

	fmt.Fprintf(w, "# HELP stacyvm_ssh_bytes_total Total SSH bytes relayed by direction.\n")
	fmt.Fprintf(w, "# TYPE stacyvm_ssh_bytes_total counter\n")
	fmt.Fprintf(w, "stacyvm_ssh_bytes_total{dir=\"in\"} %d\n", atomic.LoadInt64(&m.bytesIn))
	fmt.Fprintf(w, "stacyvm_ssh_bytes_total{dir=\"out\"} %d\n", atomic.LoadInt64(&m.bytesOut))
}
