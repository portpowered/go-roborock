package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSemanticEnumsCannotCrossRequestTypes(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	directory := t.TempDir()
	module := "module consumer.example/typed\n\ngo 1.24.0\n\nrequire github.com/portpowered/go-roborock v0.0.0\n"
	module += fmt.Sprintf("replace github.com/portpowered/go-roborock => %q\n", filepath.ToSlash(root))
	writeProbe(t, filepath.Join(directory, "go.mod"), []byte(module))

	positive := "var _ = sdk.SetFanSpeedRequest{Speed: sdk.FanSpeedMax}\n"
	positive += "var _ = sdk.SetWaterModeRequest{Mode: sdk.WaterModeHigh}"

	output, err := buildConsumer(t, directory, positive)
	if err != nil {
		t.Fatalf("valid typed consumer: %v: %s", err, output)
	}

	for _, source := range []string{
		"var _ = sdk.SetFanSpeedRequest{Speed: sdk.WaterModeHigh}",
		"var _ = sdk.SetWaterModeRequest{Mode: sdk.FanSpeedMax}",
	} {
		output, err = buildConsumer(t, directory, source)
		if err == nil || !strings.Contains(string(output), "cannot use") {
			t.Fatalf("crossed semantic enum did not produce a type error: %v: %s", err, output)
		}
	}
}

func buildConsumer(t *testing.T, directory, source string) ([]byte, error) {
	t.Helper()

	complete := "package consumer\nimport sdk \"github.com/portpowered/go-roborock/pkg/roborock\"\n" + source + "\n"
	writeProbe(t, filepath.Join(directory, "consumer.go"), []byte(complete))
	command := exec.CommandContext(t.Context(), "go", "build", "-mod=mod", "./...")
	command.Dir = directory

	command.Env = append(os.Environ(), "GOWORK=off")

	output, err := command.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("build external consumer: %w", err)
	}

	return output, nil
}
