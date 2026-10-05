package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/portpowered/go-roborock/pkg/roborock"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const (
	customerAuthExchangeCount = 5
	fixtureCustomerMQTTUser   = "synthetic-user"
)

// customerAuthExchange reads only the authentication portion of the shared synthetic paired fixture.
type customerAuthExchange struct {
	Method          string            `json:"method"`
	Origin          string            `json:"origin"`
	Path            string            `json:"path"`
	Form            url.Values        `json:"form"`
	Query           url.Values        `json:"query"`
	Headers         map[string]string `json:"headers"`
	Response        json.RawMessage   `json:"response"`
	ResponseStatus  int               `json:"responseStatus"`
	ResponseHeaders http.Header       `json:"responseHeaders"`
}

type customerAuthTranscript struct {
	Provenance string                 `json:"provenance"`
	Exchanges  []customerAuthExchange `json:"exchanges"`
}

type customerAuthHTTP struct {
	t        *testing.T
	expected []customerAuthExchange
	index    int
	identity string
	nonce    string
}

func loadCustomerAuth(t *testing.T) []customerAuthExchange {
	t.Helper()

	data, err := os.ReadFile("../../tests/replay/fixtures/rest/synthetic/sdk-account.json")
	if err != nil {
		t.Fatal(err)
	}

	var transcript customerAuthTranscript

	err = json.Unmarshal(data, &transcript)
	if err != nil {
		t.Fatal(err)
	}

	if transcript.Provenance != "synthetic" || len(transcript.Exchanges) < customerAuthExchangeCount {
		t.Fatal("missing labeled authentication transcript")
	}

	return transcript.Exchanges[:customerAuthExchangeCount]
}

func (fixture *customerAuthHTTP) Do(request *http.Request) (*http.Response, error) {
	fixture.t.Helper()

	if fixture.index >= len(fixture.expected) {
		fixture.t.Fatal("unexpected or duplicate authentication exchange")
	}

	expected := fixture.expected[fixture.index]
	fixture.index++
	fixture.matchTarget(request, expected)
	fixture.matchHeaders(request, expected)
	fixture.matchBody(request, expected.Form)

	var response http.Response

	response.StatusCode = expected.ResponseStatus
	response.Header = expected.ResponseHeaders.Clone()
	response.Body = io.NopCloser(bytes.NewReader(expected.Response))

	return &response, nil
}

func (fixture *customerAuthHTTP) matchTarget(request *http.Request, expected customerAuthExchange) {
	fixture.t.Helper()

	if request.Context().Err() != nil || request.Method != expected.Method ||
		request.URL.Scheme+"://"+request.URL.Host != expected.Origin || request.URL.EscapedPath() != expected.Path {
		fixture.t.Fatal("authentication target or context mismatch")
	}

	query := request.URL.Query()
	if expected.Path == "/api/v3/key/sign" {
		fixture.nonce = query.Get("s")
		if !regexp.MustCompile(`^[a-zA-Z0-9]{16}$`).MatchString(fixture.nonce) {
			fixture.t.Fatal("invalid generated mercy nonce")
		}

		query.Set("s", "<nonce>")
	}

	wanted := expected.Query
	if wanted == nil {
		wanted = url.Values{}
	}

	if !reflect.DeepEqual(query, wanted) {
		fixture.t.Fatal("authentication query mismatch")
	}
}

func (fixture *customerAuthHTTP) matchHeaders(request *http.Request, expected customerAuthExchange) {
	fixture.t.Helper()

	for key, value := range expected.Headers {
		switch value {
		case "sdk-client":
			if fixture.identity == "" {
				fixture.identity = request.Header.Get(key)
				if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(fixture.identity) {
					fixture.t.Fatal("invalid generated customer client identity")
				}
			}

			value = fixture.identity
		case "<nonce>":
			value = fixture.nonce
		default:
		}

		if request.Header.Get(key) != value {
			fixture.t.Fatalf("authentication header %s mismatch", key)
		}
	}

	if expected.Form != nil && request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		fixture.t.Fatal("authentication form content type mismatch")
	}
}

func (fixture *customerAuthHTTP) matchBody(request *http.Request, expected url.Values) {
	fixture.t.Helper()

	var body []byte

	if request.Body != nil {
		var err error

		body, err = io.ReadAll(request.Body)
		if err != nil {
			fixture.t.Fatal(err)
		}
	}

	if expected == nil {
		if len(body) != 0 {
			fixture.t.Fatal("unexpected authentication body")
		}

		return
	}

	actual, err := url.ParseQuery(string(body))
	if err != nil || !reflect.DeepEqual(actual, expected) {
		fixture.t.Fatal("authentication form mismatch")
	}
}

func runCustomerAuth(t *testing.T, fixture *customerAuthHTTP, profile string) (string, error) {
	t.Helper()

	client, err := roborock.NewClient(roborock.WithBaseURL("https://iot.example.test"), roborock.WithHTTPClient(fixture))
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer

	err = run(t.Context(), []string{"login", "--profile", profile},
		strings.NewReader("one@example.test\n001234\n"), &stdout, &stderr, noEnvironment, client)

	output := stdout.String() + stderr.String()
	if err != nil {
		output += err.Error()
	}

	for _, secret := range []string{"001234", "synthetic-token", "synthetic-secret", "synthetic-mqtt-key",
		"synthetic-hawk-key", "synthetic-mercy-key", "one@example.test"} {
		if strings.Contains(output, secret) {
			t.Fatal("authentication output or error disclosed a secret")
		}
	}

	if fixture.index != len(fixture.expected) {
		t.Fatal("unconsumed authentication exchange")
	}

	return stdout.String(), err
}

func TestCustomerLoginCompleteHTTPExchange(t *testing.T) {
	t.Parallel()

	fixture := &customerAuthHTTP{t: t, expected: loadCustomerAuth(t), index: 0, identity: "", nonce: ""}
	profile := filepath.Join(t.TempDir(), "private", "profile.json")

	output, err := runCustomerAuth(t, fixture, profile)
	if err != nil || !strings.Contains(output, "Logged in.") {
		t.Fatal("complete customer login failed")
	}

	auth, err := readProfile(profile)
	if err != nil {
		t.Fatal(err)
	}

	wanted := roborock.AuthContext{ClientID: fixture.identity, Token: "synthetic-token",
		BaseURL: "https://iot.example.test", Mqtt: roborock.MQTTAuth{
			User: fixtureCustomerMQTTUser, Secret: "synthetic-secret", Key: "synthetic-mqtt-key",
			SigningKey: "synthetic-hawk-key", APIURL: "https://api.example.test",
			BrokerURL: "ssl://broker.example.test:8883",
		}}
	if !reflect.DeepEqual(auth, wanted) {
		t.Fatal("saved login did not preserve generated identity and account credentials")
	}
}

func TestCustomerLoginHTTPFailurePreservesProfile(t *testing.T) {
	t.Parallel()

	for failed := range customerAuthExchangeCount {
		for _, existing := range []bool{false, true} {
			t.Run(customerAuthFailureName(failed, existing), func(t *testing.T) {
				t.Parallel()

				profile := filepath.Join(t.TempDir(), "private", "profile.json")
				before := prepareCustomerAuthProfile(t, profile, existing)
				exchanges := loadCustomerAuth(t)[:failed+1]
				exchanges[failed].ResponseStatus = http.StatusUnauthorized
				exchanges[failed].Response = json.RawMessage(`{"code":401,"msg":"synthetic-token synthetic-secret 001234"}`)
				fixture := &customerAuthHTTP{t: t, expected: exchanges, index: 0, identity: "", nonce: ""}

				output, err := runCustomerAuth(t, fixture, profile)
				if !errors.Is(err, roborockerrors.New(roborockerrors.Unauthorized, "", "", nil)) || output != "" {
					t.Fatal("authentication failure was not propagated safely")
				}

				assertCustomerAuthProfileUnchanged(t, profile, before, existing)
			})
		}
	}
}

func customerAuthFailureName(index int, existing bool) string {
	names := []string{"region", "request-code", "signature", "agreement", "auth-exchange"}
	if existing {
		return names[index] + "/existing-profile"
	}

	return names[index] + "/new-profile"
}

func prepareCustomerAuthProfile(t *testing.T, profile string, existing bool) []byte {
	t.Helper()

	if !existing {
		return nil
	}

	auth := roborock.AuthContext{ClientID: "previous-client", Token: "previous-token",
		BaseURL: "https://iot.example.test", Mqtt: roborock.MQTTAuth{
			BrokerURL: "ssl://broker.example.test:8883", User: "previous-user", Secret: "previous-secret",
			Key: "previous-key", SigningKey: "previous-signing", APIURL: "https://api.example.test",
		}}

	err := saveProfile(profile, auth)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(profile) //nolint:gosec // Test-owned private profile under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func assertCustomerAuthProfileUnchanged(t *testing.T, profile string, before []byte, existing bool) {
	t.Helper()

	data, err := os.ReadFile(profile) //nolint:gosec // Test-owned private profile under t.TempDir.
	if !existing {
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal("failed authentication created a profile")
		}

		return
	}

	if err != nil || !bytes.Equal(data, before) {
		t.Fatal("failed authentication changed the previous private profile")
	}

	_, err = readProfile(profile)
	if err != nil {
		t.Fatal("previous private profile is no longer usable")
	}
}
