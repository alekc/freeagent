package freeagent

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestWithSandbox(t *testing.T) {
	t.Parallel()
	client, err := NewClient(WithoutAuth(), WithSandbox())
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	if got := client.BaseURL().String(); got != SandboxBaseURL {
		t.Fatalf("BaseURL = %q, want %q", got, SandboxBaseURL)
	}
}

func TestWithBaseURLAddsTrailingSlash(t *testing.T) {
	t.Parallel()
	// Without the trailing slash, resolving "invoices" would replace the
	// last path segment instead of appending to it.
	client, err := NewClient(WithoutAuth(), WithBaseURL("https://example.test/v2"))
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	if got := client.BaseURL().String(); got != "https://example.test/v2/" {
		t.Fatalf("BaseURL = %q, want a trailing slash", got)
	}
}

// BaseURL hands out a copy so a caller cannot reconfigure the client by
// mutating what it returns.
func TestBaseURLReturnsACopy(t *testing.T) {
	t.Parallel()
	client, err := NewClient(WithoutAuth(), WithBaseURL("https://example.test/v2/"))
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	got := client.BaseURL()
	got.Host = "evil.example.com"
	if client.BaseURL().Host != "example.test" {
		t.Fatalf("mutating the returned URL changed the client: %s", client.BaseURL())
	}
}

func TestWithoutRetry(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}, WithoutRetry())

	if _, _, err := client.Raw(context.Background(), http.MethodGet, "company", nil, nil); err == nil {
		t.Fatal("Raw succeeded, want a server error")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestWithAPIVersionOverride(t *testing.T) {
	t.Parallel()
	var got string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Api-Version")
		fmt.Fprint(w, `{}`)
	}, WithAPIVersion("2024-10-01"))

	if _, _, err := client.Raw(context.Background(), http.MethodGet, "company", nil, nil); err != nil {
		t.Fatalf("Raw = %v", err)
	}
	if got != "2024-10-01" {
		t.Fatalf("X-Api-Version = %q, want 2024-10-01", got)
	}
}

func TestNewRequestRejectsNilContext(t *testing.T) {
	t.Parallel()
	client, err := NewClient(WithoutAuth())
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	//nolint:staticcheck // passing nil is the condition under test
	if _, err := client.newRequest(nil, http.MethodGet, "company", nil, nil); err == nil {
		t.Fatal("newRequest with a nil context succeeded, want an error")
	}
}

func TestNewRequestRejectsUnencodableBody(t *testing.T) {
	t.Parallel()
	client, err := NewClient(WithoutAuth())
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	// A channel cannot be marshalled, so this fails locally rather than
	// sending a malformed request.
	_, err = client.newRequest(context.Background(), http.MethodPost, "invoices", nil, make(chan int))
	if err == nil || !strings.Contains(err.Error(), "encoding request body") {
		t.Fatalf("err = %v, want an encoding error", err)
	}
}

func TestDecodeErrorIsReported(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"widgets": not json}`)
	})
	widgets := newCollection[widget](client, widgetMeta)

	_, _, err := widgets.List(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "decoding response") {
		t.Fatalf("err = %v, want a decode error", err)
	}
}

// An empty 200 is not an error: Delete and some writes answer that way.
func TestEmptyBodyIsNotADecodeError(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	body, resp, err := client.Raw(context.Background(), http.MethodDelete, "widgets/1", nil, nil)
	if err != nil {
		t.Fatalf("Raw = %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("body = %q, want empty", body)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

func TestListOptionsErrorIsReportedBeforeAnyRequest(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be sent for invalid list options")
	})
	widgets := newCollection[widget](client, widgetMeta)
	if _, _, err := widgets.List(context.Background(), &ListOptions{PerPage: 1000}); err == nil {
		t.Fatal("List succeeded with an out-of-range per_page, want an error")
	}
}
