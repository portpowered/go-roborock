package replay_test

import (
	"bytes"
	"crypto/aes"
	"crypto/md5" //nolint:gosec // Independent oracle for vendor-mandated V1 encryption.
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"strconv"
)

func mqttResponseWithID(fixture mqttFixture, requestID int64) ([]byte, error) {
	frame, err := hex.DecodeString(fixture.ResponseFrameHex)
	if err != nil {
		return nil, fmt.Errorf("decode response vector: %w", err)
	}

	if len(frame) < 23 {
		return nil, mqttMismatch("truncated response vector")
	}

	payload, timestamp, err := decryptMQTTV1(frame, fixture.LocalKey, binary.BigEndian.Uint32(frame[3:7]), 102)
	if err != nil {
		return nil, err
	}

	var envelope mqttRPCEnvelope

	err = strictMQTTJSON(payload, &envelope)
	if err != nil {
		return nil, err
	}

	var response map[string]json.RawMessage

	err = json.Unmarshal([]byte(envelope.Dps["102"]), &response)
	if err != nil {
		return nil, fmt.Errorf("decode vector response: %w", err)
	}

	if string(response["id"]) != "1" {
		return nil, mqttMismatch("response vector original ID mismatch")
	}

	response["id"] = json.RawMessage(strconv.FormatInt(requestID, 10))

	inner, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("encode rebound response: %w", err)
	}

	envelope.Dps["102"] = string(inner)

	payload, err = json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode rebound envelope: %w", err)
	}

	return encryptMQTTResponse(frame[:19], payload, timestamp, fixture.LocalKey)
}

func encryptMQTTResponse(header, payload []byte, timestamp uint32, key string) ([]byte, error) {
	stamp := fmt.Sprintf("%08x", timestamp)
	reordered := string([]byte{stamp[5], stamp[6], stamp[3], stamp[7], stamp[1], stamp[2], stamp[0], stamp[4]})
	digest := md5.Sum([]byte(reordered + key + "TXdfu$jyZ#TZHsg4")) //nolint:gosec // Vendor V1 key derivation.

	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return nil, fmt.Errorf("response cipher: %w", err)
	}

	padding := aes.BlockSize - len(payload)%aes.BlockSize
	plain := append(bytes.Clone(payload), bytes.Repeat([]byte{byte(padding)}, padding)...)
	frame := append(bytes.Clone(header), make([]byte, len(plain)+4)...)
	binary.BigEndian.PutUint16(frame[17:19], uint16(len(plain))) //nolint:gosec // Bounded synthetic fixture payload.

	for offset := 0; offset < len(plain); offset += aes.BlockSize {
		block.Encrypt(frame[19+offset:19+offset+aes.BlockSize], plain[offset:offset+aes.BlockSize])
	}

	end := len(frame) - 4
	binary.BigEndian.PutUint32(frame[end:], crc32.ChecksumIEEE(frame[:end]))

	return frame, nil
}
