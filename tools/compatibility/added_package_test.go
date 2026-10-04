package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddedPackageValidatesCurrentAPIWithoutBaseline(t *testing.T) {
	t.Parallel()
	tool := exportFixtureTool(t)
	current := t.TempDir()
	baseline := t.TempDir()
	output := t.TempDir()
	writeFixture(t, filepath.Join(current, "api-valid"), "valid")

	changes, err := comparePackage(t.Context(), tool, current, baseline, output, "example.test/sdk", "pkg/provider")
	if err != nil || changes != "" {
		t.Fatalf("new package comparison = %q, %v", changes, err)
	}

	_, err = os.Stat(filepath.Join(output, "pkg-provider-current.export"))
	if err != nil {
		t.Fatalf("current API was not exported: %v", err)
	}

	_, err = os.Stat(filepath.Join(output, "pkg-provider-base.export"))
	if !os.IsNotExist(err) {
		t.Fatalf("absent baseline unexpectedly exported: %v", err)
	}

	_, err = comparePackage(t.Context(), tool, t.TempDir(), baseline, output, "example.test/sdk", "pkg/provider")
	if err == nil || !strings.Contains(err.Error(), "read current API") {
		t.Fatalf("invalid current API was accepted: %v", err)
	}

	writeFixture(t, filepath.Join(baseline, "pkg", "provider", "invalid.go"), "package invalid\n")

	_, err = comparePackage(t.Context(), tool, current, baseline, output, "example.test/sdk", "pkg/provider")
	if err == nil || !strings.Contains(err.Error(), "read baseline API") {
		t.Fatalf("existing baseline export failure was suppressed: %v", err)
	}
}

func TestBaselinePackagePresence(t *testing.T) {
	t.Parallel()

	baseline := t.TempDir()
	for _, name := range []string{"README.md", "probe_test.go", "probe.go"} {
		writeFixture(t, filepath.Join(baseline, "pkg", "probe", name), "package probe\n")

		present, err := baselinePackagePresent(baseline, "pkg/probe")
		if err != nil || present != (name == "probe.go") {
			t.Fatalf("package with %s = %t, %v", name, present, err)
		}
	}

	writeFixture(t, filepath.Join(baseline, "pkg", "file"), "not a directory")

	_, err := baselinePackagePresent(baseline, "pkg/file")
	if err == nil {
		t.Fatal("non-directory baseline error suppressed")
	}
}

func exportFixtureTool(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	source := `package main
import "os"
func main() {
 if len(os.Args) != 4 || os.Args[1] != "-w" { os.Exit(2) }
 if _, err := os.Stat("api-valid"); err != nil { os.Exit(3) }
 if err := os.WriteFile(os.Args[2], []byte("export"), 0600); err != nil { os.Exit(4) }
}
`
	name := filepath.Join(directory, "export.go")
	writeFixture(t, name, source)

	executable := filepath.Join(directory, "apidiff.exe")
	command := exec.CommandContext(t.Context(), "go", "build", "-o", executable, name) //nolint:gosec // Test-owned tool.
	command.Env = withEnvironment(os.Environ(), "GOWORK", "off")

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build export fixture: %v: %s", err, output)
	}

	return executable
}

func writeFixture(t *testing.T, name, data string) {
	t.Helper()

	err := os.MkdirAll(filepath.Dir(name), 0o750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(name, []byte(data), 0o600)
	if err != nil {
		t.Fatal(err)
	}
}
