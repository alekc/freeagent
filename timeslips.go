package freeagent

import (
	"context"
	"net/http"
)

// Timeslip is time recorded against a task.
//
// See https://dev.freeagent.com/docs/timeslips
type Timeslip struct {
	URL ResourceURL `json:"url,omitempty"`

	// User, Project, Task and DatedOn are all required.
	User    ResourceURL `json:"user,omitempty"`
	Project ResourceURL `json:"project,omitempty"`
	Task    ResourceURL `json:"task,omitempty"`
	DatedOn Date        `json:"dated_on,omitzero"`
	// Hours is decimal, so 1:30 is 1.5.
	Hours   *Decimal `json:"hours,omitempty"`
	Comment string   `json:"comment,omitempty"`

	// Read-only.
	BilledOnInvoice ResourceURL    `json:"billed_on_invoice,omitempty"`
	Timer           *TimeslipTimer `json:"timer,omitempty"`
	CreatedAt       Time           `json:"created_at,omitzero"`
	UpdatedAt       Time           `json:"updated_at,omitzero"`
}

// TimeslipTimer is present only while a timer is running on the timeslip.
type TimeslipTimer struct {
	Running *bool `json:"running,omitempty"`
	// StartFrom is the effective start, which already accounts for any time
	// logged before the timer was started.
	StartFrom Time `json:"start_from,omitzero"`
}

// Views accepted by the timeslips list endpoint.
const (
	TimeslipViewAll      = "all"
	TimeslipViewUnbilled = "unbilled"
	TimeslipViewRunning  = "running"
)

// TimeslipService covers https://dev.freeagent.com/docs/timeslips
type TimeslipService struct {
	Collection[Timeslip]
}

// StartTimer starts the timer on a timeslip.
func (s *TimeslipService) StartTimer(ctx context.Context, id int64) (*Timeslip, *Response, error) {
	return s.action(ctx, http.MethodPost, id, "timer", nil)
}

// StopTimer stops the timer, folding the elapsed time into the timeslip.
func (s *TimeslipService) StopTimer(ctx context.Context, id int64) (*Timeslip, *Response, error) {
	return s.action(ctx, http.MethodDelete, id, "timer", nil)
}

// RecurringInvoice is a template that generates invoices on a schedule.
//
// The API exposes reads only: creating and editing one is done in the
// FreeAgent interface, which is why this family embeds ReadCollection.
//
// See https://dev.freeagent.com/docs/recurring_invoices
type RecurringInvoice struct {
	URL ResourceURL `json:"url,omitempty"`

	Contact     ResourceURL `json:"contact,omitempty"`
	ContactName string      `json:"contact_name,omitempty"`
	Reference   string      `json:"reference,omitempty"`
	DatedOn     Date        `json:"dated_on,omitzero"`

	// Frequency is Weekly, Two Weekly, Four Weekly, Monthly, Two Monthly,
	// Quarterly, Biannually, Annually or 2-Yearly.
	Frequency string `json:"frequency,omitempty"`
	// NextRecursOn is when the next invoice will be raised.
	NextRecursOn Date `json:"next_recurs_on,omitzero"`
	// RecurringEndDate is blank when the schedule runs forever.
	RecurringEndDate Date `json:"recurring_end_date,omitzero"`
	// RecurringStatus is Draft or Active.
	RecurringStatus string `json:"recurring_status,omitempty"`

	Currency           string   `json:"currency,omitempty"`
	ExchangeRate       *Decimal `json:"exchange_rate,omitempty"`
	NetValue           *Decimal `json:"net_value,omitempty"`
	SalesTaxValue      *Decimal `json:"sales_tax_value,omitempty"`
	TotalValue         *Decimal `json:"total_value,omitempty"`
	PaymentTermsInDays *int     `json:"payment_terms_in_days,omitempty"`

	OmitHeader           *bool `json:"omit_header,omitempty"`
	AlwaysShowBICAndIBAN *bool `json:"always_show_bic_and_iban,omitempty"`

	InvoiceItems   []InvoiceItem          `json:"invoice_items,omitempty"`
	PaymentMethods *InvoicePaymentMethods `json:"payment_methods,omitempty"`
}

// Views accepted by the recurring invoices list endpoint.
const (
	RecurringInvoiceViewDraft    = "draft"
	RecurringInvoiceViewActive   = "active"
	RecurringInvoiceViewInactive = "inactive"
)

// RecurringInvoiceService covers
// https://dev.freeagent.com/docs/recurring_invoices
type RecurringInvoiceService struct {
	ReadCollection[RecurringInvoice]
}
