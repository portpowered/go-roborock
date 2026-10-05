package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestHiddenPromptPTYCancellationRestoresState(t *testing.T) {
	t.Parallel()

	slave := openPromptPTY(t)

	before, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	finished := make(chan error, 1)

	go func() {
		_, readErr := readPromptLine(ctx, slave, true)
		finished <- readErr
	}()

	waitForHiddenPTY(t, slave)
	cancel()

	select {
	case err = <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("hidden PTY cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hidden PTY read did not terminate")
	}

	after, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(before, after) {
		t.Fatal("PTY settings changed after canceled hidden read")
	}
}

func openPromptPTY(t *testing.T) *os.File {
	t.Helper()

	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = master.Close() })

	err = unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0)
	if err != nil {
		t.Fatal(err)
	}

	number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}

	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = slave.Close() })

	return slave
}

func waitForHiddenPTY(t *testing.T, slave *os.File) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
		if err != nil {
			t.Fatal(err)
		}

		if state.Lflag&unix.ECHO == 0 {
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatal("hidden PTY read did not suppress echo")
}
