package roborock

import (
	"context"

	"github.com/portpowered/go-roborock/pkg/dependencies/rest"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// CreateHomeRoom creates a cloud directory entry without changing any robot map.
// Existing room names may be rejected. The caller explicitly binds the returned ID with NameSegments.
func (c *Client) CreateHomeRoom(ctx context.Context, request CreateHomeRoomRequest) (CreateHomeRoomResult, error) {
	out, err := c.rest.CreateHomeRoom(ctx, rest.CreateHomeRoomRequest{
		Auth: authWire(request.Auth), HomeID: request.HomeID, Name: request.Name,
	})
	if err != nil {
		return CreateHomeRoomResult{}, roborockerrors.Wrap(roborockerrors.Protocol,
			"CreateHomeRoom", "room creation failed", err)
	}

	return CreateHomeRoomResult{RoomID: out.RoomID}, nil
}
