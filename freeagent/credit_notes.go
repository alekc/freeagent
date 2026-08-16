package freeagent

import (
	"context"
	"net/http"
)

// CreditNote is a credit issued against a contact, the mirror of an invoice.
//
// See https://dev.freeagent.com/docs/credit_notes
type CreditNote struct {
	URL ResourceURL `json:"url,omitempty"`

	// Contact is required.
	Contact     ResourceURL `json:"contact,omitempty"`
	Project     ResourceURL `json:"project,omitempty"`
	Property    ResourceURL `json:"property,omitempty"`
	BankAccount ResourceURL `json:"bank_account,omitempty"`

	Reference string `json:"reference,omitempty"`
	// DatedOn is required.
	DatedOn Date `json:"dated_on,omitzero"`
	DueOn   Date `json:"due_on,omitzero"`
	// PaymentTermsInDays is required; zero means due on receipt.
	PaymentTermsInDays *int   `json:"payment_terms_in_days,omitempty"`
	Currency           string `json:"currency,omitempty"`

	// Status is Draft, Open, Overdue, Refunded or Written-off, and is driven
	// by the transitions rather than set directly.
	Status string `json:"status,omitempty"`

	Comments          string   `json:"comments,omitempty"`
	DiscountPercent   *Decimal `json:"discount_percent,omitempty"`
	ClientContactName string   `json:"client_contact_name,omitempty"`
	PaymentTerms      string   `json:"payment_terms,omitempty"`
	POReference       string   `json:"po_reference,omitempty"`
	OmitHeader        *bool    `json:"omit_header,omitempty"`
	ShowProjectName   *bool    `json:"show_project_name,omitempty"`

	// ECStatus is UK/Non-EC, EC Goods, EC Services, Reverse Charge or
	// EC VAT MOSS.
	ECStatus      string `json:"ec_status,omitempty"`
	PlaceOfSupply string `json:"place_of_supply,omitempty"`

	// Construction Industry Scheme.
	CISRate              string   `json:"cis_rate,omitempty"`
	CISDeductionRate     *Decimal `json:"cis_deduction_rate,omitempty"`
	CISDeduction         *Decimal `json:"cis_deduction,omitempty"`
	CISDeductionSuffered *Decimal `json:"cis_deduction_suffered,omitempty"`

	CreditNoteItems []CreditNoteItem `json:"credit_note_items,omitempty"`

	// Read-only.
	LongStatus          string   `json:"long_status,omitempty"`
	NetValue            *Decimal `json:"net_value,omitempty"`
	SalesTaxValue       *Decimal `json:"sales_tax_value,omitempty"`
	SecondSalesTaxValue *Decimal `json:"second_sales_tax_value,omitempty"`
	TotalValue          *Decimal `json:"total_value,omitempty"`
	RefundedValue       *Decimal `json:"refunded_value,omitempty"`
	DueValue            *Decimal `json:"due_value,omitempty"`
	ExchangeRate        *Decimal `json:"exchange_rate,omitempty"`
	InvolvesSalesTax    *bool    `json:"involves_sales_tax,omitempty"`
	IsInterimUKVAT      *bool    `json:"is_interim_uk_vat,omitempty"`
	RefundedOn          Date     `json:"refunded_on,omitzero"`
	WrittenOffDate      Date     `json:"written_off_date,omitzero"`
	CreatedAt           Time     `json:"created_at,omitzero"`
	UpdatedAt           Time     `json:"updated_at,omitzero"`
}

// CreditNoteItem is one line on a credit note.
type CreditNoteItem struct {
	URL ResourceURL `json:"url,omitempty"`
	// ID identifies an existing line on a write; leave unset to add one.
	ID *int64 `json:"id,omitempty"`
	// Destroy set to 1 removes the line on a write.
	Destroy *int `json:"_destroy,omitempty"`

	Position    *Decimal `json:"position,omitempty"`
	ItemType    string   `json:"item_type,omitempty"`
	Description string   `json:"description,omitempty"`
	Quantity    *Decimal `json:"quantity,omitempty"`
	Price       *Decimal `json:"price,omitempty"`

	SalesTaxRate         *Decimal `json:"sales_tax_rate,omitempty"`
	SalesTaxStatus       string   `json:"sales_tax_status,omitempty"`
	SecondSalesTaxRate   *Decimal `json:"second_sales_tax_rate,omitempty"`
	SecondSalesTaxStatus string   `json:"second_sales_tax_status,omitempty"`

	// Category is required.
	Category  ResourceURL `json:"category,omitempty"`
	Project   ResourceURL `json:"project,omitempty"`
	StockItem ResourceURL `json:"stock_item,omitempty"`
}

// Views accepted by the credit notes list endpoint.
const (
	CreditNoteViewAll = "all"
	//nolint:gosec // G101 false positive: this is a query filter, not a credential
	CreditNoteViewRecentOpenOverdue = "recent_open_or_overdue"
	CreditNoteViewOpen              = "open"
	CreditNoteViewOverdue           = "overdue"
	CreditNoteViewOpenOrOverdue     = "open_or_overdue"
	CreditNoteViewDraft             = "draft"
	CreditNoteViewRefunded          = "refunded"
)

// CreditNoteService covers https://dev.freeagent.com/docs/credit_notes
type CreditNoteService struct {
	Collection[CreditNote]
}

// MarkAsSent moves a draft credit note to sent, or reopens a cancelled one.
func (s *CreditNoteService) MarkAsSent(ctx context.Context, id int64) (*CreditNote, *Response, error) {
	return s.action(ctx, http.MethodPut, id, "transitions/mark_as_sent", nil)
}

// MarkAsDraft returns the credit note to draft.
func (s *CreditNoteService) MarkAsDraft(ctx context.Context, id int64) (*CreditNote, *Response, error) {
	return s.action(ctx, http.MethodPut, id, "transitions/mark_as_draft", nil)
}

// PDF fetches the rendered credit note.
func (s *CreditNoteService) PDF(ctx context.Context, id int64) (*PDF, *Response, error) {
	return s.pdf(ctx, id)
}

// SendEmail emails the credit note. Pass nil to use the account's template.
func (s *CreditNoteService) SendEmail(ctx context.Context, id int64, opts *EmailOptions) (*Response, error) {
	return s.sendEmail(ctx, id, opts)
}

// CreditNoteReconciliation records how much of a credit note was applied to
// an invoice.
//
// See https://dev.freeagent.com/docs/credit_note_reconciliations
type CreditNoteReconciliation struct {
	URL ResourceURL `json:"url,omitempty"`

	// All four are required.
	CreditNote ResourceURL `json:"credit_note,omitempty"`
	Invoice    ResourceURL `json:"invoice,omitempty"`
	GrossValue *Decimal    `json:"gross_value,omitempty"`
	DatedOn    Date        `json:"dated_on,omitzero"`

	Currency     string   `json:"currency,omitempty"`
	ExchangeRate *Decimal `json:"exchange_rate,omitempty"`

	// Read-only.
	CreatedAt Time `json:"created_at,omitzero"`
	UpdatedAt Time `json:"updated_at,omitzero"`
}

// CreditNoteReconciliationService covers
// https://dev.freeagent.com/docs/credit_note_reconciliations
type CreditNoteReconciliationService struct {
	Collection[CreditNoteReconciliation]
}
