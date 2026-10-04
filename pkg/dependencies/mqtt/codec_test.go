package mqtt

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"testing"
)

func TestFrameEncryptionAndIntegrity(t *testing.T) {
	for _, version := range []string{"1.0", "A01"} {
		t.Run(version, func(t *testing.T) {
			frame := deviceFrame{version: version, sequence: 7, random: 42, timestamp: 1700000000, protocol: 101, payload: []byte(`{"dps":{"201":42},"t":1700000000}`)}
			encoded, err := encodeFrame(frame, syntheticKey)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(encoded, frame.payload) {
				t.Fatal("unencrypted payload")
			}
			decoded, consumed, err := decodeFrame(encoded, syntheticKey)
			if err != nil || consumed != len(encoded) || !bytes.Equal(decoded.payload, frame.payload) {
				t.Fatalf("decoded=%v consumed=%d error=%v", decoded, consumed, err)
			}
			encoded[20] ^= 1
			if _, _, err = decodeFrame(encoded, syntheticKey); err == nil {
				t.Fatal("corrupt CRC accepted")
			}
		})
	}
}

func TestFrameRejectsMalformed(t *testing.T) {
	cases := [][]byte{nil, make([]byte, 22), append([]byte("B01"), make([]byte, 36)...)}
	for _, data := range cases {
		if _, _, err := decodeFrame(data, syntheticKey); err == nil {
			t.Fatal("malformed frame accepted")
		}
	}
	encoded, err := encodeFrame(deviceFrame{version: "A01", payload: []byte("x")}, syntheticKey)
	if err != nil {
		t.Fatal(err)
	}
	encoded[len(encoded)-5] ^= 1
	binary.BigEndian.PutUint32(encoded[len(encoded)-4:], crc32.ChecksumIEEE(encoded[:len(encoded)-4]))
	if _, _, err = decodeFrame(encoded, syntheticKey); err == nil {
		t.Fatal("invalid padding accepted")
	}
}

func TestPacketBounds(t *testing.T) {
	for _, data := range [][]byte{{48, 255, 255, 255, 255, 0}, {48, 128, 128, 128, 128}, {48, 127}} {
		if _, _, err := readPacket(bytes.NewReader(data)); err == nil {
			t.Fatal("invalid MQTT packet accepted")
		}
	}
}
