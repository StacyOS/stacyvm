//go:build unix

package main

import (
	"os"
	"os/signal"
	"syscall"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// watchWindowResize relays terminal size changes (SIGWINCH) to the remote PTY
// until the returned stop function is called.
func watchWindowResize(sess *gossh.Session, fd int) func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-sigCh:
				if w, h, err := term.GetSize(fd); err == nil {
					_ = sess.WindowChange(h, w)
				}
			}
		}
	}()
	return func() {
		signal.Stop(sigCh)
		close(done)
	}
}
