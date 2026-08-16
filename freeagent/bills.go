package freeagent

// Bill is a purchase invoice owed to a supplier.
//
// See https://dev.freeagent.com/docs/bills
type Bill struct {
	URL ResourceURL `json:"url,omitempty"`

	// Contact is required.
	Contact  ResourceURL `json:"contact,omitempty"`
	Project  ResourceURL `json:"project,omitempty"`
	Property ResourceURL `json:"property,omitempty"`

	Reference string `json:"reference,omitempty"`
	DatedOn   Date   `json:"dated_on,omitzero"`
	DueOn     Date   `json:"due_on,omitzero"`
	Currency  string `json:"currency,omitempty"`
	Comments  string `json:"comments,omitempty"`

	// InputTotalValuesIncTax defaults to false for the native currency and
	// true otherwise.
	InputTotalValuesIncTax *bool `json:"input_total_values_inc_tax,omitempty"`
	IsPaidByHirePurchase   *bool `json:"is_paid_by_hire_purchase,omitempty"`
	// ECStatus is UK/Non-EC, EC Goods, EC Services or Reverse Charge.
	ECStatus string `json:"ec_status,omitempty"`

	// Rebilling. RebillFactor is required when RebillType is markup or price.
	RebillType      string      `json:"rebill_type,omitempty"`
	RebillFactor    *Decimal    `json:"rebill_factor,omitempty"`
	RebillToProject ResourceURL `json:"rebill_to_project,omitempty"`

	// Recurring is Weekly, Two Weekly, Four Weekly, Two Monthly, Quarterly,
	// Biannually, Annually or 2-Yearly.
	Recurring        string `json:"recurring,omitempty"`
	RecurringEndDate Date   `json:"recurring_end_date,omitzero"`

	// CISDeductionBand is cis_gross, cis_standard or cis_higher.
	CISDeductionBand string `json:"cis_deduction_band,omitempty"`

	Attachment *Attachment `json:"attachment,omitempty"`
	BillItems  []BillItem  `json:"bill_items,omitempty"`

	// Read-only.
	Status               string   `json:"status,omitempty"`
	LongStatus           string   `json:"long_status,omitempty"`
	PaidOn               Date     `json:"paid_on,omitzero"`
	TotalValue           *Decimal `json:"total_value,omitempty"`
	NetValue             *Decimal `json:"net_value,omitempty"`
	DueValue             *Decimal `json:"due_value,omitempty"`
	NativeDueValue       *Decimal `json:"native_due_value,omitempty"`
	ExchangeRate         *Decimal `json:"exchange_rate,omitempty"`
	SalesTaxValue        *Decimal `json:"sales_tax_value,omitempty"`
	SecondSalesTaxValue  *Decimal `json:"second_sales_tax_value,omitempty"`
	CISDeductionRate     *Decimal `json:"cis_deduction_rate,omitempty"`
	CISDeduction         *Decimal `json:"cis_deduction,omitempty"`
	CISDeductionSuffered *Decimal `json:"cis_deduction_suffered,omitempty"`
	CreatedAt            Time     `json:"created_at,omitzero"`
	UpdatedAt            Time     `json:"updated_at,omitzero"`
}

// BillItem is one line on a bill. A bill accepts up to 40 items.
type BillItem struct {
	URL ResourceURL `json:"url,omitempty"`
	// Destroy set to 1 removes the line on a write.
	Destroy *int `json:"_destroy,omitempty"`

	// Category is required.
	Category    ResourceURL `json:"category,omitempty"`
	Description string      `json:"description,omitempty"`
	Project     ResourceURL `json:"project,omitempty"`

	// TotalValue includes tax; TotalValueExTax is the alternative form. Send
	// one or the other, not both.
	TotalValue      *Decimal `json:"total_value,omitempty"`
	TotalValueExTax *Decimal `json:"total_value_ex_tax,omitempty"`

	ManualSalesTaxAmount *Decimal `json:"manual_sales_tax_amount,omitempty"`
	SalesTaxRate         *Decimal `json:"sales_tax_rate,omitempty"`
	// SalesTaxStatus is TAXABLE, EXEMPT or OUT_OF_SCOPE.
	SalesTaxStatus       string   `json:"sales_tax_status,omitempty"`
	SecondSalesTaxRate   *Decimal `json:"second_sales_tax_rate,omitempty"`
	SecondSalesTaxStatus string   `json:"second_sales_tax_status,omitempty"`

	// Unit is -no unit-, Hours, Days, Weeks, Months, Years, Products,
	// Services, Training or Stock.
	Unit     string   `json:"unit,omitempty"`
	Quantity *Decimal `json:"quantity,omitempty"`

	StockItem             ResourceURL `json:"stock_item,omitempty"`
	StockAlteringQuantity *Decimal    `json:"stock_altering_quantity,omitempty"`

	CISDeductionRate *Decimal `json:"cis_deduction_rate,omitempty"`

	// Read-only.
	Bill                 ResourceURL `json:"bill,omitempty"`
	CapitalAsset         ResourceURL `json:"capital_asset,omitempty"`
	StockItemDescription string      `json:"stock_item_description,omitempty"`
}

// Views accepted by the bills list endpoint.
const (
	BillViewAll                   = "all"
	BillViewOpen                  = "open"
	BillViewOverdue               = "overdue"
	BillViewOpenOrOverdue         = "open_or_overdue"
	BillViewOpenOrOverduePayments = "open_or_overdue_payments"
	BillViewOpenOrOverdueRefunds  = "open_or_overdue_refunds"
	BillViewPaid                  = "paid"
	BillViewRecurring             = "recurring"
	BillViewHirePurchase          = "hire_purchase"
	BillViewCIS                   = "cis"
)

// BillService covers https://dev.freeagent.com/docs/bills
type BillService struct {
	Collection[Bill]
}
