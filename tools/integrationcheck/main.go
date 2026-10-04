// Command integrationcheck compiles and skips the opt-in live suite without credentials.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	command := exec.CommandContext(context.Background(), "go", "test", "-race", "-tags=integration",
		"./tests/integration/...")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	environment := os.Environ()

	command.Env = make([]string, 0, len(environment))

	for _, value := range environment {
		if !strings.HasPrefix(strings.ToUpper(value), "ROBOROCK_INTEGRATION_AUTH_FILE=") {
			command.Env = append(command.Env, value)
		}
	}

	err := command.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "credential-free integration verification:", err)
		os.Exit(1)
	}
}
