package mqtt

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const (
	hubTestWait = 3 * time.Second
	hubDeviceA  = "device-a"
)

// hubBroker is a synthetic broker for any number of connections. It answers
// every RPC with the device identifier taken from the request topic, so a reply
// that reaches the wrong device is visible.
type hubBroker struct {
	mu           sync.Mutex
	dials        int
	connects     int
	refuse       byte
	dialFailure  error
	subscribed   map[string]int
	unsubscribed []string
	servers      []net.Conn
	ended        int
	pings        int
}

func newHubBroker() *hubBroker {
	return &hubBroker{
		mu: sync.Mutex{}, dials: 0, connects: 0, refuse: 0, dialFailure: nil,
		subscribed: make(map[string]int), unsubscribed: nil, servers: nil, ended: 0, pings: 0,
	}
}

func (b *hubBroker) dial(_ context.Context, network, address string) (net.Conn, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.dials++

	if b.dialFailure != nil {
		return nil, b.dialFailure
	}

	if network != "tcp" || address != "broker.example:8883" {
		return nil, fmt.Errorf("%w: dial mismatch", errSyntheticBroker)
	}

	client, server := net.Pipe()
	b.servers = append(b.servers, server)

	go b.serve(server)

	return client, nil
}

// hubCounts is a point-in-time copy of what the broker has seen.
type hubCounts struct {
	dials, connects, ended, pings int
	subscribed                    map[string]int
	unsubscribed                  []string
}

func (b *hubBroker) snapshot() hubCounts {
	b.mu.Lock()
	defer b.mu.Unlock()

	subscribed := make(map[string]int, len(b.subscribed))
	maps.Copy(subscribed, b.subscribed)

	return hubCounts{
		dials: b.dials, connects: b.connects, ended: b.ended, pings: b.pings,
		subscribed: subscribed, unsubscribed: append([]string(nil), b.unsubscribed...),
	}
}

func (b *hubBroker) dialCount() int { return b.snapshot().dials }

func (b *hubBroker) endedCount() int { return b.snapshot().ended }

func (b *hubBroker) pingCount() int { return b.snapshot().pings }

// reply writes one packet to the client, wrapping a transport failure.
func reply(server net.Conn, data []byte) error {
	_, err := server.Write(data)
	if err != nil {
		return fmt.Errorf("%w: write: %w", errSyntheticBroker, err)
	}

	return nil
}

func (b *hubBroker) set(update func(*hubBroker)) {
	b.mu.Lock()
	defer b.mu.Unlock()

	update(b)
}

// dropConnections closes every server socket as a network fault would.
func (b *hubBroker) dropConnections() {
	b.mu.Lock()
	servers := append([]net.Conn(nil), b.servers...)
	b.mu.Unlock()

	for _, server := range servers {
		_ = server.Close()
	}
}

func (b *hubBroker) serve(server net.Conn) {
	defer func() {
		_ = server.Close()

		b.mu.Lock()
		b.ended++
		b.mu.Unlock()
	}()

	for {
		header, body, err := readPacket(server)
		if err != nil {
			return
		}

		err = b.handle(server, header, body)
		if err != nil {
			return
		}
	}
}

func (b *hubBroker) handle(server net.Conn, header byte, body []byte) error {
	switch header {
	case protocol.MQTTConnect:
		b.mu.Lock()
		b.connects++
		refuse := b.refuse
		b.mu.Unlock()

		return reply(server, packet(protocol.MQTTConnAck, []byte{0, refuse}))
	case protocol.MQTTSubscribe:
		length := int(binary.BigEndian.Uint16(body[2:4]))
		topic := string(body[4 : 4+length])

		b.mu.Lock()
		b.subscribed[topic]++
		b.mu.Unlock()

		return reply(server, packet(protocol.MQTTSubAck, []byte{body[0], body[1], 0}))
	case protocol.MQTTUnsubscribe:
		length := int(binary.BigEndian.Uint16(body[2:4]))

		b.mu.Lock()
		b.unsubscribed = append(b.unsubscribed, string(body[4:4+length]))
		b.mu.Unlock()

		return reply(server, packet(protocol.MQTTUnsubAck, body[:2]))
	case protocol.MQTTPingReq:
		b.mu.Lock()
		b.pings++
		b.mu.Unlock()

		return reply(server, packet(protocol.MQTTPingResp, nil))
	case protocol.MQTTPublish:
		return b.answer(server, body)
	default:
		return fmt.Errorf("%w: unexpected packet %d", errSyntheticBroker, header)
	}
}

func (b *hubBroker) answer(server net.Conn, body []byte) error {
	length := int(binary.BigEndian.Uint16(body[:2]))
	requestTopic := string(body[2 : 2+length])

	frame, _, err := decodeFrame(body[2+length:], syntheticKey)
	if err != nil {
		return err
	}

	command, err := request(frame)
	if err != nil {
		return err
	}

	segments := strings.Split(requestTopic, "/")
	device := segments[len(segments)-1]
	replyTopic := strings.Replace(requestTopic, "/i/", "/o/", 1)

	value, err := json.Marshal(device)
	if err != nil {
		return fmt.Errorf("%w: encode: %w", errSyntheticBroker, err)
	}

	if command.Method == "silent" {
		// A stalled subscription: the request is published and no reply ever comes.
		return nil
	}

	if command.Method == "poison" {
		frame.Version = protocol.MQTTVersionA01
	}

	return broker{connection: server, topic: replyTopic}.replyRPC(frame, command.Id, value, replyTopic)
}

func hubDeviceConfig(device string) Config {
	config := testConfig(protocol.MQTTVersionV1)
	config.DeviceID = device

	return config
}

func hubOpen(t *testing.T, hub *Hub, device string) *Session {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), hubTestWait)
	defer cancel()

	session, err := hub.Open(ctx, hubDeviceConfig(device))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = session.Close() })

	return session
}

func hubCall(t *testing.T, session *Session, expectedDevice string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), hubTestWait)
	defer cancel()

	result, err := session.Call(ctx, "get_status", nil)
	if err != nil {
		t.Fatal(err)
	}

	if string(result) != quoted(expectedDevice) {
		t.Fatalf("reply for %s routed as %s", expectedDevice, result)
	}
}

func quoted(value string) string { return `"` + value + `"` }

func eventually(t *testing.T, description string, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(hubTestWait)

	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", description)
		}

		time.Sleep(2 * time.Millisecond)
	}
}

func TestHubSharesOneConnectionAcrossDevices(t *testing.T) {
	t.Parallel()

	source := newHubBroker()
	hub := NewHub(source.dial)

	t.Cleanup(hub.Close)

	devices := []string{hubDeviceA, "device-b"}
	sessions := []*Session{hubOpen(t, hub, devices[0]), hubOpen(t, hub, devices[1])}

	var group sync.WaitGroup

	for round := range 20 {
		index := round % 2

		group.Add(1)

		go func() {
			defer group.Done()

			hubCall(t, sessions[index], devices[index])
		}()
	}

	group.Wait()

	seen := source.snapshot()
	dials, connects, subscribed := seen.dials, seen.connects, seen.subscribed

	if dials != 1 || connects != 1 {
		t.Fatalf("dials=%d connects=%d, expected one shared connection", dials, connects)
	}

	if len(subscribed) != 2 {
		t.Fatalf("subscriptions: %v", subscribed)
	}
}

func TestHubConcurrentFirstOpensConnectOnce(t *testing.T) {
	t.Parallel()

	source := newHubBroker()
	hub := NewHub(source.dial)

	t.Cleanup(hub.Close)

	var group sync.WaitGroup

	for range 8 {
		group.Add(1)

		go func() {
			defer group.Done()

			hubCall(t, hubOpen(t, hub, hubDeviceA), hubDeviceA)
		}()
	}

	group.Wait()

	seen := source.snapshot()
	dials, subscribed := seen.dials, seen.subscribed

	if dials != 1 || len(subscribed) != 1 {
		t.Fatalf("dials=%d subscriptions=%v, expected one of each", dials, subscribed)
	}

	for topic, count := range subscribed {
		if count != 1 {
			t.Fatalf("%s subscribed %d times", topic, count)
		}
	}
}

func TestHubBorrowsOfOneDeviceShareState(t *testing.T) {
	t.Parallel()

	source := newHubBroker()
	hub := NewHub(source.dial)

	t.Cleanup(hub.Close)

	first, second := hubOpen(t, hub, hubDeviceA), hubOpen(t, hub, hubDeviceA)

	// Per-device gates and request correlation must be shared by every borrow.
	if first.device != second.device || first.a01Gate != second.a01Gate || first.mapGate != second.mapGate {
		t.Fatal("borrows of one device do not share its gates")
	}

	_ = first.Close()

	select {
	case <-first.Done():
	default:
		t.Fatal("closed borrow stayed open")
	}

	_ = assertErrorKind(t, first.Err(), roborockerrors.Closed)

	hubCall(t, second, hubDeviceA)

	seen := source.snapshot()
	dials, subscribed := seen.dials, seen.subscribed

	if dials != 1 || len(subscribed) != 1 {
		t.Fatalf("dials=%d subscriptions=%v", dials, subscribed)
	}
}

func TestHubIdleDeviceIsUnsubscribedAndLastDeviceClosesConnection(t *testing.T) {
	t.Parallel()

	source := newHubBroker()
	hub := NewHub(source.dial, WithIdleTimeout(40*time.Millisecond))

	t.Cleanup(hub.Close)

	first, second := hubOpen(t, hub, hubDeviceA), hubOpen(t, hub, "device-b")
	hubCall(t, first, hubDeviceA)
	hubCall(t, second, "device-b")

	_ = first.Close()

	eventually(t, "idle device unsubscription", func() bool {
		unsubscribed := source.snapshot().unsubscribed

		return len(unsubscribed) == 1 && strings.HasSuffix(unsubscribed[0], "/device-a")
	})

	// The remaining device is untouched and the connection stays open.
	hubCall(t, second, "device-b")

	ended := source.endedCount()
	if ended != 0 {
		t.Fatal("connection closed while a device was still subscribed")
	}

	_ = second.Close()

	eventually(t, "connection close with the last subscription", func() bool {
		ended := source.endedCount()

		return ended == 1
	})

	reopened := hubOpen(t, hub, hubDeviceA)
	hubCall(t, reopened, hubDeviceA)

	dials := source.dialCount()
	if dials != 2 {
		t.Fatalf("dials=%d, expected a new connection after the idle close", dials)
	}
}

func TestHubReborrowCancelsIdleCountdown(t *testing.T) {
	t.Parallel()

	source := newHubBroker()
	hub := NewHub(source.dial, WithIdleTimeout(150*time.Millisecond))

	t.Cleanup(hub.Close)

	_ = hubOpen(t, hub, hubDeviceA).Close()

	again := hubOpen(t, hub, hubDeviceA)

	time.Sleep(300 * time.Millisecond)

	hubCall(t, again, hubDeviceA)

	seen := source.snapshot()
	dials, ended, subscribed, unsubscribed := seen.dials, seen.ended, seen.subscribed, seen.unsubscribed

	if dials != 1 || ended != 0 || len(unsubscribed) != 0 || len(subscribed) != 1 {
		t.Fatalf("dials=%d ended=%d unsubscribed=%v subscribed=%v", dials, ended, unsubscribed, subscribed)
	}
}

func TestHubReconnectsImmediatelyAfterADroppedConnection(t *testing.T) {
	t.Parallel()

	source := newHubBroker()
	hub := NewHub(source.dial, WithReconnectBackoff(time.Hour, time.Hour))

	t.Cleanup(hub.Close)

	session := hubOpen(t, hub, hubDeviceA)
	hubCall(t, session, hubDeviceA)

	source.dropConnections()

	select {
	case <-session.Done():
	case <-time.After(hubTestWait):
		t.Fatal("borrow did not observe the dropped connection")
	}

	_ = assertErrorKind(t, session.Err(), roborockerrors.Unavailable)

	// One dropped connection must not stall the next command behind a backoff.
	replacement := hubOpen(t, hub, hubDeviceA)
	hubCall(t, replacement, hubDeviceA)

	dials := source.dialCount()
	if dials != 2 {
		t.Fatalf("dials=%d, expected one reconnect", dials)
	}
}

func TestHubBacksOffAfterFailedConnects(t *testing.T) {
	t.Parallel()

	source := newHubBroker()
	source.set(func(b *hubBroker) { b.dialFailure = fmt.Errorf("%w: network down", errSyntheticBroker) })

	hub := NewHub(source.dial, WithReconnectBackoff(60*time.Millisecond, 100*time.Millisecond))

	t.Cleanup(hub.Close)

	open := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), hubTestWait)
		defer cancel()

		session, err := hub.Open(ctx, hubDeviceConfig(hubDeviceA))
		if err == nil {
			_ = session.Close()
		}

		return err
	}

	_ = assertErrorKind(t, open(), roborockerrors.Unavailable)

	// Inside the window the broker is not contacted at all.
	for range 5 {
		_ = assertErrorKind(t, open(), roborockerrors.Unavailable)
	}

	dials := source.dialCount()
	if dials != 1 {
		t.Fatalf("dials=%d inside the backoff window, expected 1", dials)
	}

	time.Sleep(80 * time.Millisecond)

	_ = assertErrorKind(t, open(), roborockerrors.Unavailable)

	dials = source.dialCount()
	if dials != 2 {
		t.Fatalf("dials=%d after the first backoff, expected 2", dials)
	}

	// The second delay grows by 1.5x (90 ms) and is capped at 100 ms, so a
	// connect 60 ms later is still refused locally.
	time.Sleep(60 * time.Millisecond)

	_ = assertErrorKind(t, open(), roborockerrors.Unavailable)

	dials = source.dialCount()
	if dials != 2 {
		t.Fatalf("dials=%d inside the grown backoff window, expected 2", dials)
	}

	// Recovery resets the policy.
	source.set(func(b *hubBroker) { b.dialFailure = nil })
	time.Sleep(60 * time.Millisecond)

	err := open()
	if err != nil {
		t.Fatalf("connect after recovery: %v", err)
	}
}

func TestTimingBackoffGrowsAndCaps(t *testing.T) {
	t.Parallel()

	policy := defaultTiming()
	expected := []time.Duration{
		10 * time.Second, 15 * time.Second, 22500 * time.Millisecond, 33750 * time.Millisecond,
	}

	for index, want := range expected {
		if got := policy.backoff(index + 1); got != want {
			t.Fatalf("backoff(%d)=%v want %v", index+1, got, want)
		}
	}

	if got := policy.backoff(100); got != reconnectBackoffCap {
		t.Fatalf("backoff did not cap: %v", got)
	}
}

func TestHubUnauthorizedIsTerminalUntilNewCredentials(t *testing.T) {
	t.Parallel()

	source := newHubBroker()
	source.set(func(b *hubBroker) { b.refuse = protocol.MQTTNotAuthorized })

	hub := NewHub(source.dial)

	t.Cleanup(hub.Close)

	open := func(config Config) error {
		ctx, cancel := context.WithTimeout(context.Background(), hubTestWait)
		defer cancel()

		session, err := hub.Open(ctx, config)
		if err == nil {
			_ = session.Close()
		}

		return err
	}

	_ = assertErrorKind(t, open(hubDeviceConfig(hubDeviceA)), roborockerrors.Unauthorized)

	// The broker has recovered, but the refused credentials are never retried.
	source.set(func(b *hubBroker) { b.refuse = 0 })

	for _, device := range []string{hubDeviceA, "device-b", hubDeviceA} {
		_ = assertErrorKind(t, open(hubDeviceConfig(device)), roborockerrors.Unauthorized)
	}

	dials := source.dialCount()
	if dials != 1 {
		t.Fatalf("dials=%d, refused credentials reached the broker again", dials)
	}

	renewed := hubDeviceConfig(hubDeviceA)
	renewed.Secret = "renewed-secret"

	err := open(renewed)
	if err != nil {
		t.Fatalf("new credentials: %v", err)
	}

	dials = source.dialCount()
	if dials != 2 {
		t.Fatalf("dials=%d, new credentials did not connect", dials)
	}
}

func TestHubAccountsDoNotShareConnections(t *testing.T) {
	t.Parallel()

	source := newHubBroker()
	hub := NewHub(source.dial)

	t.Cleanup(hub.Close)

	other := hubDeviceConfig(hubDeviceA)
	other.User = "other-user"

	ctx, cancel := context.WithTimeout(context.Background(), hubTestWait)
	defer cancel()

	mine := hubOpen(t, hub, hubDeviceA)

	theirs, err := hub.Open(ctx, other)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = theirs.Close() })

	hubCall(t, mine, hubDeviceA)

	dials := source.dialCount()
	if dials != 2 {
		t.Fatalf("dials=%d, two accounts must not share a connection", dials)
	}
}

func TestHubRetiringOneDeviceLeavesTheConnectionAndOthers(t *testing.T) {
	t.Parallel()

	source := newHubBroker()
	hub := NewHub(source.dial)

	t.Cleanup(hub.Close)

	bad, good := hubOpen(t, hub, hubDeviceA), hubOpen(t, hub, "device-b")

	ctx, cancel := context.WithTimeout(context.Background(), hubTestWait)
	defer cancel()

	// The reply carries a frame version the device was not opened with.
	_, _ = bad.Call(ctx, "poison", nil)

	select {
	case <-bad.Done():
	case <-time.After(hubTestWait):
		t.Fatal("malformed device traffic did not end that device's session")
	}

	hubCall(t, good, "device-b")

	ended := source.endedCount()
	if ended != 0 {
		t.Fatal("one device's protocol error closed the shared connection")
	}
}

func TestHubDeviceKeyedByProtocolAndLocalKey(t *testing.T) {
	t.Parallel()

	source := newHubBroker()
	hub := NewHub(source.dial)

	t.Cleanup(hub.Close)

	_ = hubOpen(t, hub, hubDeviceA)

	other := hubDeviceConfig(hubDeviceA)
	other.Protocol = protocol.MQTTVersionA01

	ctx, cancel := context.WithTimeout(context.Background(), hubTestWait)
	defer cancel()

	_, err := hub.Open(ctx, other)
	_ = assertErrorKind(t, err, roborockerrors.InvalidArgument)
}

func TestHubKeepaliveSendsPings(t *testing.T) {
	t.Parallel()

	source := newHubBroker()
	hub := NewHub(source.dial)
	hub.timing.ping = 10 * time.Millisecond

	t.Cleanup(hub.Close)

	_ = hubOpen(t, hub, hubDeviceA)

	eventually(t, "keepalive pings", func() bool {
		pings := source.pingCount()

		return pings >= 2
	})
}

func TestHubCloseEndsEveryBorrow(t *testing.T) {
	t.Parallel()

	source := newHubBroker()
	hub := NewHub(source.dial)
	session := hubOpen(t, hub, hubDeviceA)

	hub.Close()

	select {
	case <-session.Done():
	case <-time.After(hubTestWait):
		t.Fatal("hub close left a borrow open")
	}

	if session.Err() == nil {
		t.Fatal("closed borrow reports no error")
	}

	// A closed hub reconnects on demand.
	hubCall(t, hubOpen(t, hub, hubDeviceA), hubDeviceA)
}
