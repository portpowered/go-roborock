//go:build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

func duplicatePromptFile(original *os.File) (*os.File, error) {
	if original == nil {
		return nil, fmt.Errorf("duplicate prompt input: %w", os.ErrInvalid)
	}

	descriptor, err := syscall.Dup(int(original.Fd()))
	if err != nil {
		return nil, fmt.Errorf("duplicate prompt input: %w", err)
	}

	syscall.CloseOnExec(descriptor)

	return os.NewFile(uintptr(descriptor), original.Name()), nil
}
