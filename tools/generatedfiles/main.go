// Command generatedfiles rejects untracked generated output in CI.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	command := exec.CommandContext(context.Background(), "git", "ls-files", "--others", "--exclude-standard", "-z", "--",
		"api", "internal/protocol", "internal/mapmodel", "pkg/dependencymodels", "pkg/roborock", "cmd/go-roborock")

	output, err := command.Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	for name := range strings.SplitSeq(string(output), "\x00") {
		if len(name) > 0 && (strings.HasSuffix(name, ".gen.go") || strings.HasSuffix(name, ".pb.go") || strings.HasSuffix(name, ".openapi.yaml") || strings.HasSuffix(name, ".asyncapi.yaml")) {
			fmt.Fprintf(os.Stderr, "untracked generated/schema output: %s\n", name)
			os.Exit(1)
		}
	}
}
