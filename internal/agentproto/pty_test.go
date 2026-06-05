//go:build unix

package agentproto

import (
	"io"
	"net"
	"strings"
	"testing"
)

// The PTY frame protocol + host-PTY server are exercised over net.Pipe, which
// stands in for the vsock transport used between the Firecracker host and guest.

func TestServePTYRoundTripReportsOutputAndExit(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	go func() {
		_ = ServePTY(serverConn, PTYOpenParams{
			Cmd:  []string{"/bin/sh", "-c", "printf hi; exit 7"},
			Cols: 80, Rows: 24,
		})
		serverConn.Close()
	}()

	stream := NewPTYStream(clientConn)
	out, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(out), "hi") {
		t.Fatalf("output = %q, want it to contain %q", out, "hi")
	}
	code, _ := stream.Wait()
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
}

func TestServePTYResizePropagates(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	go func() {
		_ = ServePTY(serverConn, PTYOpenParams{
			Cmd:  []string{"/bin/sh", "-c", "sleep 0.1; stty size"},
			Cols: 80, Rows: 24,
		})
		serverConn.Close()
	}()

	stream := NewPTYStream(clientConn)
	if err := stream.Resize(100, 40); err != nil { // cols=100 rows=40
		t.Fatalf("resize: %v", err)
	}
	out, _ := io.ReadAll(stream)
	stream.Wait()
	if !strings.Contains(string(out), "40 100") {
		t.Fatalf("stty size = %q, want it to contain %q", out, "40 100")
	}
}
