//go:build !unix

package main

import (
	gossh "golang.org/x/crypto/ssh"
)

// watchWindowResize is a no-op on platforms without SIGWINCH (e.g. Windows).
func watchWindowResize(sess *gossh.Session, fd int) func() {
	return func() {}
}
