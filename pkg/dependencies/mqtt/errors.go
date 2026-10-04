// Package mqtt implements account-bound Roborock cloud device sessions.
package mqtt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// ErrClosed indicates that the session cannot accept further commands.
var ErrClosed = errors.New("MQTT session closed")

func transportError(operation string, cause error) error {
	kind := roborockerrors.Protocol

	var (
		network    net.Error
		classified *roborockerrors.Error
	)

	switch {
	case errors.As(cause, &classified):
		kind = classified.Kind
	case errors.Is(cause, context.DeadlineExceeded):
		kind = roborockerrors.Timeout
	case errors.Is(cause, context.Canceled):
		kind = roborockerrors.Canceled
	case errors.Is(cause, ErrClosed):
		kind = roborockerrors.Closed
	case errors.As(cause, &network):
		kind = roborockerrors.Unavailable
		if network.Timeout() {
			kind = roborockerrors.Timeout
			cause = errors.Join(context.DeadlineExceeded, cause)
		}
	case operation == "open":
		kind = roborockerrors.InvalidArgument
	case operation == "connect":
		kind = roborockerrors.Unavailable
	case errors.Is(cause, io.EOF):
		kind = roborockerrors.Unavailable
	}

	return roborockerrors.New(kind, "mqtt "+operation, "MQTT operation failed", cause)
}

func protocolError(operation string, cause error) error {
	return roborockerrors.New(roborockerrors.Protocol, "mqtt "+operation, "malformed device response", cause)
}

func invalid(operation, message string) error {
	return roborockerrors.New(roborockerrors.InvalidArgument, "mqtt "+operation, message, nil)
}
func unsupported(operation string) error {
	return roborockerrors.New(roborockerrors.Unsupported, "mqtt "+operation, "unsupported device protocol", nil)
}

// RPCError is a command rejection returned by the device.
type RPCError struct {
	Code    int
	Message string
}

func (e *RPCError) Error() string { return fmt.Sprintf("device RPC %d: %s", e.Code, e.Message) }

var (
	errInvalidB01MessageID                       = errors.New("B01 response message identifier exceeds its wire range")
	errInvalidMapListShape                       = errors.New("map list response is missing a required field")
	errInvalidTimestamp                          = errors.New("unix timestamp exceeds device wire range")
	errUnexpectedMQTTPacket                      = errors.New("unexpected MQTT packet")
	errA01DatapointKeyIsNotNumeric               = errors.New("A01 datapoint key is not numeric")
	errBrokerRejectedMQTTConnection              = errors.New("broker rejected MQTT connection")
	errBrokerRejectedMQTTSubscription            = errors.New("broker rejected MQTT subscription")
	errDeviceLocalKeyMustContain16Bytes          = errors.New("device local key must contain 16 bytes")
	errDevicePayloadExceedsWireLimit             = errors.New("device payload exceeds wire limit")
	errInvalidCiphertextLength                   = errors.New("invalid ciphertext length")
	errInvalidDeviceCRC                          = errors.New("invalid device CRC")
	errInvalidDevicePadding                      = errors.New("invalid device padding")
	errInvalidMQTTAccountOrDeviceTopicIdentifier = errors.New("invalid MQTT account or device topic identifier")
	errInvalidMQTTBrokerURL                      = errors.New("invalid MQTT broker URL")
	errInvalidMQTTCredentialsOrDeviceIdentifier  = errors.New("invalid MQTT credentials or device identifier")
	errInvalidMQTTRemainingLength                = errors.New("invalid MQTT remaining length")
	errMissingMQTTDatapoints                     = errors.New("missing MQTT datapoints")
	errMQTTBrokerRequiresTLSSslOrMqtts           = errors.New("MQTT broker requires TLS (ssl or mqtts)")
	errMQTTPacketExceedsLimit                    = errors.New("MQTT packet exceeds limit")
	errRPCResponseHasNoResultOrError             = errors.New("RPC response has no result or error")
	errTruncatedDeviceFrame                      = errors.New("truncated device frame")
	errTruncatedDevicePayload                    = errors.New("truncated device payload")
	errTruncatedMQTTPacketIdentifier             = errors.New("truncated MQTT packet identifier")
	errTruncatedMQTTTopic                        = errors.New("truncated MQTT topic")
	errUnexpectedDeviceFrameVersion              = errors.New("unexpected device frame version")
	errUnsupportedDeviceProtocol                 = errors.New("unsupported device protocol")
	errUnsupportedIncomingMQTTQoS                = errors.New("unsupported incoming MQTT QoS")
)
