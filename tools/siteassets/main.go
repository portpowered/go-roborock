// Command siteassets adds the flat Flight payload names requested by static Next navigation.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
)

const assetMode = 0o644

var (
	errConflict = errors.New("flight alias conflicts with an existing asset")
	errRoot     = errors.New("asset root must be a directory, not a symbolic link")
)

func main() {
	root := flag.String("root", "site", "exported documentation directory")

	flag.Parse()

	err := run(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(directory string) error {
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("inspect asset root: %w", err)
	}

	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errRoot
	}

	root, err := os.OpenRoot(directory)
	if err != nil {
		return fmt.Errorf("open asset root: %w", err)
	}

	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		return visitAsset(root, name, entry, walkErr)
	})

	closeErr := root.Close()
	if err != nil || closeErr != nil {
		return fmt.Errorf("prepare Flight aliases: %w", errors.Join(err, closeErr))
	}

	return nil
}

func visitAsset(root *os.Root, name string, entry fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return fmt.Errorf("walk site assets: %w", walkErr)
	}

	if !entry.Type().IsRegular() || entry.IsDir() {
		return nil
	}

	target := aliasName(name)
	if target == "" {
		return nil
	}

	return createAlias(root, name, target)
}

func aliasName(name string) string {
	if !strings.HasSuffix(name, ".txt") {
		return ""
	}

	components := strings.Split(name, "/")
	for index, component := range components[:len(components)-1] {
		if strings.HasPrefix(component, "__next.") {
			return path.Join(strings.Join(components[:index], "/"), strings.Join(components[index:], "."))
		}
	}

	return ""
}

func createAlias(root *os.Root, source, target string) error {
	data, err := fs.ReadFile(root.FS(), source)
	if err != nil {
		return fmt.Errorf("read Flight asset %s: %w", source, err)
	}

	exists, err := checkExisting(root, target, data)
	if err != nil || exists {
		return err
	}

	file, err := root.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, assetMode)
	if err != nil {
		return fmt.Errorf("create Flight alias %s: %w", target, err)
	}

	_, writeErr := file.Write(data)

	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return fmt.Errorf("write Flight alias %s: %w", target, errors.Join(writeErr, closeErr))
	}

	return nil
}

func checkExisting(root *os.Root, target string, data []byte) (bool, error) {
	info, err := root.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("inspect Flight alias %s: %w", target, err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return true, nil
	}

	existing, err := fs.ReadFile(root.FS(), target)
	if err != nil {
		return true, fmt.Errorf("read existing Flight alias %s: %w", target, err)
	}

	if !bytes.Equal(existing, data) {
		return true, fmt.Errorf("%w: %s", errConflict, target)
	}

	return true, nil
}
