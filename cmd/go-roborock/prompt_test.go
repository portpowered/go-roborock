package main

import (
	"errors"
	"os"
	"testing"
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

func TestDuplicatePromptFileRejectsNil(t *testing.T) {
	t.Parallel()

	_, err := duplicatePromptFile(nil)
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("nil input accepted: %v", err)
	}
}
