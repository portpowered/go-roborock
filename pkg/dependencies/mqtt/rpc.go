package mqtt

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"math"
	"time"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// Call sends one V1 device RPC and awaits its correlated response without retries.
func (s *Session) Call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	if s.config.Protocol != protocol.MQTTVersionV1 {
		return nil, unsupported("call")
	}

	if !validMethod(method) || (len(params) > 0 && !json.Valid(params)) {
		return nil, invalid("call", "invalid RPC method or params")
	}

	if len(params) == 0 {
		params = json.RawMessage("[]")
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	requestID := int64(s.sequence.Add(1))

	reply, err := s.beginRPC(requestID)
	if err != nil {
		return nil, err
	}

	defer s.endRPC(requestID)

	payload, err := s.rpcPayload(requestID, method, params)
	if err != nil {
		return nil, err
	}

	err = s.send(ctx, payload)
	if err != nil {
		return nil, transportError("call", err)
	}

	return s.awaitRPC(ctx, reply)
}

func (s *Session) beginRPC(requestID int64) (chan response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.pending) >= maxPendingCommands {
		return nil, roborockerrors.New(roborockerrors.Backpressure, "mqtt call", "too many pending commands", nil)
	}

	reply := make(chan response, 1)
	s.pending[requestID] = reply

	return reply, nil
}

func (s *Session) endRPC(requestID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.pending, requestID)
}

func (s *Session) rpcPayload(requestID int64, method string, params json.RawMessage) ([]byte, error) {
	request := dependencymodels.MQTTRPCRequest{Id: requestID, Method: method, Params: params, Security: s.security}

	inner, err := json.Marshal(request)
	if err != nil {
		return nil, transportError("request", err)
	}

	text, err := json.Marshal(string(inner))
	if err != nil {
		return nil, transportError("request string", err)
	}

	envelope := dependencymodels.MQTTEnvelope{
		Dps: map[string]json.RawMessage{protocol.MQTTRPCRequestDatapoint: text}, T: time.Now().Unix(),
	}

	payload, err := json.Marshal(envelope)
	if err != nil {
		return nil, transportError("request envelope", err)
	}

	return payload, nil
}

func (s *Session) awaitRPC(ctx context.Context, reply <-chan response) (json.RawMessage, error) {
	select {
	case result := <-reply:
		return result.value, result.err
	case <-ctx.Done():
		return nil, transportError("call", ctx.Err())
	case <-s.done:
		select {
		case result := <-reply:
			return result.value, result.err
		default:
		}

		return nil, s.closedError()
	}
}

func (s *Session) send(ctx context.Context, payload []byte) error {
	var random [protocol.MQTTFrameTimestampOffset - protocol.MQTTFrameRandomOffset]byte

	_, err := rand.Read(random[:])
	if err != nil {
		return transportError("frame nonce", err)
	}

	timestamp := time.Now().Unix()
	if timestamp < 0 || timestamp > math.MaxUint32 {
		return transportError("frame timestamp", errInvalidTimestamp)
	}

	frame := deviceFrame{
		Version: s.config.Protocol, Sequence: s.sequence.Add(1), Random: binary.BigEndian.Uint32(random[:]),
		Timestamp: uint32(timestamp), Protocol: protocol.MQTTProtocolRequest, Payload: payload,
	}

	encoded, err := encodeFrame(frame, s.config.LocalKey)
	if err != nil {
		return err
	}

	return s.write(ctx, publishPacket(s.publishTopic, encoded))
}

func validMethod(method string) bool {
	if method == "" {
		return false
	}

	for _, char := range method {
		if !validMethodCharacter(char) {
			return false
		}
	}

	return true
}

func validMethodCharacter(char rune) bool {
	return char == '_' || (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9')
}
