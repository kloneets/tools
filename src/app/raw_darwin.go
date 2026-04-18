//go:build darwin

package app

import "golang.org/x/sys/unix"

const (
	termiosReadRequest  = unix.TIOCGETA
	termiosWriteRequest = unix.TIOCSETA
)
