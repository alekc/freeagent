package freeagent

// Contact is a client or supplier.
//
// Either OrganisationName, or FirstName and LastName, is required. Several
// fields need the Contacts and Projects permission and are absent otherwise.
//
// See https://dev.freeagent.com/docs/contacts
type Contact struct {
	URL ResourceURL `json:"url,omitempty"`

	FirstName        string `json:"first_name,omitempty"`
	LastName         string `json:"last_name,omitempty"`
	OrganisationName string `json:"organisation_name,omitempty"`

	Email        string `json:"email,omitempty"`
	BillingEmail string `json:"billing_email,omitempty"`
	PhoneNumber  string `json:"phone_number,omitempty"`
	Mobile       string `json:"mobile,omitempty"`

	Address1 string `json:"address1,omitempty"`
	Address2 string `json:"address2,omitempty"`
	Address3 string `json:"address3,omitempty"`
	Town     string `json:"town,omitempty"`
	Region   string `json:"region,omitempty"`
	Postcode string `json:"postcode,omitempty"`
	Country  string `json:"country,omitempty"`

	// Status is Active or Hidden.
	Status string `json:"status,omitempty"`
	// Locale is one of the language codes FreeAgent supports for invoices.
	Locale string `json:"locale,omitempty"`
	// ChargeSalesTax is Auto, Always or Never.
	ChargeSalesTax             string `json:"charge_sales_tax,omitempty"`
	SalesTaxRegistrationNumber string `json:"sales_tax_registration_number,omitempty"`
	ContactNameOnInvoices      *bool  `json:"contact_name_on_invoices,omitempty"`
	UsesContactInvoiceSequence *bool  `json:"uses_contact_invoice_sequence,omitempty"`
	DefaultPaymentTermsInDays  *int   `json:"default_payment_terms_in_days,omitempty"`

	// Construction Industry Scheme. CISDeductionRate is required when
	// IsCISSubcontractor is true, and is one of cis_gross, cis_standard or
	// cis_higher.
	IsCISSubcontractor              *bool  `json:"is_cis_subcontractor,omitempty"`
	CISDeductionRate                string `json:"cis_deduction_rate,omitempty"`
	UniqueTaxReference              string `json:"unique_tax_reference,omitempty"`
	SubcontractorVerificationNumber string `json:"subcontractor_verification_number,omitempty"`

	// Read-only.
	AccountBalance          *Decimal `json:"account_balance,omitempty"`
	ActiveProjectsCount     Int64    `json:"active_projects_count,omitempty"`
	DirectDebitMandateState string   `json:"direct_debit_mandate_state,omitempty"`
	CreatedAt               Time     `json:"created_at,omitzero"`
	UpdatedAt               Time     `json:"updated_at,omitzero"`
}

// Views accepted by the contacts list endpoint.
const (
	ContactViewAll               = "all"
	ContactViewActive            = "active"
	ContactViewClients           = "clients"
	ContactViewSuppliers         = "suppliers"
	ContactViewActiveProjects    = "active_projects"
	ContactViewCompletedProjects = "completed_projects"
	ContactViewOpenClients       = "open_clients"
	ContactViewOpenSuppliers     = "open_suppliers"
	ContactViewHidden            = "hidden"
)

// ContactService covers https://dev.freeagent.com/docs/contacts
type ContactService struct {
	Collection[Contact]
}
