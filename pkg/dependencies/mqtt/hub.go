package mqtt

import (
	"context"
	"crypto/md5" //nolint:gosec // Roborock's mandated account authentication uses MD5.
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/portpowered/go-roborock/internal/protocol"
)

const (
	commandTimeout     = 10 * time.Second
	handshakeTimeout   = 10 * time.Second
	unsubscribeTimeout = 2 * time.Second
	responseTimeout    = 60 * time.Second
	maxPendingCommands = 64

	// pingInterval stays below the 45 s MQTT keepalive the connection announces.
	pingInterval = 30 * time.Second
	// idleTimeout follows python-roborock: a device topic is dropped after 60 s
	// without a borrower, and the connection goes with the last topic.
	idleTimeout = 60 * time.Second
	// The reconnect backoff follows python-roborock's start and multiplier. Its 6 h
	// ceiling is lowered because reconnects here are demand driven: a long
	// ceiling would refuse a user's command for hours after one outage.
	reconnectBackoffStart      = 10 * time.Second
	reconnectBackoffMultiplier = 1.5
	reconnectBackoffCap        = 5 * time.Minute
)

// Config binds a device session to one account and device encryption key.
type Config struct {
	BrokerURL string
	User      string
	Secret    string
	Key       string
	DeviceID  string
	LocalKey  string
	// Protocol is "1.0" (vacuum/camera), "A01" (wet cleaner/washer), or "B01" (Q7/Q10).
	Protocol string
}

// DialFunc replaces the socket creation, including TLS, for offline replay.
type DialFunc func(context.Context, string, string) (net.Conn, error)

type timing struct {
	idle         time.Duration
	ping         time.Duration
	backoffStart time.Duration
	backoffCap   time.Duration
	multiplier   float64
}

func defaultTiming() timing {
	return timing{
		idle: idleTimeout, ping: pingInterval,
		backoffStart: reconnectBackoffStart, backoffCap: reconnectBackoffCap, multiplier: reconnectBackoffMultiplier,
	}
}

// backoff returns the delay after the given number of consecutive failed connects.
func (t timing) backoff(failures int) time.Duration {
	delay := float64(t.backoffStart)

	for range failures - 1 {
		delay *= t.multiplier
		if delay >= float64(t.backoffCap) {
			return t.backoffCap
		}
	}

	return min(time.Duration(delay), t.backoffCap)
}

// accountKey identifies the credentials one broker connection authenticates with.
// New credentials for the same user form a new account.
type accountKey struct{ broker, user, secret, key string }

// Hub shares one broker connection per account among all of that account's
// devices. It is safe for concurrent use; the zero value is not usable.
type Hub struct {
	dial     DialFunc
	timing   timing
	mu       sync.Mutex
	accounts map[accountKey]*account
}

// HubOption adjusts the timing policy of a hub.
type HubOption func(*timing)

// WithIdleTimeout sets how long a device subscription outlives its last borrower
// before it is dropped; the connection closes with the account's last subscription.
func WithIdleTimeout(idle time.Duration) HubOption {
	return func(t *timing) { t.idle = idle }
}

// WithReconnectBackoff sets the delay after the first failed connect and its
// ceiling. Each further consecutive failure multiplies the delay by 1.5.
func WithReconnectBackoff(start, ceiling time.Duration) HubOption {
	return func(t *timing) { t.backoffStart, t.backoffCap = start, ceiling }
}

// NewHub creates a hub that dials through dial, or verified TLS when dial is nil.
func NewHub(dial DialFunc, options ...HubOption) *Hub {
	if dial == nil {
		dial = defaultDial
	}

	policy := defaultTiming()

	for _, option := range options {
		option(&policy)
	}

	return &Hub{dial: dial, timing: policy, mu: sync.Mutex{}, accounts: make(map[accountKey]*account)}
}

// Open borrows a device session on the account's shared connection, connecting and
// subscribing when needed. The context governs establishment only. Close returns
// the borrow; the subscription lingers for the idle timeout and the connection
// closes with the account's last subscription. Rejected credentials are terminal
// for the account: every later Open with the same credentials returns the same
// Unauthorized error without contacting the broker.
func (h *Hub) Open(ctx context.Context, config Config) (*Session, error) {
	endpoint, err := validateConfig(config)
	if err != nil {
		return nil, transportError("open", err)
	}

	if config.Protocol == "" {
		config.Protocol = protocol.MQTTVersionV1
	}

	return h.account(config, endpoint).open(ctx, config)
}

// Close ends every connection and device session. The hub stays usable and
// reconnects on the next Open; terminal credential refusals are kept.
func (h *Hub) Close() {
	h.mu.Lock()
	accounts := make([]*account, 0, len(h.accounts))

	for _, held := range h.accounts {
		accounts = append(accounts, held)
	}

	h.mu.Unlock()

	for _, held := range accounts {
		held.close()
	}
}

func (h *Hub) account(config Config, endpoint *url.URL) *account {
	key := accountKey{broker: config.BrokerURL, user: config.User, secret: config.Secret, key: config.Key}

	h.mu.Lock()
	defer h.mu.Unlock()

	if held := h.accounts[key]; held != nil {
		return held
	}

	// New credentials replace a refused set for the same user and broker.
	for other, held := range h.accounts {
		if other.broker == key.broker && other.user == key.user && held.refused() {
			delete(h.accounts, other)
		}
	}

	username, password := mqttCredentials(config)
	created := newAccount(h, endpoint, username, password)
	h.accounts[key] = created

	return created
}

func mqttCredentials(config Config) (string, string) {
	userInput := fmt.Sprintf(protocol.MQTTAccountDigestInputFormat, config.User, config.Key)
	secretInput := fmt.Sprintf(protocol.MQTTAccountDigestInputFormat, config.Secret, config.Key)
	userHash := md5.Sum([]byte(userInput))     //nolint:gosec // Vendor username derivation.
	secretHash := md5.Sum([]byte(secretInput)) //nolint:gosec // Vendor password derivation.
	username := hex.EncodeToString(userHash[:])[protocol.MQTTUsernameDigestHexStart:protocol.MQTTUsernameDigestHexEnd]
	password := hex.EncodeToString(secretHash[:])[protocol.MQTTPasswordDigestHexStart:]

	return username, password
}
