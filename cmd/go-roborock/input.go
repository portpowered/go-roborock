package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const maxInputBytes = 1024 * 1024
const credentialFileMode = 0o600

func readInput(path string, stdin io.Reader, getenv func(string) string) (CommandInput, error) {
	var input CommandInput

	reader := stdin

	if path != "-" {
		file, err := os.Open(path) //nolint:gosec // G304: the user explicitly selects the input file.
		if err != nil {
			return input, fmt.Errorf("open input: %w", err)
		}

		defer func() { _ = file.Close() }()

		reader = file
	} else if value := getenv("ROBOROCK_INPUT"); value != "" {
		reader = strings.NewReader(value)
	}

	data, err := io.ReadAll(io.LimitReader(reader, maxInputBytes+1))
	if err != nil {
		return input, fmt.Errorf("read input: %w", err)
	}

	if len(data) > maxInputBytes {
		return input, errInputSize
	}

	if !strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
		return input, errInputObject
	}

	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()

	err = decoder.Decode(&input)
	if err != nil {
		return input, errInputFields
	}

	var extra json.RawMessage

	err = decoder.Decode(&extra)
	if !errors.Is(err, io.EOF) {
		return input, errInputTrailing
	}

	return input, nil
}

func exportCredentials(path string, value any) error {
	//nolint:gosec // G304: explicit export destination; exclusive creation refuses existing paths and symlinks.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, credentialFileMode)
	if err != nil {
		return fmt.Errorf("create credential export: %w", err)
	}

	err = json.NewEncoder(file).Encode(value)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(path)

		return fmt.Errorf("write credential export: %w", err)
	}

	err = file.Close()
	if err != nil {
		return fmt.Errorf("close credential export: %w", err)
	}

	return nil
}
