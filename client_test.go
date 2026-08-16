package freeagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// fastRetry keeps the retry path exercisable without slowing the suite down.
var fastRetry = RetryPolicy{
	MaxAttempts:   3,
	BaseDelay:     time.Millisecond,
	MaxDelay:      5 * time.Millisecond,
	MaxRetryAfter: time.Second,
}

// newTestClient wires a client to a throwaway server with throttling off and
// a fast retry schedule. Extra options are applied last so a test can
// override any of it.
func newTestClient(t *testing.T, handler http.HandlerFunc, extra ...Option) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	opts := []Option{
		WithBaseURL(server.URL + "/v2/"),
		WithoutAuth(),
		WithoutRateLimit(),
		WithRetryPolicy(fastRetry),
	}
	client, err := NewClient(append(opts, extra...)...)
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	return client
}

func TestNewClientRequiresAuthDecision(t *testing.T) {
	t.Parallel()
	if _, err := NewClient(); !errors.Is(err, ErrNoTokenSource) {
		t.Fatalf("NewClient() error = %v, want ErrNoTokenSource", err)
	}
	if _, err := NewClient(WithoutAuth()); err != nil {
		t.Fatalf("NewClient(WithoutAuth()) = %v", err)
	}
}

func TestNewClientRejectsBadOptions(t *testing.T) {
	t.Parallel()
	tests := map[string]Option{
		"relative base URL": WithBaseURL("/v2/"),
		"empty user agent":  WithUserAgent("  "),
		"bad api version":   WithAPIVersion("2024-10"),
		"nil http client":   WithHTTPClient(nil),
		"zero attempts":     WithRetryPolicy(RetryPolicy{MaxAttempts: 0}),
		"negative limits":   WithRateLimits(-1, 0),
	}
	for name, opt := range tests {
		if _, err := NewClient(WithoutAuth(), opt); err == nil {
			t.Fatalf("%s: NewClient succeeded, want an error", name)
		}
	}
}

func TestRequestHeaders(t *testing.T) {
	t.Parallel()
	var got http.Header
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		fmt.Fprint(w, `{"ok":true}`)
	},
		WithTokenSource(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "tok", TokenType: "Bearer"})),
		WithUserAgent("acme/1.0"),
		WithRateLimitTest(true),
	)

	if _, _, err := client.Raw(context.Background(), http.MethodGet, "company", nil, nil); err != nil {
		t.Fatalf("Raw = %v", err)
	}
	want := map[string]string{
		"Accept":           "application/json",
		"User-Agent":       "acme/1.0",
		"X-Api-Version":    DefaultAPIVersion,
		"Authorization":    "Bearer tok",
		"X-Ratelimit-Test": "true",
	}
	for key, value := range want {
		if got.Get(key) != value {
			t.Fatalf("header %s = %q, want %q", key, got.Get(key), value)
		}
	}
}

func TestRequestPathAndQuery(t *testing.T) {
	t.Parallel()
	var gotURL string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		fmt.Fprint(w, `{}`)
	})
	opts := &ListOptions{PerPage: 100, Sort: "-updated_at"}
	query, err := opts.Values()
	if err != nil {
		t.Fatalf("Values = %v", err)
	}
	if _, _, err := client.Raw(context.Background(), http.MethodGet, "invoices", query, nil); err != nil {
		t.Fatalf("Raw = %v", err)
	}
	if want := "/v2/invoices?per_page=100&sort=-updated_at"; gotURL != want {
		t.Fatalf("request URL = %q, want %q", gotURL, want)
	}
}

func TestRetriesOn429(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, "You must not exceed 120 requests per 60 seconds")
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	})

	body, _, err := client.Raw(context.Background(), http.MethodGet, "company", nil, nil)
	if err != nil {
		t.Fatalf("Raw = %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d, want 3", calls.Load())
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("body = %s", body)
	}
}

// A 429 is retried for POST too: the request was rejected, not executed.
func TestRetries429EvenForPost(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	})
	if _, _, err := client.Raw(context.Background(), http.MethodPost, "invoices", nil, map[string]string{"a": "b"}); err != nil {
		t.Fatalf("Raw = %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
}

// When the server asks for longer than the policy will wait, give up
// immediately rather than blocking the caller for an unbounded time.
func TestRetryAfterBeyondCapGivesUp(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	start := time.Now()
	_, _, err := client.Raw(context.Background(), http.MethodGet, "company", nil, nil)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("took %s, want an immediate return", elapsed)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.RetryAfter != time.Hour {
		t.Fatalf("RetryAfter not surfaced on the error: %v", err)
	}
}

func TestRetriesIdempotent5xxButNotPost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		method    string
		wantCalls int32
	}{
		{http.MethodGet, 3},
		{http.MethodPut, 3},
		{http.MethodDelete, 3},
		{http.MethodPost, 1},
	}
	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			})
			_, _, err := client.Raw(context.Background(), tc.method, "invoices", nil, nil)
			if !errors.Is(err, ErrServer) {
				t.Fatalf("err = %v, want ErrServer", err)
			}
			if calls.Load() != tc.wantCalls {
				t.Fatalf("calls = %d, want %d", calls.Load(), tc.wantCalls)
			}
		})
	}
}

// A retried write has to send the same body again, which only works if the
// request body was buffered for replay.
func TestRetryRewindsRequestBody(t *testing.T) {
	t.Parallel()
	var (
		calls  atomic.Int32
		bodies []string
	)
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		if calls.Add(1) < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	})

	if _, _, err := client.Raw(context.Background(), http.MethodPut, "invoices/1", nil, map[string]string{"status": "Sent"}); err != nil {
		t.Fatalf("Raw = %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("saw %d requests, want 2", len(bodies))
	}
	if bodies[0] != bodies[1] {
		t.Fatalf("retry sent a different body:\n%q\n%q", bodies[0], bodies[1])
	}
	if !strings.Contains(bodies[1], `"status":"Sent"`) {
		t.Fatalf("body = %q, want the original payload", bodies[1])
	}
}

func TestContextCancellationStopsRetries(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}, WithRetryPolicy(RetryPolicy{MaxAttempts: 5, BaseDelay: time.Second, MaxDelay: time.Second, MaxRetryAfter: time.Minute}))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, _, err := client.Raw(ctx, http.MethodGet, "company", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestResponseSizeCap(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, strings.Repeat("x", 4096))
	}, WithMaxResponseBytes(1024))

	if _, _, err := client.Raw(context.Background(), http.MethodGet, "company", nil, nil); err == nil ||
		!strings.Contains(err.Error(), "exceeds 1024 bytes") {
		t.Fatalf("err = %v, want a size cap error", err)
	}
}

// The Response body stays readable after do drained it, so callers can
// inspect the payload the client already parsed.
func TestResponseBodyRemainsReadable(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"company":{"name":"Acme"}}`)
	})
	_, resp, err := client.Raw(context.Background(), http.MethodGet, "company", nil, nil)
	if err != nil {
		t.Fatalf("Raw = %v", err)
	}
	again, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("re-reading body = %v", err)
	}
	if string(again) != `{"company":{"name":"Acme"}}` {
		t.Fatalf("re-read body = %s", again)
	}
}

func TestTokenSourceFailureIsNotRetried(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{}`)
	}, WithTokenSource(failingTokenSource{}))

	_, _, err := client.Raw(context.Background(), http.MethodGet, "company", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "obtaining access token") {
		t.Fatalf("err = %v, want a token error", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("calls = %d, want 0: the request must not be sent without a token", calls.Load())
	}
}

type failingTokenSource struct{}

func (failingTokenSource) Token() (*oauth2.Token, error) {
	return nil, errors.New("refresh token revoked")
}
