package roborock

import (
	"context"
	"encoding/json"
	"github.com/portpowered/go-roborock/internal/protocol"

	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// A01Operations lists typed controls for A01 wet cleaners and washing machines.
// Select query properties supported by the appliance's discovered product schema.
// A successful write reports MQTT publication, not device acceptance or cycle completion.
type A01Operations interface {
	GetDyadState(ctx context.Context, request GetDyadStateRequest) (DyadState, error)
	GetZeoState(ctx context.Context, request GetZeoStateRequest) (ZeoState, error)
	SetDyadSettings(ctx context.Context, request SetDyadSettingsRequest) error
	SetZeoSettings(ctx context.Context, request SetZeoSettingsRequest) error
	StartZeo(ctx context.Context, request StartZeoRequest) error
}

// GetDyadState queries exactly the selected appliance-supported datapoints.
// Unselected fields remain nil. Unknown numeric enum values are retained.
func (s *DeviceSession) GetDyadState(ctx context.Context, request GetDyadStateRequest) (DyadState, error) {
	var state DyadState

	err := queryA01State[DyadState, dependencymodels.DyadStateWire](
		ctx, s, request.Properties, "get dyad state", &state,
	)

	return state, err
}

// GetZeoState queries exactly the selected appliance-supported datapoints.
// A deadline prevents waiting indefinitely for a property unsupported by the appliance.
func (s *DeviceSession) GetZeoState(ctx context.Context, request GetZeoStateRequest) (ZeoState, error) {
	var state ZeoState

	err := queryA01State[ZeoState, dependencymodels.ZeoStateWire](
		ctx, s, request.Properties, "get zeo state", &state,
	)

	return state, err
}

// SetDyadSettings publishes only the supplied settings; it never retries the command.
func (s *DeviceSession) SetDyadSettings(ctx context.Context, request SetDyadSettingsRequest) error {
	if !validA01Time(request.SilentModeStartTime) || !validA01Time(request.SilentModeEndTime) ||
		(request.Volume != nil && (*request.Volume < 0 || *request.Volume > 100)) {
		return roborockerrors.New(roborockerrors.InvalidArgument, "set dyad settings", "invalid volume or minute of day", nil)
	}

	return setA01Settings[SetDyadSettingsRequest, dependencymodels.SetDyadSettingsWire](
		ctx, s, request, "set dyad settings",
	)
}

// SetZeoSettings publishes only independent settings supplied by the caller.
// Cycle parameters belong in StartZeo and must be bundled with the start signal.
func (s *DeviceSession) SetZeoSettings(ctx context.Context, request SetZeoSettingsRequest) error {
	if !validA01Time(request.SilentModeStartTime) || !validA01Time(request.SilentModeEndTime) {
		return roborockerrors.New(roborockerrors.InvalidArgument, "set zeo settings", "invalid minute of day", nil)
	}

	return setA01Settings[SetZeoSettingsRequest, dependencymodels.SetZeoSettingsWire](ctx, s, request, "set zeo settings")
}

// StartZeo bundles the start signal with the required mode and program and optional cycle parameters.
// Success reports publication; observe GetZeoState for physical cycle progress.
func (s *DeviceSession) StartZeo(ctx context.Context, request StartZeoRequest) error {
	const operation = "start zeo"

	err := s.requireA01(operation)
	if err != nil {
		return err
	}

	if request.Mode == ZeoModeNull || request.Program == ZeoProgramNull {
		return roborockerrors.New(roborockerrors.InvalidArgument, operation, "mode and program must be supplied", nil)
	}

	values, err := encodeA01[StartZeoRequest, dependencymodels.StartZeoWire](request)
	if err != nil {
		return roborockerrors.New(roborockerrors.InvalidArgument, operation, "invalid cycle parameters", err)
	}

	var start dependencymodels.StartZeoWire
	start.Start = protocol.A01ZeoStartEnabled
	encodedStart, err := json.Marshal(start.Start)
	if err != nil {
		return roborockerrors.New(roborockerrors.InvalidArgument, operation, "invalid start signal", err)
	}
	values[int(ZeoPropertyStart)] = encodedStart

	return publishA01(ctx, s, values, operation)
}

func (s *DeviceSession) requireA01(operation string) error {
	if s == nil || s.rpc == nil {
		return roborockerrors.New(roborockerrors.Closed, operation, "device session is unavailable", nil)
	}

	if s.protocol != string(ProtocolA01) {
		return roborockerrors.New(roborockerrors.Unsupported, operation, "device does not use the A01 protocol", nil)
	}

	return nil
}

var _ A01Operations = (*DeviceSession)(nil)

func validA01Time(value *int) bool {
	const minutesPerDay = 24 * 60

	return value == nil || (*value >= 0 && *value < minutesPerDay)
}
