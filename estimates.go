package freeagent

import (
	"context"
	"net/http"
)

// Estimate is a quote, proposal or estimate sent to a contact.
//
// See https://dev.freeagent.com/docs/estimates
type Estimate struct {
	URL ResourceURL `json:"url,omitempty"`

	Contact ResourceURL `json:"contact,omitempty"`
	Project ResourceURL `json:"project,omitempty"`

	// EstimateType is Estimate, Quote or Proposal.
	EstimateType string `json:"estimate_type,omitempty"`
	// Status is Draft, Sent, Open, Approved, Rejected or Invoiced.
	//
	// Unlike an invoice, an estimate will not accept a create without one:
	// omitting it returns 422 "status is not valid". Send "Draft" and use the
	// transitions from there.
	Status string `json:"status,omitempty"`

	Reference string `json:"reference,omitempty"`
	DatedOn   Date   `json:"dated_on,omitzero"`
	Currency  string `json:"currency,omitempty"`
	Notes     string `json:"notes,omitempty"`

	DiscountPercent   *Decimal `json:"discount_percent,omitempty"`
	ClientContactName string   `json:"client_contact_name,omitempty"`
	// ECStatus is UK/Non-EC, EC Goods, EC Services, Reverse Charge or
	// EC VAT MOSS.
	ECStatus      string `json:"ec_status,omitempty"`
	PlaceOfSupply string `json:"place_of_supply,omitempty"`

	IncludeSalesTaxOnTotalValue *bool `json:"include_sales_tax_on_total_value,omitempty"`

	EstimateItems []EstimateItem `json:"estimate_items,omitempty"`

	// Read-only.
	NetValue      *Decimal `json:"net_value,omitempty"`
	SalesTaxValue *Decimal `json:"sales_tax_value,omitempty"`
	CreatedAt     Time     `json:"created_at,omitzero"`
	UpdatedAt     Time     `json:"updated_at,omitzero"`
}

// EstimateItem is one line on an estimate.
type EstimateItem struct {
	URL ResourceURL `json:"url,omitempty"`

	// Position starts at 1.
	Position *int `json:"position,omitempty"`
	// ItemType is Hours, Days, Weeks, Months, Years, -no unit-, Products,
	// Services, Training, Expenses, Comments, Bills, Discount or Credit.
	ItemType    string   `json:"item_type,omitempty"`
	Description string   `json:"description,omitempty"`
	Quantity    *Decimal `json:"quantity,omitempty"`
	Price       *Decimal `json:"price,omitempty"`

	SalesTaxRate         *Decimal `json:"sales_tax_rate,omitempty"`
	SalesTaxStatus       string   `json:"sales_tax_status,omitempty"`
	SalesTaxValue        *Decimal `json:"sales_tax_value,omitempty"`
	SecondSalesTaxRate   *Decimal `json:"second_sales_tax_rate,omitempty"`
	SecondSalesTaxStatus string   `json:"second_sales_tax_status,omitempty"`
	SecondSalesTaxValue  *Decimal `json:"second_sales_tax_value,omitempty"`

	Category ResourceURL `json:"category,omitempty"`

	CreatedAt Time `json:"created_at,omitzero"`
	UpdatedAt Time `json:"updated_at,omitzero"`
}

// Views accepted by the estimates list endpoint.
const (
	EstimateViewAll      = "all"
	EstimateViewRecent   = "recent"
	EstimateViewDraft    = "draft"
	EstimateViewNonDraft = "non_draft"
	EstimateViewSent     = "sent"
	EstimateViewApproved = "approved"
	EstimateViewRejected = "rejected"
	EstimateViewInvoiced = "invoiced"
)

// EstimateService covers https://dev.freeagent.com/docs/estimates
type EstimateService struct {
	Collection[Estimate]
}

// MarkAsSent marks the estimate sent.
func (s *EstimateService) MarkAsSent(ctx context.Context, id int64) (*Estimate, *Response, error) {
	return s.action(ctx, http.MethodPut, id, "transitions/mark_as_sent", nil)
}

// MarkAsDraft returns the estimate to draft.
func (s *EstimateService) MarkAsDraft(ctx context.Context, id int64) (*Estimate, *Response, error) {
	return s.action(ctx, http.MethodPut, id, "transitions/mark_as_draft", nil)
}

// MarkAsApproved marks the estimate approved.
func (s *EstimateService) MarkAsApproved(ctx context.Context, id int64) (*Estimate, *Response, error) {
	return s.action(ctx, http.MethodPut, id, "transitions/mark_as_approved", nil)
}

// MarkAsRejected marks the estimate rejected.
func (s *EstimateService) MarkAsRejected(ctx context.Context, id int64) (*Estimate, *Response, error) {
	return s.action(ctx, http.MethodPut, id, "transitions/mark_as_rejected", nil)
}

// ConvertToInvoice turns the estimate into an invoice. The reply is the
// estimate as the API returns it, not the new invoice.
func (s *EstimateService) ConvertToInvoice(ctx context.Context, id int64) (*Estimate, *Response, error) {
	return s.action(ctx, http.MethodPut, id, "transitions/convert_to_invoice", nil)
}

// Duplicate copies an existing estimate into a new draft.
func (s *EstimateService) Duplicate(ctx context.Context, id int64) (*Estimate, *Response, error) {
	return s.action(ctx, http.MethodPost, id, "duplicate", nil)
}

// PDF fetches the rendered estimate.
func (s *EstimateService) PDF(ctx context.Context, id int64) (*PDF, *Response, error) {
	return s.pdf(ctx, id)
}

// SendEmail emails the estimate. Pass nil to use the account's template.
func (s *EstimateService) SendEmail(ctx context.Context, id int64, opts *EmailOptions) (*Response, error) {
	return s.sendEmail(ctx, id, opts)
}
