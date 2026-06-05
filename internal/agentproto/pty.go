package agentproto

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"
)

// MethodPTYOpen requests an interactive PTY. After the host sends a Request with
// this method, both sides switch the connection to the binary PTY frame
// protocol below (one PTY per connection).
const MethodPTYOpen = "pty_open"

// PTYOpenParams are the parameters for an interactive PTY session.
type PTYOpenParams struct {
	Cmd     []string          `json:"cmd,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	WorkDir string            `json:"work_dir,omitempty"`
	Term    string            `json:"term,omitempty"`
	Cols    uint16            `json:"cols,omitempty"`
	Rows    uint16            `json:"rows,omitempty"`
}

// PTY frame wire format: [type:1][len:uint32 BE][payload]. DATA flows both ways;
// RESIZE/SIGNAL are host->guest; EXIT is guest->host.
type ptyFrameType byte

const (
	ptyData   ptyFrameType = 1
	ptyResize ptyFrameType = 2
	ptySignal ptyFrameType = 3
	ptyExit   ptyFrameType = 4
)

const maxPTYFrame = 1 << 20 // 1 MB

func writePTYFrame(w io.Writer, t ptyFrameType, payload []byte) error {
	var hdr [5]byte
	hdr[0] = byte(t)
	binary.BigEndian.PutUint32(hdr[1:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

func readPTYFrame(r io.Reader) (ptyFrameType, []byte, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[1:])
	if n > maxPTYFrame {
		return 0, nil, errors.New("pty frame too large")
	}
	payload := make([]byte, n)
	if n > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}
	}
	return ptyFrameType(hdr[0]), payload, nil
}

func encodeResize(cols, rows uint16) []byte {
	var b [4]byte
	binary.BigEndian.PutUint16(b[0:], cols)
	binary.BigEndian.PutUint16(b[2:], rows)
	return b[:]
}

func decodeResize(p []byte) (cols, rows uint16, ok bool) {
	if len(p) < 4 {
		return 0, 0, false
	}
	return binary.BigEndian.Uint16(p[0:]), binary.BigEndian.Uint16(p[2:]), true
}

func encodeExit(code int) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(int32(code)))
	return b[:]
}

func decodeExit(p []byte) int {
	if len(p) < 4 {
		return -1
	}
	return int(int32(binary.BigEndian.Uint32(p)))
}

// PTYStream is the host-side client of the PTY frame protocol. It satisfies the
// providers.PTYSession interface structurally (Read/Write/Close/Resize/Signal/
// Wait), so the Firecracker provider can return it directly.
type PTYStream struct {
	conn   io.ReadWriteCloser
	wmu    sync.Mutex
	pr     *io.PipeReader
	pw     *io.PipeWriter
	exit   int
	exitCh chan struct{}
	once   sync.Once
}

// NewPTYStream wraps a connection (already switched to PTY mode) as a session.
func NewPTYStream(conn io.ReadWriteCloser) *PTYStream {
	pr, pw := io.Pipe()
	s := &PTYStream{conn: conn, pr: pr, pw: pw, exitCh: make(chan struct{})}
	go s.readLoop()
	return s
}

func (s *PTYStream) readLoop() {
	for {
		t, payload, err := readPTYFrame(s.conn)
		if err != nil {
			_ = s.pw.CloseWithError(io.EOF)
			s.finish(-1)
			return
		}
		switch t {
		case ptyData:
			if _, werr := s.pw.Write(payload); werr != nil {
				return
			}
		case ptyExit:
			_ = s.pw.Close()
			s.finish(decodeExit(payload))
			return
		}
	}
}

func (s *PTYStream) finish(code int) {
	s.once.Do(func() {
		s.exit = code
		close(s.exitCh)
	})
}

func (s *PTYStream) Read(p []byte) (int, error) { return s.pr.Read(p) }

func (s *PTYStream) Write(p []byte) (int, error) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if err := writePTYFrame(s.conn, ptyData, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *PTYStream) Resize(cols, rows uint16) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return writePTYFrame(s.conn, ptyResize, encodeResize(cols, rows))
}

func (s *PTYStream) Signal(sig string) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return writePTYFrame(s.conn, ptySignal, []byte(sig))
}

func (s *PTYStream) Wait() (int, error) {
	<-s.exitCh
	return s.exit, nil
}

func (s *PTYStream) Close() error { return s.conn.Close() }
