package mqtt

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const lifecycleTestTimeout = 3 * time.Second

type heartbeatExchange struct {
	RequestHex  string `json:"requestHex"`
	ResponseHex string `json:"responseHex"`
}

type heartbeatExpiry struct {
	RequestHex string `json:"requestHex"`
	NoResponse bool   `json:"noResponse"`
	ErrorKind  string `json:"errorKind"`
}

type heartbeatFixture struct {
	Provenance string              `json:"provenance"`
	Source     string              `json:"source"`
	Exchanges  []heartbeatExchange `json:"exchanges"`
	Expiry     heartbeatExpiry     `json:"expiry"`
}

func loadHeartbeat(t *testing.T) heartbeatFixture {
	t.Helper()

	data, err := os.ReadFile("../../../tests/replay/fixtures/mqtt/synthetic/heartbeat.json")
	if err != nil {
		t.Fatal(err)
	}

	var fixture heartbeatFixture

	err = json.Unmarshal(data, &fixture)
	if err != nil {
		t.Fatal(err)
	}

	if fixture.Provenance != "synthetic" || fixture.Source == "" || len(fixture.Exchanges) != 2 ||
		!fixture.Expiry.NoResponse || fixture.Expiry.ErrorKind != string(roborockerrors.Timeout) {
		t.Fatal("invalid synthetic heartbeat transcript")
	}

	return fixture
}

func heartbeatBytes(t *testing.T, encoded string) []byte {
	t.Helper()

	wire, err := hex.DecodeString(encoded)
	if err != nil || len(wire) != 2 {
		t.Fatalf("invalid synthetic heartbeat bytes %q: %v", encoded, err)
	}

	return wire
}

func expectPing(t *testing.T, server net.Conn, ticks chan<- time.Time, requestHex string) {
	t.Helper()

	expected := heartbeatBytes(t, requestHex)
	wire := make([]byte, len(expected))

	ticks <- time.Now()

	_, err := io.ReadFull(server, wire)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(wire, expected) {
		t.Fatalf("unexpected PINGREQ bytes: %x expected %x", wire, expected)
	}
}

type deadlineConnection struct {
	net.Conn

	armed  chan time.Time
	closes atomic.Int32
}

func (connection *deadlineConnection) SetReadDeadline(deadline time.Time) error {
	err := connection.Conn.SetReadDeadline(deadline)
	if err != nil {
		return fmt.Errorf("set synthetic read deadline: %w", err)
	}

	connection.armed <- deadline

	return nil
}

func (connection *deadlineConnection) Close() error {
	connection.closes.Add(1)

	err := connection.Conn.Close()
	if err != nil {
		return fmt.Errorf("close synthetic deadline connection: %w", err)
	}

	return nil
}

func controlledSession(t *testing.T) (*Session, net.Conn, chan time.Time, *deadlineConnection) {
	t.Helper()

	client, server := net.Pipe()
	connection := &deadlineConnection{Conn: client, armed: make(chan time.Time, 4), closes: atomic.Int32{}}

	session, _, _, err := newSession(connection, testConfig(protocol.MQTTVersionV1))
	if err != nil {
		t.Fatal(err)
	}

	ticks := make(chan time.Time, 1)

	go session.readLoop(context.Background())
	go session.keepaliveTicks(context.Background(), ticks)

	t.Cleanup(func() { _ = server.Close(); closeLifecycle(t, session) })

	err = server.SetDeadline(time.Now().Add(lifecycleTestTimeout))
	if err != nil {
		t.Fatal(err)
	}

	return session, server, ticks, connection
}

func awaitLifecycle[T any](t *testing.T, events <-chan T, result *T) {
	t.Helper()

	select {
	case event := <-events:
		if result != nil {
			*result = event
		}
	case <-time.After(lifecycleTestTimeout):
		t.Fatal("synthetic lifecycle exchange timed out")
	}
}

func closeLifecycle(t *testing.T, session *Session) {
	t.Helper()

	closed := make(chan error, 1)
	go func() { closed <- session.Close() }()

	var err error

	awaitLifecycle(t, closed, &err)

	if err != nil {
		t.Fatal(err)
	}

	awaitLifecycle(t, session.stopped, nil)
	awaitLifecycle(t, session.keepaliveStopped, nil)
}

// This synthetic ordered transcript rejects any unexpected bytes without a fallback pong.
func TestSessionPingPongTranscript(t *testing.T) {
	t.Parallel()

	session, server, ticks, connection := controlledSession(t)
	fixture := loadHeartbeat(t)
	awaitLifecycle(t, connection.armed, nil)

	for _, exchange := range fixture.Exchanges {
		expectPing(t, server, ticks, exchange.RequestHex)

		_, err := server.Write(heartbeatBytes(t, exchange.ResponseHex))
		if err != nil {
			t.Fatal(err)
		}

		awaitLifecycle(t, connection.armed, nil)
		assertSessionClosed(t, session, false)
	}

	closeLifecycle(t, session)

	_, _, err := readPacket(server)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("unexpected frame after transcript: %v", err)
	}
}

func TestSessionHeartbeatExpiry(t *testing.T) {
	t.Parallel()

	session, server, ticks, connection := controlledSession(t)
	fixture := loadHeartbeat(t)

	var deadline time.Time

	awaitLifecycle(t, connection.armed, &deadline)

	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > responseTimeout {
		t.Fatalf("unexpected heartbeat deadline: %v", remaining)
	}

	expectPing(t, server, ticks, fixture.Expiry.RequestHex)

	// Expire the real socket's armed read deadline deterministically, without waiting for policy time.
	err := connection.Conn.SetReadDeadline(time.Now().Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}

	awaitLifecycle(t, session.Done(), nil)
	terminal := session.Err()
	_ = assertErrorKind(t, terminal, roborockerrors.Timeout)

	var cause net.Error
	if !errors.As(terminal, &cause) || !cause.Timeout() {
		t.Fatalf("read timeout cause lost: %v", terminal)
	}

	closeLifecycle(t, session)
	_ = assertErrorKind(t, session.Err(), roborockerrors.Timeout)

	if count := connection.closes.Load(); count != 1 {
		t.Fatalf("heartbeat expiry closed socket %d times", count)
	}

	_, _, err = readPacket(server)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("socket remained open after expiry: %v", err)
	}
}

func queuePressureExchange(accepted chan<- struct{}) func(broker) error {
	return func(active broker) error {
		identifiers := make(map[int64]struct{}, maxPendingCommands)

		for range maxPendingCommands {
			frame, err := active.command()
			if err != nil {
				return err
			}

			command, err := request(frame)
			if err != nil {
				return err
			}

			_, duplicate := identifiers[command.Id]
			if duplicate || !validRPCRequest(command) {
				return fmt.Errorf("%w: duplicate or invalid queued RPC", errSyntheticBroker)
			}

			identifiers[command.Id] = struct{}{}

			accepted <- struct{}{}
		}

		// EOF is the only legal next exchange: a 65th PUBLISH is a transcript mismatch.
		return waitForBrokerClose(active)
	}
}

func TestSessionRPCQueuePressure(t *testing.T) {
	t.Parallel()
	exerciseQueuePressure(t, false)
}

func TestSessionRPCQueueCancellation(t *testing.T) {
	t.Parallel()
	exerciseQueuePressure(t, true)
}

func exerciseQueuePressure(t *testing.T, cancelPending bool) {
	t.Helper()

	accepted := make(chan struct{}, maxPendingCommands)
	session, completed := openTestSession(t, protocol.MQTTVersionV1, queuePressureExchange(accepted))

	ctx, cancel := context.WithTimeout(context.Background(), lifecycleTestTimeout)
	defer cancel()

	results := make(chan error, maxPendingCommands)

	for range maxPendingCommands {
		go func() {
			_, err := session.Call(ctx, "get_status", nil)
			results <- err
		}()

		awaitLifecycle(t, accepted, nil)
	}

	_, err := session.Call(ctx, "get_status", nil)
	_ = assertErrorKind(t, err, roborockerrors.Backpressure)
	assertSessionClosed(t, session, false)

	expected := ErrClosed

	if cancelPending {
		cancel()

		expected = context.Canceled
	} else {
		closeLifecycle(t, session)
	}

	for range maxPendingCommands {
		var result error

		awaitLifecycle(t, results, &result)

		if !errors.Is(result, expected) {
			t.Fatalf("pending call terminal cause=%v expected %v", result, expected)
		}
	}

	closeLifecycle(t, session)

	awaitLifecycle(t, completed, &err)

	if err != nil {
		t.Fatalf("unexpected queue transcript: %v", err)
	}

	session.mu.Lock()
	pending := len(session.pending)
	session.mu.Unlock()

	if pending != 0 {
		t.Fatalf("close left %d pending commands", pending)
	}
}
