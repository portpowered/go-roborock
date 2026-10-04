package mqtt

import (
	"encoding/binary"
	"github.com/portpowered/go-roborock/internal/protocol"
	"io"
)

const maxPacketSize = 1 << 20

func mqttString(value string) []byte {
	result := make([]byte, protocol.MQTTUint16Size+len(value))

	length := len(value)
	if length < 0 || length > protocol.MQTTMaxStringLength {
		return nil
	}

	binary.BigEndian.PutUint16(result, uint16(length))
	copy(result[protocol.MQTTUint16Size:], value)

	return result
}

func packet(header byte, body []byte) []byte {
	result := []byte{header}

	remaining := len(body)

	for {
		digit := byte(remaining % protocol.MQTTVarintBase)

		remaining /= protocol.MQTTVarintBase

		if remaining > 0 {
			digit |= protocol.MQTTVarintContinuation
		}

		result = append(result, digit)

		if remaining == 0 {
			break
		}
	}

	return append(result, body...)
}

func readPacket(reader io.Reader) (byte, []byte, error) {
	var header [1]byte

	_, err := io.ReadFull(reader, header[:])
	if err != nil {
		return 0, nil, transportError("packet header", err)
	}

	length := 0
	multiplier := 1

	for range protocol.MQTTMaxRemainingLengthOctets {
		var digit [1]byte

		_, err = io.ReadFull(reader, digit[:])
		if err != nil {
			return 0, nil, transportError("remaining length", err)
		}

		length += int(digit[0]&protocol.MQTTVarintMask) * multiplier
		if length > maxPacketSize {
			return 0, nil, errMQTTPacketExceedsLimit
		}

		if digit[0]&protocol.MQTTVarintContinuation == 0 {
			body := make([]byte, length)

			_, err = io.ReadFull(reader, body)
			if err != nil {
				return 0, nil, transportError("packet body", err)
			}

			return header[0], body, nil
		}

		multiplier *= protocol.MQTTVarintBase
	}

	return 0, nil, errInvalidMQTTRemainingLength
}

func connectPacket(clientID, username, password string) []byte {
	body := append(mqttString(protocol.MQTTProtocolName),
		protocol.MQTTProtocolLevel, protocol.MQTTConnectFlags, 0, protocol.MQTTKeepaliveSeconds)
	body = append(body, mqttString(clientID)...)
	body = append(body, mqttString(username)...)
	body = append(body, mqttString(password)...)

	return packet(protocol.MQTTConnect, body)
}

func subscribePacket(topic string) []byte {
	body := append([]byte{0, 1}, mqttString(topic)...)

	return packet(protocol.MQTTSubscribe, append(body, protocol.MQTTQoSAtMostOnce))
}

func publishPacket(topic string, payload []byte) []byte {
	return packet(protocol.MQTTPublish, append(mqttString(topic), payload...))
}
