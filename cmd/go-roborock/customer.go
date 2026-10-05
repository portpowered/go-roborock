package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/portpowered/go-roborock/pkg/roborock"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
	"golang.org/x/term"
)

const maximumPromptBytes = 1024

var (
	errPrompt          = errors.New("enter a nonempty value of at most 1024 bytes")
	errAmbiguousDevice = errors.New("choose a device with --device DEVICE_ID from go-roborock devices list")
	errMissingDevice   = errors.New("device not found; run go-roborock devices list")
)

func runCustomer(
	ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, client roborock.ClientAPI,
) error {
	if wantsCustomerHelp(args) {
		return writeCustomerMessage(stdout, helpText)
	}

	options, err := parseCustomer(args, stderr)

	if errors.Is(err, flag.ErrHelp) {
		return nil
	}

	if err != nil {
		return err
	}

	input, err := customerInput(options)
	if err != nil {
		return err
	}

	switch options.command {
	case commandLogin:
		return customerLogin(ctx, options, stdin, stdout, stderr, client)

	case commandLogout:
		return customerLogout(options.profile, stdout)

	default:
		return customerOperation(ctx, options, input, stdout, client)
	}
}

func wantsCustomerHelp(args []string) bool {
	if len(args) == 1 && optionsRequireSubcommand(args[0]) {
		return true
	}

	for _, argument := range args {
		if argument == helpFlag || argument == "-h" {
			return true
		}
	}

	return false
}

func optionsRequireSubcommand(command string) bool {
	switch command {
	case commandDevices, commandMaps, commandRooms, "zones":
		return true

	default:
		return false
	}
}

func customerLogout(path string, stdout io.Writer) error {
	err := checkPrivateProfileFile(path)

	if errors.Is(err, os.ErrNotExist) {
		return writeCustomerMessage(stdout, "Logged out.\n")
	}

	if err != nil {
		return fmt.Errorf("check login profile: %w", err)
	}

	err = os.Remove(path)
	if err != nil {
		return fmt.Errorf("remove login profile: %w", err)
	}

	return writeCustomerMessage(stdout, "Logged out.\n")
}

func customerLogin(
	ctx context.Context, options customerOptions, stdin io.Reader, stdout, stderr io.Writer, client roborock.ClientAPI,
) error {
	stdin, cleanup, err := customerPromptInput(ctx, stdin)
	if err != nil {
		return err
	}

	defer cleanup()

	reader := bufio.NewReader(stdin)

	email, err := promptEmail(ctx, stdin, reader, stderr)
	if err != nil {
		return customerPromptError(ctx, err)
	}

	identity, err := randomIdentity()
	if err != nil {
		return err
	}

	loginContext, err := customerRequestCode(ctx, options, email, identity, client)
	if err != nil {
		return err
	}

	code, err := promptCode(ctx, stdin, reader, stderr)
	if err != nil {
		return customerPromptError(ctx, err)
	}

	operation, cancel := context.WithTimeout(ctx, options.timeout)

	result, err := client.LoginWithCode(operation, roborock.LoginWithCodeRequest{Login: loginContext, Code: code})

	cancel()

	if err != nil {
		return safeCustomerError("exchange email code", err)
	}

	err = saveProfile(options.profile, result.Auth)
	if err != nil {
		return err
	}

	return writeCustomerMessage(stdout, "Logged in. Run go-roborock devices list.\n")
}

func customerPromptInput(ctx context.Context, stdin io.Reader) (io.Reader, func(), error) {
	var restore func()

	file, ok := stdin.(*os.File)

	if ok && term.IsTerminal(int(file.Fd())) {
		state, err := term.GetState(int(file.Fd()))
		if err != nil {
			return nil, nil, fmt.Errorf("read terminal settings: %w", err)
		}

		duplicate, err := duplicatePromptFile(file)
		if err != nil {
			return nil, nil, err
		}

		stdin = duplicate

		restore = func() { _ = term.Restore(int(file.Fd()), state); _ = duplicate.Close() }
	}

	stop := func() bool { return true }

	if closer, ok := stdin.(io.Closer); ok && restore == nil {
		stop = context.AfterFunc(ctx, func() { _ = closer.Close() })
	}

	cleanup := func() {
		stop()

		if restore != nil {
			restore()
		}
	}

	return stdin, cleanup, nil
}

func customerRequestCode(
	ctx context.Context, options customerOptions, email, identity string, client roborock.ClientAPI,
) (roborock.LoginContext, error) {
	operation, cancel := context.WithTimeout(ctx, options.timeout)

	loginContext, err := client.ResolveLogin(operation, roborock.ResolveLoginRequest{Email: email, ClientID: identity})

	cancel()

	if err != nil {
		return loginContext, safeCustomerError("resolve login", err)
	}

	operation, cancel = context.WithTimeout(ctx, options.timeout)

	request, err := client.RequestLoginCode(operation, roborock.LoginCodeRequest{Login: loginContext})

	cancel()

	if err != nil {
		return loginContext, safeCustomerError("request email code", err)
	}

	if !request.Accepted {
		return loginContext, roborockerrors.New(roborockerrors.Protocol,
			"request email code", "email code request was not accepted", nil)
	}

	return loginContext, nil
}

func promptCustomer(reader *bufio.Reader, stderr io.Writer, label string) (string, error) {
	_, err := io.WriteString(stderr, label)
	if err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}

	var value strings.Builder

	for value.Len() <= maximumPromptBytes {
		character, err := reader.ReadByte()

		if errors.Is(err, io.EOF) && value.Len() > 0 {
			break
		}

		if err != nil {
			return "", fmt.Errorf("read prompt: %w", err)
		}

		if character == '\n' {
			break
		}

		_ = value.WriteByte(character)
	}

	result := strings.TrimSpace(value.String())

	if result == "" || value.Len() > maximumPromptBytes {
		return "", errPrompt
	}

	return result, nil
}

func customerPromptError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return safeCustomerError("login", ctx.Err())
	}

	return err
}

func promptEmail(ctx context.Context, stdin io.Reader, reader *bufio.Reader, stderr io.Writer) (string, error) {
	file, ok := stdin.(*os.File)

	if !ok || !term.IsTerminal(int(file.Fd())) {
		return promptCustomer(reader, stderr, "Email: ")
	}

	_, err := io.WriteString(stderr, "Email: ")
	if err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}

	value, err := readPromptLine(ctx, file, false)
	if err != nil {
		return "", safeCustomerError("read email", err)
	}

	return value, nil
}

func promptCode(ctx context.Context, stdin io.Reader, reader *bufio.Reader, stderr io.Writer) (string, error) {
	file, ok := stdin.(*os.File)

	if !ok || !term.IsTerminal(int(file.Fd())) {
		return promptCustomer(reader, stderr, "Email code: ")
	}

	_, err := io.WriteString(stderr, "Email code: ")
	if err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}

	value, err := readPromptLine(ctx, file, true)

	_, writeErr := io.WriteString(stderr, "\n")
	if writeErr != nil {
		return "", fmt.Errorf("write prompt: %w", writeErr)
	}

	if err != nil {
		return "", fmt.Errorf("read email code: %w", err)
	}

	result := strings.TrimSpace(value)

	if result == "" || len(value) > maximumPromptBytes {
		return "", errPrompt
	}

	return result, nil
}

func customerOperation(
	ctx context.Context, options customerOptions, input CommandInput, stdout io.Writer, client roborock.ClientAPI,
) error {
	auth, err := readProfile(options.profile)
	if err != nil {
		return err
	}

	operation, cancel := context.WithTimeout(ctx, options.timeout)

	inventory, err := client.ListDevices(operation, roborock.AccountRequest{Auth: auth})

	cancel()

	if err != nil {
		return safeCustomerError("list devices", err)
	}

	if options.command == commandDevices {
		return writeCustomerResult(stdout, options.command, inventory, options.asJSON)
	}

	device, err := chooseDevice(inventory.Devices, options.device)
	if err != nil {
		return err
	}

	input.Auth, input.DeviceID, input.LocalKey, input.Protocol = auth, device.ID, device.LocalKey, device.Protocol

	operation, cancel = context.WithTimeout(ctx, options.timeout)

	defer cancel()

	result, err := executeDevice(operation, client, options.command, input, "")
	if err != nil {
		return safeCustomerError(options.command, err)
	}

	return writeCustomerResult(stdout, options.command, result, options.asJSON)
}

func chooseDevice(devices []roborock.Device, deviceID string) (roborock.Device, error) {
	var selected roborock.Device

	if deviceID == "" {
		if len(devices) != 1 {
			return selected, errAmbiguousDevice
		}

		return devices[0], nil
	}

	count := 0

	for _, device := range devices {
		if device.ID == deviceID {
			selected = device

			count++
		}
	}

	if count == 0 {
		return selected, errMissingDevice
	}

	if count != 1 {
		return selected, errAmbiguousDevice
	}

	return selected, nil
}

func safeCustomerError(operation string, cause error) error {
	wrapped := roborockerrors.Wrap(roborockerrors.Unavailable, operation, "check device availability", cause)

	if errors.Is(cause, context.Canceled) {
		wrapped.Kind = roborockerrors.Canceled
	}

	if errors.Is(cause, context.DeadlineExceeded) {
		wrapped.Kind = roborockerrors.Timeout
	}

	messages := map[roborockerrors.Kind]string{
		roborockerrors.Unauthorized:    "run go-roborock login again",
		roborockerrors.Unsupported:     "check device capabilities and supported device families",
		roborockerrors.NotFound:        "run go-roborock devices list and check the device ID",
		roborockerrors.Timeout:         "operation timed out; increase --timeout for reads",
		roborockerrors.Canceled:        "operation canceled",
		roborockerrors.InvalidArgument: "check command arguments and the device's supported bounds",
		roborockerrors.RateLimited:     "the service is busy; wait before sending another command",
	}

	if message, ok := messages[wrapped.Kind]; ok {
		wrapped.Message = message
	}

	return wrapped
}

func writeCustomerMessage(stdout io.Writer, message string) error {
	_, err := io.WriteString(stdout, message)
	if err != nil {
		return fmt.Errorf("write result: %w", err)
	}

	return nil
}

func writeCustomerJSON(stdout io.Writer, value any) error {
	err := json.NewEncoder(stdout).Encode(value)
	if err != nil {
		return fmt.Errorf("write result: %w", err)
	}

	return nil
}
