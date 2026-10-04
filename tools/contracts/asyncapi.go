package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/portpowered/go-roborock/internal/schemaadapter"
)

func asyncSchemas(ctx context.Context, root string) error {
	names, err := filepath.Glob(filepath.Join(root, "api", "*.asyncapi.yaml"))
	if err != nil {
		return fmt.Errorf("find MQTT schemas: %w", err)
	}

	for _, name := range names {
		err = validateAsyncSchema(ctx, name)
		if err != nil {
			return err
		}
	}

	return nil
}

func validateAsyncSchema(ctx context.Context, name string) error {
	npm := "npm"
	if runtime.GOOS == "windows" {
		npm = "npm.cmd"
	}
	// LIB-13: the official pinned validator checks the full AsyncAPI document.
	//nolint:gosec // Local schema path passed without shell expansion.
	command := exec.CommandContext(ctx, npm, "exec", "--yes", "--package=@asyncapi/cli@6.2.0",
		"--", "asyncapi", "validate", name)

	command.Env = append(os.Environ(), "CI=true", "SUPPRESS_NO_CONFIG_WARNING=true")

	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("validate AsyncAPI %s: %w: %s", name, err, output)
	}

	data, err := os.ReadFile(name) //nolint:gosec // Local schema discovery.
	if err != nil {
		return fmt.Errorf("read MQTT schema: %w", err)
	}

	projected, err := schemaadapter.Project(data)
	if err != nil {
		return fmt.Errorf("project MQTT components: %w", err)
	}

	loader := openapi3.NewLoader()
	loader.Context = ctx

	doc, err := loader.LoadFromData(projected)
	if err != nil {
		return fmt.Errorf("load MQTT components: %w", err)
	}

	err = doc.Validate(ctx)
	if err != nil {
		return fmt.Errorf("validate MQTT components: %w", err)
	}

	return nil
}
