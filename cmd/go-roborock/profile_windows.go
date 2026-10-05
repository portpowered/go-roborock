package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const privateDirectoryMode = 0o700

func privateProfileDescriptor() (*windows.SECURITY_DESCRIPTOR, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("find profile owner: %w", err)
	}

	// Protect the DACL from inherited grants. SYSTEM retains access for OS services.
	sddl := "D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)"

	descriptor, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return nil, fmt.Errorf("create private profile ACL: %w", err)
	}

	return descriptor, nil
}

func profileHandle(path string, writable, directory bool) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, fmt.Errorf("profile path: %w", err)
	}

	access := uint32(windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES)
	if writable {
		access |= windows.WRITE_DAC
	}

	sharing := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)

	handle, err := windows.CreateFile(name, access, sharing,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return windows.InvalidHandle, fmt.Errorf("open profile permissions: %w", err)
	}

	var info windows.ByHandleFileInformation

	err = windows.GetFileInformationByHandle(handle, &info)
	if err != nil {
		_ = windows.CloseHandle(handle)

		return windows.InvalidHandle, fmt.Errorf("inspect profile object: %w", err)
	}

	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory {
		_ = windows.CloseHandle(handle)

		return windows.InvalidHandle, fmt.Errorf("profile must be a direct regular file or directory: %w", os.ErrPermission)
	}

	return handle, nil
}

func privateProfileAttributes() (*windows.SecurityAttributes, error) {
	descriptor, err := privateProfileDescriptor()
	if err != nil {
		return nil, err
	}

	attributes := windows.SecurityAttributes{
		Length:             0,
		SecurityDescriptor: descriptor,
		InheritHandle:      0,
	}
	attributes.Length = uint32(unsafe.Sizeof(attributes))

	return &attributes, nil
}

func ensurePrivateProfileDir(path string) error {
	err := os.MkdirAll(filepath.Dir(path), privateDirectoryMode)
	if err != nil {
		return fmt.Errorf("create profile parent: %w", err)
	}

	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("profile directory path: %w", err)
	}

	attributes, err := privateProfileAttributes()
	if err != nil {
		return err
	}

	err = windows.CreateDirectory(name, attributes)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return fmt.Errorf("create private profile directory: %w", err)
	}

	return checkPrivateProfileObject(path, true)
}

func checkPrivateProfileFile(path string) error {
	return checkPrivateProfileObject(path, false)
}

func checkPrivateProfileObject(path string, directory bool) error {
	handle, err := profileHandle(path, false, directory)
	if err != nil {
		return err
	}

	defer func() { _ = windows.CloseHandle(handle) }()

	actual, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read profile permissions: %w", err)
	}

	expected, err := privateProfileDescriptor()
	if err != nil {
		return err
	}

	if actual.String() != expected.String() {
		return fmt.Errorf("profile permissions must allow only the current user and SYSTEM: %w", os.ErrPermission)
	}

	return nil
}

func createPrivateProfileFile(path string) (*os.File, error) {
	attributes, err := privateProfileAttributes()
	if err != nil {
		return nil, err
	}

	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("profile path: %w", err)
	}

	handle, err := windows.CreateFile(name, windows.GENERIC_WRITE, 0, attributes,
		windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("create private profile: %w", err)
	}

	return os.NewFile(uintptr(handle), path), nil
}
