package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.size += n
	return n, err
}

// Flush forwards to the underlying writer when it supports flushing.
// Required so streaming handlers (SSE, NDJSON) can flush per chunk after
// the logging middleware has wrapped the writer.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap exposes the underlying writer so http.ResponseController can reach
// optional interfaces (Flusher, Hijacker, etc.) past this wrapper.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// Hijack lets connection-upgrade handlers take over the underlying connection
// through this wrapper. The SSH-over-WebSocket tunnel (GET /api/v1/ssh/connect)
// needs this: nhooyr.io/websocket type-asserts http.Hijacker on the
// ResponseWriter directly rather than going through Unwrap, so without this
// method the upgrade fails with HTTP 501.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying ResponseWriter does not implement http.Hijacker")
	}
	return hj.Hijack()
}

func Logging(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rw, r)

			logger.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", rw.status).
				Int("size", rw.size).
				Dur("duration", time.Since(start)).
				Str("request_id", GetRequestID(r.Context())).
				Msg("request")
		})
	}
}
