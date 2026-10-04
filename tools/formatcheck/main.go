// Command formatcheck checks every repository Go source without modifying files.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

var errUnformatted = errors.New("go files require gofmt")

func main() {
	err := run(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	args := []string{"-l"}

	err := filepath.WalkDir(".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk formatting inventory: %w", walkErr)
		}

		if entry.IsDir() && slices.Contains([]string{".git", "reference", "site", "tmp", "vendor"}, entry.Name()) {
			return filepath.SkipDir
		}

		if !entry.IsDir() && strings.HasSuffix(name, ".go") {
			args = append(args, name)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("scan formatting inventory: %w", err)
	}
	// GO-01: invoke a fixed formatter with repository-walked source paths, without shell evaluation.
	command := exec.CommandContext(ctx, "gofmt", args...)

	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("check gofmt: %w: %s", err, output)
	}

	if len(output) != 0 {
		return fmt.Errorf("%w:\n%s", errUnformatted, output)
	}

	return nil
}
