//go:build !windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUnixProfilePermissions(t *testing.T) {
	t.Parallel()

	directory := filepath.Join(t.TempDir(), "profile")

	err := ensurePrivateProfileDir(directory)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(directory, "auth.json")

	file, err := createPrivateProfileFile(path)
	if err != nil {
		t.Fatal(err)
	}

	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = checkPrivateProfileFile(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = createPrivateProfileFile(path)
	if err == nil {
		t.Fatal("existing profile was overwritten")
	}

	//nolint:gosec // LIB-19: synthetic broad permissions verify fail-closed credential reads.
	err = os.Chmod(path, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	err = checkPrivateProfileFile(path)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("broad file permissions accepted: %v", err)
	}

	//nolint:gosec // LIB-19: synthetic shared directory verifies refusal without modifying its permissions.
	err = os.Chmod(directory, 0o755)
	if err != nil {
		t.Fatal(err)
	}

	err = ensurePrivateProfileDir(directory)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("broad directory permissions accepted: %v", err)
	}
}

func TestUnixProfileRejectsSymlink(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "profile")
	target := filepath.Join(directory, "target")

	err := os.WriteFile(target, nil, credentialFileMode)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Symlink(target, path)
	if err != nil {
		t.Fatal(err)
	}

	err = checkPrivateProfileFile(path)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("symlink profile accepted: %v", err)
	}
}
