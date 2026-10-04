package mqtt

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/crc32"
	"os"
	"testing"

	"github.com/portpowered/go-roborock/internal/protocol"
)

func TestFrameEncryptionAndIntegrity(t *testing.T) {
	t.Parallel()

	for _, version := range []string{protocol.MQTTVersionV1, protocol.MQTTVersionA01} {
		t.Run(version, func(t *testing.T) {
			t.Parallel()

			frame := referenceFrame(version)

			encoded, err := encodeFrame(frame, syntheticKey)
			if err != nil {
				t.Fatal(err)
			}

			if bytes.Contains(encoded, frame.Payload) {
				t.Fatal("unencrypted payload")
			}

			decoded, consumed, err := decodeFrame(encoded, syntheticKey)
			if err != nil || consumed != len(encoded) || !bytes.Equal(decoded.Payload, frame.Payload) {
				t.Fatalf("decoded=%v consumed=%d error=%v", decoded, consumed, err)
			}

			encoded[protocol.MQTTFramePayloadOffset] ^= 1

			_, _, err = decodeFrame(encoded, syntheticKey)
			if !errors.Is(err, errInvalidDeviceCRC) {
				t.Fatalf("corrupt CRC error=%v", err)
			}
		})
	}
}

func referenceFrame(version string) deviceFrame {
	return deviceFrame{
		Version: version, Sequence: 7, Random: 42, Timestamp: 1700000000,
		Protocol: protocol.MQTTProtocolRequest, Payload: []byte(`{"dps":{"201":42},"t":1700000000}`),
	}
}

func TestFrameSyntheticReferenceVectors(t *testing.T) {
	t.Parallel()
	// Synthetic plaintext and keys, using pinned Python protocol.py wire rules.
	// AES/PKCS7 ciphertext was independently calculated with .NET Aes; the full
	// big-endian header and CRC32 were calculated separately with Python struct/zlib.
	// These immutable values detect shared encoder/decoder bugs in round-trip tests.
	vectors := map[string]string{
		protocol.MQTTVersionV1: "312e30000000070000002a6553f10000650030" +
			"230b03724e8185856135a525f8aed23de211097fddeaa5933aab3101901b70acc6478b87525b20e9ff917243aa27d1bb609b89cb",
		protocol.MQTTVersionA01: "413031000000070000002a6553f10000650030" +
			"62a73cf248f4457e9df9c17985f2e61893b70efa0eb3c3535e569b7912772792fc1d5daa2071c4570fedc842d7f142fa582a2fc9",
	}
	for version, encodedHex := range vectors {
		t.Run(version, func(t *testing.T) {
			t.Parallel()

			expected, err := hex.DecodeString(encodedHex)
			if err != nil {
				t.Fatal(err)
			}

			frame := referenceFrame(version)

			encoded, err := encodeFrame(frame, syntheticKey)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(encoded, expected) {
				t.Fatalf("encoded=%x expected=%x", encoded, expected)
			}

			decoded, consumed, err := decodeFrame(expected, syntheticKey)
			if err != nil || consumed != len(expected) || !bytes.Equal(decoded.Payload, frame.Payload) {
				t.Fatalf("reference decode=%v consumed=%d error=%v", decoded, consumed, err)
			}
		})
	}
}

func TestFrameRejectsMalformed(t *testing.T) {
	t.Parallel()

	cases := [][]byte{nil, make([]byte, protocol.MQTTFrameHeaderSize+protocol.MQTTFrameChecksumSize-1),
		append([]byte("B01"), make([]byte, 36)...)}
	for _, data := range cases {
		_, _, err := decodeFrame(data, syntheticKey)
		if err == nil {
			t.Fatal("malformed frame accepted")
		}
	}

	encoded, err := encodeFrame(referenceFrame(protocol.MQTTVersionA01), syntheticKey)
	if err != nil {
		t.Fatal(err)
	}

	checksumOffset := len(encoded) - protocol.MQTTFrameChecksumSize
	encoded[checksumOffset-1] ^= 1
	binary.BigEndian.PutUint32(encoded[checksumOffset:], crc32.ChecksumIEEE(encoded[:checksumOffset]))

	_, _, err = decodeFrame(encoded, syntheticKey)
	if !errors.Is(err, errInvalidDevicePadding) {
		t.Fatalf("invalid padding error=%v", err)
	}
}

func TestFramePayloadLimit(t *testing.T) {
	t.Parallel()

	frame := referenceFrame(protocol.MQTTVersionV1)

	frame.Payload = make([]byte, maxCipherPayload-1)

	_, err := encodeFrame(frame, syntheticKey)
	if err != nil {
		t.Fatal(err)
	}

	frame.Payload = make([]byte, maxCipherPayload)

	_, err = encodeFrame(frame, syntheticKey)
	if !errors.Is(err, errDevicePayloadExceedsWireLimit) {
		t.Fatalf("oversized payload error=%v", err)
	}
}

func TestPacketBounds(t *testing.T) {
	t.Parallel()

	for _, data := range [][]byte{{48, 255, 255, 255, 255, 0}, {48, 128, 128, 128, 128}, {48, 127}} {
		_, _, err := readPacket(bytes.NewReader(data))
		if err == nil {
			t.Fatal("invalid MQTT packet accepted")
		}
	}
}

func TestFrameRejectsUnsupportedVersionAndKey(t *testing.T) {
	t.Parallel()

	frame := referenceFrame("B01")

	_, err := encodeFrame(frame, syntheticKey)
	if !errors.Is(err, errUnsupportedDeviceProtocol) {
		t.Fatalf("unsupported version error=%v", err)
	}

	frame.Version = protocol.MQTTVersionA01

	_, err = encodeFrame(frame, "short")
	if err == nil {
		t.Fatal("invalid A01 AES key accepted")
	}
}

func TestFrameRejectsCiphertextLength(t *testing.T) {
	t.Parallel()

	for _, length := range []int{0, 1, 15, 17} {
		frame := referenceFrame(protocol.MQTTVersionV1)

		encoded, err := marshalFrame(frame, make([]byte, length))
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = decodeFrame(encoded, syntheticKey)
		if !errors.Is(err, errInvalidCiphertextLength) {
			t.Fatalf("length=%d error=%v", length, err)
		}
	}
}

func TestFrameRejectsTruncatedPayload(t *testing.T) {
	t.Parallel()

	encoded, err := encodeFrame(referenceFrame(protocol.MQTTVersionV1), syntheticKey)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = decodeFrame(encoded[:len(encoded)-1], syntheticKey)
	if !errors.Is(err, errTruncatedDevicePayload) {
		t.Fatalf("truncated payload error=%v", err)
	}
}

type pythonFrameVectors struct {
	Provenance string              `json:"provenance"`
	Source     string              `json:"source"`
	Revision   string              `json:"revision"`
	Vectors    []pythonFrameVector `json:"vectors"`
}

type pythonFrameVector struct {
	Version   string `json:"version"`
	Sequence  uint32 `json:"sequence"`
	Random    uint32 `json:"random"`
	Timestamp uint32 `json:"timestamp"`
	Protocol  uint16 `json:"protocol"`
	LocalKey  string `json:"localKey"`
	Payload   string `json:"payload"`
	FrameHex  string `json:"frameHex"`
}

func TestFramePinnedPythonVectors(t *testing.T) {
	t.Parallel()

	const path = "../../../tests/replay/fixtures/mqtt/synthetic/python-vectors.json"

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var fixture pythonFrameVectors

	err = json.Unmarshal(data, &fixture)
	if err != nil {
		t.Fatal(err)
	}

	if fixture.Provenance != "synthetic" || fixture.Revision != "a8260c5211e60647fc352d6b496b827865938b21" {
		t.Fatalf("unexpected Python vector provenance: %s/%s", fixture.Provenance, fixture.Revision)
	}

	if len(fixture.Vectors) != 2 {
		t.Fatalf("expected both protocol vectors, got %d", len(fixture.Vectors))
	}

	for _, vector := range fixture.Vectors {
		t.Run(vector.Version, func(t *testing.T) {
			t.Parallel()
			verifyPythonFrameVector(t, vector)
		})
	}
}

func verifyPythonFrameVector(t *testing.T, vector pythonFrameVector) {
	t.Helper()

	frame := deviceFrame{
		Version: vector.Version, Sequence: vector.Sequence, Random: vector.Random,
		Timestamp: vector.Timestamp, Protocol: vector.Protocol, Payload: []byte(vector.Payload),
	}

	expected, err := hex.DecodeString(vector.FrameHex)
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := encodeFrame(frame, vector.LocalKey)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(encoded, expected) {
		t.Fatalf("encoded=%x Python=%x", encoded, expected)
	}

	decoded, consumed, err := decodeFrame(expected, vector.LocalKey)
	if err != nil || consumed != len(expected) || !bytes.Equal(decoded.Payload, frame.Payload) {
		t.Fatalf("Python decode=%v consumed=%d error=%v", decoded, consumed, err)
	}

	verifyFrameHeader(t, decoded, frame)
}

func verifyFrameHeader(t *testing.T, decoded, frame deviceFrame) {
	t.Helper()

	if decoded.Version != frame.Version || decoded.Sequence != frame.Sequence || decoded.Random != frame.Random ||
		decoded.Timestamp != frame.Timestamp || decoded.Protocol != frame.Protocol {
		t.Fatalf("decoded Python header=%v expected=%v", decoded, frame)
	}
}
