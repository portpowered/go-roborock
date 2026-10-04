package mqtt

import (
	"context"
	"encoding/binary"
	"encoding/json"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

func (s *Session) receivePublish(owner context.Context, header byte, body []byte) error {
	if len(body) < protocol.MQTTUint16Size {
		return errTruncatedMQTTTopic
	}

	topicLength := int(binary.BigEndian.Uint16(body[:protocol.MQTTUint16Size]))
	if topicLength > len(body)-protocol.MQTTUint16Size {
		return errTruncatedMQTTTopic
	}

	topic := string(body[protocol.MQTTUint16Size : protocol.MQTTUint16Size+topicLength])

	payload, err := s.acknowledgePublish(owner, header, body[protocol.MQTTUint16Size+topicLength:])
	if err != nil {
		return err
	}

	if topic != s.subscribeTopic {
		return nil
	}

	return s.receiveDeviceFrames(payload)
}

func (s *Session) acknowledgePublish(owner context.Context, header byte, payload []byte) ([]byte, error) {
	qos := (header >> protocol.MQTTQoSShift) & protocol.MQTTQoSMask
	if qos > protocol.MQTTQoSAtLeastOnce {
		return nil, errUnsupportedIncomingMQTTQoS
	}

	if qos == protocol.MQTTQoSAtMostOnce {
		return payload, nil
	}

	if len(payload) < protocol.MQTTUint16Size {
		return nil, errTruncatedMQTTPacketIdentifier
	}

	ack := packet(protocol.MQTTPubAck, payload[:protocol.MQTTUint16Size])

	ctx, cancel := context.WithTimeout(owner, handshakeTimeout)
	defer cancel()

	err := s.write(ctx, ack)
	if err != nil {
		return nil, err
	}

	return payload[protocol.MQTTUint16Size:], nil
}

func (s *Session) receiveDeviceFrames(payload []byte) error {
	for len(payload) > 0 {
		frame, consumed, err := decodeFrame(payload, s.config.LocalKey)
		if err != nil {
			return err
		}

		if frame.Version != s.config.Protocol {
			return errUnexpectedDeviceFrameVersion
		}

		if frame.Protocol == protocol.MQTTProtocolResponse {
			err = s.deliver(frame.Payload)
			if err != nil {
				return err
			}
		}

		if frame.Protocol == protocol.MapsProtocolResponse {
			err = s.deliverMap(frame.Payload)
			if err != nil {
				return err
			}
		}

		payload = payload[consumed:]
	}

	return nil
}

func (s *Session) deliver(payload []byte) error {
	if s.config.Protocol == protocol.B01Version {
		return s.deliverB01(payload)
	}

	var envelope dependencymodels.MQTTEnvelope

	err := json.Unmarshal(payload, &envelope)
	if err != nil {
		return transportError("envelope", err)
	}

	if envelope.Dps == nil {
		return errMissingMQTTDatapoints
	}

	if s.config.Protocol == protocol.MQTTVersionA01 {
		return s.deliverA01(envelope.Dps)
	}

	encoded, ok := envelope.Dps[protocol.MQTTRPCResponseDatapoint]
	if !ok {
		return nil
	}

	result, err := decodeRPCResponse(encoded)
	if err != nil {
		return err
	}

	s.mu.Lock()
	pending := s.pending[result.Id]
	s.mu.Unlock()

	if pending != nil {
		select {
		case pending <- rpcResult(result):
		default:
		}
	}

	return nil
}

func decodeRPCResponse(encoded json.RawMessage) (dependencymodels.MQTTRPCResponse, error) {
	var (
		result dependencymodels.MQTTRPCResponse
		inner  string
	)

	err := json.Unmarshal(encoded, &inner)
	if err != nil {
		return result, transportError("RPC envelope", err)
	}

	err = json.Unmarshal([]byte(inner), &result)
	if err != nil {
		return result, transportError("RPC response", err)
	}

	if result.Id <= 0 || (result.Result == nil && result.Error == nil) {
		return result, errRPCResponseHasNoResultOrError
	}

	return result, nil
}

func rpcResult(result dependencymodels.MQTTRPCResponse) response {
	var reply response
	if result.Result != nil {
		reply.value = *result.Result
		if isUnknownMethod(reply.value) {
			reply.err = unsupported("call")
		}
	}

	if result.Error != nil {
		reply.err = rpcRejection(*result.Error)
	}

	return reply
}

func isUnknownMethod(value json.RawMessage) bool {
	var result string

	return json.Unmarshal(value, &result) == nil && result == protocol.MQTTUnknownMethodResult
}

func rpcRejection(rejection dependencymodels.MQTTRPCError) error {
	code := 0
	message := "device rejected command"

	if rejection.Code != nil {
		code = *rejection.Code
	}

	if rejection.Message != nil {
		message = *rejection.Message
	}

	return &roborockerrors.Error{
		Kind: roborockerrors.Protocol, Operation: "mqtt call", Code: code, Message: "device rejected command",
		Cause: &RPCError{Code: code, Message: message},
	}
}
