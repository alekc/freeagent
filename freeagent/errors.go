package freeagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Sentinels for errors.Is. They classify by HTTP status so callers can branch
// without importing net/http or matching on message text.
var (
	ErrUnauthorized = errors.New("freeagent: unauthorized")
	ErrForbidden    = errors.New("freeagent: forbidden")
	ErrNotFound     = errors.New("freeagent: not found")
	ErrValidation   = errors.New("freeagent: validation failed")
	ErrRateLimited  = errors.New("freeagent: rate limited")
	ErrServer       = errors.New("freeagent: server error")
)

// errorBodyLimit caps how much of an error response is read and retained.
// Error bodies are occasionally an HTML page from an intermediary rather than
// the documented JSON, and those are worth sampling, not storing.
const errorBodyLimit = 8 << 10

// maxFieldErrors bounds the per-field validation detail retained from a 422.
const maxFieldErrors = 100

// FieldError is one validation failure attributed to a specific attribute.
// Field is empty when the API reported the failure without naming one.
type FieldError struct {
	Field   string
	Message string
}

func (f FieldError) String() string {
	if f.Field == "" {
		return f.Message
	}
	return f.Field + ": " + f.Message
}

// APIError is returned for any response with a 4xx or 5xx status.
type APIError struct {
	StatusCode int
	Method     string
	URL        string
	Message    string
	Errors     []FieldError
	RetryAfter time.Duration
	RequestID  string
	// Body is the raw response, truncated to errorBodyLimit. It is kept so a
	// caller can log the original when the parsed message proves unhelpful.
	Body []byte
}

// Error implements the error interface.
func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "freeagent: %s %s: %d %s", e.Method, e.URL, e.StatusCode, http.StatusText(e.StatusCode))
	if e.Message != "" {
		b.WriteString(": " + e.Message)
	}
	if n := len(e.Errors); n > 0 {
		parts := make([]string, 0, n)
		for _, fe := range e.Errors {
			parts = append(parts, fe.String())
		}
		b.WriteString(" (" + strings.Join(parts, "; ") + ")")
	}
	if e.RequestID != "" {
		b.WriteString(" [request " + e.RequestID + "]")
	}
	return b.String()
}

// Is maps the status code onto the package sentinels.
func (e *APIError) Is(target error) bool {
	s := sentinelForStatus(e.StatusCode)
	return s != nil && errors.Is(s, target)
}

func sentinelForStatus(code int) error {
	switch {
	case code == http.StatusUnauthorized:
		return ErrUnauthorized
	case code == http.StatusForbidden:
		return ErrForbidden
	case code == http.StatusNotFound:
		return ErrNotFound
	case code == http.StatusUnprocessableEntity:
		return ErrValidation
	case code == http.StatusTooManyRequests:
		return ErrRateLimited
	case code >= 500:
		return ErrServer
	default:
		return nil
	}
}

// newAPIError builds an APIError from a response and its already-read body.
func newAPIError(resp *http.Response, body []byte) *APIError {
	if len(body) > errorBodyLimit {
		body = body[:errorBodyLimit]
	}
	message, fields := parseErrorBody(body)
	e := &APIError{
		StatusCode: resp.StatusCode,
		Message:    message,
		Errors:     fields,
		RequestID:  firstHeader(resp.Header, "X-Request-Id", "X-Request-ID", "X-Correlation-Id"),
		RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
		Body:       body,
	}
	if resp.Request != nil {
		e.Method = resp.Request.Method
		if resp.Request.URL != nil {
			e.URL = resp.Request.URL.Path
		}
	}
	return e
}

// parseErrorBody copes with the several error shapes the API emits: the OAuth
// error/error_description pair, a nested errors.error object, an array of
// messages, and a field-to-messages map. Anything unrecognised degrades to
// the first line of the raw body.
func parseErrorBody(body []byte) (string, []FieldError) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "", nil
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return firstLine(trimmed), nil
	}

	message := firstNonEmpty(
		stringField(root, "error_description"),
		stringField(root, "message"),
		stringField(root, "error"),
	)

	fields := parseErrorsMember(root["errors"])
	if message == "" && len(fields) == 1 && fields[0].Field == "" {
		return fields[0].Message, nil
	}
	if message == "" && len(fields) == 0 {
		return firstLine(trimmed), nil
	}
	return message, fields
}

func parseErrorsMember(raw json.RawMessage) []FieldError {
	if len(raw) == 0 {
		return nil
	}

	// {"errors": "something went wrong"}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if asString == "" {
			return nil
		}
		return []FieldError{{Message: asString}}
	}

	// {"errors": [{"message": "..."}, ...]}
	var asList []struct {
		Field   string `json:"field"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &asList); err == nil {
		out := make([]FieldError, 0, len(asList))
		for _, item := range asList {
			if item.Message == "" {
				continue
			}
			out = append(out, FieldError{Field: item.Field, Message: item.Message})
			if len(out) == maxFieldErrors {
				break
			}
		}
		return out
	}

	var asObject map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asObject); err != nil {
		return nil
	}

	// {"errors": {"error": {"message": "..."}}} is the most common shape.
	if nested, ok := asObject["error"]; ok {
		if inner := parseErrorsMember(nested); len(inner) > 0 {
			return inner
		}
	}
	if msg := stringField(asObject, "message"); msg != "" {
		return []FieldError{{Message: msg}}
	}

	// {"errors": {"attribute": ["is invalid", ...]}}
	return fieldMapErrors(asObject)
}

func fieldMapErrors(obj map[string]json.RawMessage) []FieldError {
	names := make([]string, 0, len(obj))
	for name := range obj {
		names = append(names, name)
	}
	// Sorted so the rendered error string is stable between runs.
	sort.Strings(names)

	out := make([]FieldError, 0, len(names))
	for _, name := range names {
		for _, msg := range stringOrList(obj[name]) {
			out = append(out, FieldError{Field: name, Message: msg})
			if len(out) == maxFieldErrors {
				return out
			}
		}
	}
	return out
}

func stringOrList(raw json.RawMessage) []string {
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		if one == "" {
			return nil
		}
		return []string{one}
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		return many
	}
	return nil
}

func stringField(obj map[string]json.RawMessage, key string) string {
	raw, ok := obj[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// parseRetryAfter reads the header in either of its RFC 9110 forms, seconds
// or an HTTP date. A missing or unparsable value yields 0, which callers
// treat as "fall back to the backoff schedule".
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}

func firstHeader(h http.Header, keys ...string) string {
	for _, k := range keys {
		if v := h.Get(k); v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return truncate(strings.TrimSpace(s), 200)
}
