package roborock

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/portpowered/go-roborock/pkg/dependencies/mqtt"
	"github.com/portpowered/go-roborock/pkg/dependencies/rest"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const defaultRequestTimeout = 30 * time.Second

// HTTPDoer lets callers inject every HTTP exchange.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Option configures immutable service settings and network seams.
type Option func(*clientOptions) error

type clientOptions struct {
	baseURL string
	http    HTTPDoer
	dial    mqtt.DialFunc
}

// Client owns immutable configuration. It retains no account tokens or connections.
type Client struct {
	rest    *rest.Client
	dial    mqtt.DialFunc
	baseURL string
}

// WithBaseURL selects the regional HTTPS origin. Login requests may override it explicitly.
func WithBaseURL(origin string) Option {
	return func(o *clientOptions) error { o.baseURL = origin; return nil }
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

// NewClient validates functional options without accessing the network.
func NewClient(options ...Option) (*Client, error) {
	o := clientOptions{http: &http.Client{Timeout: defaultRequestTimeout}}
	for _, option := range options {
		if option == nil {
			return nil, roborockerrors.New(roborockerrors.InvalidArgument, "configure", "option is nil", nil)
		}
		if err := option(&o); err != nil {
			return nil, err
		}
	}
	opts := []rest.Option{rest.WithHTTPClient(o.http)}
	if o.baseURL != "" {
		opts = append(opts, rest.WithBaseURL(o.baseURL))
	}
	r, err := rest.New(opts...)
	if err != nil {
		return nil, err
	}
	return &Client{rest: r, dial: o.dial, baseURL: o.baseURL}, nil
}

type deviceRPC interface {
	Call(context.Context, string, json.RawMessage) (json.RawMessage, error)
	QueryA01(context.Context, []int) (map[int]json.RawMessage, error)
	SetA01(context.Context, map[int]json.RawMessage) error
	Close() error
}

// DeviceSession owns a single account-bound device connection and its pending requests.
// Close releases the connection; canceling the opening context also closes the session.
type DeviceSession struct {
	rpc       deviceRPC
	protocol  string
	life      context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
	closeErr  error
	mu        sync.Mutex
	closing   bool
	cameras   []*CameraSession
}

// OpenDevice opens a fresh connection; it never reuses another account's session.
func (c *Client) OpenDevice(ctx context.Context, request OpenDeviceRequest) (*DeviceSession, error) {
	a := request.Auth.Mqtt
	s, err := mqtt.Open(ctx, mqtt.Config{BrokerURL: a.BrokerURL, User: a.User, Secret: a.Secret, Key: a.Key, DeviceID: request.DeviceID, LocalKey: request.LocalKey, Protocol: string(request.Protocol)}, c.dial)
	if err != nil {
		return nil, err
	}
	life, cancel := context.WithCancel(ctx)
	session := &DeviceSession{rpc: s, protocol: string(request.Protocol), life: life, cancel: cancel}
	go func() { <-life.Done(); _ = session.Close() }()
	return session, nil
}

// Close cancels pending requests and releases the owned connection. Repeated calls are safe.
func (s *DeviceSession) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closing = true
		cameras := append([]*CameraSession(nil), s.cameras...)
		s.mu.Unlock()
		for _, camera := range cameras {
			_ = camera.Close()
		}
		if s.cancel != nil {
			s.cancel()
		}
		s.closeErr = s.rpc.Close()
	})
	return s.closeErr
}
