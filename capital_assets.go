package freeagent

import (
	"context"
	"net/url"
)

// Depreciation methods accepted by DepreciationProfile.
const (
	DepreciationStraightLine     = "straight_line"
	DepreciationReducingBalance  = "reducing_balance"
	DepreciationNone             = "no_depreciation"
	DepreciationFrequencyMonthly = "monthly"
	DepreciationFrequencyAnnual  = "annually"
)

// DepreciationProfile describes how an asset loses value.
//
// It is not a resource of its own despite having a documentation page: there
// is no /v2/depreciation_profiles endpoint. It appears nested on a capital
// asset, and is accepted nested on expenses, bill items and bank transaction
// explanations when those create an asset.
//
// AssetLifeYears applies to straight_line, AnnualDepreciationPercentage to
// reducing_balance; which one is required depends on Method.
//
// See https://dev.freeagent.com/docs/depreciation_profiles
type DepreciationProfile struct {
	Method                       string `json:"method,omitempty"`
	AssetLifeYears               *int   `json:"asset_life_years,omitempty"`
	AnnualDepreciationPercentage *int   `json:"annual_depreciation_percentage,omitempty"`
	// Frequency is monthly (the default) or annually.
	Frequency string `json:"frequency,omitempty"`
}

// CapitalAsset is a purchase written down over time.
//
// The API exposes reads only: assets come into being through the expense,
// bill or bank explanation that bought them, which is why this family embeds
// ReadCollection.
//
// See https://dev.freeagent.com/docs/capital_assets
type CapitalAsset struct {
	URL ResourceURL `json:"url,omitempty"`

	Description string `json:"description,omitempty"`
	// AssetType is a capital asset type name, not a URL.
	AssetType   string `json:"asset_type,omitempty"`
	PurchasedOn Date   `json:"purchased_on,omitzero"`
	DisposedOn  Date   `json:"disposed_on,omitzero"`

	DepreciationProfile *DepreciationProfile `json:"depreciation_profile,omitempty"`
	// AssetLifeYears is superseded by DepreciationProfile.
	//
	// Deprecated: use DepreciationProfile.AssetLifeYears.
	AssetLifeYears *int `json:"asset_life_years,omitempty"`

	// CapitalAssetHistory is only populated when include_history=true.
	CapitalAssetHistory []CapitalAssetHistoryEntry `json:"capital_asset_history,omitempty"`

	CreatedAt Time `json:"created_at,omitzero"`
	UpdatedAt Time `json:"updated_at,omitzero"`
}

// CapitalAssetHistoryEntry is one event in an asset's life.
type CapitalAssetHistoryEntry struct {
	// Type is purchase, depreciation, annual_investment_allowance or
	// disposal.
	Type        string      `json:"type,omitempty"`
	Description string      `json:"description,omitempty"`
	Date        Date        `json:"date,omitzero"`
	Value       *Decimal    `json:"value,omitempty"`
	TaxValue    *Decimal    `json:"tax_value,omitempty"`
	Link        ResourceURL `json:"link,omitempty"`
}

// Views accepted by the capital assets list endpoint.
const (
	CapitalAssetViewAll        = "all"
	CapitalAssetViewDisposed   = "disposed"
	CapitalAssetViewDisposable = "disposable"
)

// CapitalAssetService covers https://dev.freeagent.com/docs/capital_assets
type CapitalAssetService struct {
	ReadCollection[CapitalAsset]
}

// ListWithHistory is List with include_history=true, which populates
// CapitalAssetHistory. It is off by default because the history is large.
func (s *CapitalAssetService) ListWithHistory(ctx context.Context, opts *ListOptions) ([]CapitalAsset, *Response, error) {
	return s.List(ctx, withHistory(opts))
}

// GetWithHistory is Get with include_history=true.
func (s *CapitalAssetService) GetWithHistory(ctx context.Context, id int64) (*CapitalAsset, *Response, error) {
	return s.get(ctx, s.memberPath(id, ""), url.Values{"include_history": {"true"}})
}

// withHistory copies opts and adds the include_history flag, leaving the
// caller's own options untouched.
func withHistory(opts *ListOptions) *ListOptions {
	scoped := opts.clone()
	extra := url.Values{}
	for key, values := range scoped.Extra {
		extra[key] = values
	}
	extra.Set("include_history", "true")
	scoped.Extra = extra
	return &scoped
}

// CapitalAssetType names a class of capital asset. Four are seeded by
// FreeAgent and marked SystemDefault; the rest are user-created and are the
// only ones that may be changed or removed.
//
// See https://dev.freeagent.com/docs/capital_asset_types
type CapitalAssetType struct {
	URL ResourceURL `json:"url,omitempty"`

	Name string `json:"name,omitempty"`

	// Read-only.
	SystemDefault *bool `json:"system_default,omitempty"`
	CreatedAt     Time  `json:"created_at,omitzero"`
	UpdatedAt     Time  `json:"updated_at,omitzero"`
}

// CapitalAssetTypeService covers
// https://dev.freeagent.com/docs/capital_asset_types
type CapitalAssetTypeService struct {
	Collection[CapitalAssetType]
}

// HirePurchase is a bill paid off in instalments.
//
// Every field is read-only: the record is created by flagging a bill as paid
// by hire purchase, not through this endpoint. UK companies only.
//
// See https://dev.freeagent.com/docs/hire_purchases
type HirePurchase struct {
	URL ResourceURL `json:"url,omitempty"`

	Description string      `json:"description,omitempty"`
	Bill        ResourceURL `json:"bill,omitempty"`

	LiabilitiesOverOneYearCategory  ResourceURL `json:"liabilities_over_one_year_category,omitempty"`
	LiabilitiesUnderOneYearCategory ResourceURL `json:"liabilities_under_one_year_category,omitempty"`
}

// HirePurchaseService covers https://dev.freeagent.com/docs/hire_purchases
type HirePurchaseService struct {
	ReadCollection[HirePurchase]
}
