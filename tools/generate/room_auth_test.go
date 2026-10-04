package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestRoomRequestsReuseLocalAuthType(t *testing.T) {
	t.Parallel()

	file, err := parser.ParseFile(token.NewFileSet(), "../../pkg/roborock/map_client_models.gen.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	for _, item := range file.Imports {
		if item.Path.Value == `"github.com/portpowered/go-roborock/pkg/roborock"` {
			t.Fatal("public models import their own package")
		}
	}

	seen := 0

	ast.Inspect(file, func(node ast.Node) bool {
		named, isType := node.(*ast.TypeSpec)
		if isType && checkRoomAuthType(t, named) {
			seen++
		}

		return true
	})

	if seen != 2 {
		t.Fatal("both room operations must reuse AuthContext")
	}
}

func checkRoomAuthType(t *testing.T, named *ast.TypeSpec) bool {
	t.Helper()

	if named.Name.Name == "AuthContext" {
		t.Fatal("room models duplicated the shared credential type")
	}

	if named.Name.Name != "HomeRoomsRequest" && named.Name.Name != "SharedDeviceRoomsRequest" {
		return false
	}

	structure, isStruct := named.Type.(*ast.StructType)
	if !isStruct {
		t.Fatal("room request is not a struct")
	}

	for _, field := range structure.Fields.List {
		if len(field.Names) != 1 || field.Names[0].Name != "Auth" {
			continue
		}

		kind, isLocal := field.Type.(*ast.Ident)
		if !isLocal || kind.Name != "AuthContext" {
			t.Fatal("room credentials do not use the shared type")
		}

		return true
	}

	return false
}
