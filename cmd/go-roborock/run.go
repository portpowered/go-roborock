package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/portpowered/go-roborock/pkg/roborock"
)

const defaultOperationTimeout = 30 * time.Second

const helpText = `Usage: go-roborock COMMAND [--input PATH|-] [--export PATH] [--timeout 30s]

Commands:
  resolve-login  Discover login region (email, clientId)
  request-code   Request an email login code (login)
  login-code     Exchange a one-time code (login, code)
  login-password Exchange a password (login, password)
  home           Get the account home (auth)
  devices        List devices, with local keys redacted (auth)
  status         Read vacuum status
  consumables    Read vacuum consumable counters
  summary        Read cleaning summary
  start          Start cleaning (explicit movement)
  stop           Stop cleaning (explicit movement)
  pause          Pause cleaning (explicit movement)
  dock           Return to dock (explicit movement)
  dyad           Read selected dyadProperties for an A01 wet cleaner
  zeo            Read selected zeoProperties for an A01 washing machine
  camera         Negotiate camera signaling and close preview (camera)

Device commands require auth, deviceId, localKey, and protocol in JSON input.
Input defaults to stdin; ROBOROCK_INPUT supplies JSON when --input is "-".
Secrets (code, password, token, local keys) are accepted only through JSON input.
--export explicitly writes login credentials, discovery local keys, or camera TURN
credentials to a new file (0600 on Unix; inherited directory access on Windows).
Ordinary login output contains account identity only. Results are JSON on stdout.
Device sessions close after each operation. Ctrl+C cancels network work.
`

func run(
	ctx context.Context,
	args []string,
	stdin io.Reader,
	stdout,
	stderr io.Writer,
	getenv func(string) string,
	client roborock.ClientAPI,
) error {
	if wantsHelp(args) {
		_, err := io.WriteString(stdout, helpText)
		if err != nil {
			return fmt.Errorf("write help: %w", err)
		}

		return nil
	}

	options, err := parseCommand(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}

	if err != nil {
		return err
	}

	input, err := readInput(options.inputPath, stdin, getenv)
	if err != nil {
		return err
	}

	operationContext, cancel := context.WithTimeout(ctx, options.timeout)
	defer cancel()

	result, err := execute(operationContext, client, options.command, input, options.exportPath)
	if err != nil {
		return err
	}

	err = json.NewEncoder(stdout).Encode(result)
	if err != nil {
		return fmt.Errorf("write result: %w", err)
	}

	return nil
}

func knownCommand(command string) bool {
	switch command {
	case commandResolveLogin, "request-code", commandLoginCode, commandLoginPassword, commandHome, commandDevices,
		commandStatus, "consumables", "summary", "start", "stop", "pause", "dock", "dyad", "zeo", commandCamera:
		return true
	default:
		return false
	}
}

//nolint:wrapcheck // SDK account errors already identify their operation, kind, and vendor code.
func execute(
	ctx context.Context,
	client roborock.ClientAPI,
	command string,
	input CommandInput,
	exportPath string,
) (any, error) {
	switch command {
	case commandResolveLogin:
		return resolveLogin(ctx, client, input, exportPath)
	case "request-code":
		return client.RequestLoginCode(ctx, roborock.LoginCodeRequest{Login: input.Login})
	case commandLoginCode, commandLoginPassword:
		return login(ctx, client, command, input, exportPath)
	case commandHome:
		return client.GetHome(ctx, roborock.AccountRequest{Auth: input.Auth})
	case commandDevices:
		return discoverDevices(ctx, client, input, exportPath)
	default:
		return executeDevice(ctx, client, command, input, exportPath)
	}
}

func resolveLogin(
	ctx context.Context,
	client roborock.ClientAPI,
	input CommandInput,
	path string,
) (roborock.LoginContext, error) {
	result, err := client.ResolveLogin(ctx, roborock.ResolveLoginRequest{Email: input.Email, ClientID: input.ClientID})
	if err == nil && path != "" {
		err = exportCredentials(path, CredentialExport{Login: &result, Auth: nil})
	}

	return result, err
}

func discoverDevices(
	ctx context.Context,
	client roborock.ClientAPI,
	input CommandInput,
	path string,
) (roborock.ListDevicesResult, error) {
	result, err := client.ListDevices(ctx, roborock.AccountRequest{Auth: input.Auth})
	if err == nil && path != "" {
		err = exportCredentials(path, result)
	}

	for index := range result.Devices {
		result.Devices[index].LocalKey = ""
	}

	return result, err
}

func login(
	ctx context.Context,
	client roborock.ClientAPI,
	command string,
	input CommandInput,
	exportPath string,
) (LoginIdentity, error) {
	var (
		result roborock.LoginResult
		err    error
	)

	if command == commandLoginCode {
		result, err = client.LoginWithCode(ctx, roborock.LoginWithCodeRequest{Login: input.Login, Code: input.Code})
	} else {
		request := roborock.LoginWithPasswordRequest{Login: input.Login, Password: input.Password}
		result, err = client.LoginWithPassword(ctx, request)
	}

	if err == nil && exportPath != "" {
		err = exportCredentials(exportPath, CredentialExport{Auth: &result.Auth, Login: nil})
	}

	return LoginIdentity{UserID: result.UserID, Nickname: result.Nickname}, err
}
