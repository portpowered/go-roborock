package replay_test

import (
	"context"
	"crypto/hmac"
	"crypto/md5" //nolint:gosec // Independent Hawk oracle verifies the mandated path digest.
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/go-roborock/pkg/roborock"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const (
	familyIOTOrigin  = "https://iot.example.test"
	familyReplayUser = "synthetic-user"
)

type familyReplayCase struct {
	Name             string           `json:"name"`
	Exchanges        []replayExchange `json:"exchanges"`
	Error            string           `json:"error"`
	MapTrace         bool             `json:"mapTrace"`
	ZoneCleaning     bool             `json:"zoneCleaning"`
	Result           json.RawMessage  `json:"result"`
	Names            []*string        `json:"names"`
	ResponseFrameHex string           `json:"responseFrameHex"`
}

type familyReplayFixture struct {
	Provenance string             `json:"provenance"`
	Source     string             `json:"source"`
	Cases      []familyReplayCase `json:"cases"`
}

type familyReplayHTTP struct {
	paired replayDoer
}

func (d *familyReplayHTTP) Do(request *http.Request) (*http.Response, error) {
	d.paired.t.Helper()

	if d.paired.index >= len(d.paired.exchanges) {
		d.paired.t.Fatal("unexpected or duplicate account HTTP exchange")
	}

	expected := d.paired.exchanges[d.paired.index]
	d.paired.index++
	d.paired.verifyRequest(request, expected)
	d.paired.verifyBody(request, expected)

	if expected.Hawk {
		verifyFamilyHawk(d.paired.t, request.Header.Get("Authorization"), expected.Path)
	}

	return pairedFixtureResponse(d.paired.t, expected.ResponseStatus, expected.ResponseHeaders, expected.Response), nil
}

func verifyFamilyHawk(t *testing.T, header, path string) {
	t.Helper()

	pattern := regexp.MustCompile(`^Hawk id="synthetic-user",s="synthetic-secret",ts="([0-9]+)",` +
		`nonce="([a-zA-Z0-9_-]{8})",mac="([a-zA-Z0-9+/]+=*)"$`)

	match := pattern.FindStringSubmatch(header)
	if match == nil {
		t.Fatal("Hawk account or volatile field format mismatch")
	}

	stamp, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil || stamp < time.Now().Unix()-5 || stamp > time.Now().Unix()+5 {
		t.Fatal("Hawk timestamp outside replay window")
	}

	digest := md5.Sum([]byte(path)) //nolint:gosec // Vendor Hawk path signing requires MD5.
	message := strings.Join([]string{
		familyReplayUser, "synthetic-secret", match[2], match[1], hex.EncodeToString(digest[:]), "", "",
	}, ":")
	mac := hmac.New(sha256.New, []byte("synthetic-hawk-key"))
	_, _ = mac.Write([]byte(message))

	if match[3] != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
		t.Fatal("Hawk MAC does not bind the account and expected escaped path")
	}
}

func loadFamilyReplay(t *testing.T, path string) familyReplayFixture {
	t.Helper()

	data, err := os.ReadFile(path) //nolint:gosec // Both callers supply fixed repository-owned synthetic fixture paths.
	if err != nil {
		t.Fatal(err)
	}

	var fixture familyReplayFixture

	err = json.Unmarshal(data, &fixture)
	if err != nil {
		t.Fatal(err)
	}

	if fixture.Provenance != "synthetic" || fixture.Source == "" || len(fixture.Cases) == 0 {
		t.Fatal("missing synthetic fixture provenance or cases")
	}

	return fixture
}

func familyReplayAuth(account mqttFixture) roborock.AuthContext {
	return roborock.AuthContext{
		BaseURL: familyIOTOrigin, ClientID: "sdk-client", Token: "synthetic-token",
		Mqtt: roborock.MQTTAuth{
			APIURL: "https://api.example.test", BrokerURL: account.Broker,
			User: account.User, Secret: account.Secret, Key: account.Key, SigningKey: "synthetic-hawk-key",
		},
	}
}

func TestPairedSDKFamilyMetadata(t *testing.T) {
	t.Parallel()

	fixture := loadFamilyReplay(t, "fixtures/rest/synthetic/sdk-family-metadata.json")
	for _, replay := range fixture.Cases {
		t.Run(replay.Name, func(t *testing.T) { t.Parallel(); runSDKFamilyReplay(t, replay) })
	}
}

func runSDKFamilyReplay(t *testing.T, replay familyReplayCase) {
	t.Helper()

	account := loadMQTTFixture(t)
	doer := &familyReplayHTTP{paired: replayDoer{t: t, exchanges: replay.Exchanges, index: 0}}
	connection, broker, ctx := familyReplayPipe(t)
	done := make(chan error, 1)

	if replay.Error == "" {
		go func() { done <- replayFamilyHandshake(broker, account) }()
	}

	client := familySDKClient(t, doer, connection, replay.Error != "")
	session, err := client.OpenDevice(ctx, roborock.OpenDeviceRequest{
		Auth: familyReplayAuth(account), DeviceID: account.DeviceID, LocalKey: account.LocalKey,
		Protocol: roborock.ProtocolB01,
	})
	assertFamilyReplayError(t, err, replay.Error)

	if replay.Error == "" {
		t.Cleanup(func() { _ = session.Close() })

		assertFamilyCapabilities(ctx, t, session, replay)

		closeFamilyReplay(t, session, done)
	} else if session != nil {
		t.Fatal("metadata rejection returned a session")
	}

	assertFamilyHTTPConsumed(t, doer)
}

func assertFamilyCapabilities(
	ctx context.Context, t *testing.T, session *roborock.DeviceSession, replay familyReplayCase,
) {
	t.Helper()

	capabilities, err := session.GetCapabilities(ctx, roborock.EmptyRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if !capabilities.MapContent || !capabilities.MapList || !capabilities.MapRooms ||
		!capabilities.RoomCleaning || capabilities.MapSelection ||
		capabilities.MapTrace != replay.MapTrace || capabilities.ZoneCleaning != replay.ZoneCleaning {
		t.Fatalf("authenticated inventory selected incorrect family capabilities: %+v", capabilities)
	}
}

func familyReplayPipe(t *testing.T) (net.Conn, net.Conn, context.Context) {
	t.Helper()

	connection, broker := net.Pipe()

	t.Cleanup(func() { _ = connection.Close(); _ = broker.Close() })

	deadline := time.Now().Add(5 * time.Second)

	err := broker.SetDeadline(deadline)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithDeadline(t.Context(), deadline)
	t.Cleanup(cancel)

	return connection, broker, ctx
}

func familySDKClient(t *testing.T, doer *familyReplayHTTP, connection net.Conn, rejectDial bool) *roborock.Client {
	t.Helper()

	client, err := roborock.NewClient(roborock.WithHTTPClient(doer),
		roborock.WithMQTTDial(func(_ context.Context, network, address string) (net.Conn, error) {
			if rejectDial || network != mqttReplayNetwork || address != mqttReplayAddress {
				return nil, mqttMismatch("unexpected SDK MQTT dial: %s %s", network, address)
			}

			return connection, nil
		}))
	if err != nil {
		t.Fatal(err)
	}

	return client
}

func replayFamilyHandshake(conn net.Conn, account mqttFixture) error {
	err := replayMQTTHandshake(conn, account)
	if err != nil {
		return err
	}

	var extra [1]byte

	_, err = conn.Read(extra[:])
	if !errors.Is(err, io.EOF) {
		return mqttMismatch("unexpected or duplicate SDK family frame: %x (%v)", extra, err)
	}

	return nil
}

func assertFamilyReplayError(t *testing.T, err error, kind string) {
	t.Helper()

	if kind == "" {
		if err != nil {
			t.Fatal(err)
		}

		return
	}

	if !errors.Is(err, roborockerrors.New(roborockerrors.Kind(kind), "replay", "", nil)) {
		t.Fatalf("error kind=%s, received %v", kind, err)
	}
}

func closeFamilyReplay(t *testing.T, session *roborock.DeviceSession, done <-chan error) {
	t.Helper()

	for range 2 {
		err := session.Close()
		if err != nil {
			t.Fatal(err)
		}
	}

	select {
	case <-session.Done():
	default:
		t.Fatal("Close did not end the public session")
	}

	assertFamilyReplayError(t, session.Err(), string(roborockerrors.Closed))
	_, err := session.GetRooms(t.Context(), roborock.EmptyRequest{})
	assertFamilyReplayError(t, err, string(roborockerrors.Canceled))

	err = <-done
	if err != nil {
		t.Fatal(err)
	}
}

func assertFamilyHTTPConsumed(t *testing.T, doer *familyReplayHTTP) {
	t.Helper()

	if doer.paired.index != len(doer.paired.exchanges) {
		t.Fatalf("unconsumed SDK account exchanges: %d", len(doer.paired.exchanges)-doer.paired.index)
	}
}

func TestPairedSDKRoomNames(t *testing.T) {
	t.Parallel()

	fixture := loadFamilyReplay(t, "fixtures/mqtt/synthetic/sdk-room-names.json")
	for _, replay := range fixture.Cases {
		t.Run(replay.Name, func(t *testing.T) { t.Parallel(); runSDKRoomReplay(t, replay) })
	}
}

func runSDKRoomReplay(t *testing.T, replay familyReplayCase) {
	t.Helper()

	account := loadMQTTFixture(t)
	account.Method = "get_room_mapping"
	account.Params = json.RawMessage(`[]`)
	account.Result = replay.Result
	account.ResponseFrameHex = replay.ResponseFrameHex
	doer := &familyReplayHTTP{paired: replayDoer{t: t, exchanges: replay.Exchanges, index: 0}}
	connection, broker, ctx := familyReplayPipe(t)
	done := make(chan error, 1)

	go func() { done <- replayMQTT(broker, account) }()

	client := familySDKClient(t, doer, connection, false)

	session, err := client.OpenDevice(ctx, roborock.OpenDeviceRequest{
		Auth: familyReplayAuth(account), DeviceID: account.DeviceID, LocalKey: account.LocalKey,
		Protocol: roborock.ProtocolV1,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = session.Close() })

	rooms, err := session.GetRooms(ctx, roborock.EmptyRequest{})
	assertFamilyReplayError(t, err, replay.Error)

	if replay.Error == "" {
		assertSDKRoomNames(t, rooms, replay.Names)
	}

	assertFamilyHTTPConsumed(t, doer)
	closeFamilyReplay(t, session, done)
}

func assertSDKRoomNames(t *testing.T, result roborock.MapRoomsResult, names []*string) {
	t.Helper()

	if len(result.Rooms) != len(names) {
		t.Fatalf("room count=%d expected=%d", len(result.Rooms), len(names))
	}

	for index, room := range result.Rooms {
		if room.SegmentID != int64(16+index) || room.IoTID == nil || *room.IoTID != strconv.Itoa(index+1) {
			t.Fatalf("segment and cloud IDs lost: %+v", room)
		}

		if names[index] == nil {
			if room.Name != nil {
				t.Fatalf("unknown room name invented: %q", *room.Name)
			}
		} else if room.Name == nil || *room.Name != *names[index] {
			t.Fatalf("room %d name mismatch: %+v", index, room)
		}
	}
}
