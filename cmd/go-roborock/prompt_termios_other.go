//go:build aix || solaris

package main

import "golang.org/x/sys/unix"

const promptReadTermios = unix.TCGETS
const promptWriteTermios = unix.TCSETS
