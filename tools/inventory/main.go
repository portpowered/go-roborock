// Command inventory records schema-owned declarations and resolved production uses.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
)

const inventoryFileMode = 0o600

var errInventory = errors.New("model inventory verification failed")

func main() {
	check := flag.Bool("check", false, "verify the checked-in inventory")
	flag.Parse()

	err := run(*check)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(check bool) error {
	result, err := inspect()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode inventory: %w", err)
	}

	data = append(data, '\n')

	const output = "api/model-inventory.json"
	if check {
		current, readErr := os.ReadFile(output)
		if readErr != nil {
			return fmt.Errorf("read inventory: %w", readErr)
		}

		if !bytes.Equal(current, data) {
			return fmt.Errorf("%w: regenerate with go run ./tools/inventory", errInventory)
		}

		return nil
	}

	err = os.WriteFile(output, data, inventoryFileMode)
	if err != nil {
		return fmt.Errorf("write inventory: %w", err)
	}

	return nil
}
