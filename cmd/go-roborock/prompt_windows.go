package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"golang.org/x/sys/windows"
)

const promptCancelInterval = 10 * time.Millisecond

type promptThread struct {
	handle windows.Handle
	err    error
}

type promptReadResult struct {
	value string
	err   error
}

func duplicatePromptFile(original *os.File) (*os.File, error) {
	if original == nil {
		return nil, fmt.Errorf("duplicate prompt input: %w", os.ErrInvalid)
	}

	process := windows.CurrentProcess()

	var duplicate windows.Handle

	err := windows.DuplicateHandle(process, windows.Handle(original.Fd()), process,
		&duplicate, 0, false, windows.DUPLICATE_SAME_ACCESS)
	if err != nil {
		return nil, fmt.Errorf("duplicate prompt input: %w", err)
	}

	return os.NewFile(uintptr(duplicate), original.Name()), nil
}

func beginHiddenPrompt(file *os.File) (func(), error) {
	if file == nil {
		return nil, fmt.Errorf("hide prompt input: %w", os.ErrInvalid)
	}

	handle := windows.Handle(file.Fd())

	var mode uint32

	err := windows.GetConsoleMode(handle, &mode)
	if err != nil {
		return nil, fmt.Errorf("read prompt console mode: %w", err)
	}

	err = windows.SetConsoleMode(handle, mode&^windows.ENABLE_ECHO_INPUT)
	if err != nil {
		return nil, fmt.Errorf("hide prompt input: %w", err)
	}

	return func() { _ = windows.SetConsoleMode(handle, mode) }, nil
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

	cancelRead := windows.NewLazySystemDLL("kernel32.dll").NewProc("CancelSynchronousIo")

	err := cancelRead.Find()
	if err != nil {
		return "", fmt.Errorf("load prompt cancellation: %w", err)
	}

	started := make(chan promptThread, 1)

	finished := make(chan promptReadResult, 1)
	go readPromptOnThread(ctx, file, started, finished)

	thread := <-started
	if thread.err != nil {
		return "", thread.err
	}

	defer func() { _ = windows.CloseHandle(thread.handle) }()

	select {
	case result := <-finished:
		if ctx.Err() != nil {
			return "", fmt.Errorf("read prompt input: %w", ctx.Err())
		}

		return result.value, result.err
	case <-ctx.Done():
		cancelPromptThread(cancelRead, thread.handle, finished)

		return "", fmt.Errorf("read prompt input: %w", ctx.Err())
	}
}

func readPromptOnThread(
	ctx context.Context, file *os.File, started chan<- promptThread, finished chan<- promptReadResult,
) {
	runtime.LockOSThread()

	defer runtime.UnlockOSThread()

	handle, err := windows.OpenThread(windows.THREAD_TERMINATE, false, windows.GetCurrentThreadId())
	if err != nil {
		err = fmt.Errorf("open prompt reader thread: %w", err)
	}

	started <- promptThread{handle: handle, err: err}

	if err != nil {
		return
	}

	err = ctx.Err()
	if err != nil {
		finished <- promptReadResult{value: "", err: err}

		return
	}

	value, err := promptCustomer(bufio.NewReader(file), io.Discard, "")
	finished <- promptReadResult{value: value, err: err}
}

func cancelPromptThread(cancelRead *windows.LazyProc, thread windows.Handle, finished <-chan promptReadResult) {
	ticker := time.NewTicker(promptCancelInterval)
	defer ticker.Stop()

	for {
		// Cancellation can race the worker entering ReadConsole. Keep canceling until it exits.
		_, _, _ = cancelRead.Call(uintptr(thread))

		select {
		case <-finished:
			return
		case <-ticker.C:
		}
	}
}
