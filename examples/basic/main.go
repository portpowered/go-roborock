// Command basic lists account devices without printing their encryption keys.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/portpowered/go-roborock/pkg/roborock"
)

const requestTimeout = 30 * time.Second

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var auth roborock.AuthContext

	err := json.NewDecoder(os.Stdin).Decode(&auth)
	if err != nil {
		return fmt.Errorf("read private AuthContext JSON from stdin: %w", err)
	}

	client, err := roborock.NewClient()
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	result, err := client.ListDevices(ctx, roborock.AccountRequest{Auth: auth})
	if err != nil {
		return fmt.Errorf("list devices: %w", err)
	}

	for _, device := range result.Devices {
		_, err = fmt.Fprintf(os.Stdout, "%s\t%s\t%s\n", device.ID, device.Name, device.Protocol)
		if err != nil {
			return fmt.Errorf("write device listing: %w", err)
		}
	}

	return nil
}
