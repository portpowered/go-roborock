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
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
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

func TestSessionCanceledB01MapRetiresConnection(t *testing.T) {
	t.Parallel()

	for _, operation := range []string{"map", "trace", "list", "q7"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()

			published := make(chan struct{})
			session, completed := openTestSession(t, "B01", func(active broker) error {
				_, err := active.command()
				if err != nil {
					return err
				}

				close(published)

				return waitForBrokerClose(active)
			})
			ctx, cancel := context.WithCancel(context.Background())

			result := make(chan error, 1)

			go func() { result <- canceledB01Operation(ctx, session, operation) }()

			<-published
			cancel()

			err := <-result
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel error=%v", err)
			}

			<-session.Done()

			if !errors.Is(session.Err(), context.Canceled) {
				t.Fatalf("terminal error=%v", session.Err())
			}

			var wait sync.WaitGroup
			for range 16 {
				wait.Add(1)

				go func() { defer wait.Done(); _ = session.Close() }()
			}

			wait.Wait()

			err = <-completed
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func canceledB01Operation(ctx context.Context, session *Session, operation string) error {
	switch operation {
	case "list":
		_, err := session.QueryMapListQ10(ctx)

		return err
	case "trace":
		_, err := session.FetchTraceQ10(ctx)

		return err
	case "q7":
		_, err := session.FetchMapQ7(ctx, 7, "synthetic-serial", "sc01")

		return err
	default:
		_, err := session.FetchMapQ10(ctx)

		return err
	}
}

func TestSessionB01RPCBackpressure(t *testing.T) {
	t.Parallel()

	session, completed := openTestSession(t, "B01", waitForBrokerClose)
	for requestID := int64(1); requestID <= maxPendingCommands; requestID++ {
		_, err := session.beginRPC(requestID)
		if err != nil {
			t.Fatal(err)
		}
	}

	_, err := session.CallB01(context.Background(), dependencymodels.ServiceGetMapList, json.RawMessage(`{}`))
	_ = assertErrorKind(t, err, roborockerrors.Backpressure)

	err = session.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = <-completed
	if err != nil {
		t.Fatal(err)
	}
}

func (activeBroker broker) mapPush(frame deviceFrame, payload []byte) error {
	frame.Protocol = protocol.MapsProtocolResponse
	frame.Payload = payload

	encoded, err := encodeFrame(frame, syntheticKey)
	if err != nil {
		return err
	}

	_, err = activeBroker.connection.Write(publishPacket(activeBroker.topic, encoded))
	if err != nil {
		return fmt.Errorf("write synthetic map push: %w", err)
	}

	return nil
}

func TestSessionQ10NextObservedPushHasNoCausalFreshness(t *testing.T) {
	t.Parallel()

	payload := []byte{protocol.MapsQ10CurrentPrefix, protocol.MapsQ10PrefixVersion, 'A'}

	session, completed := openTestSession(t, "B01", func(active broker) error {
		for range 2 {
			frame, err := active.command()
			if err != nil {
				return err
			}
			// A delayed duplicate of A is indistinguishable from B's response.
			err = active.mapPush(frame, payload)
			if err != nil {
				return err
			}
		}

		return waitForBrokerClose(active)
	})
	for range 2 {
		observed, err := session.FetchMapQ10(context.Background())
		if err != nil || !bytes.Equal(observed, payload) {
			t.Fatalf("observed=%x err=%v", observed, err)
		}
	}

	err := session.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = <-completed
	if err != nil {
		t.Fatal(err)
	}
}

func TestSessionQ10IgnoresOtherMapKinds(t *testing.T) {
	t.Parallel()

	want := []byte{protocol.MapsQ10CurrentPrefix, protocol.MapsQ10PrefixVersion, 'M'}
	session, completed := openTestSession(t, "B01", func(active broker) error {
		frame, err := active.command()
		if err != nil {
			return err
		}

		for _, payload := range [][]byte{
			{protocol.MapsQ10TracePrefix, protocol.MapsQ10PrefixVersion, 'T'}, {},
			{protocol.MapsQ10CurrentPrefix, 0, 'X'}, want,
		} {
			err = active.mapPush(frame, payload)
			if err != nil {
				return err
			}
		}

		return waitForBrokerClose(active)
	})

	observed, err := session.FetchMapQ10(context.Background())
	if err != nil || !bytes.Equal(observed, want) {
		t.Fatalf("observed=%x err=%v", observed, err)
	}

	err = session.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = <-completed
	if err != nil {
		t.Fatal(err)
	}
}

func TestSessionV1MapEndpointAndRequestCorrelation(t *testing.T) {
	t.Parallel()
	fixture := loadMapCryptoFixture(t)

	encrypted, err := hex.DecodeString(fixture.V1EncryptedMap)
	if err != nil {
		t.Fatal(err)
	}

	session, completed := openTestSession(t, "1.0", func(active broker) error {
		return sendV1MapOracle(active, encrypted)
	})
	// A fixed nonce enables a precomputed independent Python AES/gzip response.
	session.security.Nonce = hex.EncodeToString([]byte(syntheticKey))

	observed, err := session.FetchMapV1(context.Background())
	if err != nil || string(observed) != "synthetic-v1-map" {
		t.Fatalf("observed=%q err=%v", observed, err)
	}

	err = session.Close()
	if err != nil {
		t.Fatal(err)
	}

	err = <-completed
	if err != nil {
		t.Fatal(err)
	}
}

func sendV1MapOracle(active broker, encrypted []byte) error {
	frame, err := active.command()
	if err != nil {
		return err
	}

	command, err := request(frame)
	if err != nil {
		return err
	}

	if command.Method != protocol.MapsV1GetMethod || string(command.Params) != "[]" ||
		!validRPCSecurity(command.Security) {
		return fmt.Errorf("%w: V1 map request mismatch", errSyntheticBroker)
	}

	payload := make([]byte, protocol.MapsV1HeaderSize+len(encrypted))
	copy(payload, command.Security.Endpoint)
	requestID := uint16(command.Id) //nolint:gosec // FetchMapV1 bounds IDs to uint16.
	binary.LittleEndian.PutUint16(payload[protocol.MapsV1RequestIDOffset:protocol.MapsV1RequestIDEnd], requestID)
	copy(payload[protocol.MapsV1HeaderSize:], encrypted)
	wrongEndpoint := bytes.Clone(payload)
	wrongEndpoint[protocol.MapsV1EndpointOffset] = 'X'
	wrongID := bytes.Clone(payload)

	wrongID[protocol.MapsV1RequestIDOffset] ^= 1

	for _, push := range [][]byte{wrongEndpoint, wrongID, payload} {
		err = active.mapPush(frame, push)
		if err != nil {
			return err
		}
	}

	return waitForBrokerClose(active)
}

type broker struct {
	connection net.Conn
	topic      string
}

type countedConnection struct {
	net.Conn

	closes atomic.Int32
}

func (connection *countedConnection) Close() error {
	connection.closes.Add(1)

	err := connection.Conn.Close()
	if err != nil {
		return fmt.Errorf("close counted connection: %w", err)
	}

	return nil
}

func validConnect(header byte, body []byte) bool {
	return header == protocol.MQTTConnect && len(body) >= 10 &&
		bytes.Equal(body[:10], []byte{0, 4, 'M', 'Q', 'T', 'T', 4, 194, 0, 45})
}

func validSubscribe(header byte, body []byte) bool {
	return header == protocol.MQTTSubscribe && len(body) >= 5 &&
		binary.BigEndian.Uint16(body[:2]) == 1 && body[len(body)-1] == 0
}

func validRPCSecurity(security dependencymodels.MQTTRPCSecurity) bool {
	nonce, err := hex.DecodeString(security.Nonce)

	return security.Endpoint == "goOmJ7S+" && len(nonce) == 16 && err == nil &&
		hex.EncodeToString(nonce) == security.Nonce
}

func validRPCRequest(command dependencymodels.MQTTRPCRequest) bool {
	return command.Method == "get_status" && string(command.Params) == "[]" &&
		validRPCSecurity(command.Security)
}
func startBroker(connection net.Conn) (broker, error) {
	header, body, err := readPacket(connection)
	if err != nil {
		return broker{}, fmt.Errorf("synthetic broker exchange: %w", err)
	}

	if !validConnect(header, body) {
		return broker{}, fmt.Errorf("%w: CONNECT mismatch", errSyntheticBroker)
	}

	_, err = connection.Write(packet(protocol.MQTTConnAck, []byte{0, 0}))
	if err != nil {
		return broker{}, fmt.Errorf("synthetic broker exchange: %w", err)
	}

	header, body, err = readPacket(connection)
	if err != nil {
		return broker{}, fmt.Errorf("synthetic broker exchange: %w", err)
	}

	if !validSubscribe(header, body) {
		return broker{}, fmt.Errorf("%w: SUBSCRIBE mismatch", errSyntheticBroker)
	}

	length := int(binary.BigEndian.Uint16(body[2:4]))

	if len(body) != 5+length {
		return broker{}, fmt.Errorf("%w: topic mismatch", errSyntheticBroker)
	}

	subscribed := string(body[4 : 4+length])

	_, err = connection.Write(packet(protocol.MQTTSubAck, []byte{0, 1, 0}))
	if err != nil {
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

	if consumed != len(body)-2-length || frame.Protocol != protocol.MQTTProtocolRequest {
		return frame, fmt.Errorf("%w: unexpected frame", errSyntheticBroker)
	}

	return frame, nil
}

func (activeBroker broker) reply(frame deviceFrame, payload []byte, topic string) error {
	frame.Protocol = protocol.MQTTProtocolResponse
	frame.Payload = payload

	encoded, err := encodeFrame(frame, syntheticKey)
	if err != nil {
		return err
	}

	_, err = activeBroker.connection.Write(publishPacket(topic, encoded))
	if err != nil {
		return fmt.Errorf("publish synthetic response: %w", err)
	}

	return nil
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

	return activeBroker.replyRPCBody(frame, inner, topic)
}

func (activeBroker broker) replyRPCBody(frame deviceFrame, inner []byte, topic string) error {
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

	err := json.Unmarshal(frame.Payload, &envelope)
	if err != nil {
		return result, fmt.Errorf("decode synthetic request: %w", err)
	}

	var inner string

	err = json.Unmarshal(envelope.Dps[protocol.MQTTRPCRequestDatapoint], &inner)
	if err != nil {
		return result, fmt.Errorf("decode synthetic request: %w", err)
	}

	err = json.Unmarshal([]byte(inner), &result)
	if err != nil {
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

		if !validRPCRequest(command) {
			return fmt.Errorf("%w: RPC request mismatch", errSyntheticBroker)
		}

		err = activeBroker.replyRPC(frame, command.Id, json.RawMessage(`"wrong-device"`), activeBroker.topic+"-other")
		if err != nil {
			return err
		}

		err = activeBroker.replyRPC(frame, command.Id+1, json.RawMessage(`"wrong-id"`), activeBroker.topic)
		if err != nil {
			return err
		}

		return activeBroker.replyRPC(frame, command.Id, json.RawMessage(`[{"battery":81}]`), activeBroker.topic)
	})
	result, err := session.Call(context.Background(), "get_status", nil)

	if err != nil || string(result) != `[{"battery":81}]` {
		t.Fatalf("result=%s error=%v", result, err)
	}

	err = <-completed
	if err != nil {
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

		if !bytes.Contains(frame.Payload, []byte(`"10000":"[201,202]"`)) {
			return fmt.Errorf("%w: unexpected query %s", errSyntheticBroker, frame.Payload)
		}

		err = activeBroker.reply(frame, []byte(`{"dps":{"201":42,"999":0},"t":1700000000}`), activeBroker.topic)
		if err != nil {
			return err
		}

		return activeBroker.reply(frame, []byte(`{"dps":{"202":"ready"},"t":1700000000}`), activeBroker.topic)
	})
	values, err := session.QueryA01(context.Background(), []int{201, 202})

	if err != nil || len(values) != 2 || string(values[201]) != "42" || string(values[202]) != `"ready"` {
		t.Fatalf("values=%v error=%v", values, err)
	}

	err = <-completed
	if err != nil {
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

		if !bytes.Contains(frame.Payload, []byte(`"201":3`)) {
			return fmt.Errorf("%w: incorrect datapoint", errSyntheticBroker)
		}

		return nil
	})

	err := session.SetA01(context.Background(), map[int]json.RawMessage{201: json.RawMessage("3")})
	if err != nil {
		t.Fatal(err)
	}

	err = <-completed
	if err != nil {
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

	resultErr := <-result
	if !errors.Is(resultErr, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", resultErr)
	}

	var wait sync.WaitGroup

	for range 16 {
		wait.Add(1)

		go func() { defer wait.Done(); _ = session.Close() }()
	}

	wait.Wait()

	completionErr := <-completed
	if completionErr != nil {
		t.Fatal(completionErr)
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
			err := activeBroker.replyRPC(frames[index], requests[index].Id, requests[index].Params, activeBroker.topic)
			if err != nil {
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

	err := <-completed
	if err != nil {
		t.Fatal(err)
	}
}

func TestOpenCancellationDuringHandshake(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	counted := &countedConnection{Conn: client, closes: atomic.Int32{}}

	defer func() { _ = server.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := Open(ctx, testConfig("1.0"), func(context.Context, string, string) (net.Conn, error) { return counted, nil })

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}

	if count := counted.closes.Load(); count != 1 {
		t.Fatalf("handshake cancellation closed underlying connection %d times", count)
	}
}

func TestOpenRejectsPlaintext(t *testing.T) {
	t.Parallel()

	config := testConfig("1.0")
	config.BrokerURL = "tcp://broker.example:1883"

	_, err := Open(context.Background(), config, func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("unexpected dial")

		return nil, errSyntheticBroker
	})
	if err == nil {
		t.Fatal("plaintext accepted")
	}
}

func waitForBrokerClose(activeBroker broker) error {
	_, _, err := readPacket(activeBroker.connection)
	if errors.Is(err, io.EOF) {
		return nil
	}

	return err
}

func assertErrorKind(t *testing.T, err error, expected roborockerrors.Kind) *roborockerrors.Error {
	t.Helper()

	var typed *roborockerrors.Error
	if !errors.As(err, &typed) {
		t.Fatalf("expected typed %s failure, got %v", expected, err)
	}

	if typed.Kind != expected {
		t.Fatalf("expected %s failure, got %v", expected, typed)
	}

	return typed
}

func TestSessionRPCFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		body   string
		kind   roborockerrors.Kind
		code   int
		closes bool
	}{
		{name: "device rejection", body: `{"id":%d,"error":{"code":-10003,"message":"synthetic rejection"}}`,
			kind: roborockerrors.Protocol, code: -10003, closes: false},
		{name: "unsupported command", body: `{"id":%d,"result":"unknown_method"}`,
			kind: roborockerrors.Unsupported, code: 0, closes: false},
		{name: "escaped unsupported command", body: `{"id":%d,"result":"unknown_\u006dethod"}`,
			kind: roborockerrors.Unsupported, code: 0, closes: false},
		{name: "id without result", body: `{"id":%d}`, kind: roborockerrors.Protocol, code: 0, closes: true},
		{name: "null result", body: `{"id":%d,"result":null}`, kind: roborockerrors.Protocol, code: 0, closes: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			session, completed := openTestSession(t, "1.0", rpcFailureExchange(testCase.body))

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			_, err := session.Call(ctx, "get_status", nil)

			typed := assertErrorKind(t, err, testCase.kind)
			if typed.Code != testCase.code {
				t.Fatalf("code=%d expected=%d", typed.Code, testCase.code)
			}

			if testCase.code != 0 {
				var rejection *RPCError
				if !errors.As(err, &rejection) || rejection.Code != testCase.code {
					t.Fatalf("device error cause not preserved: %v", err)
				}
			}

			assertSessionClosed(t, session, testCase.closes)

			_ = session.Close()

			err = <-completed
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSessionCanceledA01ClosesSession(t *testing.T) {
	t.Parallel()

	published := make(chan struct{})
	session, completed := openTestSession(t, "A01", func(activeBroker broker) error {
		_, err := activeBroker.command()
		if err != nil {
			return err
		}

		close(published)

		return waitForBrokerClose(activeBroker)
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	result := make(chan error, 1)

	go func() {
		_, err := session.QueryA01(ctx, []int{201})
		result <- err
	}()

	select {
	case <-published:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	cancel()

	err := <-result
	_ = assertErrorKind(t, err, roborockerrors.Canceled)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation cause lost: %v", err)
	}

	select {
	case <-session.done:
	default:
		t.Fatal("canceled A01 query did not close session")
	}

	err = <-completed
	if err != nil {
		t.Fatal(err)
	}
}

func TestOpenRejectsUnauthorizedConnAck(t *testing.T) {
	t.Parallel()

	for _, code := range []byte{protocol.MQTTBadUsernamePassword, protocol.MQTTNotAuthorized} {
		t.Run(fmt.Sprintf("code-%d", code), func(t *testing.T) {
			t.Parallel()

			client, server := net.Pipe()
			completed := make(chan error, 1)

			go func() {
				defer func() { _ = server.Close() }()

				_, _, err := readPacket(server)
				if err == nil {
					_, err = server.Write(packet(protocol.MQTTConnAck, []byte{0, code}))
				}

				completed <- err
			}()

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			_, err := Open(ctx, testConfig("1.0"), func(context.Context, string, string) (net.Conn, error) {
				return client, nil
			})
			_ = assertErrorKind(t, err, roborockerrors.Unauthorized)

			err = <-completed
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSessionA01AcknowledgesQoSOne(t *testing.T) {
	t.Parallel()
	session, completed := openTestSession(t, "A01", func(activeBroker broker) error {
		frame, err := activeBroker.command()
		if err != nil {
			return err
		}

		frame.Protocol = protocol.MQTTProtocolResponse
		frame.Payload = []byte(`{"dps":{"201":42},"t":1700000000}`)

		encoded, err := encodeFrame(frame, syntheticKey)
		if err != nil {
			return err
		}

		body := append(mqttString(activeBroker.topic), 0, 37)
		body = append(body, encoded...)

		_, err = activeBroker.connection.Write(packet(protocol.MQTTPublish|2, body))
		if err != nil {
			return fmt.Errorf("send synthetic QoS1 response: %w", err)
		}

		header, ack, err := readPacket(activeBroker.connection)
		if err != nil {
			return err
		}

		if header != protocol.MQTTPubAck || !bytes.Equal(ack, []byte{0, 37}) {
			return fmt.Errorf("%w: QoS1 PUBACK mismatch", errSyntheticBroker)
		}

		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	values, err := session.QueryA01(ctx, []int{201})
	if err != nil || string(values[201]) != "42" {
		t.Fatalf("values=%v error=%v", values, err)
	}

	err = <-completed
	if err != nil {
		t.Fatal(err)
	}
}

func rpcFailureExchange(body string) func(broker) error {
	return func(activeBroker broker) error {
		frame, err := activeBroker.command()
		if err != nil {
			return err
		}

		command, err := request(frame)
		if err != nil {
			return err
		}

		inner := []byte(fmt.Sprintf(body, command.Id))

		err = activeBroker.replyRPCBody(frame, inner, activeBroker.topic)
		if err != nil {
			return err
		}

		return waitForBrokerClose(activeBroker)
	}
}

func assertSessionClosed(t *testing.T, session *Session, expected bool) {
	t.Helper()

	closed := false

	select {
	case <-session.done:
		closed = true
	default:
	}

	if closed != expected {
		t.Fatalf("session closed=%t expected=%t", closed, expected)
	}
}
