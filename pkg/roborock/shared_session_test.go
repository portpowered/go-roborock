package roborock_test

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/go-roborock/pkg/roborock"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const (
	sharedSessionLocalKey = "0123456789abcdef"
	packetConnect         = 0x10
	packetConnAck         = 0x20
	packetSubscribe       = 0x82
	packetSubAck          = 0x90
	packetUnsubscribe     = 0xa2
	packetUnsubAck        = 0xb0
	packetPingReq         = 0xc0
	packetPingResp        = 0xd0
	connAckRefused        = 5
)

// sessionBroker accepts MQTT connections and acknowledges control packets only.
type sessionBroker struct {
	mu     sync.Mutex
	dials  int
	ended  int
	refuse byte
}

func (b *sessionBroker) dial(context.Context, string, string) (net.Conn, error) {
	b.mu.Lock()
	b.dials++
	b.mu.Unlock()

	client, server := net.Pipe()

	go b.serve(server)

	return client, nil
}

func (b *sessionBroker) dialCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.dials
}

func (b *sessionBroker) endedCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.ended
}

func (b *sessionBroker) serve(server net.Conn) {
	defer func() {
		_ = server.Close()

		b.mu.Lock()
		b.ended++
		b.mu.Unlock()
	}()

	for {
		header := make([]byte, 2)

		_, err := io.ReadFull(server, header)
		if err != nil {
			return
		}

		body := make([]byte, header[1])

		_, err = io.ReadFull(server, body)
		if err != nil {
			return
		}

		var reply []byte

		switch header[0] {
		case packetConnect:
			reply = []byte{packetConnAck, 2, 0, b.refuse}
		case packetSubscribe:
			reply = []byte{packetSubAck, 3, body[0], body[1], 0}
		case packetUnsubscribe:
			reply = []byte{packetUnsubAck, 2, body[0], body[1]}
		case packetPingReq:
			reply = []byte{packetPingResp, 0}
		default:
			return
		}

		_, err = server.Write(reply)
		if err != nil {
			return
		}
	}
}

func sharedSessionClient(t *testing.T, broker *sessionBroker) *roborock.Client {
	t.Helper()

	client, err := roborock.NewClient(
		roborock.WithMQTTDial(broker.dial), roborock.WithMQTTIdleTimeout(40*time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = client.Close() })

	return client
}

func sharedSessionRequest(secret, device string) roborock.OpenDeviceRequest {
	return roborock.OpenDeviceRequest{
		Auth: roborock.AuthContext{
			Token: "token", DeviceIdentifier: "link", ClientID: "client", BaseURL: "https://iot.example.test",
			Mqtt: roborock.MQTTAuth{
				BrokerURL: "ssl://broker.example:8883", User: "user", Secret: secret, Key: "key",
				APIURL: "https://api.example.test", SigningKey: "signing",
			},
		},
		DeviceID: device, LocalKey: sharedSessionLocalKey, Protocol: roborock.ProtocolV1,
	}
}

func eventually(t *testing.T, description string, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)

	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", description)
		}

		time.Sleep(2 * time.Millisecond)
	}
}

func openShared(t *testing.T, client *roborock.Client, secret, device string) *roborock.DeviceSession {
	t.Helper()

	session, err := client.OpenDevice(t.Context(), sharedSessionRequest(secret, device))
	if err != nil {
		t.Fatal(err)
	}

	return session
}

func newSessionBroker(refuse byte) *sessionBroker {
	return &sessionBroker{mu: sync.Mutex{}, dials: 0, ended: 0, refuse: refuse}
}

func TestOpenDeviceSharesOneConnectionAndSiblingsSurviveClose(t *testing.T) {
	t.Parallel()

	broker := newSessionBroker(0)
	client := sharedSessionClient(t, broker)

	first := openShared(t, client, "secret", "device-a")
	second := openShared(t, client, "secret", "device-b")

	// Closing one device session releases its borrow and leaves the other open.
	err := first.Close()
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(120 * time.Millisecond)

	select {
	case <-second.Done():
		t.Fatalf("closing one device ended its sibling: %v", second.Err())
	default:
	}

	if broker.dialCount() != 1 || broker.endedCount() != 0 {
		t.Fatalf("dials=%d ended=%d, expected one open shared connection", broker.dialCount(), broker.endedCount())
	}
}

func TestOpenDeviceConnectionClosesWithItsLastSubscription(t *testing.T) {
	t.Parallel()

	broker := newSessionBroker(0)
	client := sharedSessionClient(t, broker)

	err := openShared(t, client, "secret", "device-a").Close()
	if err != nil {
		t.Fatal(err)
	}

	eventually(t, "the connection to close with its last subscription", func() bool {
		return broker.endedCount() == 1
	})

	err = openShared(t, client, "secret", "device-a").Close()
	if err != nil {
		t.Fatal(err)
	}

	if broker.dialCount() != 2 {
		t.Fatalf("dials=%d, expected a fresh connection after the idle close", broker.dialCount())
	}
}

func TestOpenDeviceUnauthorizedIsTerminalForTheCredentials(t *testing.T) {
	t.Parallel()

	broker := newSessionBroker(connAckRefused)
	client := sharedSessionClient(t, broker)

	for range 3 {
		_, err := client.OpenDevice(t.Context(), sharedSessionRequest("secret", "device-a"))

		var failure *roborockerrors.Error
		if !errors.As(err, &failure) || failure.Kind != roborockerrors.Unauthorized {
			t.Fatalf("expected an unauthorized failure, got %v", err)
		}
	}

	if broker.dialCount() != 1 {
		t.Fatalf("dials=%d, the refused credentials were retried", broker.dialCount())
	}

	// New credentials form a new account and connect again.
	broker.mu.Lock()
	broker.refuse = 0
	broker.mu.Unlock()

	err := openShared(t, client, "renewed", "device-a").Close()
	if err != nil {
		t.Fatal(err)
	}
}
