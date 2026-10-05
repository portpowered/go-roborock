package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

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
