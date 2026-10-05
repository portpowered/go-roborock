package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestDuplicatePromptFilePreservesOriginal(t *testing.T) {
	t.Parallel()

	original, err := os.CreateTemp(t.TempDir(), "prompt")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = original.Close() }()

	duplicate, err := duplicatePromptFile(original)
	if err != nil {
		t.Fatal(err)
	}

	err = duplicate.Close()
	if err != nil {
		t.Fatal(err)
	}

	_, err = original.WriteString("still owned by the caller")
	if err != nil {
		t.Fatalf("closing duplicate invalidated original: %v", err)
	}
}

func TestReadPromptLineCancelsBlockedNativeRead(t *testing.T) {
	t.Parallel()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = reader.Close(); _ = writer.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	finished := make(chan error, 1)

	go func() {
		_, readErr := readPromptLine(ctx, reader, false)
		finished <- readErr
	}()

	// Allow the worker to enter an actual native read before requesting cancellation.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err = <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("native read cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = writer.Close()

		t.Fatal("native read did not terminate after cancellation")
	}
}

func TestDuplicatePromptFileRejectsNil(t *testing.T) {
	t.Parallel()

	_, err := duplicatePromptFile(nil)
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("nil input accepted: %v", err)
	}
}
