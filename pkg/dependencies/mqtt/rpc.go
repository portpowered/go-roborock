package mqtt

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
	"time"
)

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

	id := int64(s.sequence.Add(1))
	reply := make(chan response, 1)

	s.mu.Lock()

	if len(s.pending) >= maxPendingCommands {
		s.mu.Unlock()

		return nil, roborockerrors.New(roborockerrors.Backpressure, "mqtt call", "too many pending commands", nil)
	}

	s.pending[id] = reply
	s.mu.Unlock()

	defer func() { s.mu.Lock(); delete(s.pending, id); s.mu.Unlock() }()

	inner, err := json.Marshal(dependencymodels.MQTTRPCRequest{Id: id, Method: method, Params: params, Security: s.security})
	if err != nil {
		return nil, err
	}

	text, _ := json.Marshal(string(inner))

	payload, err := json.Marshal(dependencymodels.MQTTEnvelope{Dps: map[string]json.RawMessage{protocol.MQTTRPCRequestDatapoint: text}, T: time.Now().Unix()})
	if err != nil {
		return nil, err
	}

	if err = s.send(ctx, payload); err != nil {
		return nil, transportError("call", err)
	}

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
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}

	frame := deviceFrame{version: s.config.Protocol, sequence: s.sequence.Add(1), random: binary.BigEndian.Uint32(random[:]), timestamp: uint32(time.Now().Unix()), protocol: protocol.MQTTProtocolRequest, payload: payload}

	encoded, err := encodeFrame(frame, s.config.LocalKey)
	if err != nil {
		return err
	}

	return s.write(ctx, publishPacket(s.publishTopic, encoded))
}

// Call sends one V1 RPC and waits for its matching device response.

func validMethod(method string) bool {
	if method == "" {
		return false
	}

	for _, char := range method {
		if char != '_' && (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') {
			return false
		}
	}

	return true
}
