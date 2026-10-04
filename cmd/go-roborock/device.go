package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/go-roborock/pkg/roborock"
)

//nolint:contextcheck,wrapcheck,nonamedreturns // SDK Close owns cleanup; named returns propagate cleanup errors.
func executeDevice(
	ctx context.Context,
	client roborock.ClientAPI,
	command string,
	input CommandInput,
	exportPath string,
) (result any, err error) {
	if input.DeviceID == "" || input.LocalKey == "" || input.Protocol == "" {
		return nil, errDeviceInput
	}

	request := roborock.OpenDeviceRequest{
		Auth: input.Auth, DeviceID: input.DeviceID, LocalKey: input.LocalKey, Protocol: input.Protocol,
	}

	session, err := client.OpenDevice(ctx, request)
	if err != nil {
		return nil, err
	}

	defer func() { err = errors.Join(err, session.Close()) }()

	switch command {
	case "dyad":
		return session.GetDyadState(ctx, roborock.GetDyadStateRequest{Properties: input.DyadProperties})
	case "zeo":
		return session.GetZeoState(ctx, roborock.GetZeoStateRequest{Properties: input.ZeoProperties})
	case commandCamera:
		return cameraOperation(ctx, session, input, exportPath)
	default:
		return deviceOperation(ctx, session, command)
	}
}

//nolint:contextcheck,wrapcheck,nonamedreturns // SDK camera owns teardown; named returns propagate cleanup errors.
func cameraOperation(
	ctx context.Context,
	session *roborock.DeviceSession,
	input CommandInput,
	exportPath string,
) (result CameraIdentity, err error) {
	camera, err := session.OpenCamera(ctx, input.Camera)
	if err != nil {
		return result, err
	}

	defer func() { err = errors.Join(err, camera.Close()) }()

	description := camera.Description()
	if exportPath != "" {
		err = exportCredentials(exportPath, description)
	}

	return CameraIdentity{Negotiated: true}, err
}

//nolint:wrapcheck // Forward the SDK's typed operation errors without replacing their caller-facing context.
func deviceOperation(ctx context.Context, session *roborock.DeviceSession, command string) (any, error) {
	request := roborock.EmptyRequest{}

	switch command {
	case commandStatus:
		return session.GetStatus(ctx, request)
	case "consumables":
		return session.GetConsumables(ctx, request)
	case "summary":
		return session.GetCleaningSummary(ctx, request)
	case "start":
		return session.StartCleaning(ctx, request)
	case "stop":
		return session.StopCleaning(ctx, request)
	case "pause":
		return session.PauseCleaning(ctx, request)
	case "dock":
		return session.ReturnToDock(ctx, request)
	default:
		return nil, fmt.Errorf("%w: %q", errUnknownCommand, command)
	}
}
