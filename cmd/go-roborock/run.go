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

const helpText = `Usage: go-roborock COMMAND [OPTIONS]

Get started:
  go-roborock login
  go-roborock devices list
  go-roborock devices vacuum DEVICE_ID status
  go-roborock maps show --device DEVICE_ID
  go-roborock devices vacuum DEVICE_ID start

Customer commands:
  login                                  Prompt for email and email code; save private login
  logout                                 Remove the saved login
  devices list                           List device IDs and names
  devices vacuum DEVICE_ID [ACTION]       status (default), start, pause, stop, dock,
                                         consumables, summary, capabilities
  maps list [--device DEVICE_ID]          List saved map IDs and names
  maps show [MAP_ID] [--device DEVICE_ID]  Read the current map or a saved Q7 map
  maps select MAP_ID --device DEVICE_ID   Select a saved map explicitly
  maps trace [--device DEVICE_ID]         Read the current cleaning trace
  rooms list [--device DEVICE_ID]         List room IDs and names
  rooms clean ROOM_ID... --device DEVICE_ID [--repeats 1]
  zones clean --device DEVICE_ID --zone x1,y1,x2,y2 [--zone ...] [--repeats 1]

Use --profile PATH to choose another saved login; --timeout 30s bounds each
network exchange. Login prompts have no deadline. With several devices, select
one explicitly. Cleaning commands send once and do not wait for completion.
Coordinates use map millimeters; see the zone-cleaning guide before cleaning.

Use --json for machine-readable lists. Run go-roborock help advanced for JSON commands.
`

const advancedHelpText = `Usage: go-roborock COMMAND [--input PATH|-] [--export PATH] [--timeout 30s]

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
  maps           List saved map identifiers and names
  map            Read map snapshot and geometry (optional mapId)
  rooms          List map room identifiers and names
  trace          Read the current map trace and geometry
  capabilities   Read device capabilities
  select-map     Select a saved map (mapId; explicit change)
  clean-zones    Clean rectangular zones (cleanZones; explicit movement)
  clean-rooms    Clean selected room identifiers (cleanRooms; explicit movement)
  start          Start cleaning (explicit movement)
  stop           Stop cleaning (explicit movement)
  pause          Pause cleaning (explicit movement)
  dock           Return to dock (explicit movement)
  dyad           Read selected dyadProperties for an A01 wet cleaner
  zeo            Read selected zeoProperties for an A01 washing machine
  camera         Negotiate camera signaling and close preview (camera)

Advanced device commands require auth, deviceId, localKey, and protocol in JSON input.
Input defaults to stdin; ROBOROCK_INPUT supplies JSON when --input is "-".
Advanced secrets are accepted through JSON input, never command-line flags.
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
		return writeCommandHelp(args, stdout)
	}

	if customerCommand(args) {
		return runCustomer(ctx, args, stdin, stdout, stderr, client)
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

func writeCommandHelp(args []string, stdout io.Writer) error {
	text := helpText

	if len(args) > 1 && args[0] == "help" && args[1] == "advanced" {
		text = advancedHelpText
	}

	_, err := io.WriteString(stdout, text)
	if err != nil {
		return fmt.Errorf("write help: %w", err)
	}

	return nil
}

func knownCommand(command string) bool {
	if isMapCommand(command) {
		return true
	}

	switch command {
	case commandResolveLogin, "request-code", commandLoginCode, commandLoginPassword, commandHome, commandDevices,
		commandStatus, commandConsumables, commandSummary, commandStart, commandStop, commandPause,
		commandDock, "dyad", "zeo", commandCamera:
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
