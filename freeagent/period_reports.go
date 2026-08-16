package freeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// periodService is the shape shared by the filing families: a collection
// addressed by the period end date rather than a numeric id, with
// mark_as_filed style transitions hanging off each member and, for some,
// further transitions hanging off a dated payment inside it.
//
// Final accounts, VAT, corporation tax and income tax all work this way, so
// they share one implementation. base is passed per call because income tax
// returns are nested under a user and the rest are not.
type periodService[T any] struct {
	client *Client
	meta   ResourceMeta
}

// Meta returns the resource metadata.
func (s *periodService[T]) Meta() ResourceMeta { return s.meta }

func (s *periodService[T]) list(ctx context.Context, base string) ([]T, *Response, error) {
	req, err := s.client.newRequest(ctx, http.MethodGet, base, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	var envelope map[string]json.RawMessage
	resp, err := s.client.do(req, &envelope)
	if err != nil {
		return nil, resp, err
	}
	items, err := unwrapList[T](envelope, s.meta.Plural, base)
	if err != nil {
		return nil, resp, err
	}
	return items, resp, nil
}

func (s *periodService[T]) get(ctx context.Context, base string, periodEndsOn Date) (*T, *Response, error) {
	path, err := periodPath(base, periodEndsOn, "")
	if err != nil {
		return nil, nil, err
	}
	req, err := s.client.newRequest(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	return decodeSingle[T](s.client, req, s.meta)
}

func (s *periodService[T]) transition(ctx context.Context, base string, periodEndsOn Date, action string) (*T, *Response, error) {
	path, err := periodPath(base, periodEndsOn, action)
	if err != nil {
		return nil, nil, err
	}
	req, err := s.client.newRequest(ctx, http.MethodPut, path, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	return decodeSingle[T](s.client, req, s.meta)
}

// paymentTransition acts on one dated payment inside a period, which is how
// VAT and income tax record a payment as settled.
func (s *periodService[T]) paymentTransition(ctx context.Context, base string, periodEndsOn, paymentDate Date, action string) (*T, *Response, error) {
	if paymentDate.IsZero() {
		return nil, nil, fmt.Errorf("freeagent: a payment date is required")
	}
	suffix := "payments/" + paymentDate.String() + "/" + action
	path, err := periodPath(base, periodEndsOn, suffix)
	if err != nil {
		return nil, nil, err
	}
	req, err := s.client.newRequest(ctx, http.MethodPut, path, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	return decodeSingle[T](s.client, req, s.meta)
}

// periodPath renders a member path. Dates are formatted rather than taken as
// text, so nothing arbitrary can reach the URL.
func periodPath(base string, periodEndsOn Date, suffix string) (string, error) {
	if periodEndsOn.IsZero() {
		return "", fmt.Errorf("freeagent: a period end date is required")
	}
	path := base + "/" + periodEndsOn.String()
	if suffix != "" {
		path += "/" + suffix
	}
	return path, nil
}

// FilingPayment is one payment due against a filing period.
type FilingPayment struct {
	Label     string   `json:"label,omitempty"`
	DueOn     Date     `json:"due_on,omitzero"`
	AmountDue *Decimal `json:"amount_due,omitempty"`
	// Status is unpaid or marked_as_paid, and is omitted entirely when the
	// amount due is zero or negative.
	Status string `json:"status,omitempty"`
}

// Payment statuses used across the filing families.
const (
	PaymentStatusUnpaid       = "unpaid"
	PaymentStatusMarkedAsPaid = "marked_as_paid"
)

// VATReturn is one VAT period and its filing state.
//
// See https://dev.freeagent.com/docs/vat_returns
type VATReturn struct {
	// URL ends in the period end date, so ResourceURL.ID does not apply.
	URL ResourceURL `json:"url,omitempty"`

	PeriodStartsOn Date `json:"period_starts_on,omitzero"`
	PeriodEndsOn   Date `json:"period_ends_on,omitzero"`
	FilingDueOn    Date `json:"filing_due_on,omitzero"`

	// FilingStatus is unfiled, pending, rejected, filed or marked_as_filed.
	FilingStatus   string `json:"filing_status,omitempty"`
	FiledAt        Time   `json:"filed_at,omitzero"`
	FiledReference string `json:"filed_reference,omitempty"`

	Payments  []FilingPayment `json:"payments,omitempty"`
	Breakdown *VATBreakdown   `json:"breakdown,omitempty"`
}

// VATBreakdown is the box-by-box detail of a return.
type VATBreakdown struct {
	Title string            `json:"title,omitempty"`
	Rows  []VATBreakdownRow `json:"rows,omitempty"`
}

// VATBreakdownRow is one box on the return.
type VATBreakdownRow struct {
	Title string   `json:"title,omitempty"`
	Value *Decimal `json:"value,omitempty"`
	Key   string   `json:"key,omitempty"`
	// BoxNumber is documented as a string but the live API sends a bare
	// number, which would fail to decode into one. Int64 takes either.
	BoxNumber Int64 `json:"box_number,omitempty"`
}

// VATReturnService covers https://dev.freeagent.com/docs/vat_returns
//
// Returns are addressed by period end date. The account must be VAT
// registered; otherwise the collection is empty.
type VATReturnService struct {
	periodService[VATReturn]
}

// List returns every VAT period.
func (s *VATReturnService) List(ctx context.Context) ([]VATReturn, *Response, error) {
	return s.list(ctx, s.meta.Path)
}

// Get fetches one period by its end date.
func (s *VATReturnService) Get(ctx context.Context, periodEndsOn Date) (*VATReturn, *Response, error) {
	return s.get(ctx, s.meta.Path, periodEndsOn)
}

// MarkAsFiled records the return as filed outside FreeAgent. Needs Full
// Access.
func (s *VATReturnService) MarkAsFiled(ctx context.Context, periodEndsOn Date) (*VATReturn, *Response, error) {
	return s.transition(ctx, s.meta.Path, periodEndsOn, "mark_as_filed")
}

// MarkAsUnfiled reverses MarkAsFiled.
func (s *VATReturnService) MarkAsUnfiled(ctx context.Context, periodEndsOn Date) (*VATReturn, *Response, error) {
	return s.transition(ctx, s.meta.Path, periodEndsOn, "mark_as_unfiled")
}

// MarkPaymentAsPaid records the payment due on paymentDate as settled.
func (s *VATReturnService) MarkPaymentAsPaid(ctx context.Context, periodEndsOn, paymentDate Date) (*VATReturn, *Response, error) {
	return s.paymentTransition(ctx, s.meta.Path, periodEndsOn, paymentDate, "mark_as_paid")
}

// MarkPaymentAsUnpaid reverses MarkPaymentAsPaid.
func (s *VATReturnService) MarkPaymentAsUnpaid(ctx context.Context, periodEndsOn, paymentDate Date) (*VATReturn, *Response, error) {
	return s.paymentTransition(ctx, s.meta.Path, periodEndsOn, paymentDate, "mark_as_unpaid")
}

// CorporationTaxReturn is one corporation tax period.
//
// See https://dev.freeagent.com/docs/corporation_tax_returns
type CorporationTaxReturn struct {
	URL ResourceURL `json:"url,omitempty"`

	PeriodStartsOn Date `json:"period_starts_on,omitzero"`
	PeriodEndsOn   Date `json:"period_ends_on,omitzero"`
	FilingDueOn    Date `json:"filing_due_on,omitzero"`

	// FilingStatus is draft, unfiled, pending, rejected, filed or
	// marked_as_filed.
	FilingStatus   string `json:"filing_status,omitempty"`
	FiledAt        Time   `json:"filed_at,omitzero"`
	FiledReference string `json:"filed_reference,omitempty"`

	AmountDue *Decimal `json:"amount_due,omitempty"`
	// PaymentDueOn and PaymentStatus sit on the return itself here, rather
	// than in a payments array as VAT and income tax have them.
	PaymentDueOn  Date   `json:"payment_due_on,omitzero"`
	PaymentStatus string `json:"payment_status,omitempty"`
}

// CorporationTaxReturnService covers
// https://dev.freeagent.com/docs/corporation_tax_returns
type CorporationTaxReturnService struct {
	periodService[CorporationTaxReturn]
}

// List returns every corporation tax period.
func (s *CorporationTaxReturnService) List(ctx context.Context) ([]CorporationTaxReturn, *Response, error) {
	return s.list(ctx, s.meta.Path)
}

// Get fetches one period by its end date.
func (s *CorporationTaxReturnService) Get(ctx context.Context, periodEndsOn Date) (*CorporationTaxReturn, *Response, error) {
	return s.get(ctx, s.meta.Path, periodEndsOn)
}

// MarkAsFiled records the return as filed. Needs Full Access.
func (s *CorporationTaxReturnService) MarkAsFiled(ctx context.Context, periodEndsOn Date) (*CorporationTaxReturn, *Response, error) {
	return s.transition(ctx, s.meta.Path, periodEndsOn, "mark_as_filed")
}

// MarkAsUnfiled reverses MarkAsFiled.
func (s *CorporationTaxReturnService) MarkAsUnfiled(ctx context.Context, periodEndsOn Date) (*CorporationTaxReturn, *Response, error) {
	return s.transition(ctx, s.meta.Path, periodEndsOn, "mark_as_unfiled")
}

// MarkAsPaid records the tax as paid. Unlike VAT, the payment is a property
// of the return rather than a dated entry, so there is no payment date.
func (s *CorporationTaxReturnService) MarkAsPaid(ctx context.Context, periodEndsOn Date) (*CorporationTaxReturn, *Response, error) {
	return s.transition(ctx, s.meta.Path, periodEndsOn, "mark_as_paid")
}

// MarkAsUnpaid reverses MarkAsPaid.
func (s *CorporationTaxReturnService) MarkAsUnpaid(ctx context.Context, periodEndsOn Date) (*CorporationTaxReturn, *Response, error) {
	return s.transition(ctx, s.meta.Path, periodEndsOn, "mark_as_unpaid")
}

// IncomeTaxReturn is one self assessment period for a user.
//
// The documentation has two pages for this, Self Assessment Returns and
// Income Tax Returns; the former redirects to the latter and they are the
// same resource. Neither /v2/self_assessment_returns nor
// /v2/income_tax_returns exists: the collection is nested under a user.
//
// See https://dev.freeagent.com/docs/income_tax_returns
type IncomeTaxReturn struct {
	URL ResourceURL `json:"url,omitempty"`

	PeriodStartsOn Date `json:"period_starts_on,omitzero"`
	PeriodEndsOn   Date `json:"period_ends_on,omitzero"`
	FilingDueOn    Date `json:"filing_due_on,omitzero"`

	// FilingStatus is unfiled, pending, rejected, provisionally_filed, filed
	// or marked_as_filed. provisionally_filed is unique to this family.
	FilingStatus   string `json:"filing_status,omitempty"`
	FiledAt        Time   `json:"filed_at,omitzero"`
	FiledReference string `json:"filed_reference,omitempty"`

	Payments []FilingPayment `json:"payments,omitempty"`
}

// IncomeTaxReturnService covers
// https://dev.freeagent.com/docs/income_tax_returns
//
// Every call is scoped to a user, so each method takes the user's URL.
type IncomeTaxReturnService struct {
	periodService[IncomeTaxReturn]
}

// basePath builds the user-nested collection path.
func (s *IncomeTaxReturnService) basePath(user ResourceURL) (string, error) {
	if user.Kind() != "users" {
		return "", fmt.Errorf("freeagent: income tax returns are scoped to a user, got %q",
			truncate(user.String(), 64))
	}
	id, err := user.ID()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("users/%d/%s", id, s.meta.Path), nil
}

// ListForUser returns every self assessment period for a user.
func (s *IncomeTaxReturnService) ListForUser(ctx context.Context, user ResourceURL) ([]IncomeTaxReturn, *Response, error) {
	base, err := s.basePath(user)
	if err != nil {
		return nil, nil, err
	}
	return s.list(ctx, base)
}

// GetForUser fetches one period for a user.
func (s *IncomeTaxReturnService) GetForUser(ctx context.Context, user ResourceURL, periodEndsOn Date) (*IncomeTaxReturn, *Response, error) {
	base, err := s.basePath(user)
	if err != nil {
		return nil, nil, err
	}
	return s.get(ctx, base, periodEndsOn)
}

// MarkAsFiled records a user's return as filed. Needs Full Access.
func (s *IncomeTaxReturnService) MarkAsFiled(ctx context.Context, user ResourceURL, periodEndsOn Date) (*IncomeTaxReturn, *Response, error) {
	base, err := s.basePath(user)
	if err != nil {
		return nil, nil, err
	}
	return s.transition(ctx, base, periodEndsOn, "mark_as_filed")
}

// MarkAsUnfiled reverses MarkAsFiled.
func (s *IncomeTaxReturnService) MarkAsUnfiled(ctx context.Context, user ResourceURL, periodEndsOn Date) (*IncomeTaxReturn, *Response, error) {
	base, err := s.basePath(user)
	if err != nil {
		return nil, nil, err
	}
	return s.transition(ctx, base, periodEndsOn, "mark_as_unfiled")
}

// MarkPaymentAsPaid records one dated payment as settled.
func (s *IncomeTaxReturnService) MarkPaymentAsPaid(ctx context.Context, user ResourceURL, periodEndsOn, paymentDate Date) (*IncomeTaxReturn, *Response, error) {
	base, err := s.basePath(user)
	if err != nil {
		return nil, nil, err
	}
	return s.paymentTransition(ctx, base, periodEndsOn, paymentDate, "mark_as_paid")
}

// MarkPaymentAsUnpaid reverses MarkPaymentAsPaid.
func (s *IncomeTaxReturnService) MarkPaymentAsUnpaid(ctx context.Context, user ResourceURL, periodEndsOn, paymentDate Date) (*IncomeTaxReturn, *Response, error) {
	base, err := s.basePath(user)
	if err != nil {
		return nil, nil, err
	}
	return s.paymentTransition(ctx, base, periodEndsOn, paymentDate, "mark_as_unpaid")
}
