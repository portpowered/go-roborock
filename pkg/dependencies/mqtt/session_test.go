package mqtt

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
)

const syntheticKey = "0123456789abcdef"

var errSyntheticBroker = errors.New("synthetic MQTT broker mismatch")

func testConfig(version string) Config {
	return Config{
		BrokerURL: "ssl://broker.example:8883",
		User:      "synthetic-user", Secret: "synthetic-secret", Key: "synthetic-key",
		DeviceID: "synthetic-device", LocalKey: syntheticKey, Protocol: version,
	}
}

type broker struct {
	connection net.Conn
	topic      string
}

func startBroker(connection net.Conn) (broker, error) {
	header, body, err := readPacket(connection)

	if err != nil {
		return broker{}, fmt.Errorf("synthetic broker exchange: %w", err)
	}

	if header != protocol.MQTTConnect || len(body) < 10 || !bytes.Equal(body[:10], []byte{0, 4, 'M', 'Q', 'T', 'T', 4, 194, 0, 45}) {
		return broker{}, fmt.Errorf("%w: CONNECT mismatch", errSyntheticBroker)
	}

	if _, err = connection.Write(packet(protocol.MQTTConnAck, []byte{0, 0})); err != nil {
		return broker{}, fmt.Errorf("synthetic broker exchange: %w", err)
	}

	header, body, err = readPacket(connection)

	if err != nil {
		return broker{}, fmt.Errorf("synthetic broker exchange: %w", err)
	}

	if header != protocol.MQTTSubscribe || len(body) < 5 || binary.BigEndian.Uint16(body[:2]) != 1 || body[len(body)-1] != 0 {
		return broker{}, fmt.Errorf("%w: SUBSCRIBE mismatch", errSyntheticBroker)
	}

	length := int(binary.BigEndian.Uint16(body[2:4]))

	if len(body) != 5+length {
		return broker{}, fmt.Errorf("%w: topic mismatch", errSyntheticBroker)
	}

	subscribed := string(body[4 : 4+length])

	if _, err = connection.Write(packet(protocol.MQTTSubAck, []byte{0, 1, 0})); err != nil {
		return broker{}, fmt.Errorf("synthetic broker exchange: %w", err)
	}

	return broker{connection: connection, topic: subscribed}, nil
}

func (activeBroker broker) command() (deviceFrame, error) {
	header, body, err := readPacket(activeBroker.connection)

	if err != nil {
		return deviceFrame{}, err
	}

	if header != protocol.MQTTPublish || len(body) < 2 {
		return deviceFrame{}, fmt.Errorf("%w: PUBLISH mismatch", errSyntheticBroker)
	}

	length := int(binary.BigEndian.Uint16(body[:2]))

	if length > len(body)-2 {
		return deviceFrame{}, fmt.Errorf("%w: PUBLISH topic truncated", errSyntheticBroker)
	}

	expected := bytes.Replace([]byte(activeBroker.topic), []byte("/o/"), []byte("/i/"), 1)

	if !bytes.Equal(body[2:2+length], expected) {
		return deviceFrame{}, fmt.Errorf("%w: PUBLISH account/device topic mismatch", errSyntheticBroker)
	}

	frame, consumed, err := decodeFrame(body[2+length:], syntheticKey)

	if err != nil {
		return frame, err
	}

	if consumed != len(body)-2-length || frame.protocol != protocol.MQTTProtocolRequest {
		return frame, fmt.Errorf("%w: unexpected frame", errSyntheticBroker)
	}

	return frame, nil
}

func (activeBroker broker) reply(frame deviceFrame, payload []byte, topic string) error {
	frame.protocol = protocol.MQTTProtocolResponse
	frame.payload = payload
	encoded, err := encodeFrame(frame, syntheticKey)

	if err != nil {
		return err
	}

	_, err = activeBroker.connection.Write(publishPacket(topic, encoded))

	return err
}

func openTestSession(t *testing.T, version string, exchange func(broker) error) (*Session, <-chan error) {
	t.Helper()
	client, server := net.Pipe()
	completed := make(chan error, 1)
	go func() {
		defer func() { _ = server.Close() }()
		active, err := startBroker(server)

		if err == nil {
			err = exchange(active)
		}

		completed <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := Open(ctx, testConfig(version), func(_ context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != "broker.example:8883" {
			return nil, fmt.Errorf("%w: dial mismatch", errSyntheticBroker)
		}

		return client, nil
	})

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = session.Close() })

	return session, completed
}

func (activeBroker broker) replyRPC(frame deviceFrame, id int64, value json.RawMessage, topic string) error {
	inner, err := json.Marshal(dependencymodels.MQTTRPCResponse{Id: id, Result: &value, Error: nil})

	if err != nil {
		return fmt.Errorf("encode synthetic RPC response: %w", err)
	}

	encoded, err := json.Marshal(string(inner))

	if err != nil {
		return fmt.Errorf("encode synthetic response string: %w", err)
	}

	payload, err := json.Marshal(dependencymodels.MQTTEnvelope{
		Dps: map[string]json.RawMessage{protocol.MQTTRPCResponseDatapoint: encoded}, T: 1700000000,
	})

	if err != nil {
		return fmt.Errorf("encode synthetic response envelope: %w", err)
	}

	return activeBroker.reply(frame, payload, topic)
}

func request(frame deviceFrame) (dependencymodels.MQTTRPCRequest, error) {
	var envelope dependencymodels.MQTTEnvelope

	var result dependencymodels.MQTTRPCRequest

	if err := json.Unmarshal(frame.payload, &envelope); err != nil {
		return result, fmt.Errorf("decode synthetic request: %w", err)
	}

	var inner string

	if err := json.Unmarshal(envelope.Dps[protocol.MQTTRPCRequestDatapoint], &inner); err != nil {
		return result, fmt.Errorf("decode synthetic request: %w", err)
	}

	if err := json.Unmarshal([]byte(inner), &result); err != nil {
		return result, fmt.Errorf("decode synthetic RPC: %w", err)
	}

	return result, nil
}

func TestSessionRPCDeviceAndRequestCorrelation(t *testing.T) {
	t.Parallel()
	session, completed := openTestSession(t, "1.0", func(activeBroker broker) error {
		frame, err := activeBroker.command()

		if err != nil {
			return err
		}

		command, err := request(frame)

		if err != nil {
			return err
		}

		nonce, nonceErr := hex.DecodeString(command.Security.Nonce)

		if command.Security.Endpoint != "goOmJ7S+" || len(nonce) != 16 || nonceErr != nil || hex.EncodeToString(nonce) != command.Security.Nonce {
			return fmt.Errorf("%w: RPC security mismatch", errSyntheticBroker)
		}

		if command.Method != "get_status" || string(command.Params) != "[]" {
			return fmt.Errorf("%w: RPC request mismatch", errSyntheticBroker)
		}

		if err = activeBroker.replyRPC(frame, command.Id, json.RawMessage(`"wrong-device"`), activeBroker.topic+"-other"); err != nil {
			return err
		}

		if err = activeBroker.replyRPC(frame, command.Id+1, json.RawMessage(`"wrong-id"`), activeBroker.topic); err != nil {
			return err
		}

		return activeBroker.replyRPC(frame, command.Id, json.RawMessage(`[{"battery":81}]`), activeBroker.topic)
	})
	result, err := session.Call(context.Background(), "get_status", nil)

	if err != nil || string(result) != `[{"battery":81}]` {
		t.Fatalf("result=%s error=%v", result, err)
	}

	if err = <-completed; err != nil {
		t.Fatal(err)
	}
}

func TestSessionA01CollectsPartialResponses(t *testing.T) {
	t.Parallel()
	session, completed := openTestSession(t, "A01", func(activeBroker broker) error {
		frame, err := activeBroker.command()

		if err != nil {
			return err
		}

		if !bytes.Contains(frame.payload, []byte(`"10000":"[201,202]"`)) {
			return fmt.Errorf("%w: unexpected query %s", errSyntheticBroker, frame.payload)
		}

		if err = activeBroker.reply(frame, []byte(`{"dps":{"201":42,"999":0},"t":1700000000}`), activeBroker.topic); err != nil {
			return err
		}

		return activeBroker.reply(frame, []byte(`{"dps":{"202":"ready"},"t":1700000000}`), activeBroker.topic)
	})
	values, err := session.QueryA01(context.Background(), []int{201, 202})

	if err != nil || len(values) != 2 || string(values[201]) != "42" || string(values[202]) != `"ready"` {
		t.Fatalf("values=%v error=%v", values, err)
	}

	if err = <-completed; err != nil {
		t.Fatal(err)
	}
}

func TestSessionA01SetOnlyPublishes(t *testing.T) {
	t.Parallel()
	session, completed := openTestSession(t, "A01", func(activeBroker broker) error {
		frame, err := activeBroker.command()

		if err != nil {
			return err
		}

		if !bytes.Contains(frame.payload, []byte(`"201":3`)) {
			return fmt.Errorf("%w: incorrect datapoint", errSyntheticBroker)
		}

		return nil
	})

	if err := session.SetA01(context.Background(), map[int]json.RawMessage{201: json.RawMessage("3")}); err != nil {
		t.Fatal(err)
	}

	if err := <-completed; err != nil {
		t.Fatal(err)
	}
}

func TestSessionCanceledRPCAndConcurrentClose(t *testing.T) {
	t.Parallel()
	published := make(chan struct{})
	session, completed := openTestSession(t, "1.0", func(activeBroker broker) error {
		_, err := activeBroker.command()

		if err != nil {
			return err
		}

		close(published)
		_, _, err = readPacket(activeBroker.connection)

		if errors.Is(err, io.EOF) {
			return nil
		}

		return err
	})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, err := session.Call(ctx, "app_start", nil); result <- err }()
	<-published
	cancel()

	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}

	var wait sync.WaitGroup

	for range 16 {
		wait.Add(1)
		go func() { defer wait.Done(); _ = session.Close() }()
	}

	wait.Wait()

	if err := <-completed; err != nil {
		t.Fatal(err)
	}

	_, err := session.Call(context.Background(), "get_status", nil)

	if !errors.Is(err, ErrClosed) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected closed, got %v", err)
	}
}

func TestSessionConcurrentRPCCorrelation(t *testing.T) {
	t.Parallel()
	const commands = 8
	session, completed := openTestSession(t, "1.0", func(activeBroker broker) error {
		frames := make([]deviceFrame, commands)
		requests := make([]dependencymodels.MQTTRPCRequest, commands)

		for index := range commands {
			frame, err := activeBroker.command()

			if err != nil {
				return err
			}

			frames[index] = frame
			requests[index], err = request(frame)

			if err != nil {
				return err
			}
		}

		for index := commands - 1; index >= 0; index-- {
			if err := activeBroker.replyRPC(frames[index], requests[index].Id, requests[index].Params, activeBroker.topic); err != nil {
				return err
			}
		}

		return nil
	})

	var wait sync.WaitGroup

	for index := range commands {
		wait.Add(1)
		go func() {
			defer wait.Done()
			params := json.RawMessage(fmt.Sprintf("[%d]", index))
			result, err := session.Call(context.Background(), "get_status", params)

			if err != nil || !bytes.Equal(result, params) {
				t.Errorf("result=%s params=%s error=%v", result, params, err)
			}
		}()
	}

	wait.Wait()

	if err := <-completed; err != nil {
		t.Fatal(err)
	}
}

func TestOpenCancellationDuringHandshake(t *testing.T) {
	t.Parallel()
	client, server := net.Pipe()
	defer func() { _ = server.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := Open(ctx, testConfig("1.0"), func(context.Context, string, string) (net.Conn, error) { return client, nil })

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
}

func TestOpenRejectsPlaintext(t *testing.T) {
	t.Parallel()
	config := testConfig("1.0")
	config.BrokerURL = "tcp://broker.example:1883"
	_, err := Open(context.Background(), config, func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("unexpected dial")
		return nil, nil
	})

	if err == nil {
		t.Fatal("plaintext accepted")
	}
}
