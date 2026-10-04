package replay_test

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" //nolint:gosec // The independent A01 oracle derives the vendor IV with MD5.
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/portpowered/go-roborock/pkg/dependencies/mqtt"
)

type mqttA01Response struct {
	Payload  string `json:"payload"`
	FrameHex string `json:"frameHex"`
}

type mqttA01Fixture struct {
	Provenance string                  `json:"provenance"`
	Source     string                  `json:"source"`
	Revision   string                  `json:"revision"`
	Datapoints []int                   `json:"datapoints"`
	Result     map[int]json.RawMessage `json:"result"`
	Responses  []mqttA01Response       `json:"responses"`
}

func TestPairedMQTTA01FragmentedQuery(t *testing.T) {
	t.Parallel()
	account := loadMQTTFixture(t)
	fixture := loadMQTTA01Fixture(t)
	client, broker := net.Pipe()

	t.Cleanup(func() { _ = client.Close(); _ = broker.Close() })

	deadline := time.Now().Add(5 * time.Second)

	err := broker.SetDeadline(deadline)
	if err != nil {
		t.Fatal(err)
	}

	completed := make(chan error, 1)

	go func() {
		completed <- replayMQTTA01(broker, account, fixture)

		_ = broker.Close()
	}()

	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	session, err := mqtt.Open(ctx, mqtt.Config{
		BrokerURL: account.Broker, User: account.User, Secret: account.Secret, Key: account.Key,
		DeviceID: account.DeviceID, LocalKey: account.LocalKey, Protocol: "A01",
	}, func(_ context.Context, network, address string) (net.Conn, error) {
		if network != mqttReplayNetwork || address != mqttReplayAddress {
			return nil, mqttMismatch("unexpected dial")
		}

		return client, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = session.Close() })

	result, err := session.QueryA01(ctx, fixture.Datapoints)
	if err != nil {
		t.Fatal(err)
	}

	assertMQTTA01Result(t, result, fixture.Result)

	err = session.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = <-completed
	if err != nil {
		t.Fatal(err)
	}
}

func assertMQTTA01Result(t *testing.T, result, expected map[int]json.RawMessage) {
	t.Helper()

	if len(result) != len(expected) {
		t.Fatalf("incomplete merged response: %v", result)
	}

	for key, want := range expected {
		if !equalMQTTJSON(result[key], want) {
			t.Fatalf("datapoint %d: %s", key, result[key])
		}
	}
}

func loadMQTTA01Fixture(t *testing.T) mqttA01Fixture {
	t.Helper()

	data, err := os.ReadFile("fixtures/mqtt/synthetic/session-a01.json")
	if err != nil {
		t.Fatal(err)
	}

	var fixture mqttA01Fixture

	err = json.Unmarshal(data, &fixture)
	if err != nil {
		t.Fatal(err)
	}

	if fixture.Provenance != accountFixtureProvenance ||
		fixture.Source != "reference-derived python-roborock A01 protocol padding and create_mqtt_encoder" ||
		fixture.Revision != mqttReferenceRevision {
		t.Fatal("missing reference provenance")
	}

	return fixture
}

func replayMQTTA01(conn net.Conn, account mqttFixture, fixture mqttA01Fixture) error {
	handshakeErr := replayMQTTHandshake(conn, account)
	if handshakeErr != nil {
		return handshakeErr
	}

	body, err := readMQTTExchange(conn, 0x30)
	if err != nil {
		return err
	}

	topic, frame, err := takeMQTTString(body)
	if err != nil {
		return err
	}

	if topic != account.PublishTopic {
		return mqttMismatch("A01 publish topic mismatch")
	}

	queryErr := matchMQTTA01Query(frame, account.LocalKey, fixture.Datapoints)
	if queryErr != nil {
		return queryErr
	}

	for _, response := range fixture.Responses {
		err := sendMQTTA01Response(conn, account.SubscribeTopic, response.FrameHex)
		if err != nil {
			return err
		}
	}

	var extra [1]byte

	_, err = conn.Read(extra[:])
	if !errors.Is(err, io.EOF) {
		return mqttMismatch("unexpected MQTT data after query: %x (%v)", extra, err)
	}

	return nil
}

func matchMQTTA01Query(frame []byte, key string, datapoints []int) error {
	validationErr := validateMQTTDeviceFrame(frame, "A01", 1)
	if validationErr != nil {
		return validationErr
	}

	random := binary.BigEndian.Uint32(frame[7:11])
	digest := md5.Sum([]byte(fmt.Sprintf("%08x", random) + "726f626f726f636b2d67a6d6da")) //nolint:gosec // Vendor A01 IV.
	initializationVector := []byte(hex.EncodeToString(digest[:])[8:24])

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return fmt.Errorf("A01 cipher: %w", err)
	}

	plaintext := make([]byte, len(frame)-23)
	cipher.NewCBCDecrypter(block, initializationVector).CryptBlocks(plaintext, frame[19:len(frame)-4])

	payload, err := unpadMQTTRequest(plaintext)
	if err != nil {
		return err
	}

	return matchMQTTA01Payload(payload, frame, datapoints)
}

func matchMQTTA01Payload(payload, frame []byte, datapoints []int) error {
	var envelope mqttRPCEnvelope

	envelopeErr := strictMQTTJSON(payload, &envelope)
	if envelopeErr != nil {
		return envelopeErr
	}

	query, found := envelope.Dps["10000"]

	expected, err := json.Marshal(datapoints)
	if err != nil {
		return fmt.Errorf("A01 query expectation: %w", err)
	}

	if !found || len(envelope.Dps) != 1 || query != string(expected) {
		return mqttMismatch("A01 query must contain one string datapoint: %s", payload)
	}

	now := time.Now().Unix()

	timestamp := int64(binary.BigEndian.Uint32(frame[11:15]))

	if envelope.Timestamp < now-5 || envelope.Timestamp > now+5 ||
		timestamp < envelope.Timestamp ||
		timestamp > envelope.Timestamp+1 {
		return mqttMismatch("A01 timestamp mismatch")
	}

	return nil
}

func sendMQTTA01Response(conn net.Conn, topic, frameHex string) error {
	frame, err := hex.DecodeString(frameHex)
	if err != nil {
		return fmt.Errorf("reference A01 frame: %w", err)
	}

	body := append(mqttReplayString(topic), frame...)

	_, err = conn.Write(mqttReplayPacket(0x30, body))
	if err != nil {
		return fmt.Errorf("A01 response: %w", err)
	}

	return nil
}
