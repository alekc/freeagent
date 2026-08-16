package freeagent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestDateJSONRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string // rendered back out
		zero bool
	}{
		{name: "documented form", in: `"2019-12-01"`, want: "2019-12-01"},
		{name: "null is zero", in: `null`, zero: true},
		{name: "empty string is zero", in: `""`, zero: true},
		{name: "timestamp truncates to its date", in: `"2020-02-06T11:08:28.000Z"`, want: "2020-02-06"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got Date
			if err := json.Unmarshal([]byte(tc.in), &got); err != nil {
				t.Fatalf("Unmarshal(%s) = %v", tc.in, err)
			}
			if tc.zero {
				if !got.IsZero() {
					t.Fatalf("Unmarshal(%s) = %v, want zero", tc.in, got)
				}
				out, err := json.Marshal(got)
				if err != nil {
					t.Fatalf("Marshal = %v", err)
				}
				if string(out) != "null" {
					t.Fatalf("Marshal(zero) = %s, want null", out)
				}
				return
			}
			if got.String() != tc.want {
				t.Fatalf("String() = %q, want %q", got.String(), tc.want)
			}
			out, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("Marshal = %v", err)
			}
			if string(out) != `"`+tc.want+`"` {
				t.Fatalf("Marshal = %s, want %q", out, tc.want)
			}
		})
	}
}

func TestDateRejectsGarbage(t *testing.T) {
	t.Parallel()
	tests := []string{
		`"not-a-date"`,
		`"01/12/2019"`,
		`["2019-12-01"]`,
		`{"date":"2019-12-01"}`,
		`"` + strings.Repeat("9", maxScalarLen+1) + `"`,
	}
	for _, in := range tests {
		var got Date
		if err := json.Unmarshal([]byte(in), &got); err == nil {
			t.Fatalf("Unmarshal(%s) succeeded with %v, want an error", in, got)
		}
	}
}

func TestTimeJSONRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "seconds precision", in: `"2011-09-14T16:00:41Z"`, want: "2011-09-14T16:00:41.000Z"},
		{name: "millisecond precision", in: `"2020-02-06T11:08:28.000Z"`, want: "2020-02-06T11:08:28.000Z"},
		{name: "offset normalises to UTC", in: `"2020-02-06T12:08:28+01:00"`, want: "2020-02-06T11:08:28.000Z"},
		{name: "zone-less is read as UTC", in: `"2020-02-06T11:08:28"`, want: "2020-02-06T11:08:28.000Z"},
		{name: "bare date", in: `"2020-02-06"`, want: "2020-02-06T00:00:00.000Z"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got Time
			if err := json.Unmarshal([]byte(tc.in), &got); err != nil {
				t.Fatalf("Unmarshal(%s) = %v", tc.in, err)
			}
			out, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("Marshal = %v", err)
			}
			if string(out) != `"`+tc.want+`"` {
				t.Fatalf("Marshal = %s, want %q", out, tc.want)
			}
		})
	}
}

// The updated_since filter is the basis for incremental reads, so the query
// rendering of a timestamp has to match what the API documents.
func TestTimeRendersUpdatedSinceFormat(t *testing.T) {
	t.Parallel()
	ts := TimeOf(time.Date(2017, time.May, 22, 9, 0, 0, 0, time.UTC))
	if got, want := ts.String(), "2017-05-22T09:00:00.000Z"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestResourceURLID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      ResourceURL
		wantID  int64
		wantErr bool
		kind    string
	}{
		{name: "member URL", in: "https://api.freeagent.com/v2/invoices/123", wantID: 123, kind: "invoices"},
		{name: "trailing slash", in: "https://api.freeagent.com/v2/bank_accounts/1/", wantID: 1, kind: "bank_accounts"},
		{name: "query string ignored", in: "https://api.freeagent.com/v2/contacts/2?view=all", wantID: 2, kind: "contacts"},
		{name: "singleton has no id", in: "https://api.freeagent.com/v2/company", wantErr: true},
		{name: "report path has no id", in: "https://api.freeagent.com/v2/accounting/balance_sheet", wantErr: true},
		{name: "empty", in: "", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			id, err := tc.in.ID()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ID() = %d, want an error", id)
				}
				if !errors.Is(err, ErrNotAMember) {
					t.Fatalf("ID() error = %v, want ErrNotAMember", err)
				}
				if got := tc.in.Kind(); got != "" {
					t.Fatalf("Kind() = %q, want empty for a non-member URL", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ID() = %v", err)
			}
			if id != tc.wantID {
				t.Fatalf("ID() = %d, want %d", id, tc.wantID)
			}
			if got := tc.in.Kind(); got != tc.kind {
				t.Fatalf("Kind() = %q, want %q", got, tc.kind)
			}
		})
	}
}

// Money arrives as a JSON string and has to survive the round trip exactly,
// which is the whole reason for not using float64.
func TestDecimalRoundTripsExactly(t *testing.T) {
	t.Parallel()
	type payload struct {
		Gross Decimal  `json:"gross_value"`
		Rate  Decimal  `json:"rebill_factor"`
		Maybe *Decimal `json:"maybe"`
	}
	const in = `{"gross_value":"-90.0","rebill_factor":"0.25","maybe":null}`

	var got payload
	if err := json.Unmarshal([]byte(in), &got); err != nil {
		t.Fatalf("Unmarshal = %v", err)
	}
	if want := decimal.RequireFromString("-90.0"); !got.Gross.Equal(want) {
		t.Fatalf("gross_value = %s, want %s", got.Gross, want)
	}
	if want := decimal.RequireFromString("0.25"); !got.Rate.Equal(want) {
		t.Fatalf("rebill_factor = %s, want %s", got.Rate, want)
	}
	if got.Maybe != nil {
		t.Fatalf("maybe = %v, want nil for JSON null", got.Maybe)
	}

	// Writes must go back out as strings, which is what the API accepts.
	out, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal = %v", err)
	}
	if !strings.Contains(string(out), `"gross_value":"-90"`) {
		t.Fatalf("Marshal = %s, want gross_value as a quoted string", out)
	}
}

func FuzzParseDate(f *testing.F) {
	for _, seed := range []string{"2019-12-01", "", "null", "2020-02-06T11:08:28.000Z", "9999-99-99"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		d, err := ParseDate(in)
		if err != nil {
			return
		}
		// A successful parse must render back to something parseable.
		if !d.IsZero() {
			if _, err := ParseDate(d.String()); err != nil {
				t.Fatalf("ParseDate(%q) produced %q which does not re-parse: %v", in, d.String(), err)
			}
		}
	})
}

func FuzzResourceURLID(f *testing.F) {
	for _, seed := range []string{
		"https://api.freeagent.com/v2/invoices/1",
		"://",
		"/v2//",
		"https://api.freeagent.com/v2/company",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		ref := ResourceURL(in)
		id, err := ref.ID()
		kind := ref.Kind()
		if err == nil && id < 0 {
			t.Fatalf("ID() = %d for %q, want a non-negative id or an error", id, in)
		}
		if err != nil && kind != "" {
			t.Fatalf("Kind() = %q for %q but ID() failed, the two must agree", kind, in)
		}
	})
}

// omitempty has no effect on struct types, so a zero Date or Time tagged that
// way serialises as null and is sent on every write. Some endpoints tolerate
// it; capital asset types answer 400 "found unpermitted parameters". Every
// Date and Time field must therefore use omitzero, which does honour IsZero.
func TestDateAndTimeFieldsUseOmitzero(t *testing.T) {
	t.Parallel()

	// The behaviour this guards against, pinned so the reason stays visible.
	withOmitEmpty := struct {
		D Date `json:"d,omitempty"`
		T Time `json:"t,omitempty"`
	}{}
	out, err := json.Marshal(withOmitEmpty)
	if err != nil {
		t.Fatalf("Marshal = %v", err)
	}
	if string(out) != `{"d":null,"t":null}` {
		t.Fatalf("omitempty on a zero Date/Time emitted %s; if Go has changed, this guard can go", out)
	}

	// And the fix.
	withOmitZero := struct {
		D Date `json:"d,omitzero"`
		T Time `json:"t,omitzero"`
	}{}
	out, err = json.Marshal(withOmitZero)
	if err != nil {
		t.Fatalf("Marshal = %v", err)
	}
	if string(out) != `{}` {
		t.Fatalf("omitzero on a zero Date/Time emitted %s, want {}", out)
	}

	// A set value still serialises.
	set := struct {
		D Date `json:"d,omitzero"`
	}{D: NewDate(2026, 8, 16)}
	out, err = json.Marshal(set)
	if err != nil {
		t.Fatalf("Marshal = %v", err)
	}
	if string(out) != `{"d":"2026-08-16"}` {
		t.Fatalf("omitzero dropped a set value: %s", out)
	}
}

// No model may reintroduce omitempty on a Date or Time field.
func TestNoDateOrTimeFieldUsesOmitempty(t *testing.T) {
	t.Parallel()
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("Glob = %v", err)
	}
	offender := regexp.MustCompile(`\b(?:Date|Time)\s+` + "`" + `json:"[^"]+,omitempty"` + "`")
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) = %v", path, err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if offender.MatchString(line) {
				t.Errorf("%s:%d uses omitempty on a Date/Time field, which never omits: %s",
					path, i+1, strings.TrimSpace(line))
			}
		}
	}
}
