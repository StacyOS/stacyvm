package middleware

import (
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
