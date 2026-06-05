package middleware

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

// Regression: the logging wrapper used to swallow http.Flusher so SSE handlers
// hit "streaming not supported". The wrapper must now expose Flush() and
// Unwrap() so both type-assertion and http.ResponseController work.
func TestLoggingWrapperExposesFlusher(t *testing.T) {
	var fc flushCapture

	handler := Logging(zerolog.Nop())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Fatalf("wrapped writer does not implement http.Flusher")
		}

		rc := http.NewResponseController(w)
		if err := rc.Flush(); err != nil {
			t.Fatalf("ResponseController.Flush returned error: %v", err)
		}

		w.(http.Flusher).Flush()
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	handler.ServeHTTP(&fc, req)

	if fc.flushed < 2 {
		t.Fatalf("expected at least 2 flushes, got %d", fc.flushed)
	}
}

// flushCapture is a minimal ResponseWriter that records flush invocations.
type flushCapture struct {
	header  http.Header
	flushed int
	status  int
}

func (f *flushCapture) Header() http.Header {
	if f.header == nil {
		f.header = make(http.Header)
	}
	return f.header
}

func (f *flushCapture) Write(b []byte) (int, error) { return len(b), nil }
func (f *flushCapture) WriteHeader(status int)      { f.status = status }
func (f *flushCapture) Flush()                      { f.flushed++ }

// Regression: the logging wrapper used to swallow http.Hijacker, so the
// SSH-over-WebSocket tunnel (GET /api/v1/ssh/connect) failed its upgrade with
// HTTP 501 — nhooyr.io/websocket asserts http.Hijacker on the ResponseWriter
// directly, not via Unwrap. The wrapper must expose Hijack() so the assertion
// past the logging middleware succeeds and reaches the underlying writer.
func TestLoggingWrapperExposesHijacker(t *testing.T) {
	var hc hijackCapture

	handler := Logging(zerolog.Nop())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatalf("wrapped writer does not implement http.Hijacker")
		}
		if _, _, err := hj.Hijack(); err != nil {
			t.Fatalf("Hijack returned error: %v", err)
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ssh/connect", nil)
	handler.ServeHTTP(&hc, req)

	if !hc.hijacked {
		t.Fatalf("Hijack did not reach the underlying ResponseWriter")
	}
}

// hijackCapture is a minimal ResponseWriter that implements http.Hijacker and
// records whether Hijack was invoked.
type hijackCapture struct {
	header   http.Header
	hijacked bool
}

func (h *hijackCapture) Header() http.Header {
	if h.header == nil {
		h.header = make(http.Header)
	}
	return h.header
}

func (h *hijackCapture) Write(b []byte) (int, error) { return len(b), nil }
func (h *hijackCapture) WriteHeader(int)             {}

func (h *hijackCapture) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	return nil, nil, nil
}
