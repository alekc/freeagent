package main

import (
	"net/url"
	"strings"
	"testing"

	"github.com/alekc/freeagent-sdk/freeagent"
)

// The three irregular families are rejected locally rather than letting the
// user decode a remote 404 or 422.
func TestCheckListable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		resource string
		params   url.Values
		fetchAll bool
		wantErr  string
	}{
		{
			name:     "ordinary collection",
			resource: "invoices",
		},
		{
			name:     "attachments have no list endpoint",
			resource: "attachments",
			wantErr:  "no collection endpoint",
		},
		{
			name:     "bank transactions need an account",
			resource: "bank_transactions",
			wantErr:  "needs a bank account",
		},
		{
			name:     "bank transactions with an account",
			resource: "bank_transactions",
			params:   url.Values{"bank_account": {"https://api.freeagent.com/v2/bank_accounts/1"}},
		},
		{
			name:     "categories cannot be paged",
			resource: "categories",
			fetchAll: true,
			wantErr:  "grouped results",
		},
		{
			name:     "categories without -all",
			resource: "categories",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			meta, ok := freeagent.LookupResource(tc.resource)
			if !ok {
				t.Fatalf("resource %q is not registered", tc.resource)
			}
			err := checkListable(meta, tc.params, tc.fetchAll)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("checkListable = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("checkListable = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestParseDateRange(t *testing.T) {
	t.Parallel()
	var opts freeagent.ListOptions
	if err := parseDateRange(&opts, "2026-08-01T09:00:00Z", "2026-01-01", "2026-12-31"); err != nil {
		t.Fatalf("parseDateRange = %v", err)
	}
	if got := opts.UpdatedSince.String(); got != "2026-08-01T09:00:00.000Z" {
		t.Fatalf("updated_since = %q", got)
	}
	if opts.FromDate.String() != "2026-01-01" || opts.ToDate.String() != "2026-12-31" {
		t.Fatalf("range = %s to %s", opts.FromDate, opts.ToDate)
	}

	// Empty inputs leave the option untouched rather than zeroing it.
	var untouched freeagent.ListOptions
	if err := parseDateRange(&untouched, "", "", ""); err != nil {
		t.Fatalf("parseDateRange = %v", err)
	}
	if !untouched.UpdatedSince.IsZero() || !untouched.FromDate.IsZero() {
		t.Fatalf("empty inputs set values: %+v", untouched)
	}

	for _, bad := range [][3]string{
		{"not-a-time", "", ""},
		{"", "not-a-date", ""},
		{"", "", "not-a-date"},
	} {
		var target freeagent.ListOptions
		if err := parseDateRange(&target, bad[0], bad[1], bad[2]); err == nil {
			t.Fatalf("parseDateRange(%v) succeeded, want an error", bad)
		}
	}
}

func TestPrintJSONHandlesNonJSON(t *testing.T) {
	t.Parallel()
	// Nothing to print and unparseable input must both be non-fatal: the
	// point is to show the user whatever came back.
	if err := printJSON(nil); err != nil {
		t.Fatalf("printJSON(nil) = %v", err)
	}
	if err := printJSON([]byte("   ")); err != nil {
		t.Fatalf("printJSON(blank) = %v", err)
	}
	if err := printJSON([]byte("<html>not json</html>")); err != nil {
		t.Fatalf("printJSON(html) = %v", err)
	}
	if err := printJSON([]byte(`{"a":1}`)); err != nil {
		t.Fatalf("printJSON(json) = %v", err)
	}
}

func TestReportPagingToleratesNil(t *testing.T) {
	t.Parallel()
	reportPaging(nil)
	reportPaging(&freeagent.Response{TotalCount: -1})
	reportPaging(&freeagent.Response{TotalCount: 5, NextPage: 2, LastPage: 3})
}
