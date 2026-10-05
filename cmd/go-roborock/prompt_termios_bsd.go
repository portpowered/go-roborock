//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package main

import "golang.org/x/sys/unix"

const promptReadTermios = unix.TIOCGETA
const promptWriteTermios = unix.TIOCSETA
