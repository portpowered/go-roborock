//go:build !windows

package main

import (
	"fmt"
	"os"
)

const privateDirectoryMode = 0o700
const otherProfilePermissionBits = 0o077

func ensurePrivateProfileDir(path string) error {
	err := os.MkdirAll(path, privateDirectoryMode)
	if err != nil {
		return fmt.Errorf("create profile directory: %w", err)
	}

	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect profile directory: %w", err)
	}

	if !info.IsDir() || info.Mode().Perm()&otherProfilePermissionBits != 0 {
		return fmt.Errorf("profile directory must be private: %w", os.ErrPermission)
	}

	return nil
}

func checkPrivateProfileFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect profile file: %w", err)
	}

	if !info.Mode().IsRegular() || info.Mode().Perm()&otherProfilePermissionBits != 0 {
		return fmt.Errorf("profile file must be private and regular: %w", os.ErrPermission)
	}

	return nil
}

func createPrivateProfileFile(path string) (*os.File, error) {
	//nolint:gosec // LIB-19: explicit profile destination; exclusive creation refuses an existing object.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, credentialFileMode)
	if err != nil {
		return nil, fmt.Errorf("create private profile: %w", err)
	}

	return file, nil
}
