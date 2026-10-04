package rest_test

import (
	"errors"
	"testing"

	"github.com/portpowered/go-roborock/pkg/dependencies/rest"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

func TestRoomLookupRequiresIdentity(t *testing.T) {
	t.Parallel()

	client, err := rest.New()
	if err != nil {
		t.Fatal(err)
	}

	var auth rest.AuthContext

	_, err = client.HomeRooms(t.Context(), rest.HomeRoomsRequest{Auth: auth, HomeID: 0})
	if !errors.Is(err, roborockerrors.New(roborockerrors.InvalidArgument, "test", "", nil)) {
		t.Fatalf("missing home ID: %v", err)
	}

	_, err = client.SharedDeviceRooms(t.Context(), rest.SharedDeviceRoomsRequest{Auth: auth, DeviceID: ""})
	if !errors.Is(err, roborockerrors.New(roborockerrors.InvalidArgument, "test", "", nil)) {
		t.Fatalf("missing device ID: %v", err)
	}

	_, err = client.HomeRooms(t.Context(), rest.HomeRoomsRequest{Auth: auth, HomeID: 42})
	if !errors.Is(err, roborockerrors.New(roborockerrors.InvalidArgument, "test", "", nil)) {
		t.Fatalf("missing account origin: %v", err)
	}
}
