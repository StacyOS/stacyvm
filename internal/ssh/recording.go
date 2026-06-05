package ssh

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/StacyOs/stacyvm/internal/providers"
)

// RecorderFunc opens a destination for a session recording (e.g. a .cast file).
// Returning a nil writer (or error) disables recording for that session.
type RecorderFunc func(sandboxID string, identity Identity) (io.WriteCloser, error)

// castRecorder writes an asciinema v2 cast: a JSON header line followed by
// [elapsed, code, data] event lines ("o" output, "r" resize).
type castRecorder struct {
	mu    sync.Mutex
	w     io.WriteCloser
	start time.Time
}

func newCastRecorder(w io.WriteCloser, cols, rows uint16) (*castRecorder, error) {
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}
	r := &castRecorder{w: w, start: time.Now()}
	header, _ := json.Marshal(map[string]any{
		"version":   2,
		"width":     cols,
		"height":    rows,
		"timestamp": r.start.Unix(),
	})
	if _, err := fmt.Fprintf(w, "%s\n", header); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *castRecorder) event(code, data string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	line, _ := json.Marshal([]any{time.Since(r.start).Seconds(), code, data})
	_, _ = fmt.Fprintf(r.w, "%s\n", line)
}

func (r *castRecorder) Close() error { return r.w.Close() }

// recordingSession tees a PTY session's output (and resize events) to a cast.
type recordingSession struct {
	providers.PTYSession
	rec *castRecorder
}

func (s *recordingSession) Read(p []byte) (int, error) {
	n, err := s.PTYSession.Read(p)
	if n > 0 {
		s.rec.event("o", string(p[:n]))
	}
	return n, err
}

func (s *recordingSession) Resize(cols, rows uint16) error {
	s.rec.event("r", fmt.Sprintf("%dx%d", cols, rows))
	return s.PTYSession.Resize(cols, rows)
}

func (s *recordingSession) Close() error {
	err := s.PTYSession.Close()
	_ = s.rec.Close()
	return err
}
