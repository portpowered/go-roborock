package roborock

import (
	"context"
	"encoding/json"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencies/mqtt"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

func (s *DeviceSession) cleanFamilyRooms(
	ctx context.Context, request CleanSegmentsRequest,
) (CommandAcknowledgement, error) {
	if s.deviceFamily() != FamilyB01Q7 && s.deviceFamily() != FamilyB01Q10 {
		return CommandAcknowledgement{}, unsupportedMap("CleanSegments")
	}

	if request.Repeats != 1 {
		return CommandAcknowledgement{}, unsupportedMap("CleanSegments repeats")
	}

	transport, err := s.mapsTransport("CleanSegments")
	if err != nil {
		return CommandAcknowledgement{}, err
	}

	if s.deviceFamily() == FamilyB01Q10 {
		return cleanQ10Rooms(ctx, transport, request.Segments)
	}

	return cleanQ7Rooms(ctx, transport, request.Segments)
}

func cleanQ10Rooms(ctx context.Context, transport *mapProvider, segments []int64) (CommandAcknowledgement, error) {
	selection, err := json.Marshal(segments)
	if err != nil {
		return CommandAcknowledgement{}, operationError("CleanSegments", err)
	}

	err = transport.SetQ10Clean(ctx, dependencymodels.Q10CleanCommand{Cmd: dependencymodels.N2, CleanParamters: selection})
	if err != nil {
		return CommandAcknowledgement{}, operationError("CleanSegments", err)
	}

	return CommandAcknowledgement{Acknowledged: false, Sent: true}, nil
}

func cleanQ7Rooms(ctx context.Context, transport *mapProvider, segments []int64) (CommandAcknowledgement, error) {
	ids := make([]int, 0, len(segments))

	for _, id := range segments {
		converted := int(id)
		if id < 0 || int64(converted) != id {
			return CommandAcknowledgement{}, roborockerrors.New(roborockerrors.InvalidArgument,
				"CleanSegments", "room ID outside supported range", nil)
		}

		ids = append(ids, converted)
	}

	parameters, err := json.Marshal(dependencymodels.Q7RoomCleanRequest{
		CleanType: protocol.B01Q7RoomCleanType, CtrlValue: protocol.B01Q7CleanStart, RoomIds: ids})
	if err != nil {
		return CommandAcknowledgement{}, operationError("CleanSegments", err)
	}

	_, err = transport.CallB01(ctx, dependencymodels.ServiceSetRoomClean, parameters)
	if err != nil {
		return CommandAcknowledgement{}, operationError("CleanSegments", err)
	}

	return CommandAcknowledgement{Acknowledged: true, Sent: true}, nil
}

func (s *DeviceSession) cleanFamilyZones(
	ctx context.Context, request CleanZonesRequest,
) (CommandAcknowledgement, error) {
	if s.deviceFamily() != FamilyB01Q10 {
		return CommandAcknowledgement{}, unsupportedMap("CleanZones")
	}

	if len(request.Zones) != 1 {
		return CommandAcknowledgement{}, roborockerrors.New(roborockerrors.InvalidArgument,
			"CleanZones", "Q10 accepts one rectangle per command", nil)
	}

	zone := request.Zones[0]

	encoded, err := mqtt.EncodeQ10Zone(mqtt.Q10Zone{X1: zone.X1, Y1: zone.Y1,
		X2: zone.X2, Y2: zone.Y2, Repeats: zone.Repeats})
	if err != nil {
		return CommandAcknowledgement{}, roborockerrors.Wrap(roborockerrors.InvalidArgument,
			"CleanZones", "invalid Q10 rectangle", err)
	}

	parameters, err := json.Marshal(encoded)
	if err != nil {
		return CommandAcknowledgement{}, operationError("CleanZones", err)
	}

	transport, err := s.mapsTransport("CleanZones")
	if err != nil {
		return CommandAcknowledgement{}, err
	}

	err = transport.SetQ10Clean(ctx, dependencymodels.Q10CleanCommand{
		Cmd: dependencymodels.N3, CleanParamters: parameters})
	if err != nil {
		return CommandAcknowledgement{}, operationError("CleanZones", err)
	}

	return CommandAcknowledgement{Acknowledged: false, Sent: true}, nil
}
