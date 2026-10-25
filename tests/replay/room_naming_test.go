package replay_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/portpowered/go-roborock/pkg/roborock"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

type roomNamingCase struct {
	Name             string          `json:"name"`
	Params           json.RawMessage `json:"params"`
	ErrorKind        string          `json:"errorKind"`
	ResponseFrameHex string          `json:"responseFrameHex"`
}

type roomNamingFixture struct {
	Provenance string           `json:"provenance"`
	Source     string           `json:"source"`
	Cases      []roomNamingCase `json:"cases"`
}

func TestPublicNameSegmentsEncryptedPairedProtocol(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("fixtures/mqtt/synthetic/room-naming.json")
	if err != nil {
		t.Fatal(err)
	}

	var fixture roomNamingFixture

	err = json.Unmarshal(data, &fixture)
	if err != nil {
		t.Fatal(err)
	}

	if fixture.Provenance != accountFixtureProvenance || fixture.Source == "" || len(fixture.Cases) == 0 {
		t.Fatal("missing room naming provenance or cases")
	}

	for _, replay := range fixture.Cases {
		t.Run(replay.Name, func(t *testing.T) { t.Parallel(); replayRoomNaming(t, replay) })
	}
}

func replayRoomNaming(t *testing.T, replay roomNamingCase) {
	t.Helper()

	account := loadMQTTFixture(t)
	account.Method = "name_segment"
	account.Params = replay.Params
	account.ResponseFrameHex = replay.ResponseFrameHex
	doer := &familyReplayHTTP{paired: replayDoer{t: t, exchanges: nil, index: 0}}
	connection, broker, ctx := familyReplayPipe(t)
	done := make(chan error, 1)

	go func() { done <- replayMQTT(broker, account) }()

	client := familySDKClient(t, doer, connection, false)

	session, err := client.OpenDevice(ctx, roborock.OpenDeviceRequest{
		Auth: familyReplayAuth(account), DeviceID: account.DeviceID,
		LocalKey: account.LocalKey, Protocol: roborock.ProtocolV1,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = session.Close() })

	request := roborock.NameSegmentsRequest{Mappings: []roborock.RoomNameMapping{
		{CloudRoomID: "1001", SegmentID: 16, RobotTagID: 6},
		{CloudRoomID: "1002", SegmentID: 17, RobotTagID: 0},
		{CloudRoomID: "", SegmentID: 18, RobotTagID: 12},
	}}
	assertRejectedRoomNaming(t, session, request)

	ack, err := session.NameSegments(ctx, request)
	assertFamilyReplayError(t, err, replay.ErrorKind)

	if ack.Sent != (replay.ErrorKind == "") || ack.Acknowledged != ack.Sent {
		t.Fatalf("unexpected naming acknowledgement: %+v", ack)
	}

	assertFamilyHTTPConsumed(t, doer)
	closeFamilyReplay(t, session, done)
}

func assertRejectedRoomNaming(t *testing.T, session *roborock.DeviceSession, request roborock.NameSegmentsRequest) {
	t.Helper()

	_, err := session.NameSegments(t.Context(), roborock.NameSegmentsRequest{Mappings: nil})
	assertFamilyReplayError(t, err, string(roborockerrors.InvalidArgument))

	duplicate := roborock.NameSegmentsRequest{
		Mappings: []roborock.RoomNameMapping{request.Mappings[0], request.Mappings[0]},
	}
	_, err = session.NameSegments(t.Context(), duplicate)
	assertFamilyReplayError(t, err, string(roborockerrors.InvalidArgument))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err = session.NameSegments(ctx, request)
	assertFamilyReplayError(t, err, string(roborockerrors.Canceled))
}
