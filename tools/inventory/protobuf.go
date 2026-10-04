package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
)

func protoOwners(index schemaIndex) error {
	const output = "pkg/dependencymodels/mapproto/b01_scmap.pb.go"

	_, err := os.Stat(output)
	if os.IsNotExist(err) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("inspect protobuf output: %w", err)
	}

	file, err := parser.ParseFile(token.NewFileSet(), output, nil, 0)
	if err != nil {
		return fmt.Errorf("parse protobuf output: %w", err)
	}

	entries := make(map[string]schemaOwner)
	// SCHEMA-10: binary declarations are owned by the pinned source protocol,
	// including generated runtime descriptor bookkeeping rather than JSON pointers.
	ast.Inspect(file, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok {
			entries[normalized(identifier.Name)] = schemaOwner{"api/external/b01_scmap.proto", "#"}
		}

		return true
	})

	index[output] = entries

	return nil
}
