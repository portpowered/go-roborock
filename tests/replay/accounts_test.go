// Package replay_test verifies complete public workflows at the HTTP boundary.
package replay_test

import (
	"context"
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

type exchange struct {
	Method   string
	Origin   string
	Path     string
	Form     url.Values
	Query    url.Values
	Headers  map[string]string
	Response json.RawMessage
}

type fixture struct {
	Provenance string
	Exchanges  []exchange
}

type pairedHTTP struct {
	t        *testing.T
	expected []exchange
	index    int
	nonce    string
}

func (p *pairedHTTP) Do(r *http.Request) (*http.Response, error) {
	p.t.Helper()
	if p.index >= len(p.expected) {
		p.t.Fatal("unexpected HTTP exchange")
	}
	e := p.expected[p.index]
	p.index++
	if r.Method != e.Method || r.URL.Scheme+"://"+r.URL.Host != e.Origin || r.URL.EscapedPath() != e.Path {
		p.t.Fatalf("exchange %d: method/origin/path mismatch", p.index)
	}
	query := r.URL.Query()
	if e.Path == "/api/v3/key/sign" {
		p.nonce = query.Get("s")
		if !regexp.MustCompile(`^[a-zA-Z0-9]{16}$`).MatchString(p.nonce) {
			p.t.Fatal("invalid mercy nonce format")
		}
		query.Set("s", "<nonce>")
	}
	if e.Query == nil {
		e.Query = url.Values{}
	}
	if !reflect.DeepEqual(query, e.Query) {
		p.t.Fatalf("exchange %d: query mismatch", p.index)
	}
	for key, value := range e.Headers {
		if value == "<nonce>" {
			value = p.nonce
		}
		if key == "Authorization" && value == "<hawk>" {
			if !strings.HasPrefix(r.Header.Get(key), `Hawk id="synthetic-user",s="synthetic-secret",ts="`) {
				p.t.Fatal("Hawk account mismatch")
			}
			continue
		}
		if r.Header.Get(key) != value {
			p.t.Fatalf("exchange %d: header %s mismatch", p.index, key)
		}
	}
	var body []byte
	if r.Body != nil {
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			p.t.Fatal(err)
		}
	}
	if e.Form != nil {
		actual, err := url.ParseQuery(string(body))
		if err != nil || !reflect.DeepEqual(actual, e.Form) {
			p.t.Fatalf("exchange %d: form mismatch", p.index)
		}
	} else if len(body) != 0 {
		p.t.Fatal("unexpected body")
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(e.Response)))}, nil
}

func TestAccountWorkflow(t *testing.T) {
	b, err := os.ReadFile("fixtures/rest/synthetic/sdk-account.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err = json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	if f.Provenance != "synthetic" {
		t.Fatal("unlabelled fixture")
	}
	doer := &pairedHTTP{t: t, expected: f.Exchanges}
	c, err := roborock.NewClient(roborock.WithBaseURL("https://iot.example.test"), roborock.WithHTTPClient(doer))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	login, err := c.ResolveLogin(ctx, roborock.ResolveLoginRequest{Email: "one@example.test", ClientID: "sdk-client"})
	if err != nil || login.Country != "GB" || login.CountryCode != "44" {
		t.Fatalf("resolve login: %v", err)
	}
	accepted, err := c.RequestLoginCode(ctx, roborock.LoginCodeRequest{Login: login})
	if err != nil || !accepted.Accepted {
		t.Fatalf("code: %v", err)
	}
	result, err := c.LoginWithCode(ctx, roborock.LoginWithCodeRequest{Login: login, Code: "001234"})
	if err != nil || result.UserID != 123 || result.Auth.BaseURL != "https://iot.example.test" {
		t.Fatalf("login: %v", err)
	}
	devices, err := c.ListDevices(ctx, roborock.AccountRequest{Auth: result.Auth})
	if err != nil {
		t.Fatal(err)
	}
	if len(devices.Devices) != 3 || devices.Devices[0].Model != "roborock.vacuum.synthetic" || devices.Devices[1].Protocol != "FutureProtocol" || !devices.Devices[2].Shared || devices.Devices[0].RoomID == nil {
		t.Fatalf("discovery projection: %+v", devices)
	}
	if doer.index != len(doer.expected) {
		t.Fatal("unconsumed exchanges")
	}
}
