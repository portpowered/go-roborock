package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishedChecksumsRequireExactVersionPair(t *testing.T) {
	t.Parallel()

	for _, fixture := range []struct {
		name      string
		sums      string
		published bool
	}{
		{
			name:      "stale version",
			sums:      sdkModule + " v0.1.0 h1:old\n" + sdkModule + " v0.1.0/go.mod h1:oldmod\n",
			published: false,
		},
		{name: "module only", sums: sdkModule + " v0.2.0 h1:module\n", published: false},
		{name: "metadata only", sums: sdkModule + " v0.2.0/go.mod h1:metadata\n", published: false},
		{
			name:      "mixed versions",
			sums:      sdkModule + " v0.2.0 h1:module\n" + sdkModule + " v0.1.0/go.mod h1:oldmod\n",
			published: false,
		},
		{
			name:      "matching pair",
			sums:      sdkModule + " v0.2.0/go.mod h1:metadata\n" + sdkModule + " v0.2.0 h1:module\n",
			published: true,
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			directory := t.TempDir()
			writeMetadata(t, directory, "go.mod", "module example.test/cli\n\ngo 1.24.0\n\nrequire "+sdkModule+" v0.2.0\n")
			writeMetadata(t, directory, "go.sum", fixture.sums)

			published, err := publishedChecksums(directory)
			if err != nil {
				t.Fatal(err)
			}

			if published != fixture.published {
				t.Fatalf("published = %v, want %v", published, fixture.published)
			}
		})
	}
}

func TestPublishedChecksumsRejectReplacements(t *testing.T) {
	t.Parallel()

	for _, replacement := range []string{
		"replace " + sdkModule + " => ../..\n",
		"replace (\n example.test/helper => ../helper\n)\n",
	} {
		directory := t.TempDir()
		writeMetadata(t, directory, "go.mod", "module example.test/cli\n\nrequire "+sdkModule+" v0.2.0\n"+replacement)
		writeMetadata(t, directory, "go.sum", sdkModule+" v0.2.0 h1:module\n"+sdkModule+" v0.2.0/go.mod h1:metadata\n")

		_, err := publishedChecksums(directory)
		if !errors.Is(err, errModuleDrift) {
			t.Fatalf("replacement did not fail closed: %v", err)
		}
	}
}

func writeMetadata(t *testing.T, directory, name, content string) {
	t.Helper()

	err := os.WriteFile(filepath.Join(directory, name), []byte(content), privateFileMode)
	if err != nil {
		t.Fatal(err)
	}
}
