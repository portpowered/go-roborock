package main

import (
	"go/parser"
	"go/token"
	"testing"

	"golang.org/x/tools/go/packages"
)

const schemaTypeKey = "type"

func TestHandwrittenSerializationRejected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		source   string
		rejected bool
	}{
		{"unused exported", "package sample; type Unused struct { Value string `json:\"value\"` }", true},
		{"anonymous nested", "package sample; type State struct { Child struct { Value string `json:\"value\"` } }", true},
		{"behavior", "package sample; type State struct { Ready bool }", false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			set := token.NewFileSet()

			file, err := parser.ParseFile(set, "sample.go", test.source, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}

			var pkg packages.Package

			pkg.Fset = set
			pkg.PkgPath = "sample"

			var result report

			err = handwritten(file, &pkg, &result)
			if (err != nil) != test.rejected {
				t.Fatalf("rejection=%v, expected %v", err, test.rejected)
			}
		})
	}
}

func TestSchemaBindingIncludesImplicitModels(t *testing.T) {
	t.Parallel()

	entries := make(map[string]schemaOwner)
	schema := map[string]any{
		"properties": map[string]any{
			"value": map[string]any{"anyOf": []any{
				map[string]any{schemaTypeKey: "string"},
				map[string]any{schemaTypeKey: "integer"},
			}},
		},
	}
	collectSchema(entries, "api/example.yaml", "#/components/schemas/Result", "Result", schema)

	if entries[normalized("ResultValue1")].Pointer != "#/components/schemas/Result/properties/value/anyOf/1" {
		t.Fatal("implicit union model lacks exact schema owner")
	}

	if _, exists := entries[normalized("Forged")]; exists {
		t.Fatal("unknown model received a schema owner")
	}
}

func TestSharedPrimitiveBindingIsDeterministic(t *testing.T) {
	t.Parallel()

	entries := make(map[string]schemaOwner)
	putOwner(entries, "name", schemaOwner{"api/b.yaml", "#/second"})
	putOwner(entries, "name", schemaOwner{"api/a.yaml", "#/first"})

	if entries["name"].File != "api/a.yaml" {
		t.Fatal("shared primitive selected an unstable owner")
	}
}

func TestExplicitComponentOwnsNormalizedPropertyCollision(t *testing.T) {
	t.Parallel()

	entries := make(map[string]schemaOwner)
	property := map[string]any{schemaTypeKey: "string"}
	collectSchema(entries, "api/example.yaml", "#/components/schemas/ProductProperty/properties/id",
		"ProductPropertyID", property)

	components := map[string]any{"ProductPropertyID": property}
	componentNames(entries, "api/example.yaml", components)

	if entries[normalized("ProductPropertyID")].Pointer != "#/components/schemas/ProductPropertyID" {
		t.Fatal("implicit property shadowed the explicit component owner")
	}
}
