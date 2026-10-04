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
		{"/v3/user/homes/42", protocol.RESTMethodGetHomeDatav3, `{"success":true,"result":{"id":42,"devices":[]}}`},
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
