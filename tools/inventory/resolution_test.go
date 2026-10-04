package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

func TestResolvedUseDoesNotCountShadowedIdentifier(t *testing.T) {
	t.Parallel()

	const source = "package sample; type Model struct{}; var actual Model; func shadow() { var Model int; _ = Model }"

	set := token.NewFileSet()

	file, err := parser.ParseFile(set, "sample.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	var info types.Info

	info.Defs = make(map[*ast.Ident]types.Object)
	info.Uses = make(map[*ast.Ident]types.Object)

	var config types.Config

	pkg, err := config.Check("sample", set, []*ast.File{file}, &info)
	if err != nil {
		t.Fatal(err)
	}

	model := pkg.Scope().Lookup("Model")
	matched := 0
	shadowed := 0

	for identifier, object := range info.Uses {
		if identifier.Name != "Model" {
			continue
		}

		if object == model {
			matched++
		} else {
			shadowed++
		}
	}

	if matched != 1 || shadowed != 1 {
		t.Fatalf("resolved model uses=%d, shadowed names=%d", matched, shadowed)
	}
}
