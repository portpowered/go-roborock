package mqtt

import (
	"encoding/binary"
	"github.com/portpowered/go-roborock/internal/protocol"
	"io"
)

const maxPacketSize = 1 << 20

func mqttString(value string) []byte {
	result := make([]byte, 2+len(value))
	binary.BigEndian.PutUint16(result, uint16(len(value)))
	copy(result[2:], value)

	return result
}

func packet(header byte, body []byte) []byte {
	result := []byte{header}

	remaining := len(body)

	for {
		digit := byte(remaining % 128)

		remaining /= 128

		if remaining > 0 {
			digit |= 128
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
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return 0, nil, err
	}

	length := 0
	multiplier := 1

	for range 4 {
		var digit [1]byte
		if _, err := io.ReadFull(reader, digit[:]); err != nil {
			return 0, nil, err
		}

		length += int(digit[0]&127) * multiplier
		if length > maxPacketSize {
			return 0, nil, errMQTTPacketExceedsLimit
		}

		if digit[0]&128 == 0 {
			body := make([]byte, length)
			_, err := io.ReadFull(reader, body)

			return header[0], body, err
		}

		multiplier *= 128
	}

	return 0, nil, errInvalidMQTTRemainingLength
}

func connectPacket(clientID, username, password string) []byte {
	body := append(mqttString(protocol.MQTTProtocolName), protocol.MQTTProtocolLevel, protocol.MQTTConnectFlags, 0, protocol.MQTTKeepaliveSeconds)
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
