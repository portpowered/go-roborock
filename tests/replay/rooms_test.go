package replay_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/portpowered/go-roborock/pkg/dependencies/rest"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

func TestPairedRoomRefresh(t *testing.T) {
	t.Parallel()

	fixture := loadRoomFixture(t)
	doer := &replayDoer{t: t, exchanges: fixture.Exchanges, index: 0}
	client := roomReplayClient(t, doer)

	replayRoomReads(t, client)
	replayRoomFailures(t, client)

	if doer.index != len(doer.exchanges) {
		t.Fatal("unconsumed room exchanges")
	}
}

func loadRoomFixture(t *testing.T) replayFixture {
	t.Helper()

	data, err := os.ReadFile("fixtures/rest/synthetic/rooms.json")
	if err != nil {
		t.Fatal(err)
	}

	var fixture replayFixture

	err = json.Unmarshal(data, &fixture)
	if err != nil {
		t.Fatal(err)
	}

	if fixture.Provenance != accountFixtureProvenance || fixture.Source == "" {
		t.Fatal("fixture provenance missing")
	}

	return fixture
}

func roomReplayClient(t *testing.T, doer *replayDoer) *rest.Client {
	t.Helper()

	client, err := rest.New(
		rest.WithHTTPClient(doer),
		rest.WithClock(func() time.Time { return time.Unix(1700000000, 0) }),
		rest.WithRandom(bytes.NewReader(bytes.Repeat([]byte{26}, 128))),
	)
	if err != nil {
		t.Fatal(err)
	}

	return client
}

func replayRoomReads(t *testing.T, client *rest.Client) {
	t.Helper()

	auth := roomReplayAuth()

	rooms, err := client.HomeRooms(t.Context(), rest.HomeRoomsRequest{Auth: auth, HomeID: 42})
	if err != nil {
		t.Fatal(err)
	}

	verifyRefreshedRooms(t, rooms)

	shared, err := client.SharedDeviceRooms(t.Context(), rest.SharedDeviceRoomsRequest{
		Auth: auth, DeviceID: sharedRoomDeviceID,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(shared.Rooms) != 2 || shared.Rooms[0].Id != 3 || shared.Rooms[1].Id != 4 || shared.Rooms[1].Name != nil {
		t.Fatalf("shared room IDs: %+v", shared)
	}
}

func verifyRefreshedRooms(t *testing.T, rooms rest.RoomsResult) {
	t.Helper()

	if len(rooms.Rooms) != 2 || rooms.Rooms[0].Name == nil || *rooms.Rooms[0].Name != "Kitchen" {
		t.Fatalf("room projection: %+v", rooms)
	}

	if rooms.Rooms[1].Name != nil || len(rooms.Rooms[0].AdditionalProperties["future"]) == 0 {
		t.Fatal("room name absence or unknown fields lost")
	}
}

func replayRoomFailures(t *testing.T, client *rest.Client) {
	t.Helper()

	request := rest.HomeRoomsRequest{Auth: roomReplayAuth(), HomeID: 42}

	_, err := client.HomeRooms(t.Context(), request)
	if !errors.Is(err, roborockerrors.New(roborockerrors.Protocol, "test", "", nil)) {
		t.Fatalf("missing result accepted: %v", err)
	}

	_, err = client.SharedDeviceRooms(t.Context(), rest.SharedDeviceRoomsRequest{
		Auth: request.Auth, DeviceID: sharedRoomDeviceID,
	})
	if !errors.Is(err, roborockerrors.New(roborockerrors.Protocol, "test", "", nil)) {
		t.Fatalf("shared room without identifier accepted: %v", err)
	}

	_, err = client.HomeRooms(t.Context(), request)
	if err == nil {
		t.Fatal("vendor rejection accepted")
	}
}

func roomReplayAuth() rest.AuthContext {
	origin := roomAPIOrigin

	var auth rest.AuthContext

	auth.RRiot = dependencymodels.RRiot{
		U: roomSyntheticUser, S: "synthetic-session", H: "synthetic-hawk-key", K: "synthetic-key",
		R:                    dependencymodels.RRiotReference{A: &origin, M: nil, L: nil, R: nil, AdditionalProperties: nil},
		AdditionalProperties: nil,
	}

	return auth
}
