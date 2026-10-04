package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

type site struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Symbol string `json:"symbol"`
}
type declaration struct {
	Symbol     string      `json:"symbol"`
	Kind       string      `json:"kind"`
	Definition site        `json:"definition"`
	Schema     schemaOwner `json:"schema"`
	Generator  string      `json:"generator"`
	Value      string      `json:"value,omitempty"`
	Uses       []site      `json:"productionUses"`
}
type boundary struct {
	Site      site     `json:"site"`
	Arguments []string `json:"argumentTypes"`
}
type codec struct {
	Definition site        `json:"definition"`
	Schema     schemaOwner `json:"schema"`
}
type report struct {
	Declarations []declaration `json:"schemaDeclarations"`
	Behavior     []site        `json:"untaggedBehaviorDeclarations"`
	Aliases      []site        `json:"handwrittenAliases"`
	Codecs       []codec       `json:"customJsonMethods"`
	Boundaries   []boundary    `json:"serializationAndNetworkCalls"`
}

func location(set *token.FileSet, position token.Pos, symbol string) site {
	pos := set.Position(position)
	root, _ := filepath.Abs(".")

	relative, err := filepath.Rel(root, pos.Filename)
	if err != nil {
		relative = pos.Filename
	}

	return site{filepath.ToSlash(relative), pos.Line, symbol}
}

func inspect() (report, error) {
	index, err := owners()
	if err != nil {
		return report{}, err
	}

	var config packages.Config

	config.Mode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
		packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps

	workspace, workspaceErr := filepath.Abs("go.work")
	if workspaceErr != nil {
		return report{}, fmt.Errorf("resolve workspace: %w", workspaceErr)
	}

	config.Env = append(os.Environ(), "GOWORK="+workspace)

	patterns := []string{"./pkg/...", "./internal/...", "./examples/...", "./api/...", "./cmd/go-roborock/..."}

	loaded, err := packages.Load(&config, patterns...)
	if err != nil {
		return report{}, fmt.Errorf("load production packages: %w", err)
	}

	if packages.PrintErrors(loaded) != 0 {
		return report{}, fmt.Errorf("%w: production packages do not type-check", errInventory)
	}

	result := report{
		Declarations: []declaration{}, Behavior: []site{}, Aliases: []site{},
		Codecs: []codec{}, Boundaries: []boundary{},
	}
	objects := make(map[types.Object]int)

	for _, pkg := range loaded {
		err = inspectDeclarations(pkg, index, &result, objects)
		if err != nil {
			return result, err
		}
	}

	collectUses(loaded, index, &result, objects)
	sort.Slice(result.Declarations, func(left, right int) bool {
		return result.Declarations[left].Symbol < result.Declarations[right].Symbol
	})

	for row := range result.Declarations {
		sortSites(result.Declarations[row].Uses)
	}

	sortSites(result.Behavior)
	sortSites(result.Aliases)
	sort.Slice(result.Codecs, func(left, right int) bool {
		return siteLess(result.Codecs[left].Definition, result.Codecs[right].Definition)
	})
	sort.Slice(result.Boundaries, func(left, right int) bool {
		return siteLess(result.Boundaries[left].Site, result.Boundaries[right].Site)
	})

	return result, nil
}

func handwritten(file *ast.File, pkg *packages.Package, result *report) error {
	var violation error

	ast.Inspect(file, func(node ast.Node) bool {
		if structure, ok := node.(*ast.StructType); ok {
			for _, field := range structure.Fields.List {
				if serializedField(field) {
					position := location(pkg.Fset, structure.Pos(), "")
					violation = fmt.Errorf("%w: handwritten serialization struct at %s:%d", errInventory, position.File, position.Line)
				}
			}
		}

		if named, ok := node.(*ast.TypeSpec); ok {
			position := location(pkg.Fset, named.Pos(), pkg.PkgPath+"."+named.Name.Name)
			if named.Assign.IsValid() {
				result.Aliases = append(result.Aliases, position)
			} else if _, structure := named.Type.(*ast.StructType); structure {
				result.Behavior = append(result.Behavior, position)
			}
		}

		return true
	})

	return violation
}

func sortSites(sites []site) {
	sort.Slice(sites, func(a, b int) bool { return siteLess(sites[a], sites[b]) })
}
func siteLess(left, right site) bool {
	if left.File != right.File {
		return left.File < right.File
	}

	if left.Line != right.Line {
		return left.Line < right.Line
	}

	return left.Symbol < right.Symbol
}

func inspectDeclarations(pkg *packages.Package, index schemaIndex, result *report, objects map[types.Object]int) error {
	for _, file := range pkg.Syntax {
		filename := location(pkg.Fset, file.Pos(), "").File
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}

		generated := strings.HasSuffix(filename, ".gen.go")

		validationErr := validateGenerated(file, filename, index)
		if validationErr != nil {
			return validationErr
		}

		if !generated {
			err := handwritten(file, pkg, result)
			if err != nil {
				return err
			}

			continue
		}

		modelErr := modelDeclarations(pkg, index, filename, result, objects)
		if modelErr != nil {
			return modelErr
		}
	}

	return nil
}

func modelDeclarations(
	pkg *packages.Package, index schemaIndex, filename string, result *report, objects map[types.Object]int,
) error {
	for identifier, object := range pkg.TypesInfo.Defs {
		if object == nil || location(pkg.Fset, identifier.Pos(), "").File != filename {
			continue
		}

		kind, value := declarationKind(object)
		if kind == "" {
			continue
		}

		owner, exists := index[filename][normalized(object.Name())]
		if !exists {
			if named, ok := object.Type().(*types.Named); ok {
				owner, exists = index[filename][normalized(named.Obj().Name())]
			}
		}

		if !exists {
			return fmt.Errorf("%w: no schema binding for %s:%s", errInventory, filename, object.Name())
		}

		symbol := pkg.PkgPath + "." + object.Name()
		objects[object] = len(result.Declarations)

		category := kind
		if strings.HasPrefix(filename, "pkg/roborock/") {
			category = "public-projection-" + kind
		}

		if strings.HasPrefix(filename, "cmd/") {
			category = "cli-input-" + kind
		}

		row := declaration{
			symbol, category, location(pkg.Fset, identifier.Pos(), symbol), owner,
			"go run ./tools/generate (oapi-codegen v2.5.1 models; schema constants)", value, []site{},
		}
		result.Declarations = append(result.Declarations, row)
	}

	return nil
}

func inspectBoundaries(pkg *packages.Package, index schemaIndex, result *report) {
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool { return inspectCall(node, pkg, index, result) })
	}
}

func inspectCodec(node ast.Node, pkg *packages.Package, index schemaIndex, result *report) {
	method, isMethod := node.(*ast.FuncDecl)
	if !isMethod || method.Recv == nil || (method.Name.Name != "MarshalJSON" && method.Name.Name != "UnmarshalJSON") {
		return
	}

	object := pkg.TypesInfo.Defs[method.Name]

	function, isFunction := object.(*types.Func)
	if !isFunction {
		return
	}

	signature, isSignature := function.Type().(*types.Signature)
	if !isSignature || signature.Recv() == nil {
		return
	}

	receiver := signature.Recv().Type()
	symbol := types.TypeString(receiver, nil) + "." + method.Name.Name
	position := location(pkg.Fset, method.Pos(), symbol)

	if pointer, isPointer := receiver.(*types.Pointer); isPointer {
		receiver = pointer.Elem()
	}

	var owner schemaOwner
	if named, isNamed := receiver.(*types.Named); isNamed {
		owner = index[position.File][normalized(named.Obj().Name())]
	}

	result.Codecs = append(result.Codecs, codec{position, owner})
}

func collectUses(loaded []*packages.Package, index schemaIndex, result *report, objects map[types.Object]int) {
	for _, pkg := range loaded {
		for identifier, object := range pkg.TypesInfo.Uses {
			position := location(pkg.Fset, identifier.Pos(), object.Name())
			if object.Pkg() != nil {
				position.Symbol = object.Pkg().Path() + "." + object.Name()
			}

			if strings.HasSuffix(position.File, ".gen.go") || strings.HasSuffix(position.File, "_test.go") {
				continue
			}

			if row, exists := objects[object]; exists {
				result.Declarations[row].Uses = append(result.Declarations[row].Uses, position)
			}
		}

		inspectBoundaries(pkg, index, result)
	}
}

func serializedField(field *ast.Field) bool {
	return field.Tag != nil && (strings.Contains(field.Tag.Value, "json:") ||
		strings.Contains(field.Tag.Value, "yaml:") || strings.Contains(field.Tag.Value, "xml:"))
}

func validateGenerated(file *ast.File, filename string, index schemaIndex) error {
	generated := strings.HasSuffix(filename, ".gen.go")
	if generated && !ast.IsGenerated(file) {
		return fmt.Errorf("%w: missing generator marker in %s", errInventory, filename)
	}

	if generated && index[filename] == nil && filename != "internal/protocol/rest.gen.go" {
		return fmt.Errorf("%w: unregistered generated file %s", errInventory, filename)
	}

	if ast.IsGenerated(file) && !generated {
		return fmt.Errorf("%w: unexpected generated marker in %s", errInventory, filename)
	}

	return nil
}

func declarationKind(object types.Object) (string, string) {
	var kind string

	value := ""

	switch item := object.(type) {
	case *types.TypeName:
		kind = "model"
		if item.IsAlias() {
			kind = "alias"
		}
	case *types.Const:
		kind = "constant"
		value = item.Val().ExactString()
	default:
		return "", ""
	}

	return kind, value
}

func inspectCall(node ast.Node, pkg *packages.Package, index schemaIndex, result *report) bool {
	inspectCodec(node, pkg, index, result)

	call, isCall := node.(*ast.CallExpr)
	if !isCall {
		return true
	}

	inspectInjectedDial(call, pkg, result)

	selected, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector {
		return true
	}

	object := pkg.TypesInfo.Uses[selected.Sel]
	if object == nil || object.Pkg() == nil {
		return true
	}

	symbol := object.Pkg().Path() + "." + object.Name()
	path := object.Pkg().Path()
	network := networkCall(path, object.Name(), location(pkg.Fset, call.Pos(), "").File)

	codec := path == "encoding/json" || path == "encoding/binary"

	if !network && !codec {
		return true
	}

	arguments := make([]string, 0, len(call.Args))

	for _, argument := range call.Args {
		if argumentType := pkg.TypesInfo.TypeOf(argument); argumentType != nil {
			arguments = append(arguments, types.TypeString(argumentType, nil))
		}
	}

	result.Boundaries = append(result.Boundaries, boundary{location(pkg.Fset, call.Pos(), symbol), arguments})

	return true
}

func networkCall(path, name, file string) bool {
	switch path {
	case "net/http":
		return name == "Do" || name == "NewRequestWithContext"
	case "crypto/tls":
		return name == "DialContext"
	case "net":
		return name == "Read" || name == "Write"
	case "io":
		return name == "ReadFull" && strings.Contains(file, "/mqtt/")
	default:
		return strings.HasSuffix(path, "/dependencies/rest") && name == "Do"
	}
}

func inspectInjectedDial(call *ast.CallExpr, pkg *packages.Package, result *report) {
	if identifier, direct := call.Fun.(*ast.Ident); direct {
		calledType := pkg.TypesInfo.TypeOf(identifier)
		if calledType != nil && strings.HasSuffix(types.TypeString(calledType, nil), ".DialFunc") {
			position := location(pkg.Fset, call.Pos(), "injected mqtt.DialFunc")
			result.Boundaries = append(result.Boundaries, boundary{position, []string{types.TypeString(calledType, nil)}})
		}
	}
}
