// Command contracts validates schema documents and the reviewed source boundary.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

const (
	manifestPath    = "api/source-inventory.json"
	privateFileMode = 0o600
)

var errContract = errors.New("contract verification failed")

// inventory binds each reviewed production file to its exact source bytes.
// Updating this file requires a new independent source audit, not just regeneration.
type inventory struct {
	Files map[string]string `json:"files"`
}

func main() {
	root := flag.String("root", ".", "repository root")
	snapshot := flag.Bool("snapshot", false, "record source hashes for independent audit; does not approve them")

	flag.Parse()

	err := run(context.Background(), *root, *snapshot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, root string, snapshot bool) error {
	err := schemas(ctx, root)
	if err != nil {
		return err
	}

	files, err := sources(root)
	if err != nil {
		return err
	}

	current := inventory{Files: make(map[string]string, len(files))}

	for _, name := range files {
		// LIB-13: name comes from the repository walker; source files must be read for hashing.
		data, readErr := os.ReadFile(filepath.Join(root, name)) //nolint:gosec // Repository-local source inventory.
		if readErr != nil {
			return fmt.Errorf("read source %s: %w", name, readErr)
		}

		var checkErr error
		if strings.HasSuffix(name, ".go") {
			checkErr = checkSource(name, data)
		}

		if checkErr != nil {
			return checkErr
		}

		// Git normalizes text to LF; preserve the same audit identity across checkout platforms.
		hash := sha256.Sum256(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")))
		current.Files[name] = hex.EncodeToString(hash[:])
	}

	if snapshot {
		err = verifyGenerated(ctx, root, files)
		if err != nil {
			return err
		}

		return saveInventory(root, current)
	}

	return compareInventory(root, current)
}

func saveInventory(root string, current inventory) error {
	data, marshalErr := json.MarshalIndent(current, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("encode inventory: %w", marshalErr)
	}

	writeErr := os.WriteFile(filepath.Join(root, manifestPath), append(data, '\n'), privateFileMode)
	if writeErr != nil {
		return fmt.Errorf("write inventory: %w", writeErr)
	}

	return nil
}

func compareInventory(root string, current inventory) error {
	data, err := os.ReadFile(filepath.Join(root, manifestPath)) //nolint:gosec // Read explicit repository audit manifest.
	if err != nil {
		return fmt.Errorf("read reviewed inventory: %w", err)
	}

	var approved inventory

	err = json.Unmarshal(data, &approved)
	if err != nil {
		return fmt.Errorf("decode reviewed inventory: %w", err)
	}

	if len(approved.Files) != len(current.Files) {
		return fmt.Errorf("%w: production file inventory changed; independent audit required", errContract)
	}

	for name, hash := range current.Files {
		if approved.Files[name] != hash {
			return fmt.Errorf("%w: unreviewed production source %s", errContract, name)
		}
	}

	return nil
}

func schemas(ctx context.Context, root string) error {
	names, err := filepath.Glob(filepath.Join(root, "api", "*.openapi.yaml"))
	if err != nil {
		return fmt.Errorf("find schemas: %w", err)
	}

	if len(names) == 0 {
		return fmt.Errorf("%w: no OpenAPI schemas", errContract)
	}

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	loader.Context = ctx
	for _, name := range names {
		doc, loadErr := loader.LoadFromFile(name)
		if loadErr != nil {
			return fmt.Errorf("load %s: %w", name, loadErr)
		}

		validErr := doc.Validate(ctx)
		if validErr != nil {
			return fmt.Errorf("validate %s: %w", name, validErr)
		}
	}

	return asyncSchemas(ctx, root)
}

func sources(root string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relative, relErr := filepath.Rel(root, name)
		if relErr != nil {
			return fmt.Errorf("resolve relative source: %w", relErr)
		}

		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if slices.Contains([]string{".git", "reference", "tools", "tests", "site", "tmp"}, relative) {
				return filepath.SkipDir
			}

			return nil
		}

		if strings.HasSuffix(relative, ".go") && !strings.HasSuffix(relative, "_test.go") {
			files = append(files, relative)
		}

		if contractInput(relative) {
			files = append(files, relative)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan production sources: %w", err)
	}

	inputs, err := generatorInputs(root)
	if err != nil {
		return nil, err
	}

	files = append(files, inputs...)
	slices.Sort(files)

	return files, nil
}

func generatorInputs(root string) ([]string, error) {
	inputs, err := filepath.Glob(filepath.Join(root, "tools", "*", "*.go"))
	if err != nil {
		return nil, fmt.Errorf("scan generator inputs: %w", err)
	}

	files := make([]string, 0, len(inputs))

	for _, name := range inputs {
		if strings.HasSuffix(name, "_test.go") ||
			(filepath.Base(filepath.Dir(name)) != "generate" && filepath.Base(filepath.Dir(name)) != "protogen") {
			continue
		}

		relative, relErr := filepath.Rel(root, name)
		if relErr != nil {
			return nil, fmt.Errorf("resolve generator input: %w", relErr)
		}

		files = append(files, filepath.ToSlash(relative))
	}

	return files, nil
}

func checkSource(name string, data []byte) error {
	file, err := parser.ParseFile(token.NewFileSet(), name, data, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse %s: %w", name, err)
	}

	err = checkImports(file)
	if err != nil {
		return err
	}

	if strings.HasSuffix(name, ".gen.go") && !generatedArtifact(name) {
		return fmt.Errorf("%w: unregistered generated artifact %s", errContract, name)
	}
	// Only exact inventoried generator paths may define provider wire fields.
	generated := generatedModel(name)
	if generated {
		marker := modelMarker(name)
		if !strings.Contains(string(data), marker) {
			return fmt.Errorf("%w: missing generated marker %s", errContract, name)
		}

		return nil
	}

	if strings.HasSuffix(name, ".gen.go") && handwrittenObject(file) != "" {
		return fmt.Errorf("%w: unregistered generated wire model %s", errContract, name)
	}

	violation := handwrittenObject(file)
	if violation != "" {
		return fmt.Errorf("%w: %s in %s", errContract, violation, name)
	}

	return nil
}

func checkImports(file *ast.File) error {
	for _, imported := range file.Imports {
		path := strings.Trim(imported.Path.Value, "\"")

		for _, excluded := range []string{"tools", "tests", "reference", "site", "tmp"} {
			prefix := "github.com/portpowered/go-roborock/" + excluded
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				return fmt.Errorf("%w: production import of excluded helper package %s", errContract, path)
			}
		}
	}

	return nil
}

func contractInput(name string) bool {
	if name == "go.mod" || name == "go.sum" || name == "go.work" {
		return true
	}

	if !strings.HasPrefix(name, "api/") || name == manifestPath {
		return false
	}

	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".proto")
}

func generatedModel(name string) bool {
	return slices.Contains([]string{
		"pkg/dependencymodels/auth.gen.go", "pkg/dependencymodels/devices.gen.go",
		"pkg/dependencymodels/mqtt.gen.go", "pkg/dependencymodels/vacuum.gen.go",
		"pkg/dependencymodels/camera.gen.go", "pkg/dependencymodels/a01_models.gen.go",
		"pkg/dependencymodels/maps.gen.go", "pkg/dependencymodels/b01.gen.go",
		"pkg/dependencymodels/map-format.gen.go", "internal/mapmodel/models.gen.go",
		"pkg/roborock/maps_models.gen.go", "pkg/roborock/map_client_models.gen.go",
		"pkg/dependencymodels/mapproto/b01_scmap.pb.go",
		"pkg/roborock/client_models.gen.go", "pkg/roborock/operation_models.gen.go",
		"pkg/roborock/a01_models.gen.go", "cmd/go-roborock/input.gen.go",
	}, name)
}

func generatedArtifact(name string) bool {
	return generatedModel(name) || slices.Contains([]string{
		"pkg/dependencymodels/auth_constants.gen.go", "pkg/dependencymodels/devices_constants.gen.go",
		"internal/protocol/a01-wire.gen.go", "internal/protocol/auth.gen.go", "internal/protocol/client-models.gen.go",
		"internal/protocol/mqtt.gen.go", "internal/protocol/rest.gen.go",
		"internal/protocol/maps.gen.go", "internal/protocol/b01.gen.go", "internal/protocol/map-format.gen.go",
		"pkg/dependencymodels/vacuum_constants.gen.go", "internal/mapmodel/models_constants.gen.go",
	}, name)
}

func verifyGenerated(ctx context.Context, root string, files []string) error {
	if !slices.ContainsFunc(files, generatedModel) {
		return nil
	}

	command := exec.CommandContext(ctx, "go", "run", "./tools/generate", "-check")
	command.Dir = root

	command.Env = append(os.Environ(), "GOWORK=off")

	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("snapshot requires exact reproducible generated models: %w: %s", err, output)
	}

	return nil
}

func handwrittenObject(file *ast.File) string {
	var violation string

	ast.Inspect(file, func(node ast.Node) bool {
		if directlyEncodedEmpty(file, node) {
			violation = "directly encoded empty anonymous object"
		}

		field, isField := node.(*ast.Field)
		if isField && field.Tag != nil && strings.Contains(field.Tag.Value, "json:") {
			violation = "handwritten JSON field"
		}

		literal, ok := node.(*ast.CompositeLit)
		if ok {
			if object, anonymous := literal.Type.(*ast.StructType); anonymous && len(object.Fields.List) != 0 {
				violation = "anonymous production object"
			}
		}

		return true
	})

	return violation
}

func directlyEncodedEmpty(file *ast.File, node ast.Node) bool {
	call, isCall := node.(*ast.CallExpr)
	if !isCall || !directJSONCall(file, call) {
		return false
	}

	return slices.ContainsFunc(call.Args, emptyAnonymousObject)
}

func directJSONCall(file *ast.File, call *ast.CallExpr) bool {
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector {
		return false
	}

	// Encode selectors are a conservative syntax guard, not receiver/dataflow analysis.
	if selector.Sel.Name == "Encode" {
		return true
	}

	if selector.Sel.Name != "Marshal" && selector.Sel.Name != "MarshalIndent" {
		return false
	}

	qualifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}

	for _, imported := range file.Imports {
		if strings.Trim(imported.Path.Value, "\"") != "encoding/json" {
			continue
		}

		name := "json"
		if imported.Name != nil {
			name = imported.Name.Name
		}

		if qualifier.Name == name {
			return true
		}
	}

	return false
}

func emptyAnonymousObject(expression ast.Expr) bool {
	switch value := expression.(type) {
	case *ast.ParenExpr:
		return emptyAnonymousObject(value.X)
	case *ast.UnaryExpr:
		return emptyAnonymousObject(value.X)
	case *ast.CompositeLit:
		object, ok := value.Type.(*ast.StructType)

		return ok && len(object.Fields.List) == 0
	default:
		return false
	}
}

func modelMarker(name string) string {
	switch name {
	case "pkg/roborock/maps_models.gen.go":
		return "Code generated by tools/generate from api/maps-models.openapi.yaml; DO NOT EDIT."
	case "pkg/dependencymodels/mapproto/b01_scmap.pb.go":
		return "Code generated by protoc-gen-go. DO NOT EDIT."
	default:
		return "Code generated by github.com/oapi-codegen/oapi-codegen/v2 version v2.5.1 DO NOT EDIT."
	}
}
