package roborockerrors_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

func TestFailureClassificationAndCause(t *testing.T) {
	t.Parallel()

	cause := roborockerrors.New(roborockerrors.Unavailable, "synthetic", "cause with secret", nil)

	err := roborockerrors.New(roborockerrors.Timeout, "status", "request timed out", cause)

	var timeout roborockerrors.Error

	timeout.Kind = roborockerrors.Timeout

	if !errors.Is(err, cause) || !errors.Is(err, &timeout) {
		t.Fatal("failure lost class or cause")
	}

	var unauthorized, empty roborockerrors.Error

	unauthorized.Kind = roborockerrors.Unauthorized

	if errors.Is(err, &unauthorized) || errors.Is(err, &empty) {
		t.Fatal("failure matched unrelated target")
	}

	if strings.Contains(err.Error(), cause.Error()) || !strings.Contains(err.Error(), "status") {
		t.Fatal("error exposes sensitive cause or loses operation")
	}
}

func TestWrapPreservesTypedCauseWithoutDisclosure(t *testing.T) {
	t.Parallel()

	cause := roborockerrors.New(roborockerrors.Unauthorized, "wire", "secret response", nil)
	cause.Code = 401
	wrapped := roborockerrors.Wrap(roborockerrors.Protocol, "login", "login failed", cause)

	if wrapped.Kind != roborockerrors.Unauthorized || wrapped.Code != cause.Code || !errors.Is(wrapped, cause) {
		t.Fatal("typed classification or cause was lost")
	}

	if strings.Contains(wrapped.Error(), cause.Message) {
		t.Fatal("wrapper exposed underlying secret message")
	}

	fallback := roborockerrors.Wrap(roborockerrors.Unavailable, "connect", "connection failed", nil)
	if fallback.Kind != roborockerrors.Unavailable {
		t.Fatal("fallback classification was lost")
	}
}
