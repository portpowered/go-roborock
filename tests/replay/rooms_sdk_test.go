package replay_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/portpowered/go-roborock/pkg/roborock"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const (
	sharedRoomDeviceID = "shared device"
	roomAPIOrigin      = "https://api.example.test"
	roomSyntheticUser  = "synthetic-user"
)

type pairedSDKRooms struct {
	paired   *replayDoer
	accounts []roborock.AuthContext
	failure  error
}

func (p *pairedSDKRooms) Do(request *http.Request) (*http.Response, error) {
	p.paired.t.Helper()

	if p.paired.index >= len(p.paired.exchanges) {
		p.paired.t.Fatal("unexpected SDK room exchange")
	}

	expected := p.paired.exchanges[p.paired.index]
	account := p.accounts[p.paired.index]
	p.paired.index++
	p.paired.verifyRequest(request, expected)
	p.paired.verifyBody(request, expected)
	verifySDKRoomHawk(p.paired.t, request.Header.Get("Authorization"), expected.Path, account.Mqtt)

	if p.failure != nil {
		return nil, p.failure
	}

	return pairedFixtureResponse(p.paired.t, expected.ResponseStatus, expected.ResponseHeaders, expected.Response), nil
}

func verifySDKRoomHawk(t *testing.T, header, path string, account roborock.MQTTAuth) {
	t.Helper()

	pattern := regexp.MustCompile(`^Hawk id="([^"]+)",s="([^"]+)",ts="([0-9]+)",` +
		`nonce="([a-zA-Z0-9_-]{8})",mac="([a-zA-Z0-9+/]+=*)"$`)

	match := pattern.FindStringSubmatch(header)
	if match == nil {
		t.Fatalf("invalid SDK Hawk format: %s", header)
	}

	if match[1] != account.User || match[2] != account.Secret {
		t.Fatal("SDK reused another account's Hawk credentials")
	}

	message := strings.Join(
		[]string{account.User, account.Secret, match[4], match[3], roomPathDigest(t, path), "", ""}, ":",
	)
	mac := hmac.New(sha256.New, []byte(account.SigningKey))
	_, _ = mac.Write([]byte(message))

	if match[5] != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
		t.Fatal("SDK Hawk MAC does not match the caller account and escaped path")
	}
}

func roomPathDigest(t *testing.T, path string) string {
	t.Helper()

	// Independently calculated MD5 values for the fixed synthetic fixture paths.
	switch path {
	case "/user/homes/42/rooms":
		return "13d55dec3f02c50dc8c24e52f07fc801"
	case "/user/deviceshare/query/shared%20device/rooms":
		return "6be83c76a0854855c9593847f6bab8ae"
	default:
		t.Fatalf("unregistered room fixture path: %s", path)

		return ""
	}
}

func TestPublicSDKRoomRefresh(t *testing.T) {
	t.Parallel()

	fixture := loadRoomFixture(t)
	first := sdkRoomAccount("first", roomAPIOrigin)
	second := sdkRoomAccount("second", "https://shared-api.example.test")
	accounts := []roborock.AuthContext{first, second, first, second, first}

	for index := range fixture.Exchanges {
		fixture.Exchanges[index].Origin = accounts[index].Mqtt.APIURL
	}

	pair := &pairedSDKRooms{
		paired:   &replayDoer{t: t, exchanges: fixture.Exchanges, index: 0},
		accounts: accounts, failure: nil,
	}

	client, err := roborock.NewClient(roborock.WithHTTPClient(pair))
	if err != nil {
		t.Fatal(err)
	}

	replaySDKRoomResults(t, client, first, second)
	replaySDKRoomErrors(t, client, first, second)

	if !reflect.DeepEqual(first, accounts[0]) || !reflect.DeepEqual(second, accounts[1]) {
		t.Fatal("SDK mutated caller account credentials")
	}

	if pair.paired.index != len(fixture.Exchanges) {
		t.Fatal("unconsumed SDK room exchanges")
	}
}

func sdkRoomAccount(name, origin string) roborock.AuthContext {
	var account roborock.AuthContext

	account.Token = name + "-token"
	account.ClientID = name + "-client"
	account.BaseURL = "https://room-iot.example.test"
	account.Mqtt = roborock.MQTTAuth{
		APIURL: origin, BrokerURL: "", Key: name + "-key", User: name + "-user",
		Secret: name + "-secret", SigningKey: name + "-hawk-key",
	}

	return account
}

func replaySDKRoomResults(t *testing.T, client *roborock.Client, first, second roborock.AuthContext) {
	t.Helper()

	home, err := client.GetHomeRooms(t.Context(), roborock.HomeRoomsRequest{Auth: first, HomeID: 42})
	if err != nil {
		t.Fatal(err)
	}

	if len(home.Rooms) != 2 || home.Rooms[0].ID != 1 || home.Rooms[1].ID != 2 {
		t.Fatalf("public home room IDs: %+v", home)
	}

	if home.Rooms[0].Name == nil || *home.Rooms[0].Name != "Kitchen" || home.Rooms[1].Name != nil {
		t.Fatalf("public home room names: %+v", home)
	}

	shared, err := client.GetSharedDeviceRooms(t.Context(), roborock.SharedDeviceRoomsRequest{
		Auth: second, DeviceID: sharedRoomDeviceID,
	})
	if err != nil {
		t.Fatal(err)
	}

	verifySDKSharedRooms(t, shared)
}

func verifySDKSharedRooms(t *testing.T, shared roborock.RoomsResult) {
	t.Helper()

	if len(shared.Rooms) != 2 || shared.Rooms[0].ID != 3 || shared.Rooms[1].ID != 4 {
		t.Fatalf("roomId alias or id precedence lost: %+v", shared)
	}

	if shared.Rooms[0].Name == nil || *shared.Rooms[0].Name != "Office" || shared.Rooms[1].Name != nil {
		t.Fatalf("public shared room names: %+v", shared)
	}
}

func replaySDKRoomErrors(t *testing.T, client *roborock.Client, first, second roborock.AuthContext) {
	t.Helper()

	_, err := client.GetHomeRooms(t.Context(), roborock.HomeRoomsRequest{Auth: first, HomeID: 42})
	assertSDKRoomProtocol(t, err)

	_, err = client.GetSharedDeviceRooms(t.Context(), roborock.SharedDeviceRoomsRequest{
		Auth: second, DeviceID: sharedRoomDeviceID,
	})
	assertSDKRoomProtocol(t, err)

	_, err = client.GetHomeRooms(t.Context(), roborock.HomeRoomsRequest{Auth: first, HomeID: 42})
	if !errors.Is(err, roborockerrors.New(roborockerrors.Unauthorized, "test", "", nil)) {
		t.Fatalf("vendor authorization class lost: %v", err)
	}

	var typed *roborockerrors.Error
	if !errors.As(err, &typed) || typed.Code != 2010 {
		t.Fatalf("vendor error code lost: %v", err)
	}
}

func assertSDKRoomProtocol(t *testing.T, err error) {
	t.Helper()

	if !errors.Is(err, roborockerrors.New(roborockerrors.Protocol, "test", "", nil)) {
		t.Fatalf("public error class lost: %v", err)
	}
}

func TestPublicSDKRoomCancellation(t *testing.T) {
	t.Parallel()

	fixture := loadRoomFixture(t)
	account := sdkRoomAccount("canceled", roomAPIOrigin)
	pair := &pairedSDKRooms{
		paired:   &replayDoer{t: t, exchanges: fixture.Exchanges[:2], index: 0},
		accounts: []roborock.AuthContext{account, account}, failure: context.Canceled,
	}

	client, err := roborock.NewClient(roborock.WithHTTPClient(pair))
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.GetHomeRooms(t.Context(), roborock.HomeRoomsRequest{Auth: account, HomeID: 42})
	assertSDKRoomCancellation(t, err)

	_, err = client.GetSharedDeviceRooms(t.Context(), roborock.SharedDeviceRoomsRequest{
		Auth: account, DeviceID: sharedRoomDeviceID,
	})
	assertSDKRoomCancellation(t, err)

	if pair.paired.index != len(pair.paired.exchanges) {
		t.Fatal("unconsumed cancellation exchanges")
	}
}

func assertSDKRoomCancellation(t *testing.T, err error) {
	t.Helper()

	if !errors.Is(err, context.Canceled) ||
		!errors.Is(err, roborockerrors.New(roborockerrors.Canceled, "test", "", nil)) {
		t.Fatalf("public cancellation class or cause lost: %v", err)
	}
}
