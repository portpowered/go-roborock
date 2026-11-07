package mqtt

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

var errConnectionReplaced = errors.New("account connection was replaced")

// account owns the connection, device subscriptions and reconnect state of one
// credential set. Lock order is account.mu, then device.mu.
type account struct {
	hub      *Hub
	endpoint *url.URL
	username string
	password string
	// gate serializes connection attempts so concurrent borrowers dial once.
	gate    chan struct{}
	mu      sync.Mutex
	conn    *connection
	devices map[string]*device
	// terminal is the broker's credential refusal; it ends the account for good.
	terminal error
	failures int
	retryAt  time.Time
	lastErr  error
}

func newAccount(owner *Hub, endpoint *url.URL, username, password string) *account {
	return &account{
		hub: owner, endpoint: endpoint, username: username, password: password,
		gate:    make(chan struct{}, 1),
		mu:      sync.Mutex{},
		conn:    nil,
		devices: make(map[string]*device),

		terminal: nil, failures: 0, retryAt: time.Time{}, lastErr: nil,
	}
}

func (a *account) refused() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.terminal != nil
}

func (a *account) subscribeTopic(config Config) string {
	return fmt.Sprintf(protocol.MQTTSubscribeTopicFormat, config.User, a.username, config.DeviceID)
}

// open borrows a device session. A connection that drops between being found
// and being used is replaced once at once, so a single dropped connection never
// surfaces as a failed command.
func (a *account) open(ctx context.Context, config Config) (*Session, error) {
	var last error

	for range 2 {
		conn, err := a.connection(ctx)
		if err != nil {
			return nil, err
		}

		session, err := a.attach(ctx, conn, config)
		if err == nil {
			return session, nil
		}

		select {
		case <-conn.done:
		default:
			return nil, err
		}

		if ctx.Err() != nil {
			return nil, err
		}

		last = err
	}

	return nil, last
}

// connection returns the live connection, dialing a new one when none exists.
// After failed attempts it refuses immediately until the backoff elapses.
func (a *account) connection(ctx context.Context) (*connection, error) {
	select {
	case a.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, transportError("connect", ctx.Err())
	}

	defer func() { <-a.gate }()

	a.mu.Lock()

	if a.terminal != nil {
		err := a.terminal
		a.mu.Unlock()

		return nil, err
	}

	if live := a.conn; live != nil && !live.finished() {
		a.mu.Unlock()

		return live, nil
	}

	if wait := time.Until(a.retryAt); wait > 0 {
		last := a.lastErr
		a.mu.Unlock()

		return nil, roborockerrors.Wrap(roborockerrors.Unavailable, "mqtt connect",
			fmt.Sprintf("broker reconnect is backing off for %s", wait.Round(time.Second)), last)
	}

	a.mu.Unlock()

	fresh, err := a.dial(ctx)

	a.mu.Lock()
	defer a.mu.Unlock()

	if err != nil {
		a.recordFailure(ctx, err)

		return nil, err
	}

	a.conn, a.failures, a.retryAt, a.lastErr = fresh, 0, time.Time{}, nil

	return fresh, nil
}

// recordFailure keeps a credential refusal terminal and spaces other failures out.
// A caller's own cancellation says nothing about the broker.
func (a *account) recordFailure(ctx context.Context, err error) {
	var failure *roborockerrors.Error
	if errors.As(err, &failure) && failure.Kind == roborockerrors.Unauthorized {
		a.terminal = err

		return
	}

	if ctx.Err() != nil {
		return
	}

	a.failures++
	a.retryAt = time.Now().Add(a.hub.timing.backoff(a.failures))
	a.lastErr = err
}

func (a *account) dial(ctx context.Context) (*connection, error) {
	opening, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	socket, err := a.hub.dial(opening, protocol.MQTTNetwork, a.endpoint.Host)
	if err != nil {
		return nil, transportError("connect", err)
	}

	if socket == nil {
		return nil, invalid("connect", "dialer returned no connection")
	}

	fresh := newConnection(a, socket)
	stop := context.AfterFunc(opening, func() { fresh.fail(opening.Err()) })

	defer stop()

	err = fresh.handshake(opening, a.username, a.password)
	if err != nil {
		fresh.fail(err)

		if opening.Err() != nil {
			err = opening.Err()
		}

		return nil, transportError("handshake", err)
	}

	if !stop() {
		fresh.fail(opening.Err())

		return nil, transportError("handshake", opening.Err())
	}

	owner := context.WithoutCancel(ctx)
	go fresh.readLoop(owner)
	go fresh.keepalive(owner)

	return fresh, nil
}

func (c *connection) finished() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *connection) handshake(ctx context.Context, username, password string) error {
	var client [protocol.MQTTClientRandomBytes]byte

	_, err := rand.Read(client[:])
	if err != nil {
		return transportError("client ID", err)
	}

	err = c.write(ctx, connectPacket(hex.EncodeToString(client[:]), username, password))
	if err != nil {
		return err
	}

	header, body, err := readPacket(c.conn)
	if err != nil {
		return err
	}

	return checkConnAck(header, body)
}

func defaultDial(ctx context.Context, network, address string) (net.Conn, error) {
	var config tls.Config

	config.MinVersion = tls.VersionTLS12
	dialer := tls.Dialer{NetDialer: nil, Config: &config}

	conn, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, transportError("connect", err)
	}

	return conn, nil
}

func checkConnAck(header byte, body []byte) error {
	if header != protocol.MQTTConnAck || len(body) != protocol.MQTTConnAckLength {
		return errBrokerRejectedMQTTConnection
	}

	code := body[protocol.MQTTConnAckReturnCodeOffset]
	if code == protocol.MQTTBadUsernamePassword || code == protocol.MQTTNotAuthorized {
		return roborockerrors.New(roborockerrors.Unauthorized, "mqtt handshake", "broker rejected credentials", nil)
	}

	if body[protocol.MQTTConnAckSessionOffset] != protocol.MQTTConnAckNoSession || code != protocol.MQTTConnAckAccepted {
		return errBrokerRejectedMQTTConnection
	}

	return nil
}

func checkSubAck(header byte, body []byte, expected uint16) error {
	if header != protocol.MQTTSubAck || len(body) != protocol.MQTTSubAckLength {
		return errBrokerRejectedMQTTSubscription
	}

	identifier := binary.BigEndian.Uint16(body[:protocol.MQTTUint16Size])
	if identifier != expected || body[protocol.MQTTUint16Size] != protocol.MQTTQoSAtMostOnce {
		return errBrokerRejectedMQTTSubscription
	}

	return nil
}

// attach joins or creates the device subscription on conn and returns a borrow.
func (a *account) attach(ctx context.Context, conn *connection, config Config) (*Session, error) {
	topic := a.subscribeTopic(config)

	a.mu.Lock()

	if a.conn != conn {
		a.mu.Unlock()

		return nil, errConnectionReplaced
	}

	held := a.devices[topic]
	created := held == nil

	if created {
		held = newDevice(a, conn, config, topic)
		a.devices[topic] = held
	} else if held.config.Protocol != config.Protocol || held.config.LocalKey != config.LocalKey {
		a.mu.Unlock()

		return nil, invalid("open", "device is already open with a different protocol or local key")
	}

	session := held.borrow()

	a.mu.Unlock()

	if created {
		err := conn.subscribe(ctx, topic)
		held.subscribed(err)

		if err != nil {
			a.drop(held, err, ctx.Err() != nil)

			return nil, err
		}

		return session, nil
	}

	select {
	case <-held.ready:
	case <-ctx.Done():
		_ = session.Close()

		return nil, transportError("subscribe", ctx.Err())
	}

	if held.subErr != nil {
		return nil, held.subErr
	}

	return session, nil
}

// deviceFor finds the live subscription that owns topic on conn.
func (a *account) deviceFor(conn *connection, topic string) *device {
	a.mu.Lock()
	defer a.mu.Unlock()

	if held := a.devices[topic]; held != nil && held.conn == conn {
		return held
	}

	return nil
}

// release returns one borrow. The last borrow starts the idle countdown.
func (a *account) release(held *device, session *Session) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !held.unborrow(session) || a.devices[held.topic] != held {
		return
	}

	held.armIdle(a.hub.timing.idle, func(generation uint64) { a.expire(held, generation) })
}

func (a *account) expire(held *device, generation uint64) {
	a.mu.Lock()

	if a.devices[held.topic] != held || !held.idleCurrent(generation) {
		a.mu.Unlock()

		return
	}

	a.mu.Unlock()
	a.drop(held, ErrClosed, true)
}

// retire ends one device's subscription because its traffic can no longer be
// trusted; other devices on the connection continue.
func (a *account) retire(held *device, err error) { a.drop(held, err, true) }

// drop removes a subscription. The connection ends with the account's last one.
func (a *account) drop(held *device, err error, unsubscribe bool) {
	a.mu.Lock()

	if a.devices[held.topic] != held {
		a.mu.Unlock()
		held.finish(err)

		return
	}

	delete(a.devices, held.topic)

	conn := held.conn
	last := len(a.devices) == 0 && a.conn == conn

	a.mu.Unlock()

	held.finish(err)

	switch {
	case last:
		conn.fail(ErrClosed)
	case unsubscribe:
		go conn.unsubscribe(held.topic)
	}
}

// lost fails every subscription of a connection that ended.
func (a *account) lost(conn *connection) {
	a.mu.Lock()

	if a.conn == conn {
		a.conn = nil
	}

	var victims []*device

	for topic, held := range a.devices {
		if held.conn == conn {
			victims = append(victims, held)

			delete(a.devices, topic)
		}
	}

	a.mu.Unlock()

	err := conn.closedError()

	for _, held := range victims {
		held.finish(err)
	}
}

// close ends the connection and all device sessions of the account.
func (a *account) close() {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()

	if conn == nil {
		return
	}

	conn.fail(ErrClosed)
	<-conn.stopped
	<-conn.keepaliveStopped
}
