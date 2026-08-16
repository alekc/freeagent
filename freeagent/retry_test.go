package freeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// Backoff doubles per attempt, is clamped by MaxDelay, and carries jitter so
// concurrent callers do not retry in lockstep.
func TestRetryDelayBackoff(t *testing.T) {
	t.Parallel()
	client, err := NewClient(WithoutAuth(), WithRetryPolicy(RetryPolicy{
		MaxAttempts:   5,
		BaseDelay:     100 * time.Millisecond,
		MaxDelay:      250 * time.Millisecond,
		MaxRetryAfter: time.Minute,
	}))
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}

	tests := []struct {
		attempt  int
		min, max time.Duration
	}{
		{attempt: 1, min: 50 * time.Millisecond, max: 100 * time.Millisecond},
		{attempt: 2, min: 100 * time.Millisecond, max: 200 * time.Millisecond},
		// Doubling would reach 400ms, so MaxDelay clamps it to 250ms.
		{attempt: 3, min: 125 * time.Millisecond, max: 250 * time.Millisecond},
		{attempt: 9, min: 125 * time.Millisecond, max: 250 * time.Millisecond},
	}
	for _, tc := range tests {
		got, ok := client.retryDelay(tc.attempt, 0)
		if !ok {
			t.Fatalf("attempt %d: ok = false, want a delay", tc.attempt)
		}
		if got < tc.min || got > tc.max {
			t.Fatalf("attempt %d: delay = %s, want between %s and %s", tc.attempt, got, tc.min, tc.max)
		}
	}
}

func TestRetryDelayHonoursRetryAfter(t *testing.T) {
	t.Parallel()
	client, err := NewClient(WithoutAuth(), WithRetryPolicy(RetryPolicy{
		MaxAttempts:   3,
		BaseDelay:     time.Millisecond,
		MaxDelay:      time.Second,
		MaxRetryAfter: time.Minute,
	}))
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}

	// Retry-After is used verbatim and is deliberately not clamped by
	// MaxDelay: retrying earlier than the server asked just earns another 429.
	got, ok := client.retryDelay(1, 45*time.Second)
	if !ok || got != 45*time.Second {
		t.Fatalf("retryDelay = %s, %v, want 45s", got, ok)
	}
	// Beyond MaxRetryAfter, hand the decision back to the caller.
	if _, ok := client.retryDelay(1, 2*time.Minute); ok {
		t.Fatal("retryDelay accepted a wait beyond MaxRetryAfter, want ok=false")
	}
}

// A zero BaseDelay must not produce a zero or negative sleep.
func TestRetryDelayWithZeroBaseDelay(t *testing.T) {
	t.Parallel()
	client, err := NewClient(WithoutAuth(), WithRetryPolicy(RetryPolicy{MaxAttempts: 3}))
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	got, ok := client.retryDelay(1, 0)
	if !ok || got <= 0 {
		t.Fatalf("retryDelay = %s, %v, want a positive delay", got, ok)
	}
}

func TestWithHTTPClientIsUsed(t *testing.T) {
	t.Parallel()
	custom := &http.Client{Timeout: 3 * time.Second}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{}`)
	}, WithHTTPClient(custom))

	if client.httpClient != custom {
		t.Fatal("WithHTTPClient did not install the supplied client")
	}
	if _, _, err := client.Raw(context.Background(), http.MethodGet, "company", nil, nil); err != nil {
		t.Fatalf("Raw = %v", err)
	}
}

func TestListOptionsCloneCopiesValues(t *testing.T) {
	t.Parallel()
	original := &ListOptions{Page: 3, PerPage: 50, Sort: "-updated_at"}
	copied := original.clone()
	copied.Page = 9
	if original.Page != 3 {
		t.Fatalf("clone shares state: original page = %d", original.Page)
	}
	if copied.PerPage != 50 || copied.Sort != "-updated_at" {
		t.Fatalf("clone lost fields: %+v", copied)
	}
}

// All must respect a caller-supplied page size instead of overriding it.
func TestAllRespectsSuppliedPerPage(t *testing.T) {
	t.Parallel()
	var seen []string
	widgets := newWidgets(t, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Query().Get("per_page"))
		fmt.Fprint(w, `{"widgets":[{"name":"a"}]}`)
	})
	for range widgets.All(context.Background(), &ListOptions{PerPage: 25}) { //nolint:revive // draining the iterator is the point
	}
	if len(seen) != 1 || seen[0] != "25" {
		t.Fatalf("per_page values = %v, want [25]", seen)
	}
}

func TestTimeMarshalZeroAndInvalid(t *testing.T) {
	t.Parallel()
	out, err := json.Marshal(Time{})
	if err != nil {
		t.Fatalf("Marshal = %v", err)
	}
	if string(out) != "null" {
		t.Fatalf("Marshal(zero Time) = %s, want null", out)
	}

	var got Time
	if err := json.Unmarshal([]byte(`{"nested":true}`), &got); err == nil {
		t.Fatal("Unmarshal of an object into Time succeeded, want an error")
	}
	if err := json.Unmarshal([]byte(`"nonsense"`), &got); err == nil {
		t.Fatal("Unmarshal of a junk string into Time succeeded, want an error")
	}
	if err := json.Unmarshal([]byte(`null`), &got); err != nil || !got.IsZero() {
		t.Fatalf("Unmarshal(null) = %v, %v, want a zero Time", got, err)
	}
}

func TestFirstLineTruncatesMultiLineBodies(t *testing.T) {
	t.Parallel()
	body := "429 Too Many Requests\nRetry-After: 60\nmore detail here"
	msg, _ := parseErrorBody([]byte(body))
	if msg != "429 Too Many Requests" {
		t.Fatalf("message = %q, want only the first line", msg)
	}

	long := strings.Repeat("z", 500)
	msg, _ = parseErrorBody([]byte(long))
	if len(msg) > 210 {
		t.Fatalf("message length = %d, want it truncated", len(msg))
	}
}

func TestFileStoreRejectsCorruptFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "token.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile = %v", err)
	}
	store, err := NewFileStore(path, Sandbox.Name)
	if err != nil {
		t.Fatalf("NewFileStore = %v", err)
	}
	if _, err := store.Load(context.Background()); err == nil {
		t.Fatal("Load of a corrupt file succeeded, want an error")
	}

	// A corrupt file must not block a fresh save from recovering the state.
	if err := store.Save(context.Background(), &oauth2.Token{RefreshToken: "r"}); err != nil {
		t.Fatalf("Save over a corrupt file = %v", err)
	}
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load after recovery = %v", err)
	}
	if got.RefreshToken != "r" {
		t.Fatalf("RefreshToken = %q, want r", got.RefreshToken)
	}
}

func TestFileStoreSaveFailsOnUnwritableDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	store, err := NewFileStore(filepath.Join(dir, "sub", "token.json"), Sandbox.Name)
	if err != nil {
		t.Fatalf("NewFileStore = %v", err)
	}
	if err := store.Save(context.Background(), &oauth2.Token{RefreshToken: "r"}); err == nil {
		t.Fatal("Save into an unwritable directory succeeded, want an error")
	}
}
