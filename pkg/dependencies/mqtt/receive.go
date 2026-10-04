package mqtt

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"github.com/portpowered/go-roborock/internal/protocol"

	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

func (s *Session) receivePublish(header byte, body []byte) error {
	if len(body) < 2 {
		return errTruncatedMQTTTopic
	}

	topicLength := int(binary.BigEndian.Uint16(body[:2]))
	if topicLength > len(body)-2 {
		return errTruncatedMQTTTopic
	}

	topic := string(body[2 : 2+topicLength])
	payload := body[2+topicLength:]

	qos := (header >> 1) & 3
	if qos > protocol.MQTTQoSAtLeastOnce {
		return errUnsupportedIncomingMQTTQoS
	}

	if qos == protocol.MQTTQoSAtLeastOnce {
		if len(payload) < 2 {
			return errTruncatedMQTTPacketIdentifier
		}

		ack := packet(protocol.MQTTPubAck, payload[:2])
		payload = payload[2:]
		ctx, cancel := context.WithTimeout(context.Background(), handshakeTimeout)
		err := s.write(ctx, ack)

		cancel()

		if err != nil {
			return err
		}
	}
	// A response from any other device must never satisfy this session's RPC.
	if topic != s.subscribeTopic {
		return nil
	}

	for len(payload) > 0 {
		frame, consumed, err := decodeFrame(payload, s.config.LocalKey)
		if err != nil {
			return err
		}

		if frame.version != s.config.Protocol {
			return errUnexpectedDeviceFrameVersion
		}

		if frame.protocol == protocol.MQTTProtocolResponse {
			err = s.deliver(frame.payload)
			if err != nil {
				return err
			}
		}

		payload = payload[consumed:]
	}

	return nil
}

func (s *Session) deliver(payload []byte) error {
	var envelope dependencymodels.MQTTEnvelope
	err := json.Unmarshal(payload, &envelope)
	if err != nil {
		return err
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

	var inner string
	err := json.Unmarshal(encoded, &inner)
	if err != nil {
		return err
	}

	var result dependencymodels.MQTTRPCResponse
	err := json.Unmarshal([]byte(inner), &result)
	if err != nil {
		return err
	}

	if result.Result == nil && result.Error == nil {
		return errRPCResponseHasNoResultOrError
	}

	reply := response{}
	if result.Result != nil {
		reply.value = *result.Result
		if string(reply.value) == `"unknown_method"` {
			reply.err = roborockerrors.New(roborockerrors.Unsupported, "mqtt call", "device does not support command", nil)
		}
	}

	if result.Error != nil {
		code := 0
		message := "device rejected command"

		if result.Error.Code != nil {
			code = *result.Error.Code
		}

		if result.Error.Message != nil {
			message = *result.Error.Message
		}

		reply.err = &roborockerrors.Error{Kind: roborockerrors.Protocol, Operation: "mqtt call", Code: code, Message: "device rejected command", Cause: &RPCError{Code: code, Message: message}}
	}

	s.mu.Lock()
	pending := s.pending[result.Id]
	s.mu.Unlock()

	if pending != nil {
		select {
		case pending <- reply:
		default:
		}
	}

	return nil
}
