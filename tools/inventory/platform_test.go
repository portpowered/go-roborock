package main

import (
	"encoding/json"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestInventoryEnvironmentOverridesEveryHostTarget(t *testing.T) {
	t.Parallel()

	environment := inventoryEnvironment([]string{"PATH=keep", "GOOS=windows", "goos=darwin",
		"GOARCH=arm64", "CGO_ENABLED=1", "GOWORK=old"}, "chosen", inventoryLinuxTarget)

	expected := []string{"PATH=keep", "GOWORK=chosen", "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0"}

	if !reflect.DeepEqual(environment, expected) {
		t.Fatal("host target settings survived the inventory configuration")
	}
}

func TestInventoryInspectsEverySupportedPlatformAndArgumentVariant(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writePlatformFixture(t, root)

	reports := make([]report, 0, 3)
	for _, target := range []string{"windows", inventoryLinuxTarget, "darwin"} {
		reports = append(reports, inspectPlatformFixture(t, root, target))
	}

	merged := mergeReports(reports)
	if len(merged.Behavior) != 4 || len(merged.Boundaries) != 3 {
		t.Fatal("platform-only declarations or serialization variants were lost")
	}

	assertPlatformArguments(t, merged)

	first, err := json.Marshal(merged)
	if err != nil {
		t.Fatal(err)
	}

	second, err := json.Marshal(mergeReports([]report{reports[2], reports[0], reports[1]}))
	if err != nil {
		t.Fatal(err)
	}

	if string(first) != string(second) {
		t.Fatal("platform union depends on inspection order")
	}
}

func writePlatformFixture(t *testing.T, root string) {
	t.Helper()

	files := map[string]string{
		"go.mod": "module inventory.fixture\n\ngo 1.24\n",
		"common.go": "package fixture\nimport \"encoding/json\"\ntype Shared struct{}\n" +
			"func encode() { _, _ = json.Marshal(Platform{}.Value) }\n",
		"value_windows.go": "package fixture\ntype Platform struct{ Value string }\n",
		"value_linux.go":   "package fixture\ntype Platform struct{ Value int }\n",
		"value_darwin.go":  "package fixture\ntype Platform struct{ Value bool }\n",
	}
	for name, source := range files {
		err := os.WriteFile(filepath.Join(root, name), []byte(source), inventoryFileMode)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func inspectPlatformFixture(t *testing.T, root, target string) report {
	t.Helper()
	pkg := loadPlatformFixture(t, root, target)
	result := emptyReport()
	objects := make(map[types.Object]int)

	err := inspectDeclarations(pkg, schemaIndex{}, &result, objects)
	if err != nil {
		t.Fatal(err)
	}

	collectUses([]*packages.Package{pkg}, schemaIndex{}, &result, objects)

	return result
}

func loadPlatformFixture(t *testing.T, root, target string) *packages.Package {
	t.Helper()

	var config packages.Config

	config.Mode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
		packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps
	config.Dir = root
	config.Env = inventoryEnvironment(os.Environ(), "off", target)

	loaded, err := packages.Load(&config, ".")
	if err != nil {
		t.Fatal(err)
	}

	if packages.PrintErrors(loaded) != 0 || len(loaded) != 1 {
		t.Fatal("platform fixture failed to type-check")
	}

	return loaded[0]
}

func TestPlatformOnlyHandwrittenSerializationRemainsRejected(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writePlatformFixture(t, root)

	err := os.WriteFile(filepath.Join(root, "forged_linux.go"),
		[]byte("package fixture\ntype Forged struct{ Value string `json:\"value\"` }\n"), inventoryFileMode)
	if err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{"windows", inventoryLinuxTarget, "darwin"} {
		pkg := loadPlatformFixture(t, root, target)
		result := emptyReport()
		objects := make(map[types.Object]int)

		err = inspectDeclarations(pkg, schemaIndex{}, &result, objects)
		if (err != nil) != (target == inventoryLinuxTarget) {
			t.Fatalf("platform %s handwritten serialization rejection=%v", target, err != nil)
		}
	}
}

func assertPlatformArguments(t *testing.T, merged report) {
	t.Helper()

	arguments := make(map[string]bool)

	for _, call := range merged.Boundaries {
		if call.Site.Symbol != "encoding/json.Marshal" || len(call.Arguments) != 1 {
			t.Fatal("unexpected serialization boundary")
		}

		arguments[call.Arguments[0]] = true
	}

	if !arguments["string"] || !arguments["int"] || !arguments["bool"] {
		t.Fatal("host-dependent argument types were omitted")
	}
}

func TestPlatformUnionPreservesDistinctValuesAndDeduplicatesUses(t *testing.T) {
	t.Parallel()

	var first declaration

	first.Symbol = "fixture.Model"
	first.Definition = site{File: "model.gen.go", Line: 1, Symbol: first.Symbol}
	first.Value = "one"
	first.Uses = []site{{File: "common.go", Line: 2, Symbol: first.Symbol}}
	second := first
	second.Value = "two"
	second.Uses = []site{{File: "linux.go", Line: 3, Symbol: first.Symbol}}
	linux := emptyReport()
	linux.Declarations = []declaration{second, first}
	windows := emptyReport()
	windows.Declarations = []declaration{first}
	left := mergeReports([]report{linux, windows})

	right := mergeReports([]report{windows, linux})

	if !reflect.DeepEqual(left, right) || len(left.Declarations) != 2 || len(left.Declarations[0].Uses) != 1 {
		t.Fatal("distinct declaration values or repeated production uses changed during union")
	}

	if !strings.Contains(left.Declarations[1].Uses[0].File, inventoryLinuxTarget) {
		t.Fatal("platform-only generated model use disappeared")
	}
}
