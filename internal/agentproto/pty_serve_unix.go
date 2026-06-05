//go:build unix

package agentproto

import (
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

// ServePTY runs the requested command on a real pty inside the guest and relays
// it over the PTY frame protocol on conn. It returns when the process exits or
// the peer disconnects, and closes conn before returning. Intended to run in the
// guest agent; exercised in tests over net.Pipe.
func ServePTY(conn io.ReadWriteCloser, params PTYOpenParams) error {
	cmdline := params.Cmd
	if len(cmdline) == 0 {
		cmdline = []string{"/bin/sh"}
	}
	c := exec.Command(cmdline[0], cmdline[1:]...)
	if params.WorkDir != "" {
		c.Dir = params.WorkDir
	}
	c.Env = os.Environ()
	if params.Term != "" {
		c.Env = append(c.Env, "TERM="+params.Term)
	}
	for k, v := range params.Env {
		c.Env = append(c.Env, k+"="+v)
	}

	rows, cols := params.Rows, params.Cols
	if rows == 0 {
		rows = 24
	}
	if cols == 0 {
		cols = 80
	}

	var wmu sync.Mutex
	writeFrame := func(t ptyFrameType, payload []byte) {
		wmu.Lock()
		_ = writePTYFrame(conn, t, payload)
		wmu.Unlock()
	}

	f, err := pty.StartWithSize(c, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		writeFrame(ptyExit, encodeExit(1))
		_ = conn.Close()
		return err
	}

	procDone := make(chan int, 1)
	go func() { procDone <- waitCode(c) }()

	ptyDone := make(chan struct{})
	go func() {
		defer close(ptyDone)
		buf := make([]byte, 32*1024)
		for {
			n, rerr := f.Read(buf)
			if n > 0 {
				writeFrame(ptyData, buf[:n])
			}
			if rerr != nil {
				return
			}
		}
	}()

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			t, payload, rerr := readPTYFrame(conn)
			if rerr != nil {
				return
			}
			switch t {
			case ptyData:
				_, _ = f.Write(payload)
			case ptyResize:
				if cw, rh, ok := decodeResize(payload); ok {
					_ = pty.Setsize(f, &pty.Winsize{Rows: rh, Cols: cw})
				}
			case ptySignal:
				if c.Process != nil {
					if sg, ok := signalFromName(string(payload)); ok {
						_ = c.Process.Signal(sg)
					}
				}
			}
		}
	}()

	var code int
	select {
	case code = <-procDone:
		<-ptyDone // flush remaining output before reporting exit
	case <-readDone:
		if c.Process != nil {
			_ = c.Process.Kill()
		}
		code = <-procDone
	}

	// Report exit, then tear down in an order that guarantees no goroutine
	// touches the pty file concurrently with Close: closing conn stops the
	// frame-reader (the only other user of f), and we join both goroutines
	// before closing f.
	writeFrame(ptyExit, encodeExit(code))
	_ = conn.Close()
	<-readDone
	<-ptyDone
	_ = f.Close()
	return nil
}

func waitCode(c *exec.Cmd) int {
	err := c.Wait()
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}

func signalFromName(name string) (os.Signal, bool) {
	switch name {
	case "SIGINT", "INT":
		return syscall.SIGINT, true
	case "SIGTERM", "TERM":
		return syscall.SIGTERM, true
	case "SIGKILL", "KILL":
		return syscall.SIGKILL, true
	case "SIGHUP", "HUP":
		return syscall.SIGHUP, true
	case "SIGQUIT", "QUIT":
		return syscall.SIGQUIT, true
	case "SIGWINCH", "WINCH":
		return syscall.SIGWINCH, true
	default:
		return nil, false
	}
}
