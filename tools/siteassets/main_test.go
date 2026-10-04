package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNestedFlightAliasesPreserveAssets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := "docs/guides/maps/__next.docs/$oc$slug/__PAGE__.txt"
	target := "docs/guides/maps/__next.docs.$oc$slug.__PAGE__.txt"
	payload := []byte{'1', ':', 0, '\n', 0xff}
	writeAsset(t, root, source, payload)
	writeAsset(t, root, "docs/plain.txt", []byte("unrelated"))

	for range 2 {
		err := run(root)
		if err != nil {
			t.Fatal(err)
		}

		assertAsset(t, root, source, payload)
		assertAsset(t, root, target, payload)
		assertAsset(t, root, "docs/plain.txt", []byte("unrelated"))
	}

	_, err := os.Stat(filepath.Join(root, "docs", "plain.txt.txt"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unrelated asset received an alias: %v", err)
	}
}

func TestConflictingAliasRemainsUntouched(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := "route/__next.docs/$oc$slug/__PAGE__.txt"
	target := "route/__next.docs.$oc$slug.__PAGE__.txt"

	writeAsset(t, root, source, []byte("source"))
	writeAsset(t, root, target, []byte("existing"))

	err := run(root)
	if !errors.Is(err, errConflict) {
		t.Fatalf("conflict was not rejected: %v", err)
	}

	assertAsset(t, root, source, []byte("source"))
	assertAsset(t, root, target, []byte("existing"))
}

func TestSymlinkAssetsAreSkipped(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	writeAsset(t, outside, "private.txt", []byte("outside"))
	writeAsset(t, root, "route/__next.docs/$oc$slug/__PAGE__.txt", []byte("source"))
	target := filepath.Join(root, "route", "__next.docs.$oc$slug.__PAGE__.txt")

	err := os.Symlink(filepath.Join(outside, "private.txt"), target)
	if err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	err = os.Symlink(outside, filepath.Join(root, "__next.private"))
	if err != nil {
		t.Fatal(err)
	}

	err = run(root)
	if err != nil {
		t.Fatal(err)
	}

	assertAsset(t, outside, "private.txt", []byte("outside"))

	_, err = os.Stat(filepath.Join(root, "__next.private.private.txt"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("symlink directory escaped root: %v", err)
	}
}

func TestAliasCannotEscapeRoot(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	rootPath := filepath.Join(directory, "site")
	writeAsset(t, rootPath, "source.txt", []byte("source"))
	writeAsset(t, directory, "private.txt", []byte("private"))

	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}

	err = createAlias(root, "source.txt", "../private.txt")

	closeErr := root.Close()
	if closeErr != nil {
		t.Fatal(closeErr)
	}

	if err == nil {
		t.Fatal("alias escaped its export root")
	}

	assertAsset(t, directory, "private.txt", []byte("private"))
}

func writeAsset(t *testing.T, root, name string, data []byte) {
	t.Helper()

	target := filepath.Join(root, filepath.FromSlash(name))

	err := os.MkdirAll(filepath.Dir(target), 0o750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(target, data, 0o600)
	if err != nil {
		t.Fatal(err)
	}
}

func assertAsset(t *testing.T, root, name string, expected []byte) {
	t.Helper()

	// Read only assets inside this test's temporary directory.
	actual, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name))) //nolint:gosec // Test-owned assets.
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(actual, expected) {
		t.Fatalf("asset %s changed: %q", name, actual)
	}
}
