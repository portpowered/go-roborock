package replay_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/portpowered/go-roborock/pkg/roborock"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const creationFixtureName = "Studio + bath & sun/room"

type roomCreationCase struct {
	Name         string         `json:"name"`
	RoomID       string         `json:"roomId"`
	ErrorKind    string         `json:"errorKind"`
	VendorStatus string         `json:"vendorStatus"`
	Exchange     replayExchange `json:"exchange"`
}

type roomCreationFixture struct {
	Provenance string             `json:"provenance"`
	Source     string             `json:"source"`
	Cases      []roomCreationCase `json:"cases"`
}

func TestPublicCreateHomeRoomPairedProtocol(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("fixtures/rest/synthetic/room-create.json")
	if err != nil {
		t.Fatal(err)
	}

	var fixture roomCreationFixture

	err = json.Unmarshal(data, &fixture)
	if err != nil {
		t.Fatal(err)
	}

	if fixture.Provenance != accountFixtureProvenance || fixture.Source == "" || len(fixture.Cases) == 0 {
		t.Fatal("missing room creation provenance or cases")
	}

	for _, replay := range fixture.Cases {
		t.Run(replay.Name, func(t *testing.T) { t.Parallel(); replayRoomCreation(t, replay) })
	}
}

func replayRoomCreation(t *testing.T, replay roomCreationCase) {
	t.Helper()

	account := sdkRoomAccount("creation", roomAPIOrigin)
	paired := &pairedSDKRooms{
		paired:   &replayDoer{t: t, exchanges: []replayExchange{replay.Exchange}, index: 0},
		accounts: []roborock.AuthContext{account}, failure: nil,
	}

	client, err := roborock.NewClient(roborock.WithHTTPClient(paired))
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.CreateHomeRoom(t.Context(), roborock.CreateHomeRoomRequest{
		Auth: account, HomeID: 42, Name: creationFixtureName,
	})
	assertFamilyReplayError(t, err, replay.ErrorKind)

	if result.RoomID != replay.RoomID || paired.paired.index != len(paired.paired.exchanges) {
		t.Fatalf("room ID or exchange count mismatch: %+v", result)
	}

	if err != nil {
		var failure *roborockerrors.Error

		if !errors.As(err, &failure) || failure.VendorStatus != replay.VendorStatus {
			t.Fatalf("vendor status lost through public wrapper: %v", err)
		}
	}
}

type cancelRoomCreationHTTP struct{ started chan struct{} }

func (d cancelRoomCreationHTTP) Do(request *http.Request) (*http.Response, error) {
	close(d.started)
	<-request.Context().Done()

	return nil, fmt.Errorf("synthetic room creation canceled: %w", request.Context().Err())
}

func TestCreateHomeRoomContextAndValidation(t *testing.T) {
	t.Parallel()

	doer := cancelRoomCreationHTTP{started: make(chan struct{})}

	client, err := roborock.NewClient(roborock.WithHTTPClient(doer))
	if err != nil {
		t.Fatal(err)
	}

	request := roborock.CreateHomeRoomRequest{
		Auth: sdkRoomAccount("creation", roomAPIOrigin), HomeID: 42, Name: "Temporary",
	}
	missingOrigin := request.Auth
	missingOrigin.Mqtt.APIURL = ""
	missingUser := request.Auth
	missingUser.Mqtt.User = ""

	for _, invalid := range []roborock.CreateHomeRoomRequest{
		{Auth: request.Auth, HomeID: 0, Name: request.Name},
		{Auth: request.Auth, HomeID: request.HomeID, Name: "  "},
		{Auth: missingOrigin, HomeID: request.HomeID, Name: request.Name},
		{Auth: missingUser, HomeID: request.HomeID, Name: request.Name},
	} {
		_, validationErr := client.CreateHomeRoom(t.Context(), invalid)
		assertFamilyReplayError(t, validationErr, string(roborockerrors.InvalidArgument))
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	result := make(chan error, 1)

	go func() { _, failure := client.CreateHomeRoom(ctx, request); result <- failure }()

	<-doer.started
	cancel()
	assertFamilyReplayError(t, <-result, string(roborockerrors.Canceled))

	_, err = client.CreateHomeRoom(ctx, request)
	assertFamilyReplayError(t, err, string(roborockerrors.Canceled))
}
