package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/go-roborock/pkg/roborock"
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

	if request.Header.Get("Accept") != "application/json" || request.Header.Get("Header_clientid") != transport.clientID {
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

	return &http.Response{StatusCode: transport.status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(transport.response))}, nil
}

func newTestClient(t *testing.T, transport *pairedHTTP) *roborock.Client {
	t.Helper()

	client, err := roborock.NewClient(roborock.WithBaseURL("https://example.invalid"), roborock.WithHTTPClient(transport))
	if err != nil {
		t.Fatal(err)
	}

	return client
}

func noEnvironment(string) string { return "" }

func TestHelpAndInvalidInputs(t *testing.T) {
	t.Parallel()

	for _, arguments := range [][]string{{"help"}, {"--help"}, {"status", "--help"}} {
		var stdout, stderr bytes.Buffer

		err := run(context.Background(), arguments, strings.NewReader(""), &stdout, &stderr, noEnvironment, nil)
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(stdout.String()+stderr.String(), "Usage:") {
			t.Fatal("missing help")
		}
	}

	for _, arguments := range [][]string{{"unknown"}, {"status"}, {"home", "secret-token"}, {"home", "--password", "secret"}, {"home", "--export", "out"}} {
		var stdout, stderr bytes.Buffer

		err := run(context.Background(), arguments, strings.NewReader("{}"), &stdout, &stderr, noEnvironment, nil)
		if err == nil {
			t.Fatalf("accepted invalid arguments: %v", arguments)
		}
	}
}

func TestResolveLoginPairedHTTP(t *testing.T) {
	t.Parallel()
	transport := &pairedHTTP{t: t, path: "/api/v1/getUrlByEmail", query: url.Values{"email": {"customer@example.invalid"}, "needtwostepauth": {"false"}}, status: http.StatusOK, response: `{"code":200,"data":{"url":"https://example.invalid","country":"US","countrycode":"1"}}`}

	var stdout, stderr bytes.Buffer

	err := run(context.Background(), []string{"resolve-login"}, strings.NewReader(`{"email":"customer@example.invalid","clientId":"synthetic-id"}`), &stdout, &stderr, noEnvironment, newTestClient(t, transport))
	if err != nil {
		t.Fatal(err)
	}

	if transport.calls != 1 || !strings.Contains(stdout.String(), "synthetic-id") {
		t.Fatal("missing exchange or login identity")
	}
}

func TestLoginPasswordExportAndRedaction(t *testing.T) {
	t.Parallel()
	transport := &pairedHTTP{t: t, path: "/api/v1/login", query: url.Values{"username": {"customer@example.invalid"}, "password": {"synthetic-password"}, "needtwostepauth": {"false"}}, clientID: "synthetic-id", status: http.StatusOK, response: `{"code":200,"data":{"uid":42,"nickname":"Customer","token":"synthetic-token","rriot":{"u":"synthetic-user","s":"synthetic-secret","h":"synthetic-signing","k":"synthetic-key","r":{"a":"https://example.invalid","m":"ssl://example.invalid:8883"}}}}`}
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
	if err := json.Unmarshal(data, &exported); err != nil {
		t.Fatal(err)
	}

	if exported.Auth.Token != "synthetic-token" {
		t.Fatal("export lacks token")
	}

	info, err := os.Stat(exportPath)
	if err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatal("credential export is not private")
	}

	if err := exportCredentials(exportPath, exported); err == nil {
		t.Fatal("credential export overwrote existing file")
	}
}

func TestAuthenticationErrorPairedHTTP(t *testing.T) {
	t.Parallel()
	transport := &pairedHTTP{t: t, path: "/api/v4/email/code/send", query: url.Values{}, form: url.Values{"email": {"customer@example.invalid"}, "type": {"login"}, "platform": {""}}, clientID: "synthetic-id", status: http.StatusUnauthorized, response: `{"code":401}`}

	var stdout, stderr bytes.Buffer

	err := run(context.Background(), []string{"request-code"}, strings.NewReader(`{"login":{"email":"customer@example.invalid","clientId":"synthetic-id","baseUrl":"https://example.invalid"}}`), &stdout, &stderr, noEnvironment, newTestClient(t, transport))
	if err == nil || transport.calls != 1 || stdout.Len() != 0 {
		t.Fatal("authentication failure was not propagated")
	}
}

func TestInputSourcesAndStrictJSON(t *testing.T) {
	t.Parallel()

	input, err := readInput("-", strings.NewReader(""), func(string) string { return `{"code":"synthetic-code"}` })
	if err != nil || input.Code != "synthetic-code" {
		t.Fatal("environment input failed")
	}

	for _, body := range []string{"{} {}", `{"password":"secret","unexpected":1}`, strings.Repeat(" ", maxInputBytes+1)} {
		if _, err := readInput("-", strings.NewReader(body), noEnvironment); err == nil {
			t.Fatal("accepted malformed input")
		}
	}
}
