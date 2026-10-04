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
	"github.com/portpowered/go-roborock/pkg/dependencies/rest"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

type replayExchange struct {
	Method   string
	Origin   string
	Path     string
	Query    url.Values
	Form     url.Values
	Headers  map[string]string
	Hawk     bool
	Response json.RawMessage
}
type replayFixture struct {
	Provenance string
	Source     string
	Exchanges  []replayExchange
}
type replayDoer struct {
	t         *testing.T
	exchanges []replayExchange
	index     int
}

func (d *replayDoer) Do(r *http.Request) (*http.Response, error) {
	d.t.Helper()
	if d.index >= len(d.exchanges) {
		d.t.Fatal("unexpected request")
	}
	expected := d.exchanges[d.index]
	d.index++
	if r.Method != expected.Method || r.URL.Scheme+"://"+r.URL.Host != expected.Origin || r.URL.EscapedPath() != expected.Path {
		d.t.Fatalf("request mismatch at %d: %s %s", d.index, r.Method, r.URL)
	}
	if len(expected.Query) == 0 {
		expected.Query = url.Values{}
	}
	if !reflect.DeepEqual(r.URL.Query(), expected.Query) {
		d.t.Fatalf("query mismatch at %d: %v", d.index, r.URL.Query())
	}
	for key, value := range expected.Headers {
		if r.Header.Get(key) != value {
			d.t.Fatalf("header %s mismatch at %d", key, d.index)
		}
	}
	var body []byte
	var err error
	if r.Body != nil {
		body, err = io.ReadAll(r.Body)
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
	if expected.Hawk {
		verifyHawk(d.t, r.Header.Get("Authorization"), expected.Path)
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(expected.Response))}, nil
}

func verifyHawk(t *testing.T, header, path string) {
	t.Helper()
	pattern := regexp.MustCompile(`^Hawk id="synthetic-user",s="synthetic-session",ts="1700000000",nonce="([a-zA-Z0-9_-]{8})",mac="([a-zA-Z0-9+/]+=*)"$`)
	match := pattern.FindStringSubmatch(header)
	if match == nil {
		t.Fatalf("invalid Hawk header: %s", header)
	}
	digest := md5.Sum([]byte(path))
	message := strings.Join([]string{"synthetic-user", "synthetic-session", match[1], "1700000000", hex.EncodeToString(digest[:]), "", ""}, ":")
	mac := hmac.New(sha256.New, []byte("synthetic-hawk-key"))
	_, _ = mac.Write([]byte(message))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if match[2] != expected {
		t.Fatal("invalid Hawk MAC")
	}
}

func TestPairedAuthAndInventory(t *testing.T) {
	fixtureBytes, err := os.ReadFile("fixtures/rest/synthetic/auth-inventory.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture replayFixture
	if err = json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Provenance != "synthetic" {
		t.Fatal("fixture provenance missing")
	}
	doer := &replayDoer{t: t, exchanges: fixture.Exchanges}
	entropy := bytes.Repeat([]byte{26}, 128)
	c, err := rest.New(rest.WithHTTPClient(doer), rest.WithClock(func() time.Time { return time.Unix(1700000000, 0) }), rest.WithRandom(bytes.NewReader(entropy)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	login := rest.LoginContext{Email: "one@example.test", ClientID: "client-1", Country: "GB", CountryCode: "44", BaseURL: "https://iot.example.test"}
	region, err := c.ResolveRegion(ctx, rest.RegionRequest{Email: login.Email, BaseURL: login.BaseURL})
	if err != nil || region.Region.Url != login.BaseURL {
		t.Fatalf("region: %v", err)
	}
	if _, err = c.RequestCode(ctx, rest.CodeRequest{Login: login}); err != nil {
		t.Fatal(err)
	}
	signature, err := c.SignKey(ctx, rest.SignRequest{Login: login})
	if err != nil || !regexp.MustCompile(`^[a-zA-Z0-9]{16}$`).MatchString(signature.Nonce) {
		t.Fatalf("signature: %v", err)
	}
	agreement, err := c.GetAgreement(ctx, rest.AgreementRequest{Login: login})
	if err != nil {
		t.Fatal(err)
	}
	logged, err := c.LoginCode(ctx, rest.LoginCodeRequest{Login: login, Code: "001234", Signature: signature, Agreement: agreement.Agreement})
	if err != nil {
		t.Fatal(err)
	}
	second := login
	second.Email = "two@example.test"
	second.ClientID = "client-2"
	other, err := c.LoginPassword(ctx, rest.LoginPasswordRequest{Login: second, Password: "synthetic-password"})
	if err != nil || *other.User.Uid != 456 {
		t.Fatalf("second account: %v", err)
	}
	if _, err = c.LoginLegacyCode(ctx, rest.LoginLegacyCodeRequest{Login: login, Code: "001234"}); err != nil {
		t.Fatal(err)
	}
	auth := rest.AuthContext{Token: *logged.User.Token, ClientID: login.ClientID, BaseURL: login.BaseURL, RRiot: logged.User.Rriot}
	home, err := c.HomeDetail(ctx, rest.HomeDetailRequest{Auth: auth})
	if err != nil || home.Home.RrHomeId != 42 {
		t.Fatalf("home: %v", err)
	}
	for version := 1; version <= 3; version++ {
		inventory, inventoryErr := c.HomeData(ctx, rest.HomeDataRequest{Auth: auth, HomeID: 42, Version: version})
		if inventoryErr != nil {
			t.Fatal(inventoryErr)
		}
		devices := *inventory.Home.Devices
		if devices[0].Online == nil || *devices[0].Online {
			t.Fatal("false presence lost")
		}
		if len(devices[0].AdditionalProperties["futureField"]) == 0 {
			t.Fatal("unknown field lost")
		}
		if (*inventory.Home.Products)[0].Category != "future-category" {
			t.Fatal("unknown category lost")
		}
	}
	if doer.index != len(doer.exchanges) {
		t.Fatalf("unconsumed exchanges: %d", len(doer.exchanges)-doer.index)
	}
}
