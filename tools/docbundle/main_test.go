package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundleNamespacesAndReferences(t *testing.T) {
	t.Parallel()

	// These minimal documents are synthetic generator inputs, not provider captures.
	source := t.TempDir()
	first := writeSchema(t, source, "first", `{
      "openapi":"3.0.3", "info":{"title":"first","version":"1"},
      "paths":{"/read":{"get":{"operationId":"Read", "tags":["reads"],
        "responses":{"200":{"description":"Result","content":{"application/json":{
          "schema":{"$ref":"./second.openapi.yaml#/components/schemas/Result"}}}}}}}},
      "components":{"schemas":{"Result":{"type":"string"}}}}
    `)
	second := writeSchema(t, source, "second", `{
      "openapi":"3.0.3", "info":{"title":"second","version":"1"},"paths":{},
      "components":{"schemas":{"Result":{"type":"object","properties":{
        "name":{"$ref":"#/components/schemas/Name"}}},"Name":{"type":"string"}}}}
    `)

	data, err := build(context.Background(), []string{first, second})
	if err != nil {
		t.Fatal(err)
	}

	var document object

	err = json.Unmarshal(data, &document)
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{"first.Result", "second.Result", "second.Name",
		"#/components/schemas/second.Result", "#/components/schemas/second.Name", `"Read"`} {
		if !strings.Contains(string(data), expected) {
			t.Errorf("missing namespaced contract %s", expected)
		}
	}

	_, err = build(context.Background(), []string{first, first})
	if err == nil {
		t.Error("accepted duplicate source paths")
	}
}

func TestReferenceRejectsUninventoriedSources(t *testing.T) {
	t.Parallel()

	sources := map[string]string{"known.openapi.yaml": "known"}

	references := []string{
		"./missing.openapi.yaml#/components/schemas/A", "#/unknown/A", "https://evil/#/components/schemas/A",
	}

	for _, ref := range references {
		_, err := componentRef(ref, "known.openapi.yaml", sources)
		if err == nil {
			t.Errorf("accepted unregistered reference %s", ref)
		}
	}
}

func TestBundleRejectsDuplicateOperations(t *testing.T) {
	t.Parallel()

	joined := bundle{paths: object{}, components: object{}, operations: map[string]bool{}}
	first := map[string]any{"/one": map[string]any{"get": map[string]any{"operationId": "Read"}}}
	second := map[string]any{"/two": map[string]any{"get": map[string]any{"operationId": "Read"}}}

	err := joined.addPaths(first, nil)
	if err != nil {
		t.Fatal(err)
	}

	err = joined.addPaths(second, nil)
	if err == nil {
		t.Error("accepted repeated operation ID on a different path")
	}

	_, err = build(context.Background(), nil)
	if err == nil {
		t.Error("accepted a bundle with no source documents")
	}
}

func writeSchema(t *testing.T, directory, name, data string) string {
	t.Helper()

	file := filepath.Join(directory, name+".openapi.yaml")

	err := os.WriteFile(file, []byte(data), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return file
}
