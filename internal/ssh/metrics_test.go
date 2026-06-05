package ssh

import (
	"bytes"
	"strings"
	"testing"
)

func TestMetricsWritePrometheus(t *testing.T) {
	m := NewMetrics()
	m.sessionStarted()
	m.sessionStarted()
	m.sessionEnded()
	m.authFailure()
	m.addBytesIn(100)
	m.addBytesOut(250)

	var buf bytes.Buffer
	m.WritePrometheus(&buf)
	out := buf.String()

	for _, want := range []string{
		"stacyvm_ssh_sessions_active 1",
		"stacyvm_ssh_sessions_total 2",
		"stacyvm_ssh_auth_failures_total 1",
		`stacyvm_ssh_bytes_total{dir="in"} 100`,
		`stacyvm_ssh_bytes_total{dir="out"} 250`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("prometheus output missing %q\n---\n%s", want, out)
		}
	}
}

func TestMetricsNilSafe(t *testing.T) {
	var m *Metrics // nil
	// Must not panic.
	m.sessionStarted()
	m.sessionEnded()
	m.authFailure()
	m.addBytesIn(1)
	m.addBytesOut(1)
	var buf bytes.Buffer
	m.WritePrometheus(&buf)
	if buf.Len() != 0 {
		t.Fatalf("nil metrics wrote %d bytes, want 0", buf.Len())
	}
}
