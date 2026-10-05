//go:build !windows

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

const promptPollMilliseconds = 50

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

func beginHiddenPrompt(file *os.File) (func(), error) {
	if file == nil {
		return nil, fmt.Errorf("hide prompt input: %w", os.ErrInvalid)
	}

	descriptor := int(file.Fd())

	state, err := unix.IoctlGetTermios(descriptor, promptReadTermios)
	if err != nil {
		return nil, fmt.Errorf("read prompt terminal mode: %w", err)
	}

	hidden := *state
	hidden.Lflag &^= unix.ECHO

	err = unix.IoctlSetTermios(descriptor, promptWriteTermios, &hidden)
	if err != nil {
		return nil, fmt.Errorf("hide prompt input: %w", err)
	}

	return func() { _ = unix.IoctlSetTermios(descriptor, promptWriteTermios, state) }, nil
}

func readPromptLine(ctx context.Context, file *os.File, hidden bool) (string, error) {
	if file == nil {
		return "", fmt.Errorf("read prompt input: %w", os.ErrInvalid)
	}

	if hidden {
		restore, err := beginHiddenPrompt(file)
		if err != nil {
			return "", err
		}

		defer restore()
	}

	return readPromptValue(ctx, file)
}

func waitForPromptInput(ctx context.Context, descriptor int32) error {
	descriptors := []unix.PollFd{{Fd: descriptor, Events: unix.POLLIN, Revents: 0}}

	for {
		err := ctx.Err()
		if err != nil {
			return fmt.Errorf("read prompt input: %w", err)
		}

		ready, err := unix.Poll(descriptors, promptPollMilliseconds)
		if errors.Is(err, unix.EINTR) {
			continue
		}

		if err != nil {
			return fmt.Errorf("wait for prompt input: %w", err)
		}

		if ready != 0 {
			return nil
		}
	}
}

func readPromptValue(ctx context.Context, file *os.File) (string, error) {
	descriptor := file.Fd()
	if descriptor > math.MaxInt32 {
		return "", fmt.Errorf("read prompt descriptor: %w", os.ErrInvalid)
	}

	var value strings.Builder

	for value.Len() <= maximumPromptBytes {
		err := waitForPromptInput(ctx, int32(descriptor))
		if err != nil {
			return "", err
		}

		var character [1]byte

		count, err := file.Read(character[:])
		if errors.Is(err, io.EOF) && value.Len() > 0 {
			break
		}

		if err != nil {
			return "", fmt.Errorf("read prompt input: %w", err)
		}

		if count != 0 && character[0] == '\n' {
			break
		}

		if count != 0 {
			_ = value.WriteByte(character[0])
		}
	}

	return finishPromptValue(&value)
}

func finishPromptValue(value *strings.Builder) (string, error) {
	result := strings.TrimSpace(value.String())
	if result == "" || value.Len() > maximumPromptBytes {
		return "", errPrompt
	}

	return result, nil
}
