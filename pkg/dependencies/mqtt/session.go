package mqtt

import (
	"context"
	"crypto/md5" //nolint:gosec // Wire security endpoint uses MD5 bytes 8:14.
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
)

type response struct {
	value json.RawMessage
	err   error
}

// device is one device's subscription on an account connection. Every borrow of
// the device shares it, so request correlation, the A01 query gate and the map
// gate hold across borrowers.
type device struct {
	account        *account
	conn           *connection
	config         Config
	topic          string
	publishTopic   string
	subscribeTopic string
	security       dependencymodels.MQTTRPCSecurity
	ready          chan struct{}
	subErr         error
	a01Gate        chan struct{}
	mapGate        chan struct{}
	sequence       atomic.Uint32
	mu             sync.Mutex
	mapQuery       *mapQuery
	q10Query       *q10Query
	pending        map[int64]chan response
	a01            *a01Query
	handles        map[*Session]struct{}
	idle           *time.Timer
	idleSeq        uint64
	finished       bool
}

func newDevice(owner *account, conn *connection, config Config, topic string) *device {
	var nonce [protocol.MQTTSecurityNonceBytes]byte

	// A failed read leaves a zero nonce, which only weakens map replay protection
	// for this subscription; the device accepts any nonce.
	_, _ = rand.Read(nonce[:])

	endpointHash := md5.Sum([]byte(config.Key)) //nolint:gosec // Wire security endpoint uses MD5 bytes 8:14.

	return &device{
		account: owner, conn: conn, config: config, topic: topic,
		publishTopic:   fmt.Sprintf(protocol.MQTTPublishTopicFormat, config.User, owner.username, config.DeviceID),
		subscribeTopic: topic,
		security: dependencymodels.MQTTRPCSecurity{
			Endpoint: base64.StdEncoding.EncodeToString(
				endpointHash[protocol.MQTTSecurityEndpointDigestStart:protocol.MQTTSecurityEndpointDigestEnd]),
			Nonce: hex.EncodeToString(nonce[:]),
		},
		ready:    make(chan struct{}),
		subErr:   nil,
		a01Gate:  make(chan struct{}, 1),
		mapGate:  make(chan struct{}, 1),
		mu:       sync.Mutex{},
		pending:  make(map[int64]chan response),
		handles:  make(map[*Session]struct{}),
		sequence: atomic.Uint32{}, mapQuery: nil, q10Query: nil, a01: nil, idle: nil, idleSeq: 0, finished: false,
	}
}

// borrow adds a handle and cancels any idle countdown. The caller holds account.mu.
func (d *device) borrow() *Session {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.idle != nil {
		d.idle.Stop()
		d.idle = nil
	}

	d.idleSeq++

	handle := &Session{
		device: d, done: make(chan struct{}), closeOnce: sync.Once{}, failure: nil, failureMu: sync.Mutex{},
	}
	d.handles[handle] = struct{}{}

	return handle
}

// unborrow removes a handle and reports whether none remain. The caller holds account.mu.
func (d *device) unborrow(handle *Session) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.handles, handle)

	return len(d.handles) == 0 && !d.finished
}

func (d *device) armIdle(after time.Duration, expire func(uint64)) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.idleSeq++

	generation := d.idleSeq
	d.idle = time.AfterFunc(after, func() { expire(generation) })
}

func (d *device) idleCurrent(generation uint64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.idleSeq == generation && len(d.handles) == 0
}

func (d *device) subscribed(err error) {
	d.subErr = err
	close(d.ready)
}

// finish ends the subscription and every borrow of it, once.
func (d *device) finish(err error) {
	d.mu.Lock()

	if d.finished {
		d.mu.Unlock()

		return
	}

	d.finished = true

	if d.idle != nil {
		d.idle.Stop()
		d.idle = nil
	}

	handles := make([]*Session, 0, len(d.handles))
	for handle := range d.handles {
		handles = append(handles, handle)
	}

	d.mu.Unlock()

	for _, handle := range handles {
		handle.end(err)
	}
}

func (d *device) write(ctx context.Context, data []byte) error { return d.conn.write(ctx, data) }

// Session is one borrow of a device on its account's shared connection. Close
// returns the borrow; it does not close the connection.
type Session struct {
	*device

	done      chan struct{}
	closeOnce sync.Once
	failure   error
	failureMu sync.Mutex
}

// Done closes when the borrow is closed or the device's connection fails.
func (s *Session) Done() <-chan struct{} { return s.done }

// Err returns the terminal failure after Done closes, or nil while active.
func (s *Session) Err() error {
	select {
	case <-s.done:
		return s.closedError()
	default:
		return nil
	}
}

// Close returns the borrow and aborts its in-flight requests; it is idempotent.
// The device subscription stays for the idle timeout so a following borrow reuses it.
func (s *Session) Close() error {
	s.end(ErrClosed)
	s.account.release(s.device, s)

	return nil
}

func (s *Session) end(err error) {
	s.closeOnce.Do(func() {
		s.failureMu.Lock()
		s.failure = err
		s.failureMu.Unlock()
		close(s.done)
	})
}

func (s *Session) closedError() error {
	s.failureMu.Lock()
	defer s.failureMu.Unlock()

	return transportError("session", s.failure)
}

// fail retires the device for every borrower. Operations without request IDs
// use it so a late reply cannot satisfy a later request.
func (s *Session) fail(err error) { s.account.retire(s.device, err) }
