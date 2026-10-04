// Package replay_test verifies complete public workflows at the HTTP boundary.
package replay_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/portpowered/go-roborock/pkg/roborock"
)

const accountFixtureProvenance = "synthetic"

type exchange struct {
	Method   string            `json:"method"`
	Origin   string            `json:"origin"`
	Path     string            `json:"path"`
	Form     url.Values        `json:"form"`
	Query    url.Values        `json:"query"`
	Headers  map[string]string `json:"headers"`
	Response json.RawMessage   `json:"response"`
}

type fixture struct {
	Provenance string     `json:"provenance"`
	Exchanges  []exchange `json:"exchanges"`
}

type pairedHTTP struct {
	t        *testing.T
	expected []exchange
	index    int
	nonce    string
}

func (p *pairedHTTP) Do(request *http.Request) (*http.Response, error) {
	p.t.Helper()

	if p.index >= len(p.expected) {
		p.t.Fatal("unexpected HTTP exchange")
	}

	expected := p.expected[p.index]
	p.index++
	p.matchTarget(request, expected)
	p.matchHeaders(request, expected.Headers)
	p.matchBody(request, expected.Form)

	var response http.Response

	response.StatusCode = http.StatusOK
	response.Header = http.Header{"Content-Type": {"application/json"}}
	response.Body = io.NopCloser(strings.NewReader(string(expected.Response)))

	return &response, nil
}

func (p *pairedHTTP) matchTarget(request *http.Request, expected exchange) {
	p.t.Helper()

	origin := request.URL.Scheme + "://" + request.URL.Host

	if request.Method != expected.Method || origin != expected.Origin || request.URL.EscapedPath() != expected.Path {
		p.t.Fatalf("exchange %d: method/origin/path mismatch", p.index)
	}

	query := request.URL.Query()

	if expected.Path == "/api/v3/key/sign" {
		p.nonce = query.Get("s")

		if !regexp.MustCompile(`^[a-zA-Z0-9]{16}$`).MatchString(p.nonce) {
			p.t.Fatal("invalid mercy nonce format")
		}

		query.Set("s", "<nonce>")
	}

	if expected.Query == nil {
		expected.Query = url.Values{}
	}

	if !reflect.DeepEqual(query, expected.Query) {
		p.t.Fatalf("exchange %d: query mismatch", p.index)
	}
}

func (p *pairedHTTP) matchHeaders(request *http.Request, headers map[string]string) {
	p.t.Helper()

	for key, value := range headers {
		if value == "<nonce>" {
			value = p.nonce
		}

		if key == "Authorization" && value == "<hawk>" {
			pattern := regexp.MustCompile(`^Hawk id="synthetic-user",s="synthetic-secret",ts="[0-9]+",` +
				`nonce="[a-zA-Z0-9_-]{8}",mac="[a-zA-Z0-9+/]+=*"$`)
			if !pattern.MatchString(request.Header.Get(key)) {
				p.t.Fatal("Hawk account mismatch")
			}

			continue
		}

		if request.Header.Get(key) != value {
			p.t.Fatalf("exchange %d: header %s mismatch", p.index, key)
		}
	}
}

func (p *pairedHTTP) matchBody(request *http.Request, expected url.Values) {
	p.t.Helper()

	var body []byte

	if request.Body != nil {
		var err error

		body, err = io.ReadAll(request.Body)

		if err != nil {
			p.t.Fatal(err)
		}
	}

	if expected == nil {
		if len(body) != 0 {
			p.t.Fatal("unexpected body")
		}

		return
	}

	actual, err := url.ParseQuery(string(body))

	if err != nil || !reflect.DeepEqual(actual, expected) {
		p.t.Fatalf("exchange %d: form mismatch", p.index)
	}
}

func loadAccountFixture(t *testing.T) fixture {
	t.Helper()

	encoded, err := os.ReadFile("fixtures/rest/synthetic/sdk-account.json")

	if err != nil {
		t.Fatal(err)
	}

	var expected fixture

	err = json.Unmarshal(encoded, &expected)

	if err != nil {
		t.Fatal(err)
	}

	if expected.Provenance != accountFixtureProvenance {
		t.Fatal("unlabelled fixture")
	}

	return expected
}

func TestAccountWorkflow(t *testing.T) {
	t.Parallel()
	expected := loadAccountFixture(t)
	doer := &pairedHTTP{t: t, expected: expected.Exchanges, index: 0, nonce: ""}
	client, err := roborock.NewClient(
		roborock.WithBaseURL("https://iot.example.test"), roborock.WithHTTPClient(doer),
	)

	if err != nil {
		t.Fatal(err)
	}

	login, err := client.ResolveLogin(t.Context(), roborock.ResolveLoginRequest{
		Email: "one@example.test", ClientID: "sdk-client",
	})

	if err != nil {
		t.Fatal(err)
	}

	if login.Country != "GB" || login.CountryCode != "44" {
		t.Fatalf("resolve login: %+v", login)
	}

	auth := completeCodeLogin(t, client, login)
	devices, err := client.ListDevices(t.Context(), roborock.AccountRequest{Auth: auth})

	if err != nil {
		t.Fatal(err)
	}

	verifyDiscovery(t, devices)

	if doer.index != len(doer.expected) {
		t.Fatal("unconsumed exchanges")
	}
}

func completeCodeLogin(t *testing.T, client *roborock.Client, login roborock.LoginContext) roborock.AuthContext {
	t.Helper()
	accepted, err := client.RequestLoginCode(t.Context(), roborock.LoginCodeRequest{Login: login})

	if err != nil || !accepted.Accepted {
		t.Fatalf("code: %v", err)
	}

	result, err := client.LoginWithCode(t.Context(), roborock.LoginWithCodeRequest{Login: login, Code: "001234"})

	if err != nil || result.UserID != 123 || result.Auth.BaseURL != "https://iot.example.test" {
		t.Fatalf("login: %v", err)
	}

	return result.Auth
}

func verifyDiscovery(t *testing.T, devices roborock.ListDevicesResult) {
	t.Helper()

	if len(devices.Devices) != 3 {
		t.Fatalf("discovery count: %+v", devices)
	}

	if devices.Devices[0].Model != "roborock.vacuum.synthetic" || devices.Devices[0].RoomID == nil {
		t.Fatalf("owned device projection: %+v", devices.Devices[0])
	}

	verifyAdvertisedProperties(t, devices.Devices)

	if devices.Devices[1].Protocol != "FutureProtocol" || !devices.Devices[2].Shared {
		t.Fatalf("protocol and shared device projection: %+v", devices)
	}
}

func verifyAdvertisedProperties(t *testing.T, devices []roborock.Device) {
	t.Helper()

	if !reflect.DeepEqual(devices[0].SupportedProperties, []int{101, 102}) {
		t.Fatalf("advertised property projection: %v", devices[0].SupportedProperties)
	}

	if devices[0].Online == nil || !*devices[0].Online || devices[1].Online != nil {
		t.Fatal("online state lost presence information")
	}
}
