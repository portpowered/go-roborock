package mqtt

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" //nolint:gosec // Roborock's published V1 and A01 wire algorithms require MD5.
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"math"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
)

const (
	maxCipherPayload = math.MaxUint16 - math.MaxUint16%aes.BlockSize
	a01IVHexStart    = 8
	a01IVHexEnd      = 24
)

type deviceFrame = dependencymodels.MQTTDeviceFrame

func frameCipher(frame deviceFrame, localKey string) (cipher.Block, []byte, error) {
	if frame.Version == protocol.MQTTVersionA01 {
		// A01 derives its IV from this exact MD5 digest.
		digest := md5.Sum([]byte(fmt.Sprintf("%08x%s", frame.Random, protocol.MQTTA01Hash))) //nolint:gosec

		block, err := aes.NewCipher([]byte(localKey))
		if err != nil {
			return nil, nil, fmt.Errorf("create A01 device cipher: %w", err)
		}

		return block, []byte(hex.EncodeToString(digest[:])[a01IVHexStart:a01IVHexEnd]), nil
	}

	if frame.Version != protocol.MQTTVersionV1 {
		return nil, nil, errUnsupportedDeviceProtocol
	}

	stamp := fmt.Sprintf("%08x", frame.Timestamp)
	reordered := []byte{stamp[5], stamp[6], stamp[3], stamp[7], stamp[1], stamp[2], stamp[0], stamp[4]}
	// V1 derives its AES key from this exact MD5 digest.
	digest := md5.Sum(append(append(reordered, []byte(localKey)...), []byte(protocol.MQTTWireSalt)...)) //nolint:gosec

	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return nil, nil, fmt.Errorf("create V1 device cipher: %w", err)
	}

	return block, nil, nil
}

func encodeFrame(frame deviceFrame, localKey string) ([]byte, error) {
	block, initializationVector, err := frameCipher(frame, localKey)
	if err != nil {
		return nil, err
	}

	padding := aes.BlockSize - len(frame.Payload)%aes.BlockSize

	plain := append(bytes.Clone(frame.Payload), bytes.Repeat([]byte{byte(padding)}, padding)...)
	if len(plain) > maxCipherPayload {
		return nil, errDevicePayloadExceedsWireLimit
	}

	encrypted := make([]byte, len(plain))
	if initializationVector != nil {
		cipher.NewCBCEncrypter(block, initializationVector).CryptBlocks(encrypted, plain)
	} else {
		for offset := 0; offset < len(plain); offset += aes.BlockSize {
			block.Encrypt(encrypted[offset:offset+aes.BlockSize], plain[offset:offset+aes.BlockSize])
		}
	}

	return marshalFrame(frame, encrypted)
}

func marshalFrame(frame deviceFrame, encrypted []byte) ([]byte, error) {
	// Bound the integer conversion at the serialization boundary as well as encryption.
	payloadLength := len(encrypted)
	if payloadLength < 0 || payloadLength > math.MaxUint16 {
		return nil, errDevicePayloadExceedsWireLimit
	}

	result := make([]byte, protocol.MQTTFrameHeaderSize+len(encrypted)+protocol.MQTTFrameChecksumSize)
	copy(result, frame.Version)
	binary.BigEndian.PutUint32(result[protocol.MQTTFrameSequenceOffset:protocol.MQTTFrameRandomOffset], frame.Sequence)
	binary.BigEndian.PutUint32(result[protocol.MQTTFrameRandomOffset:protocol.MQTTFrameTimestampOffset], frame.Random)
	binary.BigEndian.PutUint32(result[protocol.MQTTFrameTimestampOffset:protocol.MQTTFrameProtocolOffset], frame.Timestamp)
	protocolField := result[protocol.MQTTFrameProtocolOffset:protocol.MQTTFramePayloadLengthOffset]
	binary.BigEndian.PutUint16(protocolField, frame.Protocol)

	payloadLengthField := result[protocol.MQTTFramePayloadLengthOffset:protocol.MQTTFramePayloadOffset]
	binary.BigEndian.PutUint16(payloadLengthField, uint16(payloadLength))
	copy(result[protocol.MQTTFramePayloadOffset:], encrypted)
	checksumOffset := len(result) - protocol.MQTTFrameChecksumSize
	binary.BigEndian.PutUint32(result[checksumOffset:], crc32.ChecksumIEEE(result[:checksumOffset]))

	return result, nil
}

func decodeFrame(data []byte, localKey string) (deviceFrame, int, error) {
	var frame deviceFrame
	if len(data) < protocol.MQTTFrameHeaderSize+protocol.MQTTFrameChecksumSize {
		return frame, 0, errTruncatedDeviceFrame
	}

	payloadLength := binary.BigEndian.Uint16(data[protocol.MQTTFramePayloadLengthOffset:protocol.MQTTFramePayloadOffset])

	size := protocol.MQTTFrameHeaderSize + int(payloadLength) + protocol.MQTTFrameChecksumSize
	if size > len(data) {
		return frame, 0, errTruncatedDevicePayload
	}

	checksumOffset := size - protocol.MQTTFrameChecksumSize
	if binary.BigEndian.Uint32(data[checksumOffset:size]) != crc32.ChecksumIEEE(data[:checksumOffset]) {
		return frame, 0, errInvalidDeviceCRC
	}

	frame.Version = string(data[:protocol.MQTTFrameSequenceOffset])
	frame.Sequence = binary.BigEndian.Uint32(data[protocol.MQTTFrameSequenceOffset:protocol.MQTTFrameRandomOffset])
	frame.Random = binary.BigEndian.Uint32(data[protocol.MQTTFrameRandomOffset:protocol.MQTTFrameTimestampOffset])
	frame.Timestamp = binary.BigEndian.Uint32(data[protocol.MQTTFrameTimestampOffset:protocol.MQTTFrameProtocolOffset])
	frame.Protocol = binary.BigEndian.Uint16(data[protocol.MQTTFrameProtocolOffset:protocol.MQTTFramePayloadLengthOffset])

	payload, err := decryptPayload(frame, data[protocol.MQTTFramePayloadOffset:checksumOffset], localKey)
	if err != nil {
		return frame, 0, err
	}

	frame.Payload = payload

	return frame, size, nil
}

func decryptPayload(frame deviceFrame, encrypted []byte, localKey string) ([]byte, error) {
	if len(encrypted) == 0 || len(encrypted)%aes.BlockSize != 0 {
		return nil, errInvalidCiphertextLength
	}

	block, initializationVector, err := frameCipher(frame, localKey)
	if err != nil {
		return nil, err
	}

	plain := make([]byte, len(encrypted))
	if initializationVector != nil {
		cipher.NewCBCDecrypter(block, initializationVector).CryptBlocks(plain, encrypted)
	} else {
		for offset := 0; offset < len(encrypted); offset += aes.BlockSize {
			block.Decrypt(plain[offset:offset+aes.BlockSize], encrypted[offset:offset+aes.BlockSize])
		}
	}

	return unpadPayload(plain)
}

func unpadPayload(plain []byte) ([]byte, error) {
	paddingByte := plain[len(plain)-1]

	padding := int(paddingByte)
	if padding < 1 || padding > aes.BlockSize {
		return nil, errInvalidDevicePadding
	}

	if !bytes.Equal(plain[len(plain)-padding:], bytes.Repeat([]byte{paddingByte}, padding)) {
		return nil, errInvalidDevicePadding
	}

	return plain[:len(plain)-padding], nil
}
