package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/go-roborock/pkg/roborock"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// customerFixture injects account operations and delegates device sessions to paired MQTT.
// Embedding the interface makes every unexpected account call fail instead of returning fallback data.
type customerFixture struct {
	roborock.ClientAPI

	t       *testing.T
	login   roborock.LoginContext
	auth    roborock.AuthContext
	devices []roborock.Device
	steps   []string
}

func (fixture *customerFixture) ResolveLogin(ctx context.Context, request roborock.ResolveLoginRequest) (roborock.LoginContext, error) {
	fixture.t.Helper()

	if ctx.Err() != nil || request.Email != fixtureAccountEmail || len(request.ClientID) != 32 {
		fixture.t.Fatal("login email or generated identity mismatch")
	}

	fixture.steps = append(fixture.steps, "resolve")

	fixture.login = roborock.LoginContext{Email: request.Email, ClientID: request.ClientID, Country: "us", CountryCode: "1", BaseURL: nil}

	return fixture.login, nil
}

func (fixture *customerFixture) RequestLoginCode(ctx context.Context, request roborock.LoginCodeRequest) (roborock.LoginCodeResult, error) {
	fixture.t.Helper()

	if ctx.Err() != nil || !reflect.DeepEqual(request.Login, fixture.login) {
		fixture.t.Fatal("code request must reuse discovered login")
	}

	fixture.steps = append(fixture.steps, "request")

	return roborock.LoginCodeResult{Accepted: true}, nil
}

func (fixture *customerFixture) LoginWithCode(ctx context.Context, request roborock.LoginWithCodeRequest) (roborock.LoginResult, error) {
	fixture.t.Helper()

	if ctx.Err() != nil || !reflect.DeepEqual(request.Login, fixture.login) || request.Code != fixtureCode {
		fixture.t.Fatal("code exchange mismatch")
	}

	fixture.steps = append(fixture.steps, "exchange")

	fixture.auth = roborock.AuthContext{ClientID: fixture.login.ClientID, Token: fixtureToken, BaseURL: fixtureBaseURL, Mqtt: roborock.MQTTAuth{BrokerURL: "ssl://example.invalid:8883", User: fixtureCustomerMQTTUser, Secret: fixtureSecret, Key: "synthetic-key", SigningKey: fixtureSigningKey}}

	return roborock.LoginResult{Auth: fixture.auth, UserID: 42, Nickname: "Customer"}, nil
}

func (fixture *customerFixture) ListDevices(ctx context.Context, request roborock.AccountRequest) (roborock.ListDevicesResult, error) {
	fixture.t.Helper()

	if ctx.Err() != nil || !reflect.DeepEqual(request.Auth, fixture.auth) {
		fixture.t.Fatal("inventory must load saved account")
	}

	fixture.steps = append(fixture.steps, commandDevices)

	return roborock.ListDevicesResult{Devices: fixture.devices}, nil
}

func TestCustomerLoginInventoryAndStatus(t *testing.T) {
	t.Parallel()

	client, done := rpcTestClient(t, func(connection net.Conn) error {
		return replyRPC(connection, "get_status", `[]`, `{"battery":85}`, nil)
	})

	fixture := &customerFixture{t: t, ClientAPI: client, devices: []roborock.Device{{ID: fixtureDeviceID, Name: "Vacuum", Model: "roborock.vacuum.synthetic", LocalKey: fixtureLocalKey, Protocol: roborock.ProtocolV1}}}

	profile := filepath.Join(t.TempDir(), "private", "profile.json")

	var stdout, stderr bytes.Buffer

	commands := [][]string{{commandLogin, profileFlag, profile}, {commandDevices, fixtureList, profileFlag, profile}, {commandDevices, fixtureVacuum, fixtureDeviceID, "status", profileFlag, profile}}

	for _, args := range commands {
		err := run(context.Background(), args, strings.NewReader(fixtureAccountEmail+"\nsynthetic-code\n"), &stdout, &stderr, noEnvironment, fixture)
		if err != nil {
			t.Fatalf("customer command failed: %+v", errors.Unwrap(err))
		}
	}

	if !reflect.DeepEqual(fixture.steps, []string{"resolve", "request", "exchange", commandDevices, commandDevices}) {
		t.Fatal("missing, reordered, or repeated account exchange")
	}

	if !strings.Contains(stdout.String(), fixtureDeviceID) || !strings.Contains(stdout.String(), `"battery":85`) {
		t.Fatal("missing discovery or status result")
	}

	for _, secret := range []string{fixtureCode, fixtureToken, fixtureSecret, fixtureLocalKey, fixtureSigningKey} {
		if strings.Contains(stdout.String()+stderr.String(), secret) {
			t.Fatal("customer output disclosed a credential")
		}
	}

	err := <-done
	if err != nil {
		t.Fatal(err)
	}

	auth, err := readProfile(profile)

	if err != nil || !reflect.DeepEqual(auth, fixture.auth) {
		t.Fatal("saved profile differs from exchanged auth")
	}

	err = run(context.Background(), []string{"logout", profileFlag, profile}, strings.NewReader(""), &stdout, &stderr, noEnvironment, fixture)
	if err != nil {
		t.Fatal(err)
	}

	_, err = os.Stat(profile)

	if !errors.Is(err, os.ErrNotExist) {
		t.Fatal("logout left credentials on disk")
	}
}

func TestCustomerRejectsAmbiguousSelectionAndBadArguments(t *testing.T) {
	t.Parallel()

	fixture := &customerFixture{t: t, devices: []roborock.Device{{ID: fixtureDeviceOne}, {ID: "two"}}}

	profile := filepath.Join(t.TempDir(), "private", "profile.json")

	var stdout, stderr bytes.Buffer

	err := run(context.Background(), []string{commandLogin, profileFlag, profile}, strings.NewReader(fixtureAccountEmail+"\nsynthetic-code\n"), &stdout, &stderr, noEnvironment, fixture)
	if err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{{commandMaps, "show"}, {commandRooms, fixtureList}, {fixtureZones, fixtureClean, zoneFlag, "0,0,10,10"}, {commandDevices, fixtureVacuum, "missing", commandStart}} {
		err = run(context.Background(), append(args, profileFlag, profile), strings.NewReader(""), &stdout, &stderr, noEnvironment, fixture)

		if !errors.Is(err, errAmbiguousDevice) && !errors.Is(err, errMissingDevice) {
			t.Fatal("ambiguous or missing device opened a session")
		}
	}

	for _, args := range [][]string{{commandLogin, zoneFlag, "0,0,1,1"}, {commandDevices, fixtureList, deviceFlag, fixtureDeviceOne}, {commandDevices, fixtureVacuum, fixtureDeviceOne, commandStart, deviceFlag, "two"}, {commandRooms, fixtureClean, "1", "--map", "2"}, {fixtureZones, fixtureClean, zoneFlag, "1,0,0,1"}, {commandMaps, fixtureSelect}, {commandMaps, fixtureSelect, "1", "--device="}, {commandMaps, fixtureSelect, ""}, {commandDevices, fixtureVacuum, "", commandStart}, {commandRooms, fixtureClean, "-1"}} {
		count := len(fixture.steps)

		err = run(context.Background(), append(args, profileFlag, profile), strings.NewReader(""), &stdout, &stderr, noEnvironment, fixture)

		if err == nil || len(fixture.steps) != count {
			t.Fatal("invalid input reached account or device network work")
		}
	}
}

func TestCustomerBlockedPromptCancellationAndSecretErrors(t *testing.T) {
	t.Parallel()

	reader, writer := io.Pipe()

	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })

	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	done := make(chan error, 1)

	go func() { done <- run(ctx, []string{commandLogin}, reader, io.Discard, io.Discard, noEnvironment, nil) }()

	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled blocked login succeeded")
		}

	case <-time.After(time.Second):
		t.Fatal("cancellation did not release input reader")
	}

	secret := errors.New("https://example.invalid/login?token=PRIVATE&code=PRIVATE&password=PRIVATE")

	err := safeCustomerError(commandLogin, secret)

	if strings.Contains(err.Error(), "PRIVATE") || !errors.Is(err, secret) {
		t.Fatal("failure disclosed secrets or lost its cause")
	}

	unauthorized := roborockerrors.New(roborockerrors.Unauthorized, commandLogin, "PRIVATE", secret)

	err = safeCustomerError(commandLogin, unauthorized)

	if strings.Contains(err.Error(), "PRIVATE") || !strings.Contains(err.Error(), "login again") {
		t.Fatal("unsafe or unactionable account failure")
	}
}

func TestCustomerCodePromptDoesNotUseNetworkDeadline(t *testing.T) {
	t.Parallel()

	reader, writer := io.Pipe()

	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })

	fixture := &customerFixture{t: t}

	profile := filepath.Join(t.TempDir(), "private", "profile.json")

	done := make(chan error, 1)

	go func() {
		_, err := io.WriteString(writer, fixtureAccountEmail+"\n")
		if err == nil {
			time.Sleep(100 * time.Millisecond)

			_, err = io.WriteString(writer, fixtureCode+"\n")
		}

		done <- err
	}()

	err := run(context.Background(), []string{commandLogin, profileFlag, profile, "--timeout", "20ms"}, reader,
		io.Discard, io.Discard, noEnvironment, fixture)
	if err != nil {
		t.Fatal(err)
	}

	err = <-done
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(fixture.steps, []string{"resolve", "request", "exchange"}) {
		t.Fatal("login workflow changed while waiting for email")
	}
}

const (
	fixtureSigningKey   = "synthetic-signing"
	fixtureDeviceID     = "synthetic-device"
	deviceFlag          = "--device"
	fixtureZones        = "zones"
	fixtureBaseURL      = "https://example.invalid"
	fixtureSelect       = "select"
	fixtureInputFlag    = "--input"
	fixtureVacuum       = "vacuum"
	fixtureList         = "list"
	fixtureClean        = "clean"
	fixtureToken        = "synthetic-token"
	fixtureSecret       = "synthetic-secret"
	fixtureCode         = "synthetic-code"
	fixtureLocalKey     = "0123456789abcdef"
	profileFlag         = "--profile"
	zoneFlag            = "--zone"
	fixtureDeviceOne    = "one"
	fixtureFalse        = "false"
	fixtureContentType  = "application/json"
	fixtureClientID     = "synthetic-id"
	fixtureAccountEmail = "customer@example.invalid"
	fixtureAcknowledged = `"acknowledged":true`
)

// pairedHTTP exercises the real public client with synthetic paired exchanges.
type pairedHTTP struct {
	t        *testing.T
	path     string
	query    url.Values
	form     url.Values
	clientID string
	response string
	status   int
	calls    int
}

func (transport *pairedHTTP) Do(request *http.Request) (*http.Response, error) {
	transport.t.Helper()

	transport.calls++

	if transport.calls != 1 {
		return nil, errors.New("unexpected duplicate HTTP exchange")
	}

	if request.Method != http.MethodPost || request.URL.Scheme != "https" || request.URL.Host != "example.invalid" || request.URL.EscapedPath() != transport.path || !reflect.DeepEqual(request.URL.Query(), transport.query) {
		transport.t.Error("outbound method, origin, path, or query mismatch")

		return nil, errors.New("request mismatch")
	}

	if request.Header.Get("Accept") != fixtureContentType || request.Header.Get("Header_clientid") != transport.clientID {
		return nil, errors.New("request header mismatch")
	}

	if transport.form != nil {
		if request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			return nil, errors.New("content type mismatch")
		}

		err := request.ParseForm()
		if err != nil {
			return nil, err
		}

		if !reflect.DeepEqual(request.PostForm, transport.form) {
			return nil, errors.New("request form mismatch")
		}
	}

	return &http.Response{StatusCode: transport.status, Header: http.Header{"Content-Type": {fixtureContentType}}, Body: io.NopCloser(strings.NewReader(transport.response))}, nil
}

func newTestClient(t *testing.T, transport *pairedHTTP) *roborock.Client {
	t.Helper()

	client, err := roborock.NewClient(roborock.WithBaseURL(fixtureBaseURL), roborock.WithHTTPClient(transport))
	if err != nil {
		t.Fatal(err)
	}

	return client
}

func noEnvironment(string) string { return "" }

func TestHelpAndInvalidInputs(t *testing.T) {
	t.Parallel()

	for _, arguments := range [][]string{{"help"}, {"--help"}, {commandStatus, "--help"}} {
		var stdout, stderr bytes.Buffer

		err := run(context.Background(), arguments, strings.NewReader(""), &stdout, &stderr, noEnvironment, nil)
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(stdout.String()+stderr.String(), "Usage:") {
			t.Fatal("missing help")
		}
	}

	for _, arguments := range [][]string{{"unknown"}, {commandStatus}, {commandHome, "secret-token"}, {commandHome, "--password", "secret"}, {commandHome, "--export", "out"}} {
		var stdout, stderr bytes.Buffer

		err := run(context.Background(), arguments, strings.NewReader("{}"), &stdout, &stderr, noEnvironment, nil)
		if err == nil {
			t.Fatalf("accepted invalid arguments: %v", arguments)
		}
	}
}

func TestResolveLoginPairedHTTP(t *testing.T) {
	t.Parallel()

	transport := &pairedHTTP{t: t, path: "/api/v1/getUrlByEmail", query: url.Values{"email": {fixtureAccountEmail}, "needtwostepauth": {fixtureFalse}}, status: http.StatusOK, response: `{"code":200,"data":{"url":"https://example.invalid","country":"US","countrycode":"1"}}`}

	var stdout, stderr bytes.Buffer

	err := run(context.Background(), []string{"resolve-login"}, strings.NewReader(`{"email":"customer@example.invalid","clientId":"synthetic-id"}`), &stdout, &stderr, noEnvironment, newTestClient(t, transport))
	if err != nil {
		t.Fatal(err)
	}

	if transport.calls != 1 || !strings.Contains(stdout.String(), fixtureClientID) {
		t.Fatal("missing exchange or login identity")
	}
}

func TestLoginPasswordExportAndRedaction(t *testing.T) {
	t.Parallel()

	transport := &pairedHTTP{t: t, path: "/api/v1/login", query: url.Values{"username": {fixtureAccountEmail}, "password": {"synthetic-password"}, "needtwostepauth": {fixtureFalse}}, clientID: fixtureClientID, status: http.StatusOK, response: `{"code":200,"data":{"uid":42,"nickname":"Customer","token":"synthetic-token","rriot":{"u":"synthetic-user","s":"synthetic-secret","h":"synthetic-signing","k":"synthetic-key","r":{"a":"https://example.invalid","m":"ssl://example.invalid:8883"}}}}`}

	exportPath := filepath.Join(t.TempDir(), "credentials.json")

	var stdout, stderr bytes.Buffer

	err := run(context.Background(), []string{"login-password", "--export", exportPath}, strings.NewReader(`{"login":{"email":"customer@example.invalid","clientId":"synthetic-id","baseUrl":"https://example.invalid"},"password":"synthetic-password"}`), &stdout, &stderr, noEnvironment, newTestClient(t, transport))
	if err != nil {
		t.Fatal(err)
	}

	if transport.calls != 1 || strings.Contains(stdout.String()+stderr.String(), "synthetic-") {
		t.Fatal("missing exchange or leaked credentials")
	}

	data, err := os.ReadFile(exportPath) //nolint:gosec // G304: fixture export is within t.TempDir.
	if err != nil {
		t.Fatal(err)
	}

	var exported CommandInput

	err = json.Unmarshal(data, &exported)
	if err != nil {
		t.Fatal(err)
	}

	if exported.Auth.Token != fixtureToken {
		t.Fatal("export lacks token")
	}

	info, err := os.Stat(exportPath)
	if err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatal("credential export is not private")
	}

	err = exportCredentials(exportPath, exported)
	if err == nil {
		t.Fatal("credential export overwrote existing file")
	}
}

func TestAuthenticationErrorPairedHTTP(t *testing.T) {
	t.Parallel()

	transport := &pairedHTTP{t: t, path: "/api/v4/email/code/send", query: url.Values{}, form: url.Values{"email": {fixtureAccountEmail}, "type": {commandLogin}, "platform": {""}}, clientID: fixtureClientID, status: http.StatusUnauthorized, response: `{"code":401}`}

	var stdout, stderr bytes.Buffer

	err := run(context.Background(), []string{"request-code"}, strings.NewReader(`{"login":{"email":"customer@example.invalid","clientId":"synthetic-id","baseUrl":"https://example.invalid"}}`), &stdout, &stderr, noEnvironment, newTestClient(t, transport))

	if err == nil || transport.calls != 1 || stdout.Len() != 0 {
		t.Fatal("authentication failure was not propagated")
	}
}

func TestInputSourcesAndStrictJSON(t *testing.T) {
	t.Parallel()

	input, err := readInput("-", strings.NewReader(""), func(string) string { return `{"code":"synthetic-code"}` })

	if err != nil || input.Code != fixtureCode {
		t.Fatal("environment input failed")
	}

	for _, body := range []string{"{} {}", `{"password":"secret","unexpected":1}`, strings.Repeat(" ", maxInputBytes+1)} {
		_, err = readInput("-", strings.NewReader(body), noEnvironment)
		if err == nil {
			t.Fatal("accepted malformed input")
		}
	}
}
