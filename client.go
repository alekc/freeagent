// Package freeagent is a client for the FreeAgent v2 API.
//
// See https://dev.freeagent.com for the upstream reference. Resource types
// mirror the documented payloads; cross-references are carried as
// ResourceURL, and monetary values as Decimal, never float64.
package freeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// Version is the library version reported in the default user agent.
const Version = "0.1.0-dev"

// Endpoints. Both carry the /v2/ prefix and a trailing slash so relative
// resource paths resolve against them.
const (
	DefaultBaseURL = "https://api.freeagent.com/v2/"
	SandboxBaseURL = "https://api.sandbox.freeagent.com/v2/"
)

// DefaultAPIVersion pins the X-Api-Version header. Sending no header opts
// into pre-versioning behaviour, which drifts from the documentation this
// library was modelled on, so a date is always sent. Bump it only alongside
// the model changes the new version implies.
const DefaultAPIVersion = "2026-08-16"

// DefaultMaxResponseBytes bounds a single response body. Legitimate pages of
// 100 records are far smaller; the cap exists so a broken or hostile upstream
// cannot exhaust memory.
const DefaultMaxResponseBytes = 32 << 20

// DefaultUserAgent identifies the library. FreeAgent asks integrations to
// send something identifying, and callers should append their own name via
// WithUserAgent.
var DefaultUserAgent = "freeagent-go/" + Version + " (+https://github.com/alekc/freeagent)"

// ErrNoTokenSource is returned by NewClient when neither WithTokenSource nor
// WithoutAuth was supplied. Every FreeAgent endpoint requires a bearer token,
// so an unauthenticated client is a configuration mistake worth catching at
// construction rather than on the first 401.
var ErrNoTokenSource = errors.New("freeagent: no token source configured, pass WithTokenSource or WithoutAuth")

// ErrReadOnly is returned instead of sending a mutating request on a client
// built WithReadOnly.
var ErrReadOnly = errors.New("freeagent: client is read-only")

// RetryPolicy governs automatic retries. Retries apply to 429 on any method,
// and to transport errors and 5xx on idempotent methods only.
type RetryPolicy struct {
	// MaxAttempts counts the first try. 1 disables retries.
	MaxAttempts int
	// BaseDelay is the first backoff interval, doubled per attempt.
	BaseDelay time.Duration
	// MaxDelay caps the computed backoff. It does not cap Retry-After.
	MaxDelay time.Duration
	// MaxRetryAfter is the longest server-requested wait that is honoured.
	// Beyond it the error is returned so the caller decides, rather than the
	// client blocking for an unbounded time.
	MaxRetryAfter time.Duration
}

// DefaultRetryPolicy is applied unless overridden.
var DefaultRetryPolicy = RetryPolicy{
	MaxAttempts:   3,
	BaseDelay:     500 * time.Millisecond,
	MaxDelay:      30 * time.Second,
	MaxRetryAfter: 2 * time.Minute,
}

// Client is a FreeAgent API client. It is safe for concurrent use.
type Client struct {
	baseURL          *url.URL
	httpClient       *http.Client
	tokenSource      oauth2.TokenSource
	userAgent        string
	apiVersion       string
	limiters         *limiterSet
	retry            RetryPolicy
	rateLimitTest    bool
	allowNoAuth      bool
	readOnly         bool
	maxResponseBytes int64

	// Typed resource services. Families without a model yet are reachable
	// through Raw.
	services
}

// Option configures a Client.
type Option func(*Client) error

// WithBaseURL overrides the API endpoint. A trailing slash is added when
// missing so relative resource paths resolve correctly.
func WithBaseURL(raw string) Option {
	return func(c *Client) error {
		if !strings.HasSuffix(raw, "/") {
			raw += "/"
		}
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("freeagent: invalid base URL %q: %w", raw, err)
		}
		if u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("freeagent: base URL %q must be absolute", raw)
		}
		c.baseURL = u
		return nil
	}
}

// WithSandbox points the client at the sandbox environment.
func WithSandbox() Option { return WithBaseURL(SandboxBaseURL) }

// WithHTTPClient supplies the underlying transport.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) error {
		if h == nil {
			return errors.New("freeagent: WithHTTPClient requires a non-nil client")
		}
		c.httpClient = h
		return nil
	}
}

// WithUserAgent replaces the default user agent.
func WithUserAgent(ua string) Option {
	return func(c *Client) error {
		if strings.TrimSpace(ua) == "" {
			return errors.New("freeagent: WithUserAgent requires a non-empty value")
		}
		c.userAgent = ua
		return nil
	}
}

// WithAPIVersion overrides the X-Api-Version header. The value is a date such
// as "2024-10-01"; the API serves the newest version at or before it.
func WithAPIVersion(v string) Option {
	return func(c *Client) error {
		if _, err := time.Parse(DateLayout, v); err != nil {
			return fmt.Errorf("freeagent: API version %q must be a YYYY-MM-DD date", v)
		}
		c.apiVersion = v
		return nil
	}
}

// WithTokenSource supplies OAuth credentials. Use TokenSource for a source
// that refreshes and persists rotated refresh tokens.
func WithTokenSource(ts oauth2.TokenSource) Option {
	return func(c *Client) error {
		if ts == nil {
			return errors.New("freeagent: WithTokenSource requires a non-nil source")
		}
		c.tokenSource = ts
		return nil
	}
}

// WithoutAuth builds a client that sends no Authorization header. It exists
// for tests and for talking to a local fake; real endpoints will reject it.
func WithoutAuth() Option {
	return func(c *Client) error {
		c.tokenSource = nil
		c.allowNoAuth = true
		return nil
	}
}

// WithRateLimits sets the client-side request budgets. Zero disables the
// corresponding limiter.
func WithRateLimits(perMinute, perHour int) Option {
	return func(c *Client) error {
		if perMinute < 0 || perHour < 0 {
			return errors.New("freeagent: rate limits must not be negative")
		}
		c.limiters = newLimiterSet(perMinute, perHour)
		return nil
	}
}

// WithoutRateLimit removes client-side throttling. The server limits still
// apply, so the retry path becomes the only protection.
func WithoutRateLimit() Option { return WithRateLimits(0, 0) }

// WithRetryPolicy overrides the retry behaviour.
func WithRetryPolicy(p RetryPolicy) Option {
	return func(c *Client) error {
		if p.MaxAttempts < 1 {
			return fmt.Errorf("freeagent: RetryPolicy.MaxAttempts must be at least 1, got %d", p.MaxAttempts)
		}
		c.retry = p
		return nil
	}
}

// WithoutRetry disables automatic retries.
func WithoutRetry() Option {
	return func(c *Client) error {
		c.retry.MaxAttempts = 1
		return nil
	}
}

// WithReadOnly refuses every mutating request before it is built, returning
// ErrReadOnly instead. Only GET, HEAD and OPTIONS are allowed through.
//
// This exists for pointing a client at an account whose data must not be
// touched. It is a structural guarantee rather than a matter of discipline:
// no typed service, no transition, no Raw call can write through a read-only
// client, because the check sits in request construction rather than in each
// caller.
func WithReadOnly() Option {
	return func(c *Client) error {
		c.readOnly = true
		return nil
	}
}

// WithRateLimitTest sends X-RateLimit-Test, which lowers the sandbox budget
// to 5 requests per minute so back-off handling can be exercised for real.
func WithRateLimitTest(on bool) Option {
	return func(c *Client) error {
		c.rateLimitTest = on
		return nil
	}
}

// WithMaxResponseBytes overrides the response body cap.
func WithMaxResponseBytes(n int64) Option {
	return func(c *Client) error {
		if n <= 0 {
			return fmt.Errorf("freeagent: max response bytes must be positive, got %d", n)
		}
		c.maxResponseBytes = n
		return nil
	}
}

// NewClient builds a client. Exactly one of WithTokenSource or WithoutAuth is
// required.
func NewClient(opts ...Option) (*Client, error) {
	base, err := url.Parse(DefaultBaseURL)
	if err != nil {
		return nil, fmt.Errorf("freeagent: parsing default base URL: %w", err)
	}
	c := &Client{
		baseURL:          base,
		httpClient:       &http.Client{Timeout: 60 * time.Second},
		userAgent:        DefaultUserAgent,
		apiVersion:       DefaultAPIVersion,
		limiters:         newLimiterSet(DefaultRequestsPerMinute, DefaultRequestsPerHour),
		retry:            DefaultRetryPolicy,
		maxResponseBytes: DefaultMaxResponseBytes,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	if c.tokenSource == nil && !c.allowNoAuth {
		return nil, ErrNoTokenSource
	}
	c.initServices()
	return c, nil
}

// BaseURL returns the configured endpoint.
func (c *Client) BaseURL() *url.URL {
	u := *c.baseURL
	return &u
}

// newRequest builds a request against the API root. path is relative, for
// example "invoices" or "invoices/123".
func (c *Client) newRequest(ctx context.Context, method, path string, query url.Values, body any) (*http.Request, error) {
	if ctx == nil {
		return nil, errors.New("freeagent: nil context")
	}
	// Checked here, not at the call sites: every request in the library is
	// built through this function, so there is nowhere for a write to slip
	// past.
	if c.readOnly && !readMethod(method) {
		return nil, fmt.Errorf("%w: refusing to %s %s", ErrReadOnly, method, path)
	}
	rel, err := url.Parse(strings.TrimPrefix(path, "/"))
	if err != nil {
		return nil, fmt.Errorf("freeagent: invalid path %q: %w", path, err)
	}
	u := c.baseURL.ResolveReference(rel)
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("freeagent: encoding request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("freeagent: building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if c.apiVersion != "" {
		req.Header.Set("X-Api-Version", c.apiVersion)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.rateLimitTest {
		req.Header.Set("X-RateLimit-Test", "true")
	}
	return req, nil
}

// do sends the request, applying rate limiting, authorisation and retries,
// then decodes a successful body into out when out is non-nil.
func (c *Client) do(req *http.Request, out any) (*Response, error) {
	ctx := req.Context()
	attempts := c.retry.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; ; attempt++ {
		if attempt > 1 && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("freeagent: rewinding request body for retry: %w", err)
			}
			req.Body = body
		}
		if err := c.limiters.wait(ctx); err != nil {
			return nil, err
		}
		if err := c.authorize(req); err != nil {
			return nil, err
		}

		//nolint:bodyclose // readBody closes it; bodyclose cannot see through the helper
		httpResp, err := c.httpClient.Do(req)
		var (
			resp     *Response
			apiErr   *APIError
			body     []byte
			readErr  error
			retryFor time.Duration
		)
		if err == nil {
			body, readErr = c.readBody(httpResp)
			resp = newResponse(httpResp)
			if readErr == nil && httpResp.StatusCode >= 400 {
				apiErr = newAPIError(httpResp, body)
				retryFor = apiErr.RetryAfter
			}
		}

		switch {
		case err != nil:
			lastErr = fmt.Errorf("freeagent: %s %s: %w", req.Method, req.URL.Path, err)
		case readErr != nil:
			lastErr = readErr
		case apiErr != nil:
			lastErr = apiErr
		default:
			if decodeErr := decodeBody(body, out); decodeErr != nil {
				return resp, decodeErr
			}
			return resp, nil
		}

		if attempt >= attempts || !retryable(req.Method, httpResp, err) {
			return resp, lastErr
		}
		delay, ok := c.retryDelay(attempt, retryFor)
		if !ok {
			return resp, lastErr
		}
		if err := sleepCtx(ctx, delay); err != nil {
			return resp, err
		}
	}
}

// authorize fetches a token and stamps the Authorization header. It runs per
// attempt so a retry after a refresh carries the new token.
func (c *Client) authorize(req *http.Request) error {
	if c.tokenSource == nil {
		return nil
	}
	token, err := c.tokenSource.Token()
	if err != nil {
		return fmt.Errorf("freeagent: obtaining access token: %w", err)
	}
	token.SetAuthHeader(req)
	return nil
}

// readBody drains and replaces the response body so callers can still read it
// from the returned Response, and enforces the size cap.
func (c *Client) readBody(resp *http.Response) ([]byte, error) {
	defer func() { _ = resp.Body.Close() }()
	limit := c.maxResponseBytes
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("freeagent: reading response body: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("freeagent: response body exceeds %d bytes", limit)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func decodeBody(body []byte, out any) error {
	if out == nil || len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	// A *[]byte target means the caller wants the payload verbatim, which is
	// how Raw reaches endpoints that have no typed model yet.
	if raw, ok := out.(*[]byte); ok {
		*raw = body
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("freeagent: decoding response: %w", err)
	}
	return nil
}

// retryable reports whether another attempt is worth making. A 429 is always
// retryable because the request was rejected rather than executed; transport
// errors and 5xx are retried only for methods that are safe to repeat.
func retryable(method string, resp *http.Response, transportErr error) bool {
	if transportErr != nil {
		return idempotent(method)
	}
	if resp == nil {
		return false
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return resp.StatusCode >= 500 && idempotent(method)
}

// readMethod reports whether a verb only reads. Deliberately an allowlist:
// an unknown verb is treated as a write.
func readMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func idempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

// retryDelay honours Retry-After when the server sent one, otherwise applies
// exponential backoff with equal jitter. ok is false when the server asked
// for longer than the policy is willing to wait.
func (c *Client) retryDelay(attempt int, retryAfter time.Duration) (time.Duration, bool) {
	if retryAfter > 0 {
		if c.retry.MaxRetryAfter > 0 && retryAfter > c.retry.MaxRetryAfter {
			return 0, false
		}
		return retryAfter, true
	}
	delay := c.retry.BaseDelay
	if delay <= 0 {
		delay = DefaultRetryPolicy.BaseDelay
	}
	for i := 1; i < attempt; i++ {
		delay *= 2
		if c.retry.MaxDelay > 0 && delay >= c.retry.MaxDelay {
			delay = c.retry.MaxDelay
			break
		}
	}
	half := delay / 2
	if half <= 0 {
		return delay, true
	}
	//nolint:gosec // jitter spreads retries, it is not a security decision
	return half + time.Duration(rand.Int64N(int64(half))), true
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
