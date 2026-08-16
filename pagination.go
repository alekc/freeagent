package freeagent

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// MaxPerPage is the largest page size FreeAgent accepts.
const MaxPerPage = 100

// ListOptions carries the filters shared by the collection endpoints. Not
// every resource honours every field; the API ignores what it does not know,
// and Extra is the escape hatch for resource-specific parameters.
type ListOptions struct {
	// Page is 1-based. Zero means the server default (page 1).
	Page int
	// PerPage defaults to 25 server-side and may not exceed MaxPerPage.
	PerPage int
	// UpdatedSince filters to records changed at or after this instant. It is
	// the basis for incremental reads.
	UpdatedSince Time
	// Sort is a field name, optionally prefixed with "-" for descending, for
	// example "-updated_at".
	Sort string
	// View selects a server-side named filter such as "open_or_overdue".
	View string
	// FromDate and ToDate bound date-ranged collections such as bank
	// transactions.
	FromDate Date
	ToDate   Date
	// Extra is merged last and wins on key collisions.
	Extra url.Values
}

// Values renders the options as a query string, rejecting values the API
// would refuse rather than letting the request go out and fail remotely. It
// is exported so callers using Client.Raw get the same validation.
func (o *ListOptions) Values() (url.Values, error) {
	v := url.Values{}
	if o == nil {
		return v, nil
	}
	if o.Page < 0 {
		return nil, fmt.Errorf("freeagent: ListOptions.Page must not be negative, got %d", o.Page)
	}
	if o.PerPage < 0 || o.PerPage > MaxPerPage {
		return nil, fmt.Errorf("freeagent: ListOptions.PerPage must be between 0 and %d, got %d", MaxPerPage, o.PerPage)
	}
	if o.Page > 0 {
		v.Set("page", strconv.Itoa(o.Page))
	}
	if o.PerPage > 0 {
		v.Set("per_page", strconv.Itoa(o.PerPage))
	}
	if !o.UpdatedSince.IsZero() {
		v.Set("updated_since", o.UpdatedSince.String())
	}
	if o.Sort != "" {
		v.Set("sort", o.Sort)
	}
	if o.View != "" {
		v.Set("view", o.View)
	}
	if !o.FromDate.IsZero() {
		v.Set("from_date", o.FromDate.String())
	}
	if !o.ToDate.IsZero() {
		v.Set("to_date", o.ToDate.String())
	}
	for key, vals := range o.Extra {
		v.Del(key)
		for _, val := range vals {
			v.Add(key, val)
		}
	}
	return v, nil
}

// clone returns a copy safe to mutate while iterating. Extra is shared
// deliberately: iteration never writes to it.
func (o *ListOptions) clone() ListOptions {
	if o == nil {
		return ListOptions{}
	}
	return *o
}

// Response wraps the HTTP response with the pagination and rate-limit state
// parsed out of its headers.
type Response struct {
	*http.Response

	// Page numbers extracted from the Link header. Zero means the relation
	// was absent, so NextPage == 0 is the end of the collection.
	FirstPage int
	PrevPage  int
	NextPage  int
	LastPage  int

	// TotalCount comes from X-Total-Count and is -1 when not sent.
	TotalCount int

	RateLimit RateLimit
}

func newResponse(r *http.Response) *Response {
	resp := &Response{Response: r, TotalCount: -1}
	if r == nil {
		return resp
	}
	resp.RateLimit = parseRateLimit(r.Header)
	if v := r.Header.Get("X-Total-Count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			resp.TotalCount = n
		}
	}
	for rel, link := range parseLinkHeader(r.Header.Get("Link")) {
		page := pageFromURL(link)
		if page == 0 {
			continue
		}
		switch rel {
		case "first":
			resp.FirstPage = page
		case "prev", "previous":
			resp.PrevPage = page
		case "next":
			resp.NextPage = page
		case "last":
			resp.LastPage = page
		}
	}
	return resp
}

// parseLinkHeader reads an RFC 8288 Link header into a rel-to-URL map. Only
// the rel parameter is interesting here, so other parameters are skipped
// rather than parsed.
func parseLinkHeader(header string) map[string]string {
	if strings.TrimSpace(header) == "" {
		return nil
	}
	out := map[string]string{}
	for _, part := range splitLinkHeader(header) {
		segments := strings.Split(part, ";")
		target := strings.TrimSpace(segments[0])
		if !strings.HasPrefix(target, "<") || !strings.HasSuffix(target, ">") {
			continue
		}
		target = target[1 : len(target)-1]
		for _, param := range segments[1:] {
			key, value, found := strings.Cut(strings.TrimSpace(param), "=")
			if !found || strings.TrimSpace(key) != "rel" {
				continue
			}
			rel := strings.ToLower(strings.Trim(strings.TrimSpace(value), `"'`))
			if rel != "" {
				out[rel] = target
			}
		}
	}
	return out
}

// splitLinkHeader splits on commas that separate link-values, ignoring commas
// inside the angle-bracketed URI reference.
func splitLinkHeader(header string) []string {
	var (
		parts []string
		start int
		inURI bool
	)
	for i := 0; i < len(header); i++ {
		switch header[i] {
		case '<':
			inURI = true
		case '>':
			inURI = false
		case ',':
			if !inURI {
				parts = append(parts, header[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, header[start:])
}

// pageFromURL pulls the page number out of a Link target. The number is used
// to rebuild the request locally rather than following the URL, so a
// malformed or redirected target cannot send the client somewhere unintended.
func pageFromURL(raw string) int {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return 0
	}
	page, err := strconv.Atoi(u.Query().Get("page"))
	if err != nil || page < 1 {
		return 0
	}
	return page
}
