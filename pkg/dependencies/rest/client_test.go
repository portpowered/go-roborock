package rest

import (
	"context"
	"errors"
	"fmt"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestFailureClasses(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		cause  error
		kind   roborockerrors.Kind
	}{{"rate limit", 429, `{}`, nil, roborockerrors.RateLimited}, {"vendor limit", 200, `{"code":9002}`, nil, roborockerrors.RateLimited}, {"invalid code", 200, `{"code":2018}`, nil, roborockerrors.Unauthorized}, {"malformed", 200, `{"code":`, nil, roborockerrors.Protocol}, {"missing data", 200, `{"code":200}`, nil, roborockerrors.Protocol}, {"canceled", 0, "", context.Canceled, roborockerrors.Canceled}, {"deadline", 0, "", context.DeadlineExceeded, roborockerrors.Timeout}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			c, err := New(WithBaseURL("https://iot.example.test"), WithHTTPClient(doerFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if test.cause != nil {
					return nil, test.cause
				}
				return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body))}, nil
			})))
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.SignKey(context.Background(), SignRequest{Login: LoginContext{Email: "synthetic@example.test", ClientID: "id"}})
			var typed *roborockerrors.Error
			if !errors.As(err, &typed) || typed.Kind != test.kind {
				t.Fatalf("unexpected error %v", err)
			}
			if test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatal("cause lost")
			}
			if calls != 1 {
				t.Fatal("unexpected retry")
			}
		})
	}
}

func TestNonceEntropyFailureAndInvalidOrigin(t *testing.T) {
	c, err := New(WithRandom(strings.NewReader("")))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.SignKey(context.Background(), SignRequest{Login: LoginContext{Email: "synthetic@example.test", ClientID: "id"}})
	if !errors.Is(err, io.EOF) {
		t.Fatalf("entropy cause: %v", err)
	}
	for _, origin := range []string{"", "https://example.test/path", "https://user:secret@example.test", "https://example.test?key=secret"} {
		if err = validateOrigin(origin); err == nil {
			t.Fatal(fmt.Sprintf("accepted invalid origin %q", origin))
		}
	}
}
