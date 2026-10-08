package roborock

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"

	"github.com/portpowered/go-roborock/internal/protocol"

	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const roomMappingPairFields = 2

// GetRooms returns cleaning segment IDs and any reported names. Unknown names remain absent.
func (s *DeviceSession) GetRooms(ctx context.Context, _ EmptyRequest) (MapRoomsResult, error) {
	ctx, finish := s.operationContext(ctx)
	defer finish()

	err := ctx.Err()
	if err != nil {
		return MapRoomsResult{}, operationError("GetRooms", err)
	}

	if s.deviceFamily() == FamilyV1Vacuum {
		return s.v1Rooms(ctx)
	}

	if s.deviceFamily() != FamilyB01Q7 && s.deviceFamily() != FamilyB01Q10 {
		return MapRoomsResult{}, unsupportedMap("GetRooms")
	}

	snapshot, err := s.GetMap(ctx, GetMapRequest{MapID: ""})
	if err != nil {
		return MapRoomsResult{}, err
	}

	result := MapRoomsResult{Rooms: []MapRoomMapping{}}

	for _, room := range snapshot.Rooms {
		if room.Id != nil {
			result.Rooms = append(result.Rooms, MapRoomMapping{SegmentID: *room.Id, Name: room.Name, IoTID: nil})
		}
	}

	return result, nil
}

func (s *DeviceSession) operationContext(ctx context.Context) (context.Context, func()) {
	bound, cancel := context.WithCancel(ctx)
	if s.life == nil {
		return bound, cancel
	}

	if s.life.Err() != nil {
		cancel()

		return bound, cancel
	}

	stop := context.AfterFunc(s.life, cancel)

	return bound, func() { stop(); cancel() }
}

func (s *DeviceSession) v1Rooms(ctx context.Context) (MapRoomsResult, error) {
	raw, err := s.callV1(ctx, dependencymodels.RPCMethod(protocol.RPCGetRoomMapping), dependencymodels.NoParameters{})
	if err != nil {
		return MapRoomsResult{}, err
	}

	result, err := parseRoomMapping(raw)
	if err != nil {
		return MapRoomsResult{}, err
	}

	if s.client == nil {
		return result, nil
	}

	names, err := s.roomNames(ctx, result.Rooms)
	if err != nil {
		if ctx.Err() != nil {
			return MapRoomsResult{}, operationError("GetRooms", ctx.Err())
		}

		return result, nil
	}

	for index := range result.Rooms {
		room := &result.Rooms[index]
		if room.IoTID != nil {
			room.Name = names[*room.IoTID]
		}
	}

	return result, nil
}

func parseRoomMapping(raw json.RawMessage) (MapRoomsResult, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return MapRoomsResult{Rooms: []MapRoomMapping{}}, nil
	}

	var wire dependencymodels.MapsV1RoomsResult

	err := json.Unmarshal(raw, &wire)
	if err != nil {
		return MapRoomsResult{}, roborockerrors.New(roborockerrors.Protocol, "GetRooms", "invalid room mapping", err)
	}

	pairs, err := wire.AsMapsV1RoomMappings()
	if err != nil {
		var pair dependencymodels.MapsV1RoomMappingPair

		pair, err = wire.AsMapsV1RoomMappingPair()
		if err != nil {
			return MapRoomsResult{}, roborockerrors.New(roborockerrors.Protocol, "GetRooms", "invalid room mapping", err)
		}

		pairs = dependencymodels.MapsV1RoomMappings{pair}
	}

	result := MapRoomsResult{Rooms: []MapRoomMapping{}}

	for _, pair := range pairs {
		room, err := parseRoomPair(pair)
		if err != nil {
			return MapRoomsResult{}, err
		}

		result.Rooms = append(result.Rooms, room)
	}

	return result, nil
}

func parseRoomPair(pair dependencymodels.MapsV1RoomMappingPair) (MapRoomMapping, error) {
	// Newer firmware appends a room type; only the segment and cloud IDs are used.
	if len(pair) < roomMappingPairFields {
		return MapRoomMapping{}, roborockerrors.New(roborockerrors.Protocol,
			"GetRooms", "room mapping must contain segment and cloud IDs", nil)
	}

	if bytes.Equal(bytes.TrimSpace(pair[0]), []byte("null")) || bytes.Equal(bytes.TrimSpace(pair[1]), []byte("null")) {
		return MapRoomMapping{}, roborockerrors.New(roborockerrors.Protocol, "GetRooms", "room IDs must be present", nil)
	}

	var segment int64

	err := json.Unmarshal(pair[0], &segment)
	if err != nil {
		return MapRoomMapping{}, roborockerrors.New(roborockerrors.Protocol, "GetRooms", "invalid segment ID", err)
	}

	var identifier string

	err = json.Unmarshal(pair[1], &identifier)
	if err != nil {
		var numeric int64

		err = json.Unmarshal(pair[1], &numeric)
		if err != nil {
			return MapRoomMapping{}, roborockerrors.New(roborockerrors.Protocol, "GetRooms", "invalid cloud room ID", err)
		}

		identifier = strconv.FormatInt(numeric, 10)
	}

	return MapRoomMapping{SegmentID: segment, IoTID: &identifier, Name: nil}, nil
}

func (s *DeviceSession) roomNames(ctx context.Context, mappings []MapRoomMapping) (map[string]*string, error) {
	home, err := s.client.GetHome(ctx, AccountRequest{Auth: s.auth})
	if err != nil {
		return nil, err
	}

	data, err := s.client.GetHomeData(ctx, HomeDataRequest{Auth: s.auth, HomeID: home.ID, Version: HomeDataV1})
	if err != nil {
		return nil, err
	}

	names := make(map[string]*string)

	for _, room := range data.Rooms {
		name := room.Name
		names[strconv.FormatInt(room.ID, 10)] = &name
	}

	if !missingRoomNames(names, mappings) {
		return names, nil
	}

	refreshed, err := s.refreshRoomNames(ctx, home.ID, data.Devices)
	if err != nil {
		if ctx.Err() != nil {
			return nil, operationError("GetRooms", ctx.Err())
		}

		return names, nil
	}

	for _, room := range refreshed.Rooms {
		names[strconv.FormatInt(room.ID, 10)] = room.Name
	}

	return names, nil
}

func missingRoomNames(names map[string]*string, mappings []MapRoomMapping) bool {
	for _, mapping := range mappings {
		if mapping.IoTID != nil && names[*mapping.IoTID] == nil {
			return true
		}
	}

	return false
}

func (s *DeviceSession) refreshRoomNames(ctx context.Context, homeID int64, devices []Device) (RoomsResult, error) {
	shared := false

	for _, device := range devices {
		if device.ID == s.deviceID {
			shared = device.Shared

			break
		}
	}

	if shared {
		return s.client.GetSharedDeviceRooms(ctx, SharedDeviceRoomsRequest{Auth: s.auth, DeviceID: s.deviceID})
	}

	return s.client.GetHomeRooms(ctx, HomeRoomsRequest{Auth: s.auth, HomeID: homeID})
}
