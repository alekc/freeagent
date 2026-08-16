package freeagent

// Project is a body of work billed to a contact.
//
// See https://dev.freeagent.com/docs/projects
type Project struct {
	URL ResourceURL `json:"url,omitempty"`

	// Contact is required and references the contact to bill.
	Contact ResourceURL `json:"contact,omitempty"`
	Name    string      `json:"name,omitempty"`
	// Status is Active, Completed, Cancelled or Hidden.
	Status              string `json:"status,omitempty"`
	ContractPOReference string `json:"contract_po_reference,omitempty"`
	Currency            string `json:"currency,omitempty"`

	// Budget is required; send zero when the project has no budget.
	Budget *Decimal `json:"budget,omitempty"`
	// BudgetUnits is Hours, Days or Monetary.
	BudgetUnits       string   `json:"budget_units,omitempty"`
	HoursPerDay       *Decimal `json:"hours_per_day,omitempty"`
	NormalBillingRate *Decimal `json:"normal_billing_rate,omitempty"`
	// BillingPeriod is hour or day.
	BillingPeriod string `json:"billing_period,omitempty"`

	UsesProjectInvoiceSequence         *bool `json:"uses_project_invoice_sequence,omitempty"`
	IncludeUnbilledTimeInProfitability *bool `json:"include_unbilled_time_in_profitability,omitempty"`
	IsIR35                             *bool `json:"is_ir35,omitempty"`

	StartsOn Date `json:"starts_on,omitzero"`
	EndsOn   Date `json:"ends_on,omitzero"`

	// Read-only. ContactName is a display convenience the API adds to
	// responses; it is not accepted on write.
	ContactName string `json:"contact_name,omitempty"`
	IsDeletable *bool  `json:"is_deletable,omitempty"`
	CreatedAt   Time   `json:"created_at,omitzero"`
	UpdatedAt   Time   `json:"updated_at,omitzero"`
}

// Views accepted by the projects list endpoint.
const (
	ProjectViewActive    = "active"
	ProjectViewCompleted = "completed"
	ProjectViewCancelled = "cancelled"
	ProjectViewHidden    = "hidden"
)

// ProjectService covers https://dev.freeagent.com/docs/projects
type ProjectService struct {
	Collection[Project]
}
