package roborock

import (
	"context"

	"github.com/portpowered/go-roborock/pkg/dependencies/rest"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// GetHomeRooms refreshes the home room list without fetching device inventory.
func (c *Client) GetHomeRooms(ctx context.Context, request HomeRoomsRequest) (RoomsResult, error) {
	out, err := c.rest.HomeRooms(ctx, rest.HomeRoomsRequest{Auth: authWire(request.Auth), HomeID: request.HomeID})
	if err != nil {
		return RoomsResult{}, roborockerrors.Wrap(roborockerrors.Protocol, "get_home_rooms", "room refresh failed", err)
	}

	return roomsProjection(out), nil
}

// GetSharedDeviceRooms refreshes rooms belonging to a received device.
func (c *Client) GetSharedDeviceRooms(ctx context.Context, request SharedDeviceRoomsRequest) (RoomsResult, error) {
	out, err := c.rest.SharedDeviceRooms(ctx, rest.SharedDeviceRoomsRequest{
		Auth: authWire(request.Auth), DeviceID: request.DeviceID,
	})
	if err != nil {
		return RoomsResult{}, roborockerrors.Wrap(roborockerrors.Protocol, "get_shared_device_rooms", "room refresh failed", err)
	}

	return roomsProjection(out), nil
}

func roomsProjection(out rest.RoomsResult) RoomsResult {
	result := RoomsResult{Rooms: make([]CloudRoom, 0, len(out.Rooms))}

	for _, room := range out.Rooms {
		result.Rooms = append(result.Rooms, CloudRoom{ID: room.Id, Name: room.Name})
	}

	return result
}
