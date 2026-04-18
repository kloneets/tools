//go:build linux

package app

import "golang.org/x/sys/unix"

const (
	termiosReadRequest  = unix.TCGETS
	termiosWriteRequest = unix.TCSETS
)
