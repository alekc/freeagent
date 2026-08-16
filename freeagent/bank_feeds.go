package freeagent

import (
	"context"
	"net/http"
)

// BankFeed is a live connection importing transactions into a bank account.
//
// Read-only: a feed is established through the FreeAgent interface or a
// banking partner, not through this API.
//
// See https://dev.freeagent.com/docs/bank_feeds
type BankFeed struct {
	URL ResourceURL `json:"url,omitempty"`

	BankAccount ResourceURL `json:"bank_account,omitempty"`
	// State is the feed's current status, for example enabled.
	State string `json:"state,omitempty"`
	// FeedType is api or open_banking.
	FeedType        string `json:"feed_type,omitempty"`
	BankServiceName string `json:"bank_service_name,omitempty"`
	// SCAExpiresAt is when strong customer authentication lapses and the feed
	// needs reconnecting. API feeds only.
	SCAExpiresAt Time `json:"sca_expires_at,omitzero"`

	CreatedAt Time `json:"created_at,omitzero"`
	UpdatedAt Time `json:"updated_at,omitzero"`
}

// Feed types returned by BankFeed.FeedType.
const (
	BankFeedTypeAPI         = "api"
	BankFeedTypeOpenBanking = "open_banking"
)

// BankFeedService covers https://dev.freeagent.com/docs/bank_feeds
type BankFeedService struct {
	ReadCollection[BankFeed]
}

// CISBand is one Construction Industry Scheme deduction band.
//
// UK companies enrolled in CIS for subcontractors only; the list is empty
// otherwise.
//
// See https://dev.freeagent.com/docs/cis_bands
type CISBand struct {
	// Name is cis_gross, cis_standard or cis_higher.
	Name string `json:"name,omitempty"`
	// DeductionRate is a fraction, so 20% arrives as "0.2".
	DeductionRate        *Decimal `json:"deduction_rate,omitempty"`
	IncomeDescription    string   `json:"income_description,omitempty"`
	DeductionDescription string   `json:"deduction_description,omitempty"`
	NominalCode          string   `json:"nominal_code,omitempty"`
}

// CIS band names.
const (
	CISBandGross    = "cis_gross"
	CISBandStandard = "cis_standard"
	CISBandHigher   = "cis_higher"
)

// CISBandService covers https://dev.freeagent.com/docs/cis_bands
//
// The envelope key is available_bands, not cis_bands, so this does not fit
// the generic collection.
type CISBandService struct {
	client *Client
	meta   ResourceMeta
}

// Meta returns the resource metadata.
func (s *CISBandService) Meta() ResourceMeta { return s.meta }

// List returns the bands available to the company.
func (s *CISBandService) List(ctx context.Context) ([]CISBand, *Response, error) {
	req, err := s.client.newRequest(ctx, http.MethodGet, s.meta.Path, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	var envelope struct {
		AvailableBands []CISBand `json:"available_bands"`
	}
	resp, err := s.client.do(req, &envelope)
	if err != nil {
		return nil, resp, err
	}
	return envelope.AvailableBands, resp, nil
}

// EmailAddressService covers
// https://dev.freeagent.com/docs/email_addresses
//
// The reply is an array of plain strings, not of objects, each formatted as
// `Name <address@example.com>`. There is no model type for that reason.
type EmailAddressService struct {
	client *Client
	meta   ResourceMeta
}

// Meta returns the resource metadata.
func (s *EmailAddressService) Meta() ResourceMeta { return s.meta }

// List returns the verified sender addresses for the company.
func (s *EmailAddressService) List(ctx context.Context) ([]string, *Response, error) {
	req, err := s.client.newRequest(ctx, http.MethodGet, s.meta.Path, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	var envelope struct {
		EmailAddresses []string `json:"email_addresses"`
	}
	resp, err := s.client.do(req, &envelope)
	if err != nil {
		return nil, resp, err
	}
	return envelope.EmailAddresses, resp, nil
}

// SalesTaxPeriod is a dated set of sales tax rates.
//
// US and Universal companies only: on a UK company the endpoint is a 404.
// Verifying it needs a sandbox company created with one of those types, which
// is a free test fixture rather than a real US business.
//
// See https://dev.freeagent.com/docs/sales_tax_periods
type SalesTaxPeriod struct {
	URL ResourceURL `json:"url,omitempty"`

	SalesTaxName string `json:"sales_tax_name,omitempty"`
	// SalesTaxRegistrationStatus is Registered or Not Registered.
	SalesTaxRegistrationStatus string   `json:"sales_tax_registration_status,omitempty"`
	SalesTaxRegistrationNumber string   `json:"sales_tax_registration_number,omitempty"`
	SalesTaxIsValueAdded       *bool    `json:"sales_tax_is_value_added,omitempty"`
	SalesTaxRate1              *Decimal `json:"sales_tax_rate_1,omitempty"`
	SalesTaxRate2              *Decimal `json:"sales_tax_rate_2,omitempty"`
	SalesTaxRate3              *Decimal `json:"sales_tax_rate_3,omitempty"`

	// Universal companies only.
	SecondSalesTaxName       string   `json:"second_sales_tax_name,omitempty"`
	SecondSalesTaxRate1      *Decimal `json:"second_sales_tax_rate_1,omitempty"`
	SecondSalesTaxRate2      *Decimal `json:"second_sales_tax_rate_2,omitempty"`
	SecondSalesTaxRate3      *Decimal `json:"second_sales_tax_rate_3,omitempty"`
	SecondSalesTaxIsCompound *bool    `json:"second_sales_tax_is_compound,omitempty"`

	EffectiveDate Date `json:"effective_date,omitzero"`

	// Read-only. A locked period cannot be deleted.
	IsLocked     *bool  `json:"is_locked,omitempty"`
	LockedReason string `json:"locked_reason,omitempty"`
}

// SalesTaxPeriodService covers
// https://dev.freeagent.com/docs/sales_tax_periods
type SalesTaxPeriodService struct {
	Collection[SalesTaxPeriod]
}
