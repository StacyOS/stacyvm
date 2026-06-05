package ssh

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

type nopWriteCloser struct{ w io.Writer }

func (n nopWriteCloser) Write(p []byte) (int, error) { return n.w.Write(p) }
func (n nopWriteCloser) Close() error                { return nil }

func TestSessionRecordingProducesCast(t *testing.T) {
	var buf bytes.Buffer
	rec, err := newCastRecorder(nopWriteCloser{&buf}, 80, 24)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}

	sess := &recordingSession{PTYSession: newFakePTY("hello-rec", 0), rec: rec}
	if _, err := io.ReadAll(sess); err != nil {
		t.Fatalf("read: %v", err)
	}
	_ = sess.Close()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("cast has %d lines, want header + >=1 event:\n%s", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], `"version":2`) {
		t.Fatalf("header = %q, want asciinema v2 header", lines[0])
	}
	if !strings.Contains(buf.String(), `"o"`) || !strings.Contains(buf.String(), "hello-rec") {
		t.Fatalf("cast missing output event with data:\n%s", buf.String())
	}
}
