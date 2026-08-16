package freeagent

import (
	"context"
	"net/http"
)

// Company is the account's own details. It is a singleton: there is no
// collection and no id segment.
//
// See https://dev.freeagent.com/docs/company
type Company struct {
	URL ResourceURL `json:"url,omitempty"`
	// ID is typed as an integer in the documentation while the example on the
	// same page returns it quoted. The live API sends an unquoted number, so
	// the example is the stale one, but the type stays lenient rather than
	// betting on that staying true.
	ID Int64 `json:"id,omitempty"`

	Name      string `json:"name,omitempty"`
	Subdomain string `json:"subdomain,omitempty"`
	// Locale is undocumented but returned by the live API.
	Locale string `json:"locale,omitempty"`
	// Type is one of UkLimitedCompany, UkLimitedLiabilityPartnership,
	// UkPartnership, UkSoleTrader, UkUnincorporatedLandlord,
	// UsLimitedLiabilityCompany, UsPartnership, UsSoleProprietor, UsCCorp,
	// UsSCorp or UniversalCompany.
	Type     string `json:"type,omitempty"`
	Currency string `json:"currency,omitempty"`
	// MileageUnits is miles or kilometers.
	MileageUnits string `json:"mileage_units,omitempty"`

	CompanyStartDate        Date               `json:"company_start_date,omitzero"`
	TradingStartDate        Date               `json:"trading_start_date,omitzero"`
	FirstAccountingYearEnd  Date               `json:"first_accounting_year_end,omitzero"`
	FreeAgentStartDate      Date               `json:"freeagent_start_date,omitzero"`
	AnnualAccountingPeriods []AccountingPeriod `json:"annual_accounting_periods,omitempty"`

	Address1 string `json:"address1,omitempty"`
	Address2 string `json:"address2,omitempty"`
	Address3 string `json:"address3,omitempty"`
	Town     string `json:"town,omitempty"`
	Region   string `json:"region,omitempty"`
	Postcode string `json:"postcode,omitempty"`
	Country  string `json:"country,omitempty"`

	CompanyRegistrationNumber string `json:"company_registration_number,omitempty"`
	ContactEmail              string `json:"contact_email,omitempty"`
	ContactPhone              string `json:"contact_phone,omitempty"`
	Website                   string `json:"website,omitempty"`
	BusinessType              string `json:"business_type,omitempty"`
	BusinessCategory          string `json:"business_category,omitempty"`
	// ShortDateFormat is one of "dd mmm yy", "dd-mm-yyyy", "mm/dd/yyyy" or
	// "yyyy-mm-dd".
	ShortDateFormat string `json:"short_date_format,omitempty"`

	SalesTaxName                        string    `json:"sales_tax_name,omitempty"`
	SalesTaxRegistrationNumber          string    `json:"sales_tax_registration_number,omitempty"`
	SalesTaxRegistrationStatus          string    `json:"sales_tax_registration_status,omitempty"`
	SalesTaxEffectiveDate               Date      `json:"sales_tax_effective_date,omitzero"`
	SalesTaxIsValueAdded                *bool     `json:"sales_tax_is_value_added,omitempty"`
	SalesTaxDeregistrationEffectiveDate Date      `json:"sales_tax_deregistration_effective_date,omitzero"`
	SalesTaxRates                       []Decimal `json:"sales_tax_rates,omitempty"`
	// Undocumented, observed on the live API.
	ECVATReportingEnabled           *bool `json:"ec_vat_reporting_enabled,omitempty"`
	SupportsAutoSalesTaxOnPurchases *bool `json:"supports_auto_sales_tax_on_purchases,omitempty"`

	// Universal and US accounts only.
	SecondSalesTaxName       string    `json:"second_sales_tax_name,omitempty"`
	SecondSalesTaxRates      []Decimal `json:"second_sales_tax_rates,omitempty"`
	SecondSalesTaxIsCompound *bool     `json:"second_sales_tax_is_compound,omitempty"`

	// UK VAT accounts only. InitialVATBasis is Invoice or Cash.
	VATFirstReturnPeriodEndsOn Date   `json:"vat_first_return_period_ends_on,omitzero"`
	InitialVATBasis            string `json:"initial_vat_basis,omitempty"`
	InitiallyOnFRS             *bool  `json:"initially_on_frs,omitempty"`
	InitialVATFRSType          string `json:"initial_vat_frs_type,omitempty"`

	// Construction Industry Scheme. CISEnabled and CISSubcontractor are
	// aliases of each other in the API.
	CISEnabled       *bool `json:"cis_enabled,omitempty"`
	CISSubcontractor *bool `json:"cis_subcontractor,omitempty"`
	CISContractor    *bool `json:"cis_contractor,omitempty"`

	LockedAttributes []string `json:"locked_attributes,omitempty"`
	CreatedAt        Time     `json:"created_at,omitzero"`
	UpdatedAt        Time     `json:"updated_at,omitzero"`
}

// AccountingPeriod is one entry in a company's annual accounting periods.
type AccountingPeriod struct {
	StartsOn Date `json:"starts_on,omitzero"`
	EndsOn   Date `json:"ends_on,omitzero"`
}

// TaxTimelineItem is an upcoming tax event.
type TaxTimelineItem struct {
	Description string   `json:"description,omitempty"`
	Nature      string   `json:"nature,omitempty"`
	DatedOn     Date     `json:"dated_on,omitzero"`
	AmountDue   *Decimal `json:"amount_due,omitempty"`
	IsPersonal  *bool    `json:"is_personal,omitempty"`
}

// CompanyService covers https://dev.freeagent.com/docs/company
type CompanyService struct {
	Reader[Company]
}

// BusinessCategories lists the business categories the account may use.
func (s *CompanyService) BusinessCategories(ctx context.Context) ([]string, *Response, error) {
	var envelope struct {
		BusinessCategories []string `json:"business_categories"`
	}
	resp, err := s.sub(ctx, "business_categories", &envelope)
	if err != nil {
		return nil, resp, err
	}
	return envelope.BusinessCategories, resp, nil
}

// TaxTimeline returns upcoming tax events. It needs the Tax, Accounting and
// Users access level.
func (s *CompanyService) TaxTimeline(ctx context.Context) ([]TaxTimelineItem, *Response, error) {
	var envelope struct {
		TimelineItems []TaxTimelineItem `json:"timeline_items"`
	}
	resp, err := s.sub(ctx, "tax_timeline", &envelope)
	if err != nil {
		return nil, resp, err
	}
	return envelope.TimelineItems, resp, nil
}

func (s *CompanyService) sub(ctx context.Context, suffix string, out any) (*Response, error) {
	req, err := s.client.newRequest(ctx, http.MethodGet, s.meta.Path+"/"+suffix, nil, nil)
	if err != nil {
		return nil, err
	}
	return s.client.do(req, out)
}
