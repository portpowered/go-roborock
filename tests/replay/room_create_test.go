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

	fixture := loadRoomCreationFixture(t)
	for _, replay := range fixture.Cases {
		t.Run(replay.Name, func(t *testing.T) { t.Parallel(); replayRoomCreation(t, replay) })
	}
}

func loadRoomCreationFixture(t *testing.T) roomCreationFixture {
	t.Helper()

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

	return fixture
}

func TestCreatedRoomIdentifierEncodingsAreLosslessAndNeverRetry(t *testing.T) {
	t.Parallel()

	fixture := loadRoomCreationFixture(t)
	protocolKind := string(roborockerrors.Protocol)
	invalidKind := string(roborockerrors.InvalidArgument)

	cases := []struct{ name, body, roomID, errorKind, vendorStatus string }{
		{"numeric", `{"success":true,"result":12}`, "12", "", ""},
		{"above float precision", `{"success":true,"result":9007199254740993}`, "9007199254740993", "", ""},
		{"signed maximum", `{"success":true,"result":9223372036854775807}`, "9223372036854775807", "", ""},
		{"string above float precision", `{"success":true,"result":"9007199254740993"}`, "9007199254740993", "", ""},
		{"opaque string", `{"success":true,"result":"room-001"}`, "room-001", "", ""},
		{"padded opaque string", `{"success":true,"result":" room-001 "}`, " room-001 ", "", ""},
		{"leading zero string", `{"success":true,"result":"0012"}`, "0012", "", ""},
		{"fraction", `{"success":true,"result":12.5}`, "", protocolKind, ""},
		{"decimal integer", `{"success":true,"result":12.0}`, "", protocolKind, ""},
		{"exponent", `{"success":true,"result":12e2}`, "", protocolKind, ""},
		{"zero", `{"success":true,"result":0}`, "", protocolKind, ""},
		{"negative", `{"success":true,"result":-1}`, "", protocolKind, ""},
		{"overflow", `{"success":true,"result":9223372036854775808}`, "", protocolKind, ""},
		{"null", `{"success":true,"result":null}`, "", protocolKind, ""},
		{"boolean", `{"success":true,"result":true}`, "", protocolKind, ""},
		{"object", `{"success":true,"result":{"id":12}}`, "", protocolKind, ""},
		{"array", `{"success":true,"result":[12]}`, "", protocolKind, ""},
		{"empty", `{"success":true,"result":""}`, "", protocolKind, ""},
		{"blank", `{"success":true,"result":"  "}`, "", protocolKind, ""},
		{"missing", `{"success":true}`, "", protocolKind, ""},
		{"missing success", `{"result":12}`, "", protocolKind, ""},
		{"null success", `{"success":null,"result":12}`, "", protocolKind, ""},
		{"duplicate with numeric result", `{"success":false,"status":"userinfo.err.duplicate.name","result":12}`,
			"", invalidKind, "userinfo.err.duplicate.name"},
		{"limit with numeric result", `{"success":false,"status":"userinfo.err.room.exceed.limit","result":12}`,
			"", invalidKind, "userinfo.err.room.exceed.limit"},
		{"unknown rejection", `{"success":false,"status":"future.rejection","result":12}`,
			"", protocolKind, "future.rejection"},
		{"duplicate with invalid result", `{"success":false,"status":"userinfo.err.duplicate.name","result":0}`,
			"", protocolKind, ""},
		{"duplicate with object result", `{"success":false,"status":"userinfo.err.duplicate.name","result":{}}`,
			"", protocolKind, ""},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			replay := fixture.Cases[0]
			replay.Exchange.Response = json.RawMessage(test.body)
			replay.RoomID, replay.ErrorKind, replay.VendorStatus = test.roomID, test.errorKind, test.vendorStatus
			replayRoomCreation(t, replay)
		})
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
