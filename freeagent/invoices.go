package freeagent

import (
	"context"
	"net/http"
)

// Invoice is a sales invoice.
//
// See https://dev.freeagent.com/docs/invoices
type Invoice struct {
	URL ResourceURL `json:"url,omitempty"`

	// Contact is required.
	Contact  ResourceURL `json:"contact,omitempty"`
	Project  ResourceURL `json:"project,omitempty"`
	Property ResourceURL `json:"property,omitempty"`
	// BankAccount nominates the account shown for remittance.
	BankAccount ResourceURL `json:"bank_account,omitempty"`

	// Reference is required; omit it to use the account's invoice sequence.
	Reference string `json:"reference,omitempty"`
	// DatedOn is required.
	DatedOn Date `json:"dated_on,omitzero"`
	DueOn   Date `json:"due_on,omitzero"`
	// PaymentTermsInDays is required; zero means due on receipt.
	PaymentTermsInDays *int   `json:"payment_terms_in_days,omitempty"`
	Currency           string `json:"currency,omitempty"`

	// Status is one of Draft, Scheduled To Email, Open, Zero Value, Overdue,
	// Paid, Overpaid, Refunded, Written-off or Part written-off. It is driven
	// by the transitions rather than set directly.
	Status string `json:"status,omitempty"`

	Comments             string   `json:"comments,omitempty"`
	DiscountPercent      *Decimal `json:"discount_percent,omitempty"`
	ClientContactName    string   `json:"client_contact_name,omitempty"`
	PaymentTerms         string   `json:"payment_terms,omitempty"`
	POReference          string   `json:"po_reference,omitempty"`
	OmitHeader           *bool    `json:"omit_header,omitempty"`
	ShowProjectName      *bool    `json:"show_project_name,omitempty"`
	AlwaysShowBICAndIBAN *bool    `json:"always_show_bic_and_iban,omitempty"`

	SendNewInvoiceEmails *bool `json:"send_new_invoice_emails,omitempty"`
	SendReminderEmails   *bool `json:"send_reminder_emails,omitempty"`
	SendThankYouEmails   *bool `json:"send_thank_you_emails,omitempty"`

	// Roll-up options. Each is either null or one of the documented
	// billed_grouped_by_* values.
	IncludeTimeslips string `json:"include_timeslips,omitempty"`
	IncludeExpenses  string `json:"include_expenses,omitempty"`
	IncludeEstimates string `json:"include_estimates,omitempty"`

	// ECStatus is UK/Non-EC, EC Goods, EC Services, Reverse Charge or
	// EC VAT MOSS.
	ECStatus      string `json:"ec_status,omitempty"`
	PlaceOfSupply string `json:"place_of_supply,omitempty"`

	// Construction Industry Scheme.
	CISRate              string   `json:"cis_rate,omitempty"`
	CISDeductionRate     *Decimal `json:"cis_deduction_rate,omitempty"`
	CISDeduction         *Decimal `json:"cis_deduction,omitempty"`
	CISDeductionSuffered *Decimal `json:"cis_deduction_suffered,omitempty"`

	InvoiceItems []InvoiceItem `json:"invoice_items,omitempty"`

	// Read-only.
	LongStatus          string                 `json:"long_status,omitempty"`
	ContactName         string                 `json:"contact_name,omitempty"`
	NetValue            *Decimal               `json:"net_value,omitempty"`
	SalesTaxValue       *Decimal               `json:"sales_tax_value,omitempty"`
	SecondSalesTaxValue *Decimal               `json:"second_sales_tax_value,omitempty"`
	TotalValue          *Decimal               `json:"total_value,omitempty"`
	PaidValue           *Decimal               `json:"paid_value,omitempty"`
	DueValue            *Decimal               `json:"due_value,omitempty"`
	ExchangeRate        *Decimal               `json:"exchange_rate,omitempty"`
	InvolvesSalesTax    *bool                  `json:"involves_sales_tax,omitempty"`
	IsInterimUKVAT      *bool                  `json:"is_interim_uk_vat,omitempty"`
	PaidOn              Date                   `json:"paid_on,omitzero"`
	WrittenOffDate      Date                   `json:"written_off_date,omitzero"`
	RecurringInvoice    ResourceURL            `json:"recurring_invoice,omitempty"`
	PaymentURL          string                 `json:"payment_url,omitempty"`
	PaymentMethods      *InvoicePaymentMethods `json:"payment_methods,omitempty"`
	CreatedAt           Time                   `json:"created_at,omitzero"`
	UpdatedAt           Time                   `json:"updated_at,omitzero"`
}

// InvoiceItem is one line on an invoice.
type InvoiceItem struct {
	URL ResourceURL `json:"url,omitempty"`
	// ID identifies an existing line on a write. Leave it unset to add one.
	ID *int64 `json:"id,omitempty"`
	// Destroy set to 1 removes the line on a write.
	Destroy *int `json:"_destroy,omitempty"`

	// Position is read-only and starts at 1.
	Position *Decimal `json:"position,omitempty"`
	// ItemType is Hours, Days, Weeks, Months, Years, Products, Services,
	// Training, Expenses, Comment, Bills, Discount, Credit, VAT or Stock.
	// Blank means "no unit".
	ItemType    string   `json:"item_type,omitempty"`
	Description string   `json:"description,omitempty"`
	Quantity    *Decimal `json:"quantity,omitempty"`
	Price       *Decimal `json:"price,omitempty"`

	SalesTaxRate         *Decimal `json:"sales_tax_rate,omitempty"`
	SalesTaxStatus       string   `json:"sales_tax_status,omitempty"`
	SecondSalesTaxRate   *Decimal `json:"second_sales_tax_rate,omitempty"`
	SecondSalesTaxStatus string   `json:"second_sales_tax_status,omitempty"`

	Category  ResourceURL `json:"category,omitempty"`
	Project   ResourceURL `json:"project,omitempty"`
	StockItem ResourceURL `json:"stock_item,omitempty"`
}

// InvoicePaymentMethods reports which online payment routes are enabled. All
// fields are read-only and depend on the integrations configured.
type InvoicePaymentMethods struct {
	PayPal                   bool `json:"paypal,omitempty"`
	GoCardlessPreauth        bool `json:"gocardless_preauth,omitempty"`
	GoCardlessInstantBankPay bool `json:"gocardless_instant_bank_pay,omitempty"`
	Stripe                   bool `json:"stripe,omitempty"`
	Tyl                      bool `json:"tyl,omitempty"`
}

// Views accepted by the invoices list endpoint. LastNMonths is a template:
// substitute the month count, for example "last_3_months".
const (
	InvoiceViewAll               = "all"
	InvoiceViewRecentOpenOverdue = "recent_open_or_overdue"
	InvoiceViewOpen              = "open"
	InvoiceViewOverdue           = "overdue"
	InvoiceViewOpenOrOverdue     = "open_or_overdue"
	InvoiceViewDraft             = "draft"
	InvoiceViewPaid              = "paid"
	InvoiceViewScheduledToEmail  = "scheduled_to_email"
	InvoiceViewThankYouEmails    = "thank_you_emails"
	InvoiceViewReminderEmails    = "reminder_emails"
)

// InvoiceService covers https://dev.freeagent.com/docs/invoices
type InvoiceService struct {
	Collection[Invoice]
}

// MarkAsSent moves a draft invoice to sent, or reopens a cancelled one.
func (s *InvoiceService) MarkAsSent(ctx context.Context, id int64) (*Invoice, *Response, error) {
	return s.action(ctx, http.MethodPut, id, "transitions/mark_as_sent", nil)
}

// MarkAsScheduled schedules the invoice to be emailed.
func (s *InvoiceService) MarkAsScheduled(ctx context.Context, id int64) (*Invoice, *Response, error) {
	return s.action(ctx, http.MethodPut, id, "transitions/mark_as_scheduled", nil)
}

// MarkAsDraft returns the invoice to draft.
func (s *InvoiceService) MarkAsDraft(ctx context.Context, id int64) (*Invoice, *Response, error) {
	return s.action(ctx, http.MethodPut, id, "transitions/mark_as_draft", nil)
}

// MarkAsCancelled cancels the invoice.
func (s *InvoiceService) MarkAsCancelled(ctx context.Context, id int64) (*Invoice, *Response, error) {
	return s.action(ctx, http.MethodPut, id, "transitions/mark_as_cancelled", nil)
}

// ConvertToCreditNote turns a draft invoice with a negative total into a
// credit note.
func (s *InvoiceService) ConvertToCreditNote(ctx context.Context, id int64) (*Invoice, *Response, error) {
	return s.action(ctx, http.MethodPut, id, "transitions/convert_to_credit_note", nil)
}

// Duplicate copies an existing invoice into a new draft.
func (s *InvoiceService) Duplicate(ctx context.Context, id int64) (*Invoice, *Response, error) {
	return s.action(ctx, http.MethodPost, id, "duplicate", nil)
}

// PDF fetches the rendered invoice.
func (s *InvoiceService) PDF(ctx context.Context, id int64) (*PDF, *Response, error) {
	return s.pdf(ctx, id)
}

// SendEmail emails the invoice. Pass nil to use the account's template.
func (s *InvoiceService) SendEmail(ctx context.Context, id int64, opts *EmailOptions) (*Response, error) {
	return s.sendEmail(ctx, id, opts)
}
