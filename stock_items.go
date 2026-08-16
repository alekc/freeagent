package freeagent

// StockItem is something the company buys and sells by quantity.
//
// The quantity fields are documented as integers but arrive as quoted
// decimals ("10.0"), so they are Decimal here.
//
// See https://dev.freeagent.com/docs/stock_items
type StockItem struct {
	URL ResourceURL `json:"url,omitempty"`

	// Description doubles as the item code shown on invoices and estimates.
	Description string `json:"description,omitempty"`
	// CostOfSaleCategory is the spending category sales are accounted to.
	CostOfSaleCategory ResourceURL `json:"cost_of_sale_category,omitempty"`

	// Opening figures are as at the FreeAgent start date.
	OpeningQuantity *Decimal `json:"opening_quantity,omitempty"`
	OpeningBalance  *Decimal `json:"opening_balance,omitempty"`

	// StockOnHand is read-only and moves as stock is bought and sold.
	StockOnHand *Decimal `json:"stock_on_hand,omitempty"`

	CreatedAt Time `json:"created_at,omitzero"`
	UpdatedAt Time `json:"updated_at,omitzero"`
}

// StockItemService covers https://dev.freeagent.com/docs/stock_items
//
// Read-only: the documentation lists only GET endpoints, and POST answers 404
// Not Found rather than 405, so stock items are created in the FreeAgent
// interface or as a side effect of a stock purchase.
type StockItemService struct {
	ReadCollection[StockItem]
}

// PriceListItem is a saved line that can be dropped onto an invoice or
// estimate.
//
// See https://dev.freeagent.com/docs/price_list_items
type PriceListItem struct {
	URL ResourceURL `json:"url,omitempty"`

	// Code, Quantity, ItemType and Description are all required.
	Code string `json:"code,omitempty"`
	// ItemType is Hours, Days, Weeks, Months, Years, Products, Services,
	// Training, Expenses, Comment, Bills, Discount, Credit, VAT or Stock.
	ItemType    string   `json:"item_type,omitempty"`
	Description string   `json:"description,omitempty"`
	Quantity    *Decimal `json:"quantity,omitempty"`
	Price       *Decimal `json:"price,omitempty"`

	// VATStatus is UK only: out_of_scope (the default), reduced, standard or
	// zero. Universal and US accounts use the sales tax rates instead.
	VATStatus          string   `json:"vat_status,omitempty"`
	SalesTaxRate       *Decimal `json:"sales_tax_rate,omitempty"`
	SecondSalesTaxRate *Decimal `json:"second_sales_tax_rate,omitempty"`

	Category ResourceURL `json:"category,omitempty"`
	// StockItem is required when ItemType is Stock.
	StockItem ResourceURL `json:"stock_item,omitempty"`

	CreatedAt Time `json:"created_at,omitzero"`
	UpdatedAt Time `json:"updated_at,omitzero"`
}

// VAT statuses accepted by PriceListItem.VATStatus.
const (
	VATStatusOutOfScope = "out_of_scope"
	VATStatusReduced    = "reduced"
	VATStatusStandard   = "standard"
	VATStatusZero       = "zero"
)

// PriceListItemService covers
// https://dev.freeagent.com/docs/price_list_items
type PriceListItemService struct {
	Collection[PriceListItem]
}

// Property is a rental property. Only UkUnincorporatedLandlord companies can
// hold them; on any other company type the collection is simply empty.
//
// See https://dev.freeagent.com/docs/properties
type Property struct {
	URL ResourceURL `json:"url,omitempty"`

	// Name is absent from the documented attribute list but present in its
	// example response.
	Name string `json:"name,omitempty"`

	Address1 string `json:"address1,omitempty"`
	Address2 string `json:"address2,omitempty"`
	Address3 string `json:"address3,omitempty"`
	Town     string `json:"town,omitempty"`
	Region   string `json:"region,omitempty"`
	Postcode string `json:"postcode,omitempty"`
	// Country defaults to United Kingdom and cannot be overridden.
	Country string `json:"country,omitempty"`
}

// PropertyService covers https://dev.freeagent.com/docs/properties
type PropertyService struct {
	Collection[Property]
}
