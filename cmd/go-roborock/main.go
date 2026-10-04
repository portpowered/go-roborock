// Command go-roborock operates Roborock accounts and explicit device sessions.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/portpowered/go-roborock/pkg/roborock"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)

	client, err := roborock.NewClient()
	if err == nil {
		err = run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv, client)
	}

	stop()

	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "go-roborock:", err)
		os.Exit(1)
	}
}
