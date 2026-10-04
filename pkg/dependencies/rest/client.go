// Package rest implements stateless Roborock cloud authentication and inventory.
package rest

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const defaultTimeout = 30 * time.Second
const maximumResponseBytes = 8 << 20

// HTTPDoer makes every REST exchange injectable.
type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

// Client retains configuration only; credentials belong to each request.
type Client struct {
	http    HTTPDoer
	baseURL string
	clock   func() time.Time
	random  io.Reader
}

// Option configures a reusable client at construction.
type Option func(*Client)

// WithHTTPClient supplies an HTTP transport owned by the caller.
func WithHTTPClient(v HTTPDoer) Option { return func(c *Client) { c.http = v } }

// WithBaseURL sets the default explicit regional origin.
func WithBaseURL(v string) Option { return func(c *Client) { c.baseURL = v } }

// WithClock supplies signing time.
func WithClock(v func() time.Time) Option { return func(c *Client) { c.clock = v } }

// WithRandom supplies signing entropy. The reader must support concurrent reads.
func WithRandom(v io.Reader) Option { return func(c *Client) { c.random = v } }

// New constructs a client; a regional origin is supplied here or per request.
func New(opts ...Option) (*Client, error) {
	c := &Client{http: &http.Client{Timeout: defaultTimeout}, baseURL: "", clock: time.Now, random: rand.Reader}
	for _, o := range opts {
		if o != nil {
			o(c)
		}
	}
	if c.http == nil || c.clock == nil || c.random == nil {
		return nil, roborockerrors.New(roborockerrors.InvalidArgument, "rest.New", "nil client option", nil)
	}
	if c.baseURL != "" {
		if err := validateOrigin(c.baseURL); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func validateOrigin(base string) error {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return roborockerrors.New(roborockerrors.InvalidArgument, "rest.origin", "expected an HTTP origin", err)
	}
	return nil
}

func (c *Client) prepareRequest(
	ctx context.Context, base, method, path string, query, form url.Values, headers http.Header,
) (*http.Request, error) {
	if base == "" {
		base = c.baseURL
	}
	if err := validateOrigin(base); err != nil {
		return nil, err
	}
	endpoint := strings.TrimSuffix(base, "/") + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, failure(path, err)
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return req, nil
}

func (c *Client) exchange(
	ctx context.Context, base, method, path string, query, form url.Values, headers http.Header, result any,
) error {
	req, err := c.prepareRequest(ctx, base, method, path, query, form, headers)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return failure(path, err)
	}
	if resp == nil || resp.Body == nil {
		return roborockerrors.New(roborockerrors.Protocol, path, "missing HTTP response", nil)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return vendorError(path, resp.StatusCode)
	}
	return decodeResponse(resp.Body, method, path, result)
}

func decodeResponse(body io.Reader, method, path string, result any) error {
	limited := io.LimitReader(body, maximumResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return failure(path, err)
	}
	if len(data) > maximumResponseBytes {
		return roborockerrors.New(roborockerrors.Protocol, path, "response exceeds size limit", nil)
	}
	if err = validateResponse(method, path, data); err != nil {
		return err
	}
	if err = json.Unmarshal(data, result); err != nil {
		return roborockerrors.New(roborockerrors.Protocol, path, "invalid JSON response", err)
	}
	return nil
}

func failure(op string, err error) error {
	kind := roborockerrors.Unavailable
	switch {
	case errors.Is(err, context.Canceled):
		kind = roborockerrors.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		kind = roborockerrors.Timeout
	default:
		var timeout interface{ Timeout() bool }
		if errors.As(err, &timeout) && timeout.Timeout() {
			kind = roborockerrors.Timeout
		}
	}
	return roborockerrors.New(kind, op, "HTTP exchange failed", err)
}
func vendorError(op string, code int) *roborockerrors.Error {
	kind := roborockerrors.Protocol
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden,
		int(dependencymodels.VendorCodeInvalidCredentials), int(dependencymodels.VendorCodeInvalidCode),
		int(dependencymodels.VendorCodeInvalidAgreement), int(dependencymodels.VendorCodeAgreementRequired):
		kind = roborockerrors.Unauthorized
	case http.StatusNotFound, int(dependencymodels.VendorCodeAccountMissing), int(dependencymodels.VendorCodeWrongRegion):
		kind = roborockerrors.NotFound
	case http.StatusTooManyRequests, int(dependencymodels.VendorCodeTooFrequent):
		kind = roborockerrors.RateLimited
	case int(dependencymodels.VendorCodeInvalidEmail), int(dependencymodels.VendorCodeMissingParameters):
		kind = roborockerrors.InvalidArgument
	}
	if code >= http.StatusInternalServerError && code < 600 {
		kind = roborockerrors.Unavailable
	}
	e := roborockerrors.New(kind, op, "service rejected request", nil)
	e.Code = code
	return e
}
func checkCode(op string, code int) error {
	if code != int(dependencymodels.VendorCodeSuccess) {
		return vendorError(op, code)
	}
	return nil
}
