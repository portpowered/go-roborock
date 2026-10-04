package roborock

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

func (s *DeviceSession) callV1(
	ctx context.Context,
	method dependencymodels.RPCMethod,
	params any,
) (json.RawMessage, error) {
	if s.protocol != string(ProtocolV1) {
		return nil, roborockerrors.New(
			roborockerrors.Unsupported,
			string(method),
			"operation requires a V1 device",
			nil,
		)
	}

	err := ctx.Err()
	if err != nil {
		return nil, operationError(string(method), err)
	}

	payload, err := json.Marshal(params)
	if err != nil {
		return nil, roborockerrors.New(roborockerrors.InvalidArgument, string(method), "cannot encode parameters", err)
	}

	result, err := s.rpc.Call(ctx, string(method), payload)
	if err != nil {
		return nil, operationError(string(method), err)
	}

	return result, nil
}

func operationError(operation string, cause error) error {
	var typed *roborockerrors.Error
	if errors.As(cause, &typed) {
		return cause
	}

	kind := roborockerrors.Unavailable
	if errors.Is(cause, context.DeadlineExceeded) {
		kind = roborockerrors.Timeout
	}

	if errors.Is(cause, context.Canceled) {
		kind = roborockerrors.Canceled
	}

	return roborockerrors.New(kind, operation, "device operation failed", cause)
}

func (s *DeviceSession) command(
	ctx context.Context,
	method dependencymodels.RPCMethod,
	params any,
) (CommandAcknowledgement, error) {
	raw, err := s.callV1(ctx, method, params)
	if err != nil {
		return CommandAcknowledgement{}, err
	}

	var ack dependencymodels.Acknowledgement

	err = json.Unmarshal(raw, &ack)
	if err != nil || len(ack) != 1 || ack[0] != dependencymodels.CommandAcceptanceOK {
		return CommandAcknowledgement{}, roborockerrors.New(
			roborockerrors.Protocol,
			string(method),
			"device did not acknowledge command",
			err,
		)
	}

	return CommandAcknowledgement{Acknowledged: true, Sent: true}, nil
}

func validDND(req SetDNDRequest) bool {
	return req.StartHour >= 0 && req.StartHour <= 23 &&
		req.EndHour >= 0 && req.EndHour <= 23 &&
		req.StartMinute >= 0 && req.StartMinute <= 59 &&
		req.EndMinute >= 0 && req.EndMinute <= 59
}

func validRC(req RCMoveRequest) bool {
	return !math.IsNaN(float64(req.Velocity)) && !math.IsNaN(float64(req.Omega)) &&
		req.Velocity >= -0.3 && req.Velocity <= 0.3 &&
		req.Omega >= -0.8 && req.Omega <= 0.8 &&
		req.Duration >= 1 && req.Duration <= 5000 && req.Sequence >= 0
}

func zoneParameters(req CleanZonesRequest) (dependencymodels.ZoneParameters, error) {
	if len(req.Zones) == 0 {
		return nil, roborockerrors.New(roborockerrors.InvalidArgument, "CleanZones", "zones are required", nil)
	}

	params := make(dependencymodels.ZoneParameters, 0, len(req.Zones))

	for _, zone := range req.Zones {
		if zone.Repeats < 1 || zone.Repeats > 3 || zone.X1 >= zone.X2 || zone.Y1 >= zone.Y2 {
			return nil, roborockerrors.New(
				roborockerrors.InvalidArgument,
				"CleanZones",
				"zones require ordered corners and repeats 1 through 3",
				nil,
			)
		}

		params = append(params, []int64{zone.X1, zone.Y1, zone.X2, zone.Y2, int64(zone.Repeats)})
	}

	return params, nil
}

func objectResult(raw json.RawMessage) (json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) > 0 && raw[0] == '[' {
		var items []json.RawMessage

		err := json.Unmarshal(raw, &items)
		if err != nil {
			return nil, fmt.Errorf("decode device object: %w", err)
		}

		if len(items) != 1 {
			return nil, errResultShape
		}

		raw = bytes.TrimSpace(items[0])
	}

	if len(raw) == 0 || raw[0] != '{' {
		return nil, errResultShape
	}

	var fields map[string]json.RawMessage

	err := json.Unmarshal(raw, &fields)
	if err != nil {
		return nil, fmt.Errorf("decode device object: %w", err)
	}

	if len(fields) == 0 {
		return nil, errResultShape
	}

	return raw, nil
}

// projectResult checks the generated wire contract before projecting into the
// independent generated public type. Shared JSON names are part of the schema contract.
func projectResult[Wire, Public any](raw json.RawMessage) (Public, error) {
	var (
		wire   Wire
		public Public
	)

	err := json.Unmarshal(raw, &wire)
	if err != nil {
		return public, fmt.Errorf("project device result: %w", err)
	}

	projected, err := json.Marshal(wire)
	if err != nil {
		return public, fmt.Errorf("project device result: %w", err)
	}

	err = json.Unmarshal(projected, &public)
	if err != nil {
		return public, fmt.Errorf("decode projected result: %w", err)
	}

	return public, nil
}

func readObject[Wire, Public any](
	ctx context.Context,
	s *DeviceSession,
	method dependencymodels.RPCMethod,
) (Public, error) {
	var empty Public

	raw, err := s.callV1(ctx, method, dependencymodels.NoParameters{})
	if err != nil {
		return empty, err
	}

	raw, err = objectResult(raw)
	if err != nil {
		return empty, roborockerrors.New(roborockerrors.Protocol, string(method), "invalid device result", err)
	}

	result, err := projectResult[Wire, Public](raw)
	if err != nil {
		return empty, roborockerrors.New(roborockerrors.Protocol, string(method), "invalid device result", err)
	}

	return result, nil
}
