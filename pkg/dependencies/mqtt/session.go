package mqtt

import (
	"context"
	"encoding/json"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	commandTimeout     = 10 * time.Second
	handshakeTimeout   = 10 * time.Second
	pingInterval       = 20 * time.Second
	responseTimeout    = 60 * time.Second
	maxPendingCommands = 64
)

// Config binds a device session to one account and device encryption key.
type Config struct {
	BrokerURL string
	User      string
	Secret    string
	Key       string
	DeviceID  string
	LocalKey  string
	// Protocol is "1.0" (vacuum/camera) or "A01" (wet cleaner/washer).
	Protocol string
}

// DialFunc replaces the socket creation, including TLS, for offline replay.
type DialFunc func(context.Context, string, string) (net.Conn, error)

type response struct {
	value json.RawMessage
	err   error
}

// Session owns a single MQTT connection and all pending device requests.
type Session struct {
	conn             net.Conn
	config           Config
	publishTopic     string
	subscribeTopic   string
	done             chan struct{}
	stopped          chan struct{}
	keepaliveStopped chan struct{}
	closeOnce        sync.Once
	writeGate        chan struct{}
	a01Gate          chan struct{}
	sequence         atomic.Uint32
	mu               sync.Mutex
	pending          map[int64]chan response
	a01              *a01Query
	failure          error
	security         dependencymodels.MQTTRPCSecurity
}

// Done closes when the session ends or the connection fails.
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

// Close releases the connection and waits for owned goroutines; it is idempotent.
func (s *Session) Close() error {
	s.fail(ErrClosed)
	<-s.stopped
	<-s.keepaliveStopped

	return nil
}

func (s *Session) closedError() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return transportError("session", s.failure)
}

func (s *Session) fail(err error) {
	closeSocket := false

	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.failure = err
		s.mu.Unlock()
		close(s.done)

		closeSocket = true
	})

	if closeSocket {
		_ = s.conn.Close()
	}
}
