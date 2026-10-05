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
		err = checkPrivateProfileFile(options.profile)
		if errors.Is(err, os.ErrNotExist) {
			return writeCustomerMessage(stdout, "Logged out.\n")
		}

		if err != nil {
			return fmt.Errorf("check login profile: %w", err)
		}

		err = os.Remove(options.profile)

		if err != nil {
			return fmt.Errorf("remove login profile: %w", err)
		}

		return writeCustomerMessage(stdout, "Logged out.\n")
	default:
		return customerOperation(ctx, options, input, stdout, client)
	}
}

func customerLogin(
	ctx context.Context, options customerOptions, stdin io.Reader, stdout, stderr io.Writer, client roborock.ClientAPI,
) error {
	if closer, ok := stdin.(io.Closer); ok {
		stop := context.AfterFunc(ctx, func() { _ = closer.Close() })
		defer stop()
	}
	reader := bufio.NewReader(stdin)

	email, err := promptCustomer(reader, stderr, "Email: ")
	if err != nil {
		return err
	}

	identity, err := randomIdentity()
	if err != nil {
		return err
	}

	operation, cancel := context.WithTimeout(ctx, options.timeout)
	loginContext, err := client.ResolveLogin(operation, roborock.ResolveLoginRequest{Email: email, ClientID: identity})

	cancel()

	if err != nil {
		return safeCustomerError("resolve login", err)
	}

	operation, cancel = context.WithTimeout(ctx, options.timeout)
	_, err = client.RequestLoginCode(operation, roborock.LoginCodeRequest{Login: loginContext})

	cancel()

	if err != nil {
		return safeCustomerError("request email code", err)
	}

	code, err := promptCode(stdin, reader, stderr)
	if err != nil {
		return err
	}

	operation, cancel = context.WithTimeout(ctx, options.timeout)
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

func promptCode(stdin io.Reader, reader *bufio.Reader, stderr io.Writer) (string, error) {
	file, ok := stdin.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return promptCustomer(reader, stderr, "Email code: ")
	}

	_, err := io.WriteString(stderr, "Email code: ")
	if err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}

	value, err := term.ReadPassword(int(file.Fd()))

	_, writeErr := io.WriteString(stderr, "\n")
	if writeErr != nil {
		return "", fmt.Errorf("write prompt: %w", writeErr)
	}

	if err != nil {
		return "", fmt.Errorf("read email code: %w", err)
	}

	result := strings.TrimSpace(string(value))
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
		for index := range inventory.Devices {
			inventory.Devices[index].LocalKey = ""
		}

		return writeCustomerJSON(stdout, inventory)
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

	return writeCustomerJSON(stdout, result)
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
	switch wrapped.Kind {
	case roborockerrors.Unauthorized:
		wrapped.Message = "run go-roborock login again"
	case roborockerrors.Unsupported:
		wrapped.Message = "check device capabilities and supported device families"
	case roborockerrors.NotFound:
		wrapped.Message = "run go-roborock devices list and check the device ID"
	case roborockerrors.Timeout:
		wrapped.Message = "operation timed out; increase --timeout for reads"
	case roborockerrors.Canceled:
		wrapped.Message = "operation canceled"
	default:
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
