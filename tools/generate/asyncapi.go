package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/go-roborock/internal/schemaadapter"
)

func projectSchema(source string) (string, func(), error) {
	data, err := os.ReadFile(source) //nolint:gosec // Repository-owned schema discovery.
	if err != nil {
		return "", nil, fmt.Errorf("read MQTT schema: %w", err)
	}

	projected, err := schemaadapter.Project(data)
	if err != nil {
		return "", nil, fmt.Errorf("project MQTT schema: %w", err)
	}

	directory, err := os.MkdirTemp("", "roborock-components-")
	if err != nil {
		return "", nil, fmt.Errorf("create component adapter: %w", err)
	}

	cleanup := func() {
		removeErr := os.RemoveAll(directory)
		if removeErr != nil {
			fmt.Fprintln(os.Stderr, removeErr)
		}
	}

	target := filepath.Join(directory, strings.TrimSuffix(filepath.Base(source), ".asyncapi.yaml")+".openapi.yaml")

	err = os.WriteFile(target, projected, privateFileMode) //nolint:gosec // Write generator-owned temporary path.
	if err != nil {
		cleanup()

		return "", nil, fmt.Errorf("write component adapter: %w", err)
	}

	return target, cleanup, nil
}

func generateInput(ctx context.Context, schema string, constants map[string]string, check bool) error {
	if !strings.HasSuffix(schema, ".asyncapi.yaml") {
		return generateSchema(ctx, schema, constants, check)
	}

	target, cleanup, err := projectSchema(schema)
	if err != nil {
		return err
	}

	defer cleanup()

	return generateSchema(ctx, target, constants, check)
}
