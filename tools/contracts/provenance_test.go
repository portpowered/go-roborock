package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaMetadataAndGeneratorInputAreBound(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	schema := "openapi: 3.0.3\ninfo:\n  title: Probe\n  version: 1.0.0\npaths: {}\ncomponents:\n  schemas:\n"
	schema += "    Probe:\n      type: object\n      required: [value]\n      additionalProperties: false\n"
	schema += "      properties:\n        value:\n          type: string\n          nullable: false\n"
	schemaName := filepath.Join(root, "api", "probe.openapi.yaml")
	writeProbe(t, schemaName, []byte(schema))

	generatorName := filepath.Join(root, "tools", "generate", "main.go")
	writeProbe(t, generatorName, []byte("package main\nfunc main() {}\n"))

	err := run(context.Background(), root, true)
	if err != nil {
		t.Fatal(err)
	}

	for _, mutation := range [][2]string{
		{"required: [value]", "required: []"},
		{"additionalProperties: false", "additionalProperties: true"},
		{"nullable: false", "nullable: true"},
	} {
		writeProbe(t, schemaName, []byte(strings.Replace(schema, mutation[0], mutation[1], 1)))

		err = run(context.Background(), root, false)
		if err == nil {
			t.Fatalf("schema metadata drift accepted: %s", mutation[0])
		}
	}

	writeProbe(t, schemaName, []byte(schema))
	writeProbe(t, generatorName, []byte("package main\nfunc main() {}\n// Changed generator input.\n"))

	err = run(context.Background(), root, false)
	if err == nil {
		t.Fatal("generation owner input drift accepted")
	}
}

func TestProductionCannotReachExcludedHelpers(t *testing.T) {
	t.Parallel()

	for _, excluded := range []string{"tools", "tests", "reference", "site", "tmp"} {
		source := "package probe\nimport _ \"github.com/portpowered/go-roborock/" + excluded + "/helper\"\n"

		err := checkSource("pkg/probe/probe.go", []byte(source))
		if err == nil {
			t.Fatalf("production imported an unreviewed excluded package: %s", excluded)
		}
	}
}
