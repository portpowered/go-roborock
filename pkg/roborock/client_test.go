package roborock_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/portpowered/go-roborock/pkg/roborock"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

type failedHTTP struct{}

func (failedHTTP) Do(r *http.Request) (*http.Response, error) {
	return nil, roborockerrors.New(roborockerrors.Canceled, "synthetic", "request canceled", r.Context().Err())
}

func TestClientOptions(t *testing.T) {
	t.Parallel()

	cases := [][]roborock.Option{
		{nil}, {roborock.WithHTTPClient(nil)}, {roborock.WithMQTTDial(nil)},
		{roborock.WithBaseURL("ftp://example.test")},
		{roborock.WithBaseURL("https://user:password@example.test")},
		{roborock.WithBaseURL("https://example.test/path")},
	}

	for _, options := range cases {
		_, err := roborock.NewClient(options...)
		if !errors.Is(err, roborockerrors.New(roborockerrors.InvalidArgument, "", "", nil)) {
			t.Fatalf("option error: %v", err)
		}
	}

	_, err := roborock.NewClient()
	if err != nil {
		t.Fatal(err)
	}

	_, err = roborock.NewClient(
		roborock.WithBaseURL("https://example.test"), roborock.WithHTTPClient(failedHTTP{}),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAccountRequestsRejectMissingInputs(t *testing.T) {
	t.Parallel()

	client, err := roborock.NewClient(roborock.WithHTTPClient(failedHTTP{}))
	if err != nil {
		t.Fatal(err)
	}

	ctx := t.Context()

	var (
		emptyCode     roborock.LoginWithCodeRequest
		emptyPassword roborock.LoginWithPasswordRequest
		emptyAccount  roborock.AccountRequest
		emptyHomeData roborock.HomeDataRequest
	)

	_, err = client.ResolveLogin(ctx, roborock.ResolveLoginRequest{Email: "one@example.test",
		ClientID: "",
	})
	if err == nil {
		t.Fatal("missing identity accepted")
	}

	_, err = client.LoginWithCode(ctx, emptyCode)
	if err == nil {
		t.Fatal("empty login accepted")
	}

	_, err = client.LoginWithPassword(ctx, emptyPassword)
	if err == nil {
		t.Fatal("empty password login accepted")
	}

	_, err = client.GetHome(ctx, emptyAccount)
	if err == nil {
		t.Fatal("missing token accepted")
	}

	_, err = client.GetHomeData(ctx, emptyHomeData)
	if err == nil {
		t.Fatal("missing home accepted")
	}

	_, err = client.ListDevices(ctx, emptyAccount)
	if err == nil {
		t.Fatal("missing credentials accepted")
	}
}

func TestCanceledAccountRequestsPreserveCause(t *testing.T) {
	t.Parallel()

	client, err := roborock.NewClient(roborock.WithBaseURL("https://example.test"), roborock.WithHTTPClient(failedHTTP{}))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	login := roborock.LoginContext{Email: "one@example.test",
		ClientID:    "synthetic",
		Country:     "US",
		CountryCode: "1",
		BaseURL:     nil,
	}

	_, err = client.ResolveLogin(ctx, roborock.ResolveLoginRequest{Email: login.Email,
		ClientID: login.ClientID})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("resolve: %v", err)
	}

	_, err = client.RequestLoginCode(ctx, roborock.LoginCodeRequest{Login: login})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("code: %v", err)
	}

	_, err = client.LoginWithCode(ctx, roborock.LoginWithCodeRequest{Login: login,
		Code: "001234"})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("exchange: %v", err)
	}

	_, err = client.LoginWithPassword(ctx, roborock.LoginWithPasswordRequest{Login: login,
		Password: "synthetic"})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("password: %v", err)
	}
}

func TestUnsupportedDeviceProtocol(t *testing.T) {
	t.Parallel()

	client, err := roborock.NewClient()
	if err != nil {
		t.Fatal(err)
	}

	var request roborock.OpenDeviceRequest

	request.Protocol = roborock.ProtocolL01
	request.Auth.Mqtt.BrokerURL = "ssl://broker.example.test:8883"
	request.Auth.Mqtt.User = "synthetic-user"
	request.Auth.Mqtt.Secret = "synthetic-secret"
	request.Auth.Mqtt.Key = "synthetic-key"
	request.DeviceID = "synthetic-device"
	request.LocalKey = "0123456789abcdef"

	_, err = client.OpenDevice(t.Context(), request)
	if !errors.Is(err, roborockerrors.New(roborockerrors.Unsupported, "", "", nil)) {
		t.Fatalf("unsupported device protocol: %v", err)
	}
}

func TestOpenDeviceRequiresProtocolBeforeDial(t *testing.T) {
	t.Parallel()

	calls := 0

	client, err := roborock.NewClient(roborock.WithMQTTDial(
		func(_ context.Context, _, _ string) (net.Conn, error) {
			calls++

			return nil, roborockerrors.New(roborockerrors.Unavailable, "synthetic", "unexpected dial", nil)
		},
	))
	if err != nil {
		t.Fatal(err)
	}

	var request roborock.OpenDeviceRequest

	request.Auth.Mqtt.BrokerURL = "ssl://broker.example.test:8883"
	request.Auth.Mqtt.User = "synthetic-user"
	request.Auth.Mqtt.Secret = "synthetic-secret"
	request.Auth.Mqtt.Key = "synthetic-key"
	request.DeviceID = "synthetic-device"
	request.LocalKey = "0123456789abcdef"

	_, err = client.OpenDevice(t.Context(), request)
	if !errors.Is(err, roborockerrors.New(roborockerrors.InvalidArgument, "", "", nil)) || calls != 0 {
		t.Fatalf("empty protocol: error %v, dial calls %d", err, calls)
	}
}
