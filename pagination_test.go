package freeagent

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestListOptionsValues(t *testing.T) {
	t.Parallel()
	opts := &ListOptions{
		Page:         2,
		PerPage:      100,
		UpdatedSince: TimeOf(time.Date(2017, time.May, 22, 9, 0, 0, 0, time.UTC)),
		Sort:         "-updated_at",
		View:         "open_or_overdue",
		FromDate:     NewDate(2012, time.January, 1),
		ToDate:       NewDate(2012, time.March, 31),
		Extra:        url.Values{"bank_account": {"https://api.freeagent.com/v2/bank_accounts/1"}},
	}
	got, err := opts.Values()
	if err != nil {
		t.Fatalf("Values() = %v", err)
	}
	want := url.Values{
		"page":          {"2"},
		"per_page":      {"100"},
		"updated_since": {"2017-05-22T09:00:00.000Z"},
		"sort":          {"-updated_at"},
		"view":          {"open_or_overdue"},
		"from_date":     {"2012-01-01"},
		"to_date":       {"2012-03-31"},
		"bank_account":  {"https://api.freeagent.com/v2/bank_accounts/1"},
	}
	if got.Encode() != want.Encode() {
		t.Fatalf("Values() =\n%s\nwant\n%s", got.Encode(), want.Encode())
	}
}

func TestListOptionsZeroValueIsEmpty(t *testing.T) {
	t.Parallel()
	for name, opts := range map[string]*ListOptions{"nil": nil, "zero": {}} {
		got, err := opts.Values()
		if err != nil {
			t.Fatalf("%s: Values() = %v", name, err)
		}
		if len(got) != 0 {
			t.Fatalf("%s: Values() = %v, want empty", name, got)
		}
	}
}

// Out-of-range paging is rejected locally. Clamping silently would send a
// different request than the caller asked for.
func TestListOptionsRejectsOutOfRange(t *testing.T) {
	t.Parallel()
	for _, opts := range []*ListOptions{
		{PerPage: MaxPerPage + 1},
		{PerPage: -1},
		{Page: -1},
	} {
		if _, err := opts.Values(); err == nil {
			t.Fatalf("Values(%+v) succeeded, want an error", opts)
		}
	}
}

func TestListOptionsExtraOverridesBuiltIn(t *testing.T) {
	t.Parallel()
	opts := &ListOptions{Sort: "created_at", Extra: url.Values{"sort": {"-updated_at"}}}
	got, err := opts.Values()
	if err != nil {
		t.Fatalf("Values() = %v", err)
	}
	if want := "-updated_at"; got.Get("sort") != want {
		t.Fatalf("sort = %q, want %q", got.Get("sort"), want)
	}
	if len(got["sort"]) != 1 {
		t.Fatalf("sort = %v, want exactly one value", got["sort"])
	}
}

func TestParseLinkHeader(t *testing.T) {
	t.Parallel()
	header := `<https://api.freeagent.com/v2/invoices?page=1&per_page=25>; rel="first", ` +
		`<https://api.freeagent.com/v2/invoices?page=2&per_page=25>; rel="prev", ` +
		`<https://api.freeagent.com/v2/invoices?page=4&per_page=25>; rel="next", ` +
		`<https://api.freeagent.com/v2/invoices?page=9&per_page=25>; rel="last"`

	resp := newResponse(&http.Response{Header: http.Header{
		"Link":          {header},
		"X-Total-Count": {"214"},
	}})

	if resp.FirstPage != 1 || resp.PrevPage != 2 || resp.NextPage != 4 || resp.LastPage != 9 {
		t.Fatalf("pages = first %d prev %d next %d last %d, want 1 2 4 9",
			resp.FirstPage, resp.PrevPage, resp.NextPage, resp.LastPage)
	}
	if resp.TotalCount != 214 {
		t.Fatalf("TotalCount = %d, want 214", resp.TotalCount)
	}
}

// A comma inside the bracketed URI must not split the header.
func TestParseLinkHeaderWithCommaInURI(t *testing.T) {
	t.Parallel()
	header := `<https://api.freeagent.com/v2/invoices?page=2&view=a,b>; rel="next"`
	links := parseLinkHeader(header)
	if len(links) != 1 {
		t.Fatalf("parsed %d links, want 1: %v", len(links), links)
	}
	if got := pageFromURL(links["next"]); got != 2 {
		t.Fatalf("next page = %d, want 2", got)
	}
}

func TestResponseWithoutHeaders(t *testing.T) {
	t.Parallel()
	resp := newResponse(&http.Response{Header: http.Header{}})
	if resp.NextPage != 0 {
		t.Fatalf("NextPage = %d, want 0 when there is no Link header", resp.NextPage)
	}
	// -1 distinguishes "not sent" from a genuine zero.
	if resp.TotalCount != -1 {
		t.Fatalf("TotalCount = %d, want -1 when X-Total-Count is absent", resp.TotalCount)
	}
}

func TestPageFromURLRejectsJunk(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", "://", "https://example.com/v2/invoices", "https://example.com/v2/invoices?page=0", "https://example.com/v2/invoices?page=abc"} {
		if got := pageFromURL(in); got != 0 {
			t.Fatalf("pageFromURL(%q) = %d, want 0", in, got)
		}
	}
}
