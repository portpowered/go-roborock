// Command docbundle joins schemas for documentation without changing wire owners.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

type object map[string]any

const (
	directoryMode  = 0o750
	fileMode       = 0o600
	componentParts = 2
)

type bundle struct {
	paths      object
	components object
	operations map[string]bool
}

var errBundle = errors.New("invalid documentation bundle")

func main() {
	source := flag.String("source", "api", "checked-in schema directory")
	output := flag.String("output", "tmp/docs.openapi.yaml", "docs-only bundled schema")

	flag.Parse()

	err := run(context.Background(), *source, *output)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, source, output string) error {
	files, err := filepath.Glob(filepath.Join(source, "*.openapi.yaml"))
	if err != nil {
		return fmt.Errorf("discover documentation schemas: %w", err)
	}

	data, err := build(ctx, files)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(output), directoryMode)
	if err != nil {
		return fmt.Errorf("create documentation directory: %w", err)
	}

	err = os.WriteFile(output, data, fileMode)
	if err != nil {
		return fmt.Errorf("write documentation bundle: %w", err)
	}

	return nil
}

func build(ctx context.Context, files []string) ([]byte, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no input schemas: %w", errBundle)
	}

	sources := make(map[string]string, len(files))

	for _, file := range files {
		sources[filepath.Base(file)] = strings.TrimSuffix(filepath.Base(file), ".openapi.yaml")
	}

	joined := bundle{paths: object{}, components: object{}, operations: map[string]bool{}}

	for _, file := range files {
		err := joined.add(ctx, file, sources)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
	}

	document := object{
		"openapi": "3.0.3",
		"info": object{"title": "Roborock SDK contracts", "version": "0.1.0",
			"description": "Documentation bundle of implementation-derived contracts; original schemas remain wire owners."},
		"paths": joined.paths, "components": joined.components,
	}

	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode documentation bundle: %w", err)
	}

	loader := openapi3.NewLoader()
	loader.Context = ctx

	spec, err := loader.LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("resolve bundled references: %w", err)
	}

	err = spec.Validate(ctx)
	if err != nil {
		return nil, fmt.Errorf("validate documentation bundle: %w", err)
	}

	return append(data, '\n'), nil
}

func (b bundle) add(ctx context.Context, file string, sources map[string]string) error {
	loader := openapi3.NewLoader()
	loader.Context = ctx
	loader.IsExternalRefsAllowed = true

	spec, err := loader.LoadFromFile(file)
	if err != nil {
		return fmt.Errorf("load source schema: %w", err)
	}

	data, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("encode source schema: %w", err)
	}

	var document object

	err = json.Unmarshal(data, &document)
	if err != nil {
		return fmt.Errorf("decode source document: %w", err)
	}

	name := filepath.Base(file)

	err = rewrite(document, name, sources)
	if err != nil {
		return err
	}

	components, _ := document["components"].(map[string]any)

	for category, value := range components {
		entries, _ := value.(map[string]any)

		target, ok := b.components[category].(object)
		if !ok {
			target = object{}
			b.components[category] = target
		}

		for key, entry := range entries {
			target[sources[name]+"."+key] = entry
		}
	}

	paths, _ := document["paths"].(map[string]any)

	return b.addPaths(paths, document["servers"])
}

func (b bundle) addPaths(paths map[string]any, servers any) error {
	for route, value := range paths {
		if _, exists := b.paths[route]; exists {
			return fmt.Errorf("duplicate path %s: %w", route, errBundle)
		}

		item, _ := value.(map[string]any)

		err := b.recordOperations(item)
		if err != nil {
			return err
		}

		if _, explicit := item["servers"]; !explicit && servers != nil {
			item["servers"] = servers
		}

		b.paths[route] = item
	}

	return nil
}

func (b bundle) recordOperations(item map[string]any) error {
	for _, value := range item {
		operation, ok := value.(map[string]any)
		if !ok {
			continue
		}

		operationID, _ := operation["operationId"].(string)
		if operationID == "" {
			continue
		}

		if b.operations[operationID] {
			return fmt.Errorf("duplicate operation %s: %w", operationID, errBundle)
		}

		b.operations[operationID] = true
	}

	return nil
}

func rewrite(value any, source string, sources map[string]string) error {
	switch node := value.(type) {
	case object:
		return rewrite(map[string]any(node), source, sources)
	case map[string]any:
		for key, child := range node {
			if key == "$ref" {
				ref, _ := child.(string)

				replacement, err := componentRef(ref, source, sources)
				if err != nil {
					return err
				}

				node[key] = replacement

				continue
			}

			err := rewrite(child, source, sources)
			if err != nil {
				return err
			}
		}
	case []any:
		for _, child := range node {
			err := rewrite(child, source, sources)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func componentRef(ref, source string, sources map[string]string) (string, error) {
	file, fragment, found := strings.Cut(ref, "#")
	if !found || !strings.HasPrefix(fragment, "/components/") {
		return "", fmt.Errorf("unsupported reference %s: %w", ref, errBundle)
	}

	if file != "" {
		if !fs.ValidPath(strings.TrimPrefix(file, "./")) {
			return "", fmt.Errorf("nonlocal reference %s: %w", ref, errBundle)
		}

		source = filepath.Base(file)
	}

	prefix, exists := sources[source]
	if !exists {
		return "", fmt.Errorf("uninventoried reference %s: %w", ref, errBundle)
	}

	parts := strings.SplitN(strings.TrimPrefix(fragment, "/components/"), "/", componentParts)
	if len(parts) != componentParts {
		return "", fmt.Errorf("invalid component reference %s: %w", ref, errBundle)
	}

	return "#/components/" + parts[0] + "/" + prefix + "." + parts[1], nil
}
