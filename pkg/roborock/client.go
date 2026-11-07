package roborock

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/portpowered/go-roborock/internal/protocol"

	"github.com/portpowered/go-roborock/pkg/dependencies/mqtt"
	"github.com/portpowered/go-roborock/pkg/dependencies/rest"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const defaultRequestTimeout = 30 * time.Second

// HTTPDoer lets callers inject every HTTP exchange.
type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

// Option configures immutable service settings and network seams.
type Option func(*clientOptions) error

type clientOptions struct {
	baseURL string
	http    HTTPDoer
	dial    mqtt.DialFunc
	hub     []mqtt.HubOption
}

// Client owns immutable configuration and the shared MQTT connections of the
// accounts it opens devices for. It retains no account tokens: each connection
// is keyed by the credentials of the request that opened it, and it closes
// when its last device session has been idle for a minute or on Close.
type Client struct {
	rest    *rest.Client
	hub     *mqtt.Hub
	baseURL string
}

// WithBaseURL selects the regional HTTPS origin. Login requests may override it explicitly.
func WithBaseURL(origin string) Option {
	return func(o *clientOptions) error {
		if origin == "" {
			return roborockerrors.New(roborockerrors.InvalidArgument, "configure", "origin is empty", nil)
		}

		o.baseURL = origin

		return nil
	}
}

// WithHTTPClient injects an HTTP client. The caller owns its transport.
func WithHTTPClient(client HTTPDoer) Option {
	return func(o *clientOptions) error {
		if client == nil {
			return roborockerrors.New(roborockerrors.InvalidArgument, "configure", "HTTP client is nil", nil)
		}

		o.http = client

		return nil
	}
}

// WithMQTTDial injects a connection-producing network seam. It must honor context cancellation.
// The default dialer establishes verified TLS. An injected dialer owns transport security.
func WithMQTTDial(dial mqtt.DialFunc) Option {
	return func(o *clientOptions) error {
		if dial == nil {
			return roborockerrors.New(roborockerrors.InvalidArgument, "configure", "MQTT dialer is nil", nil)
		}

		o.dial = dial

		return nil
	}
}

// WithMQTTIdleTimeout sets how long a device subscription outlives its last session.
// The default is one minute; the account's connection closes with its last subscription.
func WithMQTTIdleTimeout(idle time.Duration) Option {
	return func(o *clientOptions) error {
		if idle <= 0 {
			return roborockerrors.New(roborockerrors.InvalidArgument, "configure", "MQTT idle timeout must be positive", nil)
		}

		o.hub = append(o.hub, mqtt.WithIdleTimeout(idle))

		return nil
	}
}

// NewClient validates functional options without accessing the network.
func NewClient(options ...Option) (*Client, error) {
	var defaultHTTP http.Client

	defaultHTTP.Timeout = defaultRequestTimeout
	defaultHTTP.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	settings := clientOptions{baseURL: protocol.RESTDefaultOrigin, http: &defaultHTTP, dial: nil, hub: nil}

	for _, option := range options {
		if option == nil {
			return nil, roborockerrors.New(roborockerrors.InvalidArgument, "configure", "option is nil", nil)
		}

		err := option(&settings)
		if err != nil {
			return nil, err
		}
	}

	opts := []rest.Option{rest.WithHTTPClient(settings.http)}

	if settings.baseURL != "" {
		opts = append(opts, rest.WithBaseURL(settings.baseURL))
	}

	restClient, err := rest.New(opts...)
	if err != nil {
		return nil, roborockerrors.Wrap(roborockerrors.InvalidArgument, "configure", "client configuration failed", err)
	}

	return &Client{rest: restClient, hub: mqtt.NewHub(settings.dial, settings.hub...), baseURL: settings.baseURL}, nil
}

// Close ends every MQTT connection and device session the client holds. The client
// stays usable and reconnects on the next OpenDevice.
func (c *Client) Close() error {
	c.hub.Close()

	return nil
}

type deviceRPC interface {
	Call(ctx context.Context, method string, parameters json.RawMessage) (json.RawMessage, error)
	QueryA01(ctx context.Context, properties []int) (map[int]json.RawMessage, error)
	SetA01(ctx context.Context, values map[int]json.RawMessage) error
	Close() error
}

type deviceLifecycle interface {
	Done() <-chan struct{}
	Err() error
}

// DeviceSession is one borrow of a device on its account's shared MQTT connection,
// with its own pending requests. Close returns the borrow and the shared connection
// closes once the account has been idle; canceling the opening context also closes the session.
type DeviceSession struct {
	rpc         deviceRPC
	protocol    string
	family      DeviceFamily
	metadata    deviceMetadata
	client      *Client
	auth        AuthContext
	deviceID    string
	life        context.Context
	cancel      context.CancelFunc
	closeOnce   sync.Once
	closeErr    error
	terminalErr error
	mu          sync.Mutex
	closing     bool
	camera      *CameraSession
}

// OpenDevice borrows the device on the account's shared MQTT connection, connecting
// when the account has none; it never reuses another account's connection.
// The broker refusing the account's credentials is terminal for those credentials:
// OpenDevice then returns an Unauthorized error without contacting the broker until
// the caller supplies different credentials.
func (c *Client) OpenDevice(ctx context.Context, request OpenDeviceRequest) (*DeviceSession, error) {
	if request.Protocol == "" {
		return nil, roborockerrors.New(roborockerrors.InvalidArgument, "open_device", "device protocol is required", nil)
	}

	metadata, err := c.openMetadata(ctx, request)
	if err != nil {
		return nil, err
	}

	auth := request.Auth.Mqtt

	rpc, err := c.hub.Open(ctx, mqtt.Config{BrokerURL: auth.BrokerURL,
		User:     auth.User,
		Secret:   auth.Secret,
		Key:      auth.Key,
		DeviceID: request.DeviceID,
		LocalKey: request.LocalKey,
		Protocol: string(request.Protocol),
	})
	if err != nil {
		return nil, roborockerrors.Wrap(roborockerrors.Unavailable, "open_device", "device connection failed", err)
	}

	life, cancel := context.WithCancel(ctx)

	var session DeviceSession

	session.rpc = rpc
	session.protocol = string(request.Protocol)
	session.family = metadata.family
	session.metadata = metadata
	session.client = c
	session.auth = request.Auth
	session.deviceID = request.DeviceID
	session.life = life
	session.cancel = cancel

	go session.observeTransport(rpc)

	return &session, nil
}

// Done closes when the opening context ends, the connection fails, or Close is called.
func (s *DeviceSession) Done() <-chan struct{} { return s.life.Done() }

// Err returns the terminal typed failure after Done closes, or nil while active.
// A connection failure retains its underlying cause after session cleanup.
func (s *DeviceSession) Err() error {
	select {
	case <-s.life.Done():
	default:
		return nil
	}

	s.mu.Lock()
	terminal := s.terminalErr
	s.mu.Unlock()

	if terminal != nil {
		return terminal
	}

	if transport, ok := s.rpc.(deviceLifecycle); ok {
		err := transport.Err()
		if err != nil {
			return roborockerrors.Wrap(roborockerrors.Unavailable, "device_session", "device session ended", err)
		}
	}

	err := s.life.Err()
	if err != nil {
		kind := roborockerrors.Canceled

		if errors.Is(err, context.DeadlineExceeded) {
			kind = roborockerrors.Timeout
		}

		return roborockerrors.New(kind, "device_session", "device context ended", err)
	}

	return nil
}

// Close cancels this session's pending requests and returns its borrow of the shared
// connection. Repeated calls are safe.
func (s *DeviceSession) Close() error {
	s.closeOnce.Do(func() {
		terminal := roborockerrors.New(roborockerrors.Closed, "device_session", "device session closed", nil)

		if transport, ok := s.rpc.(deviceLifecycle); ok {
			err := transport.Err()
			if err != nil {
				terminal = roborockerrors.Wrap(roborockerrors.Unavailable, "device_session", "device session ended", err)
			}
		}

		s.mu.Lock()
		s.closing = true

		if s.terminalErr == nil {
			s.terminalErr = terminal
		}

		camera := s.camera

		s.mu.Unlock()

		if camera != nil {
			_ = camera.Close()
		}

		if s.cancel != nil {
			s.cancel()
		}

		s.closeErr = s.rpc.Close()
	})

	return s.closeErr
}

func (s *DeviceSession) observeTransport(transport deviceLifecycle) {
	select {
	case <-s.life.Done():
		cause := s.life.Err()
		kind := roborockerrors.Canceled

		if errors.Is(cause, context.DeadlineExceeded) {
			kind = roborockerrors.Timeout
		}

		s.recordTerminalError(roborockerrors.New(kind, "device_session", "device context ended", cause))
	case <-transport.Done():
		s.recordTerminalError(roborockerrors.Wrap(
			roborockerrors.Unavailable, "device_session", "device session ended", transport.Err(),
		))
	}

	_ = s.Close()
}

func (s *DeviceSession) recordTerminalError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.terminalErr == nil {
		s.terminalErr = err
	}
}
