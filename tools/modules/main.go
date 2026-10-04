// Command modules verifies the CLI module against the local SDK without publishing replacements.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

const (
	sdkModule       = "github.com/portpowered/go-roborock"
	modCommand      = "mod"
	privateFileMode = 0o600
)

var errModuleDrift = errors.New("CLI module metadata drift")

func main() {
	err := verify(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func verify(ctx context.Context) error {
	directory := "cmd/go-roborock"

	published, err := publishedChecksums(directory)
	if err != nil {
		return err
	}

	if published {
		return verifyPublished(ctx, directory)
	}

	file, err := os.CreateTemp(directory, ".verify-*.mod")
	if err != nil {
		return fmt.Errorf("create temporary modfile: %w", err)
	}

	name := file.Name()

	err = file.Close()
	if err != nil {
		return fmt.Errorf("close temporary modfile: %w", err)
	}

	sumName := strings.TrimSuffix(name, ".mod") + ".sum"
	defer cleanup(name, sumName)

	for _, suffix := range []string{".mod", ".sum"} {
		original := filepath.Join(directory, "go"+suffix)

		data, readErr := os.ReadFile(original) //nolint:gosec // Read only local CLI metadata.
		if readErr != nil {
			return fmt.Errorf("read CLI metadata: %w", readErr)
		}

		target := strings.TrimSuffix(name, ".mod") + suffix

		writeErr := os.WriteFile(target, data, privateFileMode) //nolint:gosec // Temp modfile beside local CLI module.
		if writeErr != nil {
			return fmt.Errorf("copy CLI metadata: %w", writeErr)
		}
	}

	modfile := filepath.Base(name)
	for _, args := range [][]string{
		{modCommand, "edit", "-modfile=" + modfile, "-replace=" + sdkModule + "=../.."},
		{modCommand, "tidy", "-modfile=" + modfile},
		{modCommand, "edit", "-modfile=" + modfile, "-dropreplace=" + sdkModule},
	} {
		command := exec.CommandContext(ctx, "go", args...) //nolint:gosec // LIB-02: fixed local module checks; no shell.
		command.Dir = directory

		command.Env = append(withoutWorkspace(os.Environ()), "GOWORK=off")

		output, runErr := command.CombinedOutput()
		if runErr != nil {
			return fmt.Errorf("check CLI module: %w: %s", runErr, output)
		}
	}

	return compare(directory, name, sumName)
}

func publishedChecksums(directory string) (bool, error) {
	module, err := os.ReadFile(filepath.Join(directory, "go.mod")) //nolint:gosec // Fixed repository CLI metadata.
	if err != nil {
		return false, fmt.Errorf("read CLI module: %w", err)
	}

	version, err := requiredSDKVersion(module)
	if err != nil {
		return false, err
	}

	sums, err := os.ReadFile(filepath.Join(directory, "go.sum")) //nolint:gosec // Fixed repository CLI checksums.
	if err != nil {
		return false, fmt.Errorf("read CLI checksums: %w", err)
	}

	return matchingChecksums(sums, version), nil
}

func requiredSDKVersion(data []byte) (string, error) {
	module, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return "", fmt.Errorf("parse CLI module: %w", err)
	}

	if len(module.Replace) != 0 {
		return "", fmt.Errorf("%w: published CLI must not contain replacements", errModuleDrift)
	}

	for _, requirement := range module.Require {
		if requirement.Mod.Path == sdkModule {
			return requirement.Mod.Version, nil
		}
	}

	return "", fmt.Errorf("%w: CLI must require the SDK", errModuleDrift)
}

func matchingChecksums(data []byte, version string) bool {
	var modulePresent, metadataPresent bool

	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[0] != sdkModule {
			continue
		}

		switch fields[1] {
		case version:
			modulePresent = true
		case version + "/go.mod":
			metadataPresent = true
		}
	}

	return modulePresent && metadataPresent
}

func verifyPublished(ctx context.Context, directory string) error {
	command := exec.CommandContext(ctx, "go", "mod", "tidy", "-diff")
	command.Dir = directory

	command.Env = append(withoutWorkspace(os.Environ()), "GOWORK=off")

	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("verify published CLI module metadata: %w: %s", err, output)
	}

	return nil
}

func compare(directory, name, sumName string) error {
	pairs := [][2]string{{filepath.Join(directory, "go.mod"), name}, {filepath.Join(directory, "go.sum"), sumName}}
	for _, pair := range pairs {
		original, err := os.ReadFile(pair[0])
		if err != nil {
			return fmt.Errorf("read original metadata: %w", err)
		}

		checked, err := os.ReadFile(pair[1])
		if err != nil {
			return fmt.Errorf("read checked metadata: %w", err)
		}

		if !bytes.Equal(bytes.TrimSpace(original), bytes.TrimSpace(checked)) {
			return fmt.Errorf("%w: %s differs after tidy against local SDK", errModuleDrift, pair[0])
		}
	}

	return nil
}

func withoutWorkspace(environment []string) []string {
	result := make([]string, 0, len(environment))

	for _, value := range environment {
		if !strings.HasPrefix(value, "GOWORK=") {
			result = append(result, value)
		}
	}

	return result
}

func cleanup(names ...string) {
	for _, name := range names {
		err := os.Remove(name)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, "remove temporary metadata:", err)
		}
	}
}
