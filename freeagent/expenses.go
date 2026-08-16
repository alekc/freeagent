package freeagent

import (
	"context"
	"encoding/json"
	"net/http"
)

// Expense is an out-of-pocket cost incurred by a user.
//
// GrossValue is negative for a payment and positive for a refund. It is
// required unless the category is Mileage, in which case Mileage and
// VehicleType are.
//
// See https://dev.freeagent.com/docs/expenses
type Expense struct {
	URL ResourceURL `json:"url,omitempty"`

	// User and Category are required.
	User     ResourceURL `json:"user,omitempty"`
	Category ResourceURL `json:"category,omitempty"`
	Project  ResourceURL `json:"project,omitempty"`
	Property ResourceURL `json:"property,omitempty"`

	DatedOn          Date   `json:"dated_on,omitzero"`
	Currency         string `json:"currency,omitempty"`
	Description      string `json:"description,omitempty"`
	ReceiptReference string `json:"receipt_reference,omitempty"`

	GrossValue           *Decimal `json:"gross_value,omitempty"`
	NativeGrossValue     *Decimal `json:"native_gross_value,omitempty"`
	SalesTaxRate         *Decimal `json:"sales_tax_rate,omitempty"`
	SalesTaxValue        *Decimal `json:"sales_tax_value,omitempty"`
	NativeSalesTaxValue  *Decimal `json:"native_sales_tax_value,omitempty"`
	ManualSalesTaxAmount *Decimal `json:"manual_sales_tax_amount,omitempty"`
	// SalesTaxStatus is TAXABLE, EXEMPT or OUT_OF_SCOPE.
	SalesTaxStatus       string   `json:"sales_tax_status,omitempty"`
	SecondSalesTaxRate   *Decimal `json:"second_sales_tax_rate,omitempty"`
	SecondSalesTaxStatus string   `json:"second_sales_tax_status,omitempty"`
	// ECStatus is UK/Non-EC, EC Goods, EC Services or Reverse Charge.
	ECStatus string `json:"ec_status,omitempty"`

	// Rebilling. RebillFactor is required when RebillType is markup or price.
	RebillType      string      `json:"rebill_type,omitempty"`
	RebillFactor    *Decimal    `json:"rebill_factor,omitempty"`
	RebillToProject ResourceURL `json:"rebill_to_project,omitempty"`

	// Stock. Both are required for the purchase of stock category.
	StockItem             ResourceURL `json:"stock_item,omitempty"`
	StockAlteringQuantity *Decimal    `json:"stock_altering_quantity,omitempty"`

	// Mileage claims. VehicleType is Car, Motorcycle or Bicycle.
	Mileage            *Decimal `json:"mileage,omitempty"`
	VehicleType        string   `json:"vehicle_type,omitempty"`
	EngineType         string   `json:"engine_type,omitempty"`
	EngineSize         string   `json:"engine_size,omitempty"`
	ReclaimMileage     *int     `json:"reclaim_mileage,omitempty"`
	InitialRateMileage *Decimal `json:"initial_rate_mileage,omitempty"`
	ReclaimMileageRate *Decimal `json:"reclaim_mileage_rate,omitempty"`
	RebillMileageRate  *Decimal `json:"rebill_mileage_rate,omitempty"`
	HaveVATReceipt     *bool    `json:"have_vat_receipt,omitempty"`

	// Recurring is Weekly, Two Weekly, Four Weekly, Two Monthly, Quarterly,
	// Biannually, Annually or 2-Yearly.
	Recurring        string `json:"recurring,omitempty"`
	NextRecursOn     Date   `json:"next_recurs_on,omitzero"`
	RecurringEndDate Date   `json:"recurring_end_date,omitzero"`

	Attachment *Attachment `json:"attachment,omitempty"`

	// Read-only.
	CapitalAsset         ResourceURL `json:"capital_asset,omitempty"`
	RebilledOnInvoice    ResourceURL `json:"rebilled_on_invoice,omitempty"`
	StockItemDescription string      `json:"stock_item_description,omitempty"`
	CreatedAt            Time        `json:"created_at,omitzero"`
	UpdatedAt            Time        `json:"updated_at,omitzero"`
}

// Views accepted by the expenses list endpoint.
const (
	ExpenseViewRecent    = "recent"
	ExpenseViewRecurring = "recurring"
)

// MileageSettings reports the account's mileage configuration.
//
// Both members are historical: each entry covers a date range, because HMRC
// rates and the engine-size bands have changed over time. Pick the entry whose
// range contains the expense date rather than assuming the last one applies.
type MileageSettings struct {
	// EngineTypeAndSizeOptions maps an engine type such as "Petrol" to the
	// engine sizes valid in that period.
	EngineTypeAndSizeOptions []MileagePeriod[map[string][]string] `json:"engine_type_and_size_options,omitempty"`
	// MileageRates maps a vehicle type such as "Car" to its rates. The same
	// object also carries a basic_rate_limit, which is why the value is
	// decoded loosely rather than as a fixed struct.
	MileageRates []MileagePeriod[map[string]json.RawMessage] `json:"mileage_rates,omitempty"`
}

// MileagePeriod is one dated slice of a mileage setting.
type MileagePeriod[T any] struct {
	From  Date `json:"from,omitzero"`
	To    Date `json:"to,omitzero"`
	Value T    `json:"value,omitempty"`
}

// MileageRate is the rate pair for one vehicle type inside a MileageRates
// period. Decode a member of the period's Value into it; the sibling
// basic_rate_limit key is a number, not a rate, so it will not fit.
type MileageRate struct {
	BasicRate      *Decimal `json:"basic_rate,omitempty"`
	AdditionalRate *Decimal `json:"additional_rate,omitempty"`
}

// RatesFor decodes the per-vehicle rates in a period, skipping the
// basic_rate_limit sibling that shares the object.
func (p MileagePeriod[T]) RatesFor(vehicleType string) (MileageRate, bool) {
	raw, ok := any(p.Value).(map[string]json.RawMessage)
	if !ok {
		return MileageRate{}, false
	}
	entry, ok := raw[vehicleType]
	if !ok {
		return MileageRate{}, false
	}
	var rate MileageRate
	if err := json.Unmarshal(entry, &rate); err != nil {
		return MileageRate{}, false
	}
	return rate, rate.BasicRate != nil
}

// ExpenseService covers https://dev.freeagent.com/docs/expenses
type ExpenseService struct {
	Collection[Expense]
}

// MileageSettings returns the account's mileage configuration. The reply is
// enveloped under mileage_settings, unlike the other expense sub-resources.
func (s *ExpenseService) MileageSettings(ctx context.Context) (*MileageSettings, *Response, error) {
	req, err := s.client.newRequest(ctx, http.MethodGet, s.meta.Path+"/mileage_settings", nil, nil)
	if err != nil {
		return nil, nil, err
	}
	var envelope struct {
		MileageSettings MileageSettings `json:"mileage_settings"`
	}
	resp, err := s.client.do(req, &envelope)
	if err != nil {
		return nil, resp, err
	}
	return &envelope.MileageSettings, resp, nil
}
