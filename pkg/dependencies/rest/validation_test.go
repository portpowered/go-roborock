package rest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const syntheticHomePath = "/v3/user/homes/42"

func TestRejectMissingRequiredFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path   string
		method string
		body   string
	}{
		{protocol.RESTPathGetAgreement, protocol.RESTMethodGetAgreement, `{"code":200,"data":{"majorVersion":19}}`},
		{protocol.RESTPathGetHomeDetail, protocol.RESTMethodGetHomeDetail, `{"code":200,"data":{"name":"Synthetic"}}`},
		{protocol.RESTPathLoginCode, protocol.RESTMethodLoginCode,
			`{"code":200,"data":{"rriot":{"u":"u","s":"s","h":"h","k":"k"}}}`},
		{syntheticHomePath, protocol.RESTMethodGetHomeDatav3, `{"success":true,"result":{"id":42,"devices":[]}}`},
	}
	for _, test := range cases {
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()

			err := validateResponse(test.method, test.path, []byte(test.body))
			if !errors.Is(err, roborockerrors.New(roborockerrors.Protocol, "", "", nil)) {
				t.Fatalf("accepted malformed required fields: %v", err)
			}
		})
	}
}

func TestAcceptNullOptionalAndUnknownFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path   string
		method string
		body   string
	}{
		{protocol.RESTPathRequestCode, protocol.RESTMethodRequestCode, `{"code":200,"msg":"success","data":null}`},
		{protocol.RESTPathResolveRegion, protocol.RESTMethodResolveRegion,
			`{"code":200,"msg":null,"data":{"url":"https://iot.example.test","country":null,"future":null}}`},
		{protocol.RESTPathGetAgreement, protocol.RESTMethodGetAgreement,
			`{"code":200,"data":{"majorVersion":19,"minorVersion":0,"future":[{"nested":null}]}}`},
		{syntheticHomePath, protocol.RESTMethodGetHomeDatav3,
			`{"success":true,"result":{"id":42,"name":"Synthetic","devices":[],"lon":null}}`},
	}
	for _, test := range cases {
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()

			err := validateResponse(test.method, test.path, []byte(test.body))
			if err != nil {
				t.Fatalf("rejected null field: %v", err)
			}
		})
	}
}

func TestRejectNullRequiredValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		path   string
		method string
		body   string
	}{
		{"required member", protocol.RESTPathGetAgreement, protocol.RESTMethodGetAgreement,
			`{"code":200,"data":{"majorVersion":19,"minorVersion":null}}`},
		{"array element", syntheticHomePath, protocol.RESTMethodGetHomeDatav3,
			`{"success":true,"result":{"id":42,"name":"Synthetic","devices":[null]}}`},
		{"body", protocol.RESTPathRequestCode, protocol.RESTMethodRequestCode, `null`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := validateResponse(test.method, test.path, []byte(test.body))
			if !errors.Is(err, roborockerrors.New(roborockerrors.Protocol, "", "", nil)) {
				t.Fatalf("accepted null required value: %v", err)
			}
		})
	}
}

func TestRequestCodeAcceptsNullData(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":200,"msg":"success","data":null}`))
	}))
	defer server.Close()

	c, err := New(WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.RequestCode(context.Background(), CodeRequest{Login: syntheticLogin()})
	if err != nil {
		t.Fatalf("code request with null data failed: %v", err)
	}
}

func TestDefaultClientDoesNotFollowRedirect(t *testing.T) {
	t.Parallel()

	var redirected atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/uncontracted" {
			redirected.Store(true)
			writer.WriteHeader(http.StatusOK)

			return
		}

		writer.Header().Set("Location", "/uncontracted")
		writer.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	c, err := New(WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.RequestCode(context.Background(), CodeRequest{Login: syntheticLogin()})
	if err == nil || redirected.Load() {
		t.Fatalf("followed an uncontracted redirect: %v", err)
	}
}

func TestRoomRoutesMatchCompleteTemplate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path  string
		valid bool
	}{
		{"/user/homes/42/rooms", true},
		{"/user/deviceshare/query/shared%20device/rooms", true},
		{"/user/homes/42/rooms/extra", false},
		{"/user/deviceshare/query//rooms", false},
		{"/user/homes/42/unknown", false},
	}
	for _, testCase := range cases {
		t.Run(testCase.path, func(t *testing.T) {
			t.Parallel()

			err := validateRoute("GET", testCase.path)
			if (err == nil) != testCase.valid {
				t.Fatalf("route validation=%v expected valid=%v", err, testCase.valid)
			}
		})
	}
}
