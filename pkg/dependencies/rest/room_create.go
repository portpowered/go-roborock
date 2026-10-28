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

const createHomeRoomOperation = "create_home_room"

// CreateHomeRoomRequest creates a directory entry, independently of robot map bindings.
type CreateHomeRoomRequest struct {
	Auth   AuthContext
	HomeID int64
	Name   string
}

// CreateHomeRoomResult preserves string IDs and losslessly normalizes numeric IDs.
type CreateHomeRoomResult struct{ RoomID string }

// CreateHomeRoom creates a cloud room with account-scoped Hawk signing. It does not retry.
func (c *Client) CreateHomeRoom(ctx context.Context, request CreateHomeRoomRequest) (CreateHomeRoomResult, error) {
	if request.HomeID <= 0 || strings.TrimSpace(request.Name) == "" {
		return CreateHomeRoomResult{}, roborockerrors.New(roborockerrors.InvalidArgument,
			createHomeRoomOperation, "positive home ID and nonempty name required", nil)
	}

	if request.Auth.RRiot.R.A == nil || *request.Auth.RRiot.R.A == "" {
		return CreateHomeRoomResult{}, roborockerrors.New(roborockerrors.InvalidArgument,
			createHomeRoomOperation, "account API origin required", nil)
	}

	err := ctx.Err()
	if err != nil {
		return CreateHomeRoomResult{}, failure(createHomeRoomOperation, err)
	}

	path := strings.ReplaceAll(protocol.RESTPathCreateHomeRoom,
		"{"+protocol.RESTPathHomeID+"}", strconv.FormatInt(request.HomeID, 10))

	form := url.Values{protocol.RESTFormName: {request.Name}}

	authorization, err := c.hawkForm(request.Auth.RRiot, path, form)
	if err != nil {
		return CreateHomeRoomResult{}, err
	}

	var out dependencymodels.CreateHomeRoomResponse

	err = c.exchange(ctx, *request.Auth.RRiot.R.A, protocol.RESTMethodCreateHomeRoom, path, nil,
		form,
		http.Header{protocol.RESTHeaderAuthorization: {authorization}}, &out)
	if err != nil {
		return CreateHomeRoomResult{}, err
	}

	return createdRoom(out)
}

func createdRoom(out dependencymodels.CreateHomeRoomResponse) (CreateHomeRoomResult, error) {
	if !out.Success {
		failure := roborockerrors.New(roborockerrors.Protocol,
			createHomeRoomOperation, "service rejected room creation", nil)
		if out.Status != nil {
			failure.VendorStatus = *out.Status
			if *out.Status == protocol.RoomStatusDuplicateName || *out.Status == protocol.RoomStatusLimitExceeded {
				failure.Kind = roborockerrors.InvalidArgument
			}
		}

		return CreateHomeRoomResult{}, failure
	}

	if out.Result == nil {
		return CreateHomeRoomResult{}, roborockerrors.New(roborockerrors.Protocol,
			createHomeRoomOperation, "missing created room ID", nil)
	}

	text, err := out.Result.AsCreatedRoomID0()
	if err == nil {
		if strings.TrimSpace(text) == "" {
			return CreateHomeRoomResult{}, roborockerrors.New(roborockerrors.Protocol,
				createHomeRoomOperation, "missing created room ID", nil)
		}

		return CreateHomeRoomResult{RoomID: text}, nil
	}

	numeric, err := out.Result.AsCreatedRoomID1()
	if err != nil || numeric <= 0 {
		return CreateHomeRoomResult{}, roborockerrors.New(roborockerrors.Protocol,
			createHomeRoomOperation, "invalid created room ID", err)
	}

	return CreateHomeRoomResult{RoomID: strconv.FormatInt(numeric, 10)}, nil
}
