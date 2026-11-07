package mqtt

import (
	"net"
	"sync"
	"testing"
	"time"
)

// newTestHub returns a hub whose device subscriptions expire as soon as they are
// idle, so a test that closes its session also closes the synthetic connection.
func newTestHub(dial DialFunc) *Hub {
	hub := NewHub(dial)
	hub.timing.idle = time.Millisecond

	return hub
}

// adoptConnection installs socket as the account connection of config and borrows
// its device without a handshake or subscription. The caller starts the loops.
func adoptConnection(t *testing.T, hub *Hub, socket net.Conn, config Config) (*Session, *connection) {
	t.Helper()

	endpoint, err := validateConfig(config)
	if err != nil {
		t.Fatal(err)
	}

	owner := hub.account(config, endpoint)
	active := newConnection(owner, socket)
	held := newDevice(owner, active, config, owner.subscribeTopic(config))
	held.subscribed(nil)

	owner.mu.Lock()
	owner.conn = active
	owner.devices[held.topic] = held
	session := held.borrow()
	owner.mu.Unlock()

	return session, active
}

// bareSession is a session over an empty device, for tests of private helpers.
func bareSession() *Session {
	return &Session{
		device: new(device), done: nil, closeOnce: sync.Once{}, failure: nil, failureMu: sync.Mutex{},
	}
}
