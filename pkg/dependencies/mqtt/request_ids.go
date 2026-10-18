package mqtt

import (
	"math/rand/v2"
	"sync"
	"time"

	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

// Use the reference V1 request range, which also fits binary map replies.
// https://github.com/Python-roborock/python-roborock/blob/main/roborock/protocols/v1_protocol.py
const (
	v1RequestIDMinimum = 10000
	v1RequestIDMaximum = 32767
	v1RequestIDCount   = v1RequestIDMaximum - v1RequestIDMinimum + 1
)

type requestIDLease struct {
	active bool
	after  time.Time
}

type requestIDPool struct {
	mu     sync.Mutex
	next   int
	leases [v1RequestIDCount]requestIDLease
}

// Same-device MQTT sessions receive the same response topic. IDs therefore
// belong to the process, not a connection. Randomize only the initial cursor
// to reduce collisions with another process; leases provide local uniqueness.
var v1RequestIDs = requestIDPool{ //nolint:gochecknoglobals // Shared correlation ownership across sessions.
	mu: sync.Mutex{}, next: rand.IntN(v1RequestIDCount), //nolint:gosec // Cursor is not a security boundary.
	leases: [v1RequestIDCount]requestIDLease{},
}

func (pool *requestIDPool) acquire() (int64, func(), error) {
	return pool.acquireAt(time.Now())
}

func (pool *requestIDPool) acquireAt(now time.Time) (int64, func(), error) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	for range v1RequestIDCount {
		index := pool.next
		pool.next = (index + 1) % v1RequestIDCount
		lease := &pool.leases[index]

		if lease.active || now.Before(lease.after) {
			continue
		}

		lease.active = true

		return int64(index + v1RequestIDMinimum), sync.OnceFunc(func() { pool.release(index) }), nil
	}

	return 0, nil, roborockerrors.New(roborockerrors.Backpressure, "mqtt call", "no available request identifier", nil)
}

func (pool *requestIDPool) release(index int) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	// Quarantine after completion, cancellation or failure, including connection
	// close. This mitigates delayed replies for responseTimeout; it is not a
	// guarantee about broker retention or other SDK processes.
	pool.leases[index] = requestIDLease{active: false, after: time.Now().Add(responseTimeout)}
}
