package freeagent

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"net/url"
)

// Note is a free-text note attached to a contact or a project.
//
// See https://dev.freeagent.com/docs/notes
type Note struct {
	URL ResourceURL `json:"url,omitempty"`

	// Note is the content, and is required.
	Note string `json:"note,omitempty"`

	// Read-only. ParentURL points at the contact or project the note hangs
	// off; the parent is chosen with a query parameter on create, not by
	// setting this field.
	ParentURL ResourceURL `json:"parent_url,omitempty"`
	Author    string      `json:"author,omitempty"`
	CreatedAt Time        `json:"created_at,omitzero"`
	UpdatedAt Time        `json:"updated_at,omitzero"`
}

// NoteService covers https://dev.freeagent.com/docs/notes
//
// Notes are always scoped to a parent. Listing without one is a 400, and
// creating without one has nowhere to attach, so the inherited List, All and
// Create are shadowed in favour of the parent-taking forms.
type NoteService struct {
	Collection[Note]
}

// List is unavailable without a parent. Use ListForParent.
func (s *NoteService) List(context.Context, *ListOptions) ([]Note, *Response, error) {
	return nil, nil, errNoteParentRequired("ListForParent")
}

// All is unavailable without a parent. Use AllForParent.
func (s *NoteService) All(context.Context, *ListOptions) iter.Seq2[Note, error] {
	return func(yield func(Note, error) bool) {
		yield(Note{}, errNoteParentRequired("AllForParent"))
	}
}

// Create is unavailable without a parent. Use CreateForParent.
func (s *NoteService) Create(context.Context, *Note) (*Note, *Response, error) {
	return nil, nil, errNoteParentRequired("CreateForParent")
}

// ListForParent fetches the notes on one contact or project.
func (s *NoteService) ListForParent(ctx context.Context, parent ResourceURL, opts *ListOptions) ([]Note, *Response, error) {
	scoped, err := scopeToNoteParent(opts, parent)
	if err != nil {
		return nil, nil, err
	}
	return s.ReadCollection.List(ctx, scoped)
}

// AllForParent iterates every note on one contact or project.
func (s *NoteService) AllForParent(ctx context.Context, parent ResourceURL, opts *ListOptions) iter.Seq2[Note, error] {
	scoped, err := scopeToNoteParent(opts, parent)
	if err != nil {
		return func(yield func(Note, error) bool) {
			yield(Note{}, err)
		}
	}
	return s.ReadCollection.All(ctx, scoped)
}

// CreateForParent adds a note to one contact or project.
func (s *NoteService) CreateForParent(ctx context.Context, parent ResourceURL, in *Note) (*Note, *Response, error) {
	if in == nil {
		return nil, nil, fmt.Errorf("freeagent: CreateForParent requires a non-nil note")
	}
	query, err := noteParentQuery(parent)
	if err != nil {
		return nil, nil, err
	}
	req, err := s.client.newRequest(ctx, http.MethodPost, s.meta.Path, query, map[string]any{s.meta.Singular: in})
	if err != nil {
		return nil, nil, err
	}
	return decodeSingle[Note](s.client, req, s.meta)
}

// noteParentQuery turns a parent reference into the contact= or project=
// filter the endpoint expects, rejecting anything else.
func noteParentQuery(parent ResourceURL) (url.Values, error) {
	switch parent.Kind() {
	case "contacts":
		return url.Values{"contact": {parent.String()}}, nil
	case "projects":
		return url.Values{"project": {parent.String()}}, nil
	default:
		return nil, fmt.Errorf("freeagent: a note parent must be a contact or a project URL, got %q",
			truncate(parent.String(), 64))
	}
}

func scopeToNoteParent(opts *ListOptions, parent ResourceURL) (*ListOptions, error) {
	query, err := noteParentQuery(parent)
	if err != nil {
		return nil, err
	}
	scoped := opts.clone()
	extra := url.Values{}
	for key, values := range scoped.Extra {
		extra[key] = values
	}
	for key, values := range query {
		extra[key] = values
	}
	scoped.Extra = extra
	return &scoped, nil
}

func errNoteParentRequired(alternative string) error {
	return fmt.Errorf("freeagent: notes are scoped to a contact or project, use %s", alternative)
}

// ECMossRate is one VAT rate available for an EU country on a given date.
type ECMossRate struct {
	Percentage *Decimal `json:"percentage,omitempty"`
	// Band is Standard, Reduced, Parking or Super Reduced.
	Band string `json:"band,omitempty"`
}

// ECMossRates is the reply from the EC VAT MOSS rate lookup.
type ECMossRates struct {
	Rates []ECMossRate `json:"sales_tax_rates,omitempty"`
	// ECTaxName is the local name for the tax, usually VAT.
	ECTaxName string `json:"ec_tax_name,omitempty"`
}

// SalesTaxService covers the one endpoint on
// https://dev.freeagent.com/docs/sales_tax
//
// That page is mostly prose about how sales tax is expressed on other
// resources; the EC VAT MOSS rate lookup is the only endpoint it defines.
type SalesTaxService struct {
	client *Client
}

// ECMossRates returns the VAT rates for an EU country on a date. Both are
// required; the country is a name such as "Ireland", not a code.
func (s *SalesTaxService) ECMossRates(ctx context.Context, country string, on Date) (*ECMossRates, *Response, error) {
	if country == "" {
		return nil, nil, fmt.Errorf("freeagent: ECMossRates requires a country name")
	}
	if on.IsZero() {
		return nil, nil, fmt.Errorf("freeagent: ECMossRates requires a date")
	}
	query := url.Values{"country": {country}, "date": {on.String()}}
	req, err := s.client.newRequest(ctx, http.MethodGet, "ec_moss/sales_tax_rates", query, nil)
	if err != nil {
		return nil, nil, err
	}
	out := new(ECMossRates)
	resp, err := s.client.do(req, out)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}
