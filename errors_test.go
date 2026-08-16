package freeagent

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseErrorBodyShapes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		body       string
		wantMsg    string
		wantFields []FieldError
	}{
		{
			name:    "nested errors.error object",
			body:    `{"errors":{"error":{"message":"Access denied"}}}`,
			wantMsg: "Access denied",
		},
		{
			name:       "array of messages",
			body:       `{"errors":[{"message":"Dated on can't be blank"},{"message":"Contact is required"}]}`,
			wantFields: []FieldError{{Message: "Dated on can't be blank"}, {Message: "Contact is required"}},
		},
		{
			name:       "field to messages map",
			body:       `{"errors":{"dated_on":["can't be blank"],"contact":["is required","must be active"]}}`,
			wantFields: []FieldError{{Field: "contact", Message: "is required"}, {Field: "contact", Message: "must be active"}, {Field: "dated_on", Message: "can't be blank"}},
		},
		{
			name:    "oauth error pair",
			body:    `{"error":"invalid_grant","error_description":"The refresh token is invalid"}`,
			wantMsg: "The refresh token is invalid",
		},
		{
			name:    "bare error string",
			body:    `{"error":"invalid_client"}`,
			wantMsg: "invalid_client",
		},
		{
			name:    "plain text body",
			body:    "You must not exceed 15 requests per 60 seconds\n",
			wantMsg: "You must not exceed 15 requests per 60 seconds",
		},
		{
			name:    "html from an intermediary",
			body:    "<html><body>502 Bad Gateway</body></html>",
			wantMsg: "<html><body>502 Bad Gateway</body></html>",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			msg, fields := parseErrorBody([]byte(tc.body))
			if msg != tc.wantMsg {
				t.Fatalf("message = %q, want %q", msg, tc.wantMsg)
			}
			if len(fields) != len(tc.wantFields) {
				t.Fatalf("fields = %v, want %v", fields, tc.wantFields)
			}
			for i := range fields {
				if fields[i] != tc.wantFields[i] {
					t.Fatalf("fields[%d] = %v, want %v", i, fields[i], tc.wantFields[i])
				}
			}
		})
	}
}

func TestAPIErrorSentinels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, ErrUnauthorized},
		{http.StatusForbidden, ErrForbidden},
		{http.StatusNotFound, ErrNotFound},
		{http.StatusUnprocessableEntity, ErrValidation},
		{http.StatusTooManyRequests, ErrRateLimited},
		{http.StatusInternalServerError, ErrServer},
		{http.StatusBadGateway, ErrServer},
	}
	for _, tc := range tests {
		err := error(&APIError{StatusCode: tc.status})
		if !errors.Is(err, tc.want) {
			t.Fatalf("status %d: errors.Is(%v) = false, want true", tc.status, tc.want)
		}
	}
	// A status with no sentinel must not match any of them.
	teapot := error(&APIError{StatusCode: http.StatusTeapot})
	for _, sentinel := range []error{ErrUnauthorized, ErrNotFound, ErrValidation, ErrRateLimited, ErrServer} {
		if errors.Is(teapot, sentinel) {
			t.Fatalf("418 matched %v, want no match", sentinel)
		}
	}
}

func TestNewAPIErrorCapturesContext(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPut, "https://api.freeagent.com/v2/invoices/7", nil)
	resp := &http.Response{
		StatusCode: http.StatusUnprocessableEntity,
		Header:     http.Header{"X-Request-Id": {"abc123"}, "Retry-After": {"30"}},
		Request:    req,
	}
	err := newAPIError(resp, []byte(`{"errors":{"dated_on":["can't be blank"]}}`))

	if err.Method != http.MethodPut || err.URL != "/v2/invoices/7" {
		t.Fatalf("method/URL = %s %s, want PUT /v2/invoices/7", err.Method, err.URL)
	}
	if err.RequestID != "abc123" {
		t.Fatalf("RequestID = %q, want abc123", err.RequestID)
	}
	if err.RetryAfter != 30*time.Second {
		t.Fatalf("RetryAfter = %s, want 30s", err.RetryAfter)
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("errors.Is(ErrValidation) = false")
	}
	want := `freeagent: PUT /v2/invoices/7: 422 Unprocessable Entity (dated_on: can't be blank) [request abc123]`
	if err.Error() != want {
		t.Fatalf("Error() =\n%q\nwant\n%q", err.Error(), want)
	}
}

func TestAPIErrorBodyIsTruncated(t *testing.T) {
	t.Parallel()
	huge := make([]byte, errorBodyLimit*2)
	for i := range huge {
		huge[i] = 'x'
	}
	resp := &http.Response{StatusCode: http.StatusInternalServerError, Header: http.Header{}}
	err := newAPIError(resp, huge)
	if len(err.Body) != errorBodyLimit {
		t.Fatalf("len(Body) = %d, want %d", len(err.Body), errorBodyLimit)
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.March, 27, 20, 47, 40, 0, time.UTC)
	tests := []struct {
		in   string
		want time.Duration
	}{
		{in: "", want: 0},
		{in: "60", want: time.Minute},
		{in: "0", want: 0},
		{in: "-5", want: 0},
		{in: "garbage", want: 0},
		{in: now.Add(90 * time.Second).Format(http.TimeFormat), want: 90 * time.Second},
		{in: now.Add(-90 * time.Second).Format(http.TimeFormat), want: 0},
	}
	for _, tc := range tests {
		if got := parseRetryAfter(tc.in, now); got != tc.want {
			t.Fatalf("parseRetryAfter(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}
