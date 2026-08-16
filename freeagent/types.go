package freeagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Decimal carries money, rates and quantities. FreeAgent sends these as JSON
// strings such as "-90.0" and "0.25", so float64 would lose exactness on
// values the accounting system treats as authoritative.
type Decimal = decimal.Decimal

// Layouts used on the wire. FreeAgent documents dates as YYYY-MM-DD and
// timestamps as ISO 8601 with milliseconds, which is what updated_since
// filters expect to receive back.
const (
	DateLayout      = "2006-01-02"
	TimestampLayout = "2006-01-02T15:04:05.000Z07:00"
)

// maxScalarLen bounds the strings handed to the scalar parsers. Nothing valid
// comes close, so anything larger is a broken or hostile response rather than
// data worth spending time on.
const maxScalarLen = 64

// ErrNotAMember is returned by ResourceURL.ID for URLs that do not address a
// single member of a collection, such as the /v2/company singleton.
var ErrNotAMember = errors.New("freeagent: resource URL does not address a collection member")

// Date is a calendar date with no time component, serialised as YYYY-MM-DD.
// The zero value marshals to null, which FreeAgent reads as unset.
type Date struct {
	time.Time
}

// NewDate builds a Date in UTC.
func NewDate(year int, month time.Month, day int) Date {
	return Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// DateOf truncates t to its calendar date in t's own location.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{time.Date(y, m, d, 0, 0, 0, 0, time.UTC)}
}

// ParseDate accepts the documented YYYY-MM-DD form and, tolerantly, a full
// timestamp: several endpoints document a date but return a timestamp.
func ParseDate(s string) (Date, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Date{}, nil
	}
	if len(s) > maxScalarLen {
		return Date{}, fmt.Errorf("freeagent: date %q is implausibly long (%d bytes)", truncate(s, 32), len(s))
	}
	if t, err := time.Parse(DateLayout, s); err == nil {
		return Date{t}, nil
	}
	if t, err := parseTimestamp(s); err == nil {
		return DateOf(t), nil
	}
	return Date{}, fmt.Errorf("freeagent: cannot parse %q as a date", truncate(s, 32))
}

// String renders the date in wire format, or "" when zero.
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return d.Format(DateLayout)
}

// MarshalJSON implements json.Marshaler.
func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(d.Format(DateLayout))
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Date) UnmarshalJSON(b []byte) error {
	s, ok, err := jsonScalarString(b)
	if err != nil || !ok {
		*d = Date{}
		return err
	}
	parsed, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// Time is an instant, serialised as ISO 8601 with milliseconds in UTC. The
// zero value marshals to null.
type Time struct {
	time.Time
}

// TimeOf wraps a standard time.
func TimeOf(t time.Time) Time { return Time{t} }

// ParseTime accepts the timestamp forms FreeAgent has been observed to emit.
func ParseTime(s string) (Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Time{}, nil
	}
	if len(s) > maxScalarLen {
		return Time{}, fmt.Errorf("freeagent: timestamp %q is implausibly long (%d bytes)", truncate(s, 32), len(s))
	}
	t, err := parseTimestamp(s)
	if err != nil {
		return Time{}, err
	}
	return Time{t}, nil
}

// String renders the timestamp in wire format, or "" when zero.
func (t Time) String() string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(TimestampLayout)
}

// MarshalJSON implements json.Marshaler.
func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.UTC().Format(TimestampLayout))
}

// UnmarshalJSON implements json.Unmarshaler.
func (t *Time) UnmarshalJSON(b []byte) error {
	s, ok, err := jsonScalarString(b)
	if err != nil || !ok {
		*t = Time{}
		return err
	}
	parsed, err := ParseTime(s)
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// timestampLayouts are tried in order. RFC3339 covers both the second and
// millisecond precision the API mixes; the tail entries cover zone-less and
// date-only values seen in older payloads.
var timestampLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05",
	DateLayout,
}

func parseTimestamp(s string) (time.Time, error) {
	for _, layout := range timestampLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("freeagent: cannot parse %q as a timestamp", truncate(s, 32))
}

// jsonScalarString unwraps a JSON string, reporting ok=false for null. It
// rejects arrays and objects rather than silently zeroing the field.
func jsonScalarString(b []byte) (string, bool, error) {
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "" || trimmed == "null" {
		return "", false, nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return "", false, fmt.Errorf("freeagent: expected a JSON string, got %s", truncate(trimmed, 32))
	}
	if s == "" {
		return "", false, nil
	}
	return s, true, nil
}

// Int64 accepts either a JSON number or a numeric string, and always writes a
// number. FreeAgent is inconsistent here: the company documentation types id
// as an integer while the example on the same page returns "12345" quoted.
type Int64 int64

// Int64Of converts a plain int64.
func Int64Of(v int64) Int64 { return Int64(v) }

// Value returns the underlying integer.
func (n Int64) Value() int64 { return int64(n) }

// MarshalJSON implements json.Marshaler.
func (n Int64) MarshalJSON() ([]byte, error) {
	return strconv.AppendInt(nil, int64(n), 10), nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (n *Int64) UnmarshalJSON(b []byte) error {
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "" || trimmed == "null" {
		*n = 0
		return nil
	}
	if len(trimmed) > maxScalarLen {
		return fmt.Errorf("freeagent: numeric value is implausibly long (%d bytes)", len(trimmed))
	}
	// Strip one layer of quotes rather than rejecting the string form.
	unquoted := strings.Trim(trimmed, `"`)
	if unquoted == "" {
		*n = 0
		return nil
	}
	parsed, err := strconv.ParseInt(unquoted, 10, 64)
	if err != nil {
		return fmt.Errorf("freeagent: cannot parse %s as an integer", truncate(trimmed, 32))
	}
	*n = Int64(parsed)
	return nil
}

// ResourceURL is how FreeAgent identifies records. Every cross-reference in a
// payload is a full URL rather than a bare id, so this type carries them and
// extracts the parts callers actually need.
type ResourceURL string

// String returns the URL unchanged.
func (r ResourceURL) String() string { return string(r) }

// IsZero reports whether the reference is unset.
func (r ResourceURL) IsZero() bool { return strings.TrimSpace(string(r)) == "" }

// ID returns the numeric identifier of a collection member URL. Singleton
// URLs such as /v2/company return ErrNotAMember.
func (r ResourceURL) ID() (int64, error) {
	id, _, err := r.member()
	return id, err
}

// Kind returns the collection segment of a member URL, for example "invoices"
// for .../v2/invoices/123. It returns "" when the URL is not a member URL,
// which makes it safe to use for routing a heterogeneous set of references.
func (r ResourceURL) Kind() string {
	_, kind, err := r.member()
	if err != nil {
		return ""
	}
	return kind
}

// member decomposes a member URL into its id and collection segment. ID and
// Kind share it so the two can never disagree about whether a URL addresses
// a record.
func (r ResourceURL) member() (int64, string, error) {
	segs, err := r.segments()
	if err != nil {
		return 0, "", err
	}
	if len(segs) < 2 {
		return 0, "", fmt.Errorf("%w: %q", ErrNotAMember, truncate(string(r), 64))
	}
	id, err := strconv.ParseInt(segs[len(segs)-1], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", fmt.Errorf("%w: %q", ErrNotAMember, truncate(string(r), 64))
	}
	return id, segs[len(segs)-2], nil
}

func (r ResourceURL) segments() ([]string, error) {
	raw := strings.TrimSpace(string(r))
	if raw == "" {
		return nil, fmt.Errorf("%w: empty", ErrNotAMember)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("freeagent: invalid resource URL %q: %w", truncate(raw, 64), err)
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	out := segs[:0]
	for _, s := range segs {
		if s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
