package main

import "testing"

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
