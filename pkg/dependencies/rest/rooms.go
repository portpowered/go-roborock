package rest

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// HomeRoomsRequest refreshes rooms using a caller-owned home ID and account.
type HomeRoomsRequest struct {
	Auth   AuthContext
	HomeID int64
}

// SharedDeviceRoomsRequest selects a received device without changing account state.
type SharedDeviceRoomsRequest struct {
	Auth     AuthContext
	DeviceID string
}

// RoomsResult preserves missing room names and forward-compatible room fields.
type RoomsResult struct {
	Rooms []dependencymodels.RefreshedRoom
}

// HomeRooms reads the rooms currently associated with a home.
func (c *Client) HomeRooms(ctx context.Context, request HomeRoomsRequest) (RoomsResult, error) {
	if request.HomeID <= 0 {
		return RoomsResult{}, roborockerrors.New(
			roborockerrors.InvalidArgument, "home_rooms", "positive home ID required", nil,
		)
	}

	path := strings.ReplaceAll(
		protocol.RESTPathGetHomeRooms, "{"+protocol.RESTPathHomeID+"}", strconv.FormatInt(request.HomeID, 10),
	)

	var out dependencymodels.HomeRoomsResponse

	err := c.roomsExchange(ctx, request.Auth, protocol.RESTMethodGetHomeRooms, path, &out)
	if err != nil {
		return RoomsResult{}, err
	}

	err = checkRooms(out.Success, out.Code, out.Result != nil)
	if err != nil {
		return RoomsResult{}, err
	}

	return RoomsResult{Rooms: *out.Result}, nil
}

// SharedDeviceRooms accepts the provider's roomId alias when id is absent.
func (c *Client) SharedDeviceRooms(ctx context.Context, request SharedDeviceRoomsRequest) (RoomsResult, error) {
	if request.DeviceID == "" {
		return RoomsResult{}, roborockerrors.New(
			roborockerrors.InvalidArgument, "shared_device_rooms", "device ID required", nil,
		)
	}

	path := strings.ReplaceAll(
		protocol.RESTPathGetSharedDeviceRooms, "{"+protocol.RESTPathDeviceID+"}", url.PathEscape(request.DeviceID),
	)

	var out dependencymodels.SharedDeviceRoomsResponse

	err := c.roomsExchange(ctx, request.Auth, protocol.RESTMethodGetSharedDeviceRooms, path, &out)
	if err != nil {
		return RoomsResult{}, err
	}

	err = checkRooms(out.Success, out.Code, out.Result != nil)
	if err != nil {
		return RoomsResult{}, err
	}

	result := RoomsResult{Rooms: make([]dependencymodels.RefreshedRoom, 0, len(*out.Result))}

	for _, room := range *out.Result {
		roomID := room.Id
		if roomID == nil {
			roomID = room.RoomId
		}

		result.Rooms = append(result.Rooms, dependencymodels.RefreshedRoom{
			Id: *roomID, Name: room.Name, AdditionalProperties: room.AdditionalProperties,
		})
	}

	return result, nil
}

func (c *Client) roomsExchange(ctx context.Context, auth AuthContext, method, path string, out any) error {
	if auth.RRiot.R.A == nil || *auth.RRiot.R.A == "" {
		return roborockerrors.New(roborockerrors.InvalidArgument, "rooms", "account API origin required", nil)
	}

	authorization, err := c.hawk(auth.RRiot, path)
	if err != nil {
		return err
	}

	headers := http.Header{protocol.RESTHeaderAuthorization: {authorization}}

	return c.exchange(ctx, *auth.RRiot.R.A, method, path, nil, nil, headers, out)
}

func checkRooms(success bool, code *int, present bool) error {
	if !success {
		value := 0
		if code != nil {
			value = *code
		}

		return vendorError("rooms", value)
	}

	if !present {
		return roborockerrors.New(roborockerrors.Protocol, "rooms", "missing room list", nil)
	}

	return nil
}
