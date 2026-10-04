package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5" // Independent oracle for the vendor-required Hawk path digest.
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"github.com/portpowered/go-roborock/pkg/roborock"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

type inventoryHTTP struct{ calls int }

func (transport *inventoryHTTP) Do(request *http.Request) (*http.Response, error) {
	transport.calls++

	if request.Method != http.MethodGet || request.URL.Scheme != "https" || request.URL.Host != "example.invalid" || request.URL.RawQuery != "" || request.Header.Get("Accept") != fixtureContentType {
		return nil, errors.New("inventory request mismatch")
	}

	var response string

	switch transport.calls {
	case 1:
		if request.URL.EscapedPath() != "/api/v1/getHomeDetail" || request.Header.Get("Authorization") != "synthetic-token" || request.Header.Get("Header_clientid") != fixtureClientID {
			return nil, errors.New("home request mismatch")
		}

		response = `{"code":200,"data":{"rrHomeId":42,"name":"Home"}}`
	case 2:
		if request.URL.EscapedPath() != "/user/homes/42" {
			return nil, errors.New("inventory path mismatch")
		}

		err := verifyHawk(request.Header.Get("Authorization"))
		if err != nil {
			return nil, err
		}

		response = `{"success":true,"result":{"id":42,"name":"Home","devices":[{"duid":"synthetic-device","name":"Vacuum","localKey":"synthetic-local-secret","productId":"synthetic-product","pv":"1.0"}],"products":[{"id":"synthetic-product","name":"Vacuum","model":"roborock.vacuum.synthetic","category":"robot.vacuum.cleaner"}]}}`
	default:
		return nil, errors.New("unexpected inventory exchange")
	}

	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {fixtureContentType}}, Body: io.NopCloser(strings.NewReader(response))}, nil
}

func verifyHawk(header string) error {
	pattern := regexp.MustCompile(`^Hawk id="synthetic-user",s="synthetic-secret",ts="([0-9]+)",nonce="([A-Za-z0-9_-]+)",mac="([A-Za-z0-9+/=]+)"$`)

	parts := pattern.FindStringSubmatch(header)
	if len(parts) != 4 {
		return errors.New("Hawk fields mismatch")
	}

	_, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return errors.New("Hawk timestamp mismatch")
	}

	nonce, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(nonce) != 6 {
		return errors.New("Hawk nonce format mismatch")
	}

	digest := md5.Sum([]byte("/user/homes/42"))
	input := strings.Join([]string{"synthetic-user", "synthetic-secret", parts[2], parts[1], hex.EncodeToString(digest[:]), "", ""}, ":")
	mac := hmac.New(sha256.New, []byte("synthetic-signing"))

	_, _ = mac.Write([]byte(input))

	if parts[3] != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
		return errors.New("Hawk signature mismatch")
	}

	return nil
}

func TestDeviceDiscoveryExportIsExplicit(t *testing.T) {
	t.Parallel()

	for _, export := range []bool{false, true} {
		transport := &inventoryHTTP{}

		client, err := roborock.NewClient(roborock.WithHTTPClient(transport))
		if err != nil {
			t.Fatal(err)
		}

		path := filepath.Join(t.TempDir(), "devices.json")

		arguments := []string{"devices"}

		if export {
			arguments = append(arguments, "--export", path)
		}

		input := `{"auth":{"baseUrl":"https://example.invalid","clientId":"synthetic-id","token":"synthetic-token","mqtt":{"apiUrl":"https://example.invalid","user":"synthetic-user","secret":"synthetic-secret","signingKey":"synthetic-signing"}}}`

		var stdout, stderr bytes.Buffer

		err = run(context.Background(), arguments, strings.NewReader(input), &stdout, &stderr, noEnvironment, client)
		if err != nil {
			t.Fatal(err)
		}

		if transport.calls != 2 || strings.Contains(stdout.String()+stderr.String(), "synthetic-local-secret") {
			t.Fatal("discovery exchanges missing or local secret leaked")
		}

		data, err := os.ReadFile(path) //nolint:gosec // G304: fixture export is within t.TempDir.
		if export && (err != nil || !bytes.Contains(data, []byte("synthetic-local-secret"))) {
			t.Fatal("explicit discovery export lacks local key")
		}

		if !export && !errors.Is(err, os.ErrNotExist) {
			t.Fatal("discovery implicitly exported credentials")
		}
	}
}
