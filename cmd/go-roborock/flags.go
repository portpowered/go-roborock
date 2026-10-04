package main

import (
	"flag"
	"fmt"
	"io"
	"time"
)

type commandOptions struct {
	command    string
	inputPath  string
	exportPath string
	timeout    time.Duration
}

const (
	commandCamera    = "camera"
	commandDevices   = "devices"
	commandLoginCode = "login-code"
)

func wantsHelp(args []string) bool {
	if len(args) == 0 {
		return true
	}

	switch args[0] {
	case "help", "--help", "-h":
		return true
	default:
		return false
	}
}

func parseCommand(args []string, stderr io.Writer) (commandOptions, error) {
	options := commandOptions{command: args[0], inputPath: "-", exportPath: "", timeout: defaultOperationTimeout}
	flags := flag.NewFlagSet(options.command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&options.inputPath, "input", "-", "JSON input file; - selects stdin or ROBOROCK_INPUT")
	flags.StringVar(&options.exportPath, "export", "", "create a credential export (login, devices, or camera)")
	flags.DurationVar(&options.timeout, "timeout", defaultOperationTimeout, "total operation timeout")
	flags.Usage = func() { _, _ = io.WriteString(stderr, helpText) }

	err := flags.Parse(args[1:])
	if err != nil {
		return options, fmt.Errorf("parse command: %w", err)
	}

	if flags.NArg() != 0 {
		return options, errPositionalInput
	}

	if !knownCommand(options.command) {
		return options, fmt.Errorf("%w: %q", errUnknownCommand, options.command)
	}

	if options.timeout <= 0 {
		return options, errTimeout
	}

	if options.exportPath != "" && !canExport(options.command) {
		return options, errExportCommand
	}

	return options, nil
}

func canExport(command string) bool {
	switch command {
	case "resolve-login", commandLoginCode, "login-password", commandDevices, commandCamera:
		return true
	default:
		return false
	}
}
