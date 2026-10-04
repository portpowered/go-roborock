//go:build integration

// Package integration_test exercises opt-in, read-only operations against real Roborock cloud endpoints.
package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/go-roborock/pkg/roborock"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const (
	authFileEnvironment = "ROBOROCK_INTEGRATION_AUTH_FILE"
	homeIDEnvironment   = "ROBOROCK_INTEGRATION_HOME_ID"
	maximumAuthBytes    = 64 << 10
	liveRequestTimeout  = 60 * time.Second
)

func TestLiveListDevices(t *testing.T) {
	t.Parallel()

	auth := liveAuth(t)
	client := liveClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), liveRequestTimeout)

	defer cancel()

	result, err := client.ListDevices(ctx, roborock.AccountRequest{Auth: auth})
	assertLiveSuccess(t, "ListDevices", err)
	assertLiveDevices(t, result.Devices)
}

func TestLiveGetHomeData(t *testing.T) {
	t.Parallel()

	auth := liveAuth(t)
	homeID := liveHomeID(t)
	client := liveClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), liveRequestTimeout)

	defer cancel()

	result, err := client.GetHomeData(ctx, roborock.HomeDataRequest{
		Auth: auth, HomeID: homeID, Version: roborock.HomeDataV1,
	})
	assertLiveSuccess(t, "GetHomeData", err)

	if result.Home.ID != homeID {
		t.Fatal("GetHomeData returned a different home; identifiers omitted")
	}

	assertLiveDevices(t, result.Devices)
}

func liveAuth(t *testing.T) roborock.AuthContext {
	t.Helper()

	path := os.Getenv(authFileEnvironment)
	if path == "" {
		t.Skip("live integration requires ROBOROCK_INTEGRATION_AUTH_FILE")
	}

	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		t.Fatal("cannot open private integration credentials; path and error text omitted")
	}

	t.Cleanup(func() {
		err = file.Close()
		if err != nil {
			t.Error("cannot close private integration credentials")
		}
	})

	data, err := io.ReadAll(io.LimitReader(file, maximumAuthBytes+1))
	if err != nil || len(data) > maximumAuthBytes {
		t.Fatal("cannot read integration credentials or credential file exceeds the size limit")
	}

	var auth roborock.AuthContext

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	err = decoder.Decode(&auth)
	if err != nil {
		t.Fatal("integration credentials must contain one public AuthContext JSON object; details omitted")
	}

	var trailing json.RawMessage

	err = decoder.Decode(&trailing)
	if !errors.Is(err, io.EOF) {
		t.Fatal("integration credential file contains trailing content")
	}

	assertLiveOrigin(t, auth.BaseURL)
	assertLiveOrigin(t, auth.Mqtt.APIURL)

	return auth
}

func assertLiveOrigin(t *testing.T, origin string) {
	t.Helper()

	parsed, err := url.Parse(origin)
	if err != nil {
		t.Fatal("integration credentials contain an invalid origin; value omitted")
	}

	if parsed.Scheme != "https" || !strings.HasSuffix(parsed.Hostname(), ".roborock.com") ||
		parsed.User != nil || parsed.Port() != "" || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		t.Fatal("live integration requires a Roborock-owned HTTPS origin without credentials, port, query, or path")
	}
}

func liveHomeID(t *testing.T) int64 {
	t.Helper()

	value := os.Getenv(homeIDEnvironment)
	if value == "" {
		t.Skip("GetHomeData live integration also requires ROBOROCK_INTEGRATION_HOME_ID")
	}

	homeID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || homeID <= 0 {
		t.Fatal("integration home identifier must be a positive integer; value omitted")
	}

	return homeID
}

func liveClient(t *testing.T) *roborock.Client {
	t.Helper()

	client, err := roborock.NewClient()
	assertLiveSuccess(t, "NewClient", err)

	return client
}

func assertLiveSuccess(t *testing.T, operation string, err error) {
	t.Helper()

	if err == nil {
		return
	}

	var classified *roborockerrors.Error

	if errors.As(err, &classified) {
		t.Fatalf("%s failed: kind=%s code=%d; private error text omitted", operation, classified.Kind, classified.Code)
	}

	t.Fatalf("%s failed with an unclassified error; private error text omitted", operation)
}

func assertLiveDevices(t *testing.T, devices []roborock.Device) {
	t.Helper()

	seen := make(map[string]bool, len(devices))

	for _, device := range devices {
		if device.ID == "" || seen[device.ID] {
			t.Fatal("live inventory contains a missing or repeated device identifier; values omitted")
		}

		seen[device.ID] = true
	}
}
