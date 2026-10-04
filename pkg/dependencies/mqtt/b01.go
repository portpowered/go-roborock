package mqtt

import (
	"bytes"
	"context"
	"crypto/aes"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
)

// CallB01 sends one supported Q7 RPC and awaits its correlated response.
func (s *Session) CallB01(
	ctx context.Context, method dependencymodels.B01Method, params json.RawMessage,
) (json.RawMessage, error) {
	if s.config.Protocol != protocol.B01Version {
		return nil, unsupported("B01 call")
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	requestID := int64(protocol.B01Q7MessageIDBase) + int64(s.sequence.Add(1))

	reply, err := s.beginRPC(requestID)
	if err != nil {
		return nil, err
	}

	defer s.endRPC(requestID)

	payload, err := b01RPCPayload(requestID, method, params)
	if err != nil {
		return nil, err
	}

	err = s.send(ctx, payload)
	if err != nil {
		return nil, transportError("B01 call", err)
	}

	return s.awaitRPC(ctx, reply)
}

func b01RPCPayload(requestID int64, method dependencymodels.B01Method, params json.RawMessage) ([]byte, error) {
	switch string(method) {
	case protocol.MapsQ7ListMethod, protocol.MapsQ7UploadMethod, protocol.MapsQ7RoomCleanMethod:
	default:
		return nil, unsupported("B01 method")
	}

	if len(params) == 0 {
		params = json.RawMessage(protocol.MQTTEmptyParamsJSON)
	}

	if !json.Valid(params) {
		return nil, invalid("B01 call", "invalid parameters")
	}

	request, err := json.Marshal(dependencymodels.B01RPCRequest{
		Method: method, MsgId: strconv.FormatInt(requestID, 10), Params: params,
	})
	if err != nil {
		return nil, transportError("B01 request", err)
	}

	payload, err := json.Marshal(dependencymodels.B01Envelope{
		Dps: map[string]json.RawMessage{protocol.B01Q7Datapoint: request},
	})
	if err != nil {
		return nil, transportError("B01 envelope", err)
	}
	// Q7 pads the JSON envelope once before B01 outer frame encryption pads it again.
	padding := aes.BlockSize - len(payload)%aes.BlockSize

	return append(payload, bytes.Repeat([]byte{byte(padding)}, padding)...), nil
}

// SetQ10Clean publishes a typed room or zone clean command. Publication does not
// acknowledge device execution, and the command is never retried.
func (s *Session) SetQ10Clean(ctx context.Context, command dependencymodels.Q10CleanCommand) error {
	if s.config.Protocol != protocol.B01Version {
		return unsupported("Q10 clean")
	}

	if int(command.Cmd) != protocol.B01Q10RoomCleanCommand && int(command.Cmd) != protocol.B01Q10ZoneCleanCommand {
		return invalid("Q10 clean", "unsupported task")
	}

	if !json.Valid(command.CleanParamters) {
		return invalid("Q10 clean", "invalid parameters")
	}

	value, err := json.Marshal(command)
	if err != nil {
		return transportError("Q10 clean", err)
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	return s.sendB01DPS(ctx, protocol.B01Q10StartCleanDatapoint, value)
}

func (s *Session) sendB01DPS(ctx context.Context, key string, value json.RawMessage) error {
	payload, err := json.Marshal(dependencymodels.B01Envelope{Dps: map[string]json.RawMessage{key: value}})
	if err != nil {
		return transportError("B01 datapoints", err)
	}

	return s.send(ctx, payload)
}

func (s *Session) deliverB01(payload []byte) error {
	// Q7 replies may carry the inner JSON padding; Q10 replies do not.
	if !json.Valid(payload) && len(payload) != 0 {
		unpadded, err := unpadPayload(payload)
		if err == nil {
			payload = unpadded
		}
	}

	var envelope dependencymodels.B01Envelope

	err := json.Unmarshal(payload, &envelope)
	if err != nil {
		return transportError("B01 envelope", err)
	}

	if envelope.Dps == nil {
		return errMissingMQTTDatapoints
	}

	for _, value := range envelope.Dps {
		var inner string
		if json.Unmarshal(value, &inner) == nil {
			err := s.deliverB01RPC([]byte(inner))
			if err != nil {
				return err
			}
		}
	}

	return s.deliverQ10(envelope.Dps)
}

func (s *Session) deliverB01RPC(value []byte) error {
	object, known := knownB01RPCResponse(value)
	if !known {
		return nil
	}

	var result dependencymodels.B01RPCResponse

	err := json.Unmarshal(value, &result)
	if err != nil {
		return protocolError("B01 response", err)
	}

	err = validateB01Code(object)
	if err != nil {
		return protocolError("B01 response", err)
	}

	requestID, err := strconv.ParseInt(result.MsgId, 10, 64)
	if err != nil || requestID < protocol.B01Q7MessageIDBase ||
		requestID >= protocol.B01Q7MessageIDBase+protocol.B01Q7MessageIDRange ||
		strconv.FormatInt(requestID, 10) != result.MsgId {
		if err != nil {
			return transportError("B01 response identifier", err)
		}

		return transportError("B01 response identifier", errInvalidB01MessageID)
	}

	s.deliverReply(requestID, b01Result(result))

	return nil
}

func b01Result(result dependencymodels.B01RPCResponse) response {
	var reply response
	if result.Data != nil {
		reply.value = *result.Data
		if isUnknownMethod(reply.value) {
			reply.err = unknownMethodResult("B01 call")
		}
	}

	if result.Code != nil && *result.Code != 0 {
		reply.err = rpcRejection(dependencymodels.MQTTRPCError{Code: result.Code, Message: nil})
	}

	return reply
}

// knownB01RPCResponse separates unsolicited datapoint strings and future objects
// from RPC objects using the canonical generated identifier member.
func knownB01RPCResponse(value []byte) (map[string]json.RawMessage, bool) {
	var object map[string]json.RawMessage
	if json.Unmarshal(value, &object) != nil {
		return nil, false
	}

	field, _ := reflect.TypeFor[dependencymodels.B01RPCResponse]().FieldByName("MsgId")
	key, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	_, present := object[key]

	return object, present
}

func validateB01Code(object map[string]json.RawMessage) error {
	field, _ := reflect.TypeFor[dependencymodels.B01RPCResponse]().FieldByName("Code")
	key, _, _ := strings.Cut(field.Tag.Get("json"), ",")

	value, present := object[key]
	if present && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return errInvalidB01ResponseShape
	}

	return nil
}
