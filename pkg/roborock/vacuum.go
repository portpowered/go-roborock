package roborock

import (
	"context"

	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// DeviceOperations lists the typed V1 vacuum controls supported by a DeviceSession.
// Acknowledgements report RPC acceptance; observe GetStatus for physical completion.
type DeviceOperations interface {
	GetStatus(ctx context.Context, _ EmptyRequest) (Status, error)
	GetConsumables(ctx context.Context, _ EmptyRequest) (Consumables, error)
	GetCleaningSummary(ctx context.Context, _ EmptyRequest) (CleaningSummary, error)
	GetCleanRecord(ctx context.Context, req CleanRecordRequest) (CleanRecordResult, error)
	GetDND(ctx context.Context, _ EmptyRequest) (DND, error)
	StartCleaning(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error)
	StopCleaning(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error)
	PauseCleaning(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error)
	ReturnToDock(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error)
	SpotClean(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error)
	StartDustCollection(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error)
	StopDustCollection(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error)
	StartMopWashing(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error)
	StopMopWashing(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error)
	RCStart(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error)
	RCStop(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error)
	RCEnd(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error)
	DisableDND(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error)
	SetFanSpeed(ctx context.Context, req SetFanSpeedRequest) (CommandAcknowledgement, error)
	SetWaterMode(ctx context.Context, req SetWaterModeRequest) (CommandAcknowledgement, error)
	SetMopMode(ctx context.Context, req SetMopModeRequest) (CommandAcknowledgement, error)
	SetCleanMotorMode(ctx context.Context, req SetCleanMotorModeRequest) (CommandAcknowledgement, error)
	SetDND(ctx context.Context, req SetDNDRequest) (CommandAcknowledgement, error)
	CleanZones(ctx context.Context, req CleanZonesRequest) (CommandAcknowledgement, error)
	CleanSegments(ctx context.Context, req CleanSegmentsRequest) (CommandAcknowledgement, error)
	RCMove(ctx context.Context, req RCMoveRequest) (CommandAcknowledgement, error)
}

// StartCleaning sends the device command once without automatic retries.
func (s *DeviceSession) StartCleaning(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error) {
	return s.command(ctx, dependencymodels.RPCStartCleaning, dependencymodels.NoParameters{})
}

// StopCleaning sends the device command once without automatic retries.
func (s *DeviceSession) StopCleaning(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error) {
	return s.command(ctx, dependencymodels.RPCStopCleaning, dependencymodels.NoParameters{})
}

// PauseCleaning sends the device command once without automatic retries.
func (s *DeviceSession) PauseCleaning(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error) {
	return s.command(ctx, dependencymodels.RPCPauseCleaning, dependencymodels.NoParameters{})
}

// ReturnToDock sends the device command once without automatic retries.
func (s *DeviceSession) ReturnToDock(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error) {
	return s.command(ctx, dependencymodels.RPCReturnToDock, dependencymodels.NoParameters{})
}

// SpotClean sends the device command once without automatic retries.
func (s *DeviceSession) SpotClean(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error) {
	return s.command(ctx, dependencymodels.RPCSpotClean, dependencymodels.NoParameters{})
}

// StartDustCollection sends the device command once without automatic retries.
func (s *DeviceSession) StartDustCollection(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error) {
	return s.command(ctx, dependencymodels.RPCStartDustCollection, dependencymodels.NoParameters{})
}

// StopDustCollection sends the device command once without automatic retries.
func (s *DeviceSession) StopDustCollection(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error) {
	return s.command(ctx, dependencymodels.RPCStopDustCollection, dependencymodels.NoParameters{})
}

// StartMopWashing sends the device command once without automatic retries.
func (s *DeviceSession) StartMopWashing(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error) {
	return s.command(ctx, dependencymodels.RPCStartMopWashing, dependencymodels.NoParameters{})
}

// StopMopWashing sends the device command once without automatic retries.
func (s *DeviceSession) StopMopWashing(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error) {
	return s.command(ctx, dependencymodels.RPCStopMopWashing, dependencymodels.NoParameters{})
}

// RCStart sends the device command once without automatic retries.
func (s *DeviceSession) RCStart(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error) {
	return s.command(ctx, dependencymodels.RPCRCStart, dependencymodels.NoParameters{})
}

// RCStop sends the device command once without automatic retries.
func (s *DeviceSession) RCStop(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error) {
	return s.command(ctx, dependencymodels.RPCRCStop, dependencymodels.NoParameters{})
}

// RCEnd sends the device command once without automatic retries.
func (s *DeviceSession) RCEnd(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error) {
	return s.command(ctx, dependencymodels.RPCRCEnd, dependencymodels.NoParameters{})
}

// DisableDND sends the device command once without automatic retries.
func (s *DeviceSession) DisableDND(ctx context.Context, _ EmptyRequest) (CommandAcknowledgement, error) {
	return s.command(ctx, dependencymodels.RPCDisableDND, dependencymodels.NoParameters{})
}

// SetFanSpeed sends the device command once without automatic retries.
func (s *DeviceSession) SetFanSpeed(ctx context.Context, req SetFanSpeedRequest) (CommandAcknowledgement, error) {
	return s.command(ctx, dependencymodels.RPCSetFanSpeed, dependencymodels.IntegerParameters{int64(req.Speed)})
}

// SetWaterMode sends the device command once without automatic retries.
func (s *DeviceSession) SetWaterMode(ctx context.Context, req SetWaterModeRequest) (CommandAcknowledgement, error) {
	return s.command(ctx, dependencymodels.RPCSetWaterMode, dependencymodels.IntegerParameters{int64(req.Mode)})
}

// SetMopMode sends the device command once without automatic retries.
func (s *DeviceSession) SetMopMode(ctx context.Context, req SetMopModeRequest) (CommandAcknowledgement, error) {
	return s.command(ctx, dependencymodels.RPCSetMopMode, dependencymodels.IntegerParameters{int64(req.Mode)})
}

// SetCleanMotorMode sets fan speed, water mode and optional mop route together, as the Roborock
// app's cleaning-mode selector does; WaterModeOff selects vacuum only. It takes raw mode codes,
// unlike python-roborock's high-level set_cleaning_mode. An S8 MaxV Ultra in the app's Custom mode
// rejected SetWaterMode (-10005) but accepted this command. It is sent once without retries.
func (s *DeviceSession) SetCleanMotorMode(
	ctx context.Context,
	req SetCleanMotorModeRequest,
) (CommandAcknowledgement, error) {
	mode := dependencymodels.CleanMotorMode{
		FanPower: int64(req.FanSpeed), WaterBoxMode: int64(req.WaterMode), MopMode: nil,
	}

	if req.MopMode != nil {
		route := int64(*req.MopMode)
		mode.MopMode = &route
	}

	return s.command(ctx, dependencymodels.RPCSetCleanMotorMode, dependencymodels.CleanMotorModeParameters{mode})
}

// SetDND sends the device command once without automatic retries.
func (s *DeviceSession) SetDND(ctx context.Context, req SetDNDRequest) (CommandAcknowledgement, error) {
	if !validDND(req) {
		return CommandAcknowledgement{}, roborockerrors.New(
			roborockerrors.InvalidArgument,
			"SetDND",
			"hours must be 0 through 23 and minutes 0 through 59",
			nil,
		)
	}

	return s.command(
		ctx,
		dependencymodels.RPCSetDND,
		dependencymodels.IntegerParameters{req.StartHour, req.StartMinute, req.EndHour, req.EndMinute},
	)
}

// CleanZones sends the device command once without automatic retries.
func (s *DeviceSession) CleanZones(ctx context.Context, req CleanZonesRequest) (CommandAcknowledgement, error) {
	if s.deviceFamily() != FamilyV1Vacuum {
		return s.cleanFamilyZones(ctx, req)
	}

	params, err := zoneParameters(req)
	if err != nil {
		return CommandAcknowledgement{}, err
	}

	return s.command(ctx, dependencymodels.RPCCleanZones, params)
}

// CleanSegments sends the device command once without automatic retries.
func (s *DeviceSession) CleanSegments(ctx context.Context, req CleanSegmentsRequest) (CommandAcknowledgement, error) {
	if len(req.Segments) == 0 || req.Repeats < 1 || req.Repeats > 3 {
		return CommandAcknowledgement{}, roborockerrors.New(
			roborockerrors.InvalidArgument,
			"CleanSegments",
			"segments required and repeats must be 1 through 3",
			nil,
		)
	}

	if s.deviceFamily() != FamilyV1Vacuum {
		return s.cleanFamilyRooms(ctx, req)
	}

	return s.command(
		ctx,
		dependencymodels.RPCCleanSegments,
		dependencymodels.SegmentParameters{
			dependencymodels.SegmentCommand{Segments: req.Segments, Repeat: int64(req.Repeats)},
		},
	)
}

// RCMove sends the device command once without automatic retries.
func (s *DeviceSession) RCMove(ctx context.Context, req RCMoveRequest) (CommandAcknowledgement, error) {
	if !validRC(req) {
		return CommandAcknowledgement{}, roborockerrors.New(
			roborockerrors.InvalidArgument,
			"RCMove",
			"invalid velocity, omega, duration, or sequence",
			nil,
		)
	}

	return s.command(
		ctx,
		dependencymodels.RPCRCMove,
		dependencymodels.RCParameters{
			dependencymodels.RCCommand{
				Velocity: req.Velocity,
				Omega:    req.Omega,
				Duration: req.Duration,
				Seqnum:   int64(req.Sequence),
			},
		},
	)
}
