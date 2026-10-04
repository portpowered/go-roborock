package replay_test

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/md5" //nolint:gosec // Independent oracle verifies the vendor-mandated MD5 key derivation.
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net"
	"os"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/portpowered/go-roborock/pkg/dependencies/mqtt"
)

type mqttFixture struct {
	Provenance       string          `json:"provenance"`
	Source           string          `json:"source"`
	Revision         string          `json:"revision"`
	Broker           string          `json:"broker"`
	User             string          `json:"user"`
	Secret           string          `json:"secret"`
	Key              string          `json:"key"`
	DeviceID         string          `json:"deviceId"`
	LocalKey         string          `json:"localKey"`
	Username         string          `json:"username"`
	Password         string          `json:"password"`
	SecurityEndpoint string          `json:"securityEndpoint"`
	PublishTopic     string          `json:"publishTopic"`
	SubscribeTopic   string          `json:"subscribeTopic"`
	Method           string          `json:"method"`
	Params           json.RawMessage `json:"params"`
	Result           json.RawMessage `json:"result"`
	ResponsePayload  string          `json:"responsePayload"`
	ResponseFrameHex string          `json:"responseFrameHex"`
}

type mqttRPCEnvelope struct {
	Dps       map[string]string `json:"dps"`
	Timestamp int64             `json:"t"`
}

type mqttRPCSecurity struct {
	Endpoint string `json:"endpoint"`
	Nonce    string `json:"nonce"`
}

type mqttRPCRequest struct {
	ID       int64           `json:"id"`
	Method   string          `json:"method"`
	Params   json.RawMessage `json:"params"`
	Security mqttRPCSecurity `json:"security"`
}

func TestPairedMQTTSessionRPC(t *testing.T) {
	t.Parallel()
	fixture := loadMQTTFixture(t)
	client, broker := net.Pipe()

	t.Cleanup(func() { _ = client.Close(); _ = broker.Close() })

	deadline := time.Now().Add(5 * time.Second)

	err := broker.SetDeadline(deadline)
	if err != nil {
		t.Fatal(err)
	}

	replayDone := make(chan error, 1)

	go func() {
		replayDone <- replayMQTT(broker, fixture)

		_ = broker.Close()
	}()

	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	session, err := mqtt.Open(ctx, mqtt.Config{
		BrokerURL: fixture.Broker, User: fixture.User, Secret: fixture.Secret, Key: fixture.Key,
		DeviceID: fixture.DeviceID, LocalKey: fixture.LocalKey, Protocol: "1.0",
	}, func(_ context.Context, network, address string) (net.Conn, error) {
		if network != mqttReplayNetwork || address != mqttReplayAddress {
			return nil, mqttMismatch("unexpected dial: %s %s", network, address)
		}

		return client, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = session.Close() })

	result, err := session.Call(ctx, fixture.Method, fixture.Params)
	if err != nil {
		t.Fatal(err)
	}

	if !equalMQTTJSON(result, fixture.Result) {
		t.Fatalf("RPC result: %s", result)
	}

	err = session.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = <-replayDone
	if err != nil {
		t.Fatal(err)
	}
}

func loadMQTTFixture(t *testing.T) mqttFixture {
	t.Helper()

	data, err := os.ReadFile("fixtures/mqtt/synthetic/session-rpc.json")
	if err != nil {
		t.Fatal(err)
	}

	var fixture mqttFixture

	err = json.Unmarshal(data, &fixture)
	if err != nil {
		t.Fatal(err)
	}

	if fixture.Provenance != accountFixtureProvenance ||
		fixture.Source != "reference-derived python-roborock create_mqtt_encoder" ||
		fixture.Revision != mqttReferenceRevision {
		t.Fatal("missing independent fixture provenance")
	}

	return fixture
}

func replayMQTT(conn net.Conn, fixture mqttFixture) error {
	err := replayMQTTHandshake(conn, fixture)
	if err != nil {
		return err
	}

	body, err := readMQTTExchange(conn, 0x30)
	if err != nil {
		return fmt.Errorf("mqtt replay I/O: %w", err)
	}

	topic, frame, err := takeMQTTString(body)
	if err != nil {
		return fmt.Errorf("mqtt replay I/O: %w", err)
	}

	if topic != fixture.PublishTopic {
		return mqttMismatch("PUBLISH topic mismatch: %s", topic)
	}

	err = matchMQTTRPC(frame, fixture)
	if err != nil {
		return fmt.Errorf("mqtt replay I/O: %w", err)
	}

	response, err := hex.DecodeString(fixture.ResponseFrameHex)
	if err != nil {
		return fmt.Errorf("mqtt replay I/O: %w", err)
	}

	responseBody := append(mqttReplayString(fixture.SubscribeTopic), response...)

	_, publishErr := conn.Write(mqttReplayPacket(0x30, responseBody))
	if publishErr != nil {
		return fmt.Errorf("mqtt replay I/O: %w", publishErr)
	}
	// Every ordered exchange has been consumed. Any additional frame is a mismatch.
	var extra [1]byte

	_, err = conn.Read(extra[:])
	if !errors.Is(err, io.EOF) {
		return mqttMismatch("unexpected extra MQTT data %x: %v", extra, err)
	}

	return nil
}

func replayMQTTHandshake(conn net.Conn, fixture mqttFixture) error {
	body, err := readMQTTExchange(conn, 0x10)
	if err != nil {
		return fmt.Errorf("mqtt replay I/O: %w", err)
	}

	connectErr := matchMQTTConnect(body, fixture)
	if connectErr != nil {
		return fmt.Errorf("mqtt replay I/O: %w", connectErr)
	}

	_, connackErr := conn.Write([]byte{0x20, 2, 0, 0})
	if connackErr != nil {
		return fmt.Errorf("mqtt replay I/O: %w", connackErr)
	}

	body, err = readMQTTExchange(conn, 0x82)
	if err != nil {
		return fmt.Errorf("mqtt replay I/O: %w", err)
	}

	expected := append([]byte{0, 1}, mqttReplayString(fixture.SubscribeTopic)...)

	expected = append(expected, 0)
	if !bytes.Equal(body, expected) {
		return mqttMismatch("SUBSCRIBE mismatch: %x", body)
	}

	_, subackErr := conn.Write([]byte{0x90, 3, 0, 1, 0})
	if subackErr != nil {
		return fmt.Errorf("mqtt replay I/O: %w", subackErr)
	}

	return nil
}

func matchMQTTConnect(body []byte, fixture mqttFixture) error {
	name, rest, err := takeMQTTString(body)
	if err != nil {
		return fmt.Errorf("mqtt replay I/O: %w", err)
	}

	if name != "MQTT" || len(rest) < 4 || !bytes.Equal(rest[:4], []byte{4, 0xc2, 0, 45}) {
		return mqttMismatch("CONNECT protocol, flags, or keepalive mismatch: %x", body)
	}

	clientID, rest, err := takeMQTTString(rest[4:])
	if err != nil {
		return fmt.Errorf("mqtt replay I/O: %w", err)
	}

	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(clientID) {
		return mqttMismatch("CONNECT client ID format: %q", clientID)
	}

	return matchMQTTCredentials(rest, fixture)
}

func matchMQTTCredentials(rest []byte, fixture mqttFixture) error {
	username, rest, err := takeMQTTString(rest)
	if err != nil {
		return fmt.Errorf("mqtt replay I/O: %w", err)
	}

	password, rest, err := takeMQTTString(rest)
	if err != nil {
		return fmt.Errorf("mqtt replay I/O: %w", err)
	}

	if username != fixture.Username || password != fixture.Password || len(rest) != 0 {
		return mqttMismatch("CONNECT credentials or trailing data mismatch")
	}

	return nil
}

func matchMQTTRPC(frame []byte, fixture mqttFixture) error {
	payload, timestamp, err := decryptMQTTRequest(frame, fixture.LocalKey)
	if err != nil {
		return fmt.Errorf("mqtt replay I/O: %w", err)
	}

	var envelope mqttRPCEnvelope

	envelopeErr := strictMQTTJSON(payload, &envelope)
	if envelopeErr != nil {
		return fmt.Errorf("mqtt replay I/O: %w", envelopeErr)
	}

	now := time.Now().Unix()
	if envelope.Timestamp < now-5 || envelope.Timestamp > now+5 ||
		int64(timestamp) < envelope.Timestamp ||
		int64(timestamp) > envelope.Timestamp+1 {
		return mqttMismatch("RPC timestamp mismatch: %d %d", envelope.Timestamp, timestamp)
	}

	inner, found := envelope.Dps["101"]
	if !found || len(envelope.Dps) != 1 {
		return mqttMismatch("RPC datapoint mismatch")
	}

	var request mqttRPCRequest

	requestErr := strictMQTTJSON([]byte(inner), &request)
	if requestErr != nil {
		return fmt.Errorf("mqtt replay I/O: %w", requestErr)
	}

	return matchMQTTRPCRequest(request, fixture)
}

func matchMQTTRPCRequest(request mqttRPCRequest, fixture mqttFixture) error {
	if request.ID != 1 || request.Method != fixture.Method || !equalMQTTJSON(request.Params, fixture.Params) {
		return mqttMismatch("RPC id, method, or params mismatch: %v", request)
	}

	if request.Security.Endpoint != fixture.SecurityEndpoint ||
		!regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(request.Security.Nonce) {
		return mqttMismatch("RPC security mismatch")
	}

	return nil
}

// Decode requests independently of the implementation; responses are static Python vectors.
func decryptMQTTRequest(frame []byte, localKey string) ([]byte, uint32, error) {
	const headerSize = 19

	validationErr := validateMQTTDeviceFrame(frame, "1.0", 2)
	if validationErr != nil {
		return nil, 0, validationErr
	}

	size := int(binary.BigEndian.Uint16(frame[17:19]))
	timestamp := binary.BigEndian.Uint32(frame[11:15])
	stamp := fmt.Sprintf("%08x", timestamp)
	reordered := string([]byte{stamp[5], stamp[6], stamp[3], stamp[7], stamp[1], stamp[2], stamp[0], stamp[4]})
	digest := md5.Sum([]byte(reordered + localKey + "TXdfu$jyZ#TZHsg4")) //nolint:gosec // Vendor V1 AES key derivation.

	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return nil, 0, fmt.Errorf("mqtt replay cipher: %w", err)
	}

	plaintext := make([]byte, size)
	for offset := 0; offset < size; offset += aes.BlockSize {
		block.Decrypt(plaintext[offset:offset+aes.BlockSize], frame[headerSize+offset:headerSize+offset+aes.BlockSize])
	}

	payload, err := unpadMQTTRequest(plaintext)

	return payload, timestamp, err
}

func validateMQTTDeviceFrame(frame []byte, version string, sequence uint32) error {
	const (
		headerSize   = 19
		checksumSize = 4
	)

	if len(frame) < headerSize+checksumSize {
		return mqttMismatch("truncated device frame")
	}

	if string(frame[:3]) != version || binary.BigEndian.Uint32(frame[3:7]) != sequence ||
		binary.BigEndian.Uint16(frame[15:17]) != 101 {
		return mqttMismatch("device frame metadata mismatch")
	}

	size := int(binary.BigEndian.Uint16(frame[17:19]))
	if size == 0 || size%aes.BlockSize != 0 || len(frame) != headerSize+size+checksumSize {
		return mqttMismatch("device frame size mismatch")
	}

	end := headerSize + size
	if crc32.ChecksumIEEE(frame[:end]) != binary.BigEndian.Uint32(frame[end:]) {
		return mqttMismatch("device frame checksum mismatch")
	}

	return nil
}

func unpadMQTTRequest(plaintext []byte) ([]byte, error) {
	size := len(plaintext)

	padding := int(plaintext[size-1])
	if padding < 1 || padding > aes.BlockSize {
		return nil, mqttMismatch("invalid device frame padding")
	}

	if !bytes.Equal(plaintext[size-padding:], bytes.Repeat([]byte{byte(padding)}, padding)) {
		return nil, mqttMismatch("invalid device frame padding")
	}

	return plaintext[:size-padding], nil
}

func readMQTTExchange(reader io.Reader, expected byte) ([]byte, error) {
	var octet [1]byte

	_, headerErr := io.ReadFull(reader, octet[:])
	if headerErr != nil {
		return nil, fmt.Errorf("mqtt replay read: %w", headerErr)
	}

	if octet[0] != expected {
		return nil, mqttMismatch("unexpected MQTT frame: %x, expected %x", octet[0], expected)
	}

	size := 0
	multiplier := 1

	for range 4 {
		_, lengthErr := io.ReadFull(reader, octet[:])
		if lengthErr != nil {
			return nil, fmt.Errorf("mqtt replay read: %w", lengthErr)
		}

		size += int(octet[0]&127) * multiplier
		if size > 1<<20 {
			return nil, mqttMismatch("oversized MQTT replay frame")
		}

		if octet[0]&128 == 0 {
			body := make([]byte, size)

			_, err := io.ReadFull(reader, body)
			if err != nil {
				return nil, fmt.Errorf("mqtt replay body: %w", err)
			}

			return body, nil
		}

		multiplier *= 128
	}

	return nil, mqttMismatch("invalid MQTT remaining length")
}

func takeMQTTString(body []byte) (string, []byte, error) {
	if len(body) < 2 {
		return "", nil, mqttMismatch("truncated MQTT string")
	}

	size := int(binary.BigEndian.Uint16(body[:2]))
	if len(body) < size+2 {
		return "", nil, mqttMismatch("truncated MQTT string payload")
	}

	return string(body[2 : size+2]), body[size+2:], nil
}

func mqttReplayString(value string) []byte {
	// Fixture strings are bounded to a uint16 by construction.
	length := len(value)

	return append([]byte{byte((length >> 8) & 255), byte(length & 255)}, []byte(value)...)
}

func mqttReplayPacket(header byte, body []byte) []byte {
	result := []byte{header}

	for size := len(body); ; size /= 128 {
		octet := byte(size % 128)
		if size >= 128 {
			octet |= 128
		}

		result = append(result, octet)

		if size < 128 {
			break
		}
	}

	return append(result, body...)
}

func strictMQTTJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	err := decoder.Decode(target)
	if err != nil {
		return fmt.Errorf("mqtt replay JSON: %w", err)
	}

	return nil
}

func equalMQTTJSON(left, right []byte) bool {
	var first, second any

	return json.Unmarshal(left, &first) == nil && json.Unmarshal(right, &second) == nil && reflect.DeepEqual(first, second)
}

func TestMQTTReplayRejectsConnectMismatch(t *testing.T) {
	t.Parallel()
	fixture := loadMQTTFixture(t)

	valid := append(mqttReplayString("MQTT"), 4, 0xc2, 0, 45)
	valid = append(valid, mqttReplayString("0123456789abcdef")...)
	valid = append(valid, mqttReplayString(fixture.Username)...)
	valid = append(valid, mqttReplayString(fixture.Password)...)
	cases := map[string][]byte{
		"flags":          append([]byte(nil), valid...),
		"keepalive":      append([]byte(nil), valid...),
		"client ID":      append([]byte(nil), valid...),
		"credentials":    append([]byte(nil), valid...),
		"trailing bytes": append(append([]byte(nil), valid...), 0),
	}
	cases["flags"][7] = 0
	cases["keepalive"][9] = 0
	cases["client ID"][12] = 'z'

	cases["credentials"][len(valid)-1] ^= 1

	err := matchMQTTConnect(valid, fixture)
	if err != nil {
		t.Fatal(err)
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := matchMQTTConnect(body, fixture)
			if err == nil {
				t.Fatal("accepted mismatched CONNECT")
			}
		})
	}
}

func TestMQTTReplayRejectsUnexpectedFrame(t *testing.T) {
	t.Parallel()

	for _, header := range []byte{0x10, 0x30, 0xc0, 0xe0} {
		t.Run(fmt.Sprintf("%02x", header), func(t *testing.T) {
			t.Parallel()

			_, err := readMQTTExchange(bytes.NewReader([]byte{header, 0}), 0x82)
			if err == nil {
				t.Fatal("accepted unexpected or duplicate MQTT frame")
			}
		})
	}
}

type mqttReplayError string

const errMQTTReplayMismatch mqttReplayError = "MQTT replay mismatch"

func (e mqttReplayError) Error() string { return string(e) }
func mqttMismatch(format string, values ...any) error {
	return fmt.Errorf("%w: %s", errMQTTReplayMismatch, fmt.Sprintf(format, values...))
}
