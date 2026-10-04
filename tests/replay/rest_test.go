package replay_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/go-roborock/pkg/dependencies/rest"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
)

type replayExchange struct {
	Method   string            `json:"method"`
	Origin   string            `json:"origin"`
	Path     string            `json:"path"`
	Query    url.Values        `json:"query"`
	Form     url.Values        `json:"form"`
	Headers  map[string]string `json:"headers"`
	Hawk     bool              `json:"hawk"`
	Response json.RawMessage   `json:"response"`
}
type replayFixture struct {
	Provenance string           `json:"provenance"`
	Source     string           `json:"source"`
	Exchanges  []replayExchange `json:"exchanges"`
}
type replayDoer struct {
	t         *testing.T
	exchanges []replayExchange
	index     int
}

func (d *replayDoer) Do(request *http.Request) (*http.Response, error) {
	d.t.Helper()

	if d.index >= len(d.exchanges) {
		d.t.Fatal("unexpected request")
	}

	expected := d.exchanges[d.index]
	d.index++
	d.verifyRequest(request, expected)
	d.verifyBody(request, expected)

	if expected.Hawk {
		verifyHawk(d.t, request.Header.Get("Authorization"), expected.Path)
	}

	var response http.Response

	response.StatusCode = http.StatusOK
	response.Header = http.Header{"Content-Type": {"application/json"}}
	response.Body = io.NopCloser(bytes.NewReader(expected.Response))

	return &response, nil
}

func (d *replayDoer) verifyRequest(request *http.Request, expected replayExchange) {
	d.t.Helper()

	if request.Method != expected.Method || request.URL.Scheme+"://"+request.URL.Host != expected.Origin ||
		request.URL.EscapedPath() != expected.Path {
		d.t.Fatalf("request mismatch at %d: %s %s", d.index, request.Method, request.URL)
	}

	if len(expected.Query) == 0 {
		expected.Query = url.Values{}
	}

	if !reflect.DeepEqual(request.URL.Query(), expected.Query) {
		d.t.Fatalf("query mismatch at %d: %v", d.index, request.URL.Query())
	}

	for key, value := range expected.Headers {
		if request.Header.Get(key) != value {
			d.t.Fatalf("header %s mismatch at %d", key, d.index)
		}
	}
}

func (d *replayDoer) verifyBody(request *http.Request, expected replayExchange) {
	d.t.Helper()

	var (
		body []byte
		err  error
	)

	if request.Body != nil {
		body, err = io.ReadAll(request.Body)
	}

	if err != nil {
		d.t.Fatal(err)
	}

	if expected.Form != nil {
		actual, parseErr := url.ParseQuery(string(body))
		if parseErr != nil || !reflect.DeepEqual(actual, expected.Form) {
			d.t.Fatalf("form mismatch at %d: %s", d.index, body)
		}
	} else if len(body) != 0 {
		d.t.Fatalf("unexpected request body at %d", d.index)
	}
}

func verifyHawk(t *testing.T, header, path string) {
	t.Helper()

	pattern := regexp.MustCompile(`^Hawk id="synthetic-user",s="synthetic-session",ts="1700000000",` +
		`nonce="([a-zA-Z0-9_-]{8})",mac="([a-zA-Z0-9+/]+=*)"$`)

	match := pattern.FindStringSubmatch(header)
	if match == nil {
		t.Fatalf("invalid Hawk header: %s", header)
	}

	digest := md5.Sum([]byte(path))
	message := strings.Join([]string{
		"synthetic-user", "synthetic-session", match[1], "1700000000", hex.EncodeToString(digest[:]), "", "",
	}, ":")
	mac := hmac.New(sha256.New, []byte("synthetic-hawk-key"))
	_, _ = mac.Write([]byte(message))

	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if match[2] != expected {
		t.Fatal("invalid Hawk MAC")
	}
}

func TestPairedAuthAndInventory(t *testing.T) {
	t.Parallel()

	fixtureBytes, err := os.ReadFile("fixtures/rest/synthetic/auth-inventory.json")
	if err != nil {
		t.Fatal(err)
	}

	var fixture replayFixture
	err = json.Unmarshal(fixtureBytes, &fixture)
	if err != nil {
		t.Fatal(err)
	}

	if fixture.Provenance != "synthetic" {
		t.Fatal("fixture provenance missing")
	}

	doer := &replayDoer{t: t, exchanges: fixture.Exchanges, index: 0}
	entropy := bytes.Repeat([]byte{26}, 128)

	client, err := rest.New(
		rest.WithHTTPClient(doer), rest.WithClock(func() time.Time { return time.Unix(1700000000, 0) }),
		rest.WithRandom(bytes.NewReader(entropy)),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	login := rest.LoginContext{
		Email: "one@example.test", ClientID: "client-1", Country: "GB", CountryCode: "44",
		BaseURL: "https://iot.example.test",
	}
	auth := replayAuthentication(ctx, t, client, login)
	replayHomeInventory(ctx, t, client, auth)

	if doer.index != len(doer.exchanges) {
		t.Fatalf("unconsumed exchanges: %d", len(doer.exchanges)-doer.index)
	}
}

func replayAuthentication(ctx context.Context, t *testing.T, client *rest.Client, login rest.LoginContext) rest.AuthContext {
	t.Helper()

	region, err := client.ResolveRegion(ctx, rest.RegionRequest{Email: login.Email, BaseURL: login.BaseURL})
	checkReplayError(t, err)

	if region.Region.Url != login.BaseURL {
		t.Fatalf("region: %v", err)
	}

	_, err = client.RequestCode(ctx, rest.CodeRequest{Login: login})
	checkReplayError(t, err)
	signature, err := client.SignKey(ctx, rest.SignRequest{Login: login})
	checkReplayError(t, err)

	if !regexp.MustCompile(`^[a-zA-Z0-9]{16}$`).MatchString(signature.Nonce) {
		t.Fatalf("signature: %v", err)
	}

	agreement, err := client.GetAgreement(ctx, rest.AgreementRequest{Login: login})
	checkReplayError(t, err)
	logged, err := client.LoginCode(ctx, rest.LoginCodeRequest{
		Login: login, Code: "001234", Signature: signature, Agreement: agreement.Agreement,
	})
	checkReplayError(t, err)

	second := login
	second.Email = "two@example.test"
	second.ClientID = "client-2"
	other, err := client.LoginPassword(ctx, rest.LoginPasswordRequest{Login: second, Password: "synthetic-password"})
	checkReplayError(t, err)

	if *other.User.Uid != 456 {
		t.Fatalf("second account: %v", err)
	}

	_, err = client.LoginLegacyCode(ctx, rest.LoginLegacyCodeRequest{Login: login, Code: "001234"})
	checkReplayError(t, err)

	return rest.AuthContext{
		Token: *logged.User.Token, ClientID: login.ClientID, BaseURL: login.BaseURL, RRiot: logged.User.Rriot,
	}
}

func replayHomeInventory(ctx context.Context, t *testing.T, client *rest.Client, auth rest.AuthContext) {
	t.Helper()

	home, err := client.HomeDetail(ctx, rest.HomeDetailRequest{Auth: auth})
	checkReplayError(t, err)

	if home.Home.RrHomeId != 42 {
		t.Fatalf("home: %v", err)
	}

	for version := 1; version <= 3; version++ {
		inventory, inventoryErr := client.HomeData(ctx, rest.HomeDataRequest{Auth: auth, HomeID: 42, Version: version})
		checkReplayError(t, inventoryErr)
		verifyInventory(t, inventory.Home)
	}
}

func verifyInventory(t *testing.T, home dependencymodels.HomeData) {
	t.Helper()

	devices := *home.Devices
	if devices[0].Online == nil || *devices[0].Online {
		t.Fatal("false presence lost")
	}

	if len(devices[0].AdditionalProperties["futureField"]) == 0 {
		t.Fatal("unknown field lost")
	}

	if (*home.Products)[0].Category != "future-category" {
		t.Fatal("unknown category lost")
	}
	verifyInventoryStatus(t, devices[0].DeviceStatus)
}

func verifyInventoryStatus(t *testing.T, status *dependencymodels.HomeDeviceStatus) {
	t.Helper()
	if status == nil || status.DPS121 == nil || *status.DPS121 != 8 || status.N206 == nil {
		t.Fatal("typed inventory status lost")
	}
	flag, err := status.N206.AsInventoryBooleanOrInteger0()
	checkReplayError(t, err)
	if !flag {
		t.Fatal("typed inventory boolean lost")
	}
	metadata, err := rest.DecodeProductInfo(*status.DPS10005)
	checkReplayError(t, err)
	if metadata.Sn == nil || *metadata.Sn != "synthetic-sn" {
		t.Fatal("typed embedded inventory metadata lost")
	}
}

func checkReplayError(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatal(err)
	}
}
