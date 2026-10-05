package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsProfilePrivateCreation(t *testing.T) {
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
}

func TestWindowsProfileRejectsBroadAndNullACL(t *testing.T) {
	t.Parallel()

	for _, sddl := range []string{"D:P(A;;FA;;;WD)", "D:NO_ACCESS_CONTROL"} {
		t.Run(sddl, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "auth.json")

			err := os.WriteFile(path, []byte("private"), credentialFileMode)
			if err != nil {
				t.Fatal(err)
			}

			handle, err := profileHandle(path, true, false)
			if err != nil {
				t.Fatal(err)
			}

			defer func() { _ = windows.CloseHandle(handle) }()

			descriptor, err := windows.SecurityDescriptorFromString(sddl)
			if err != nil {
				t.Fatal(err)
			}

			dacl, _, err := descriptor.DACL()
			if err != nil {
				t.Fatal(err)
			}

			err = windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT,
				windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
			if err != nil {
				t.Fatal(err)
			}

			err = checkPrivateProfileFile(path)
			if !errors.Is(err, os.ErrPermission) {
				t.Fatalf("broad ACL accepted: %v", err)
			}
		})
	}
}

func TestWindowsProfilePreservesExistingDirectoryACL(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()

	handle, err := profileHandle(directory, false, true)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = windows.CloseHandle(handle) }()

	before, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}

	err = ensurePrivateProfileDir(directory)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("existing inherited directory ACL accepted: %v", err)
	}

	after, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}

	if before.String() != after.String() {
		t.Fatal("existing parent directory ACL was changed")
	}
}

func TestWindowsProfileRejectsDirectory(t *testing.T) {
	t.Parallel()

	err := checkPrivateProfileFile(t.TempDir())
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("directory accepted as profile: %v", err)
	}
}

func TestWindowsProfileRejectsSymlink(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	link := filepath.Join(directory, "link")

	err := os.WriteFile(target, nil, credentialFileMode)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Symlink(target, link)
	if errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) {
		t.Skip("Windows account cannot create symlinks")
	}
	if err != nil {
		t.Fatal(err)
	}

	err = checkPrivateProfileFile(link)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("symlink accepted as profile: %v", err)
	}
}
