package freeagent

import "sort"

// ResourceMeta describes one FreeAgent resource family. The same metadata
// drives the typed services and the facli generic commands, so a resource is
// added in exactly one place.
type ResourceMeta struct {
	// Name is the registry key and the name facli accepts.
	Name string
	// Path is relative to the API root, for example "invoices" or
	// "accounting/profit_and_loss/summary".
	Path string
	// Singular is the JSON envelope key for a single record. Empty means the
	// endpoint returns an unenveloped body.
	Singular string
	// Plural is the JSON envelope key for a list. Empty for singletons.
	Plural string
	// Singleton marks endpoints with no id segment, such as company.
	Singleton bool
	// ReadOnly marks endpoints with no write verbs.
	ReadOnly bool
	// Grouped marks endpoints whose results are split across several
	// envelope keys instead of one plural key. Categories is the only one.
	Grouped bool
	// NoList marks families with no collection endpoint, such as
	// attachments, which are reached only through a parent record.
	NoList bool
	// RequiresBankAccount marks list endpoints that reject a request without
	// a bank_account filter.
	RequiresBankAccount bool
	// CustomEnvelope marks families whose response uses neither the singular
	// nor the plural key, such as payroll, which answers with periods and
	// payments. Their services decode the envelope themselves.
	CustomEnvelope bool
	// Doc is the upstream documentation URL.
	Doc string
}

// Resources holds the families whose paths have been verified against the
// upstream documentation. Entries are added as each wave of typed models
// lands; until then, facli raw reaches any endpoint by path.
var Resources = map[string]ResourceMeta{
	// Attachments have no collection endpoint: they are created through the
	// parent record's attachment field and read or removed by id.
	"attachments": {
		Name: "attachments", Path: "attachments",
		Singular: "attachment", NoList: true,
		Doc: "https://dev.freeagent.com/docs/attachments",
	},
	"balance_sheet": {
		Name: "balance_sheet", Path: "accounting/balance_sheet",
		Singleton: true, ReadOnly: true,
		Doc: "https://dev.freeagent.com/docs/balance_sheet",
	},
	"bank_accounts": {
		Name: "bank_accounts", Path: "bank_accounts",
		Singular: "bank_account", Plural: "bank_accounts",
		Doc: "https://dev.freeagent.com/docs/bank_accounts",
	},
	"bank_transaction_explanations": {
		Name: "bank_transaction_explanations", Path: "bank_transaction_explanations",
		Singular: "bank_transaction_explanation", Plural: "bank_transaction_explanations",
		RequiresBankAccount: true,
		Doc:                 "https://dev.freeagent.com/docs/bank_transaction_explanations",
	},
	// Transactions arrive by statement upload or bank feed, so there are no
	// per-record write verbs.
	"bank_transactions": {
		Name: "bank_transactions", Path: "bank_transactions",
		Singular: "bank_transaction", Plural: "bank_transactions",
		ReadOnly: true, RequiresBankAccount: true,
		Doc: "https://dev.freeagent.com/docs/bank_transactions",
	},
	// Read-only: feeds are established in the FreeAgent interface or by a
	// banking partner.
	"bank_feeds": {
		Name: "bank_feeds", Path: "bank_feeds",
		Singular: "bank_feed", Plural: "bank_feeds",
		ReadOnly: true,
		Doc:      "https://dev.freeagent.com/docs/bank_feeds",
	},
	"bills": {
		Name: "bills", Path: "bills",
		Singular: "bill", Plural: "bills",
		Doc: "https://dev.freeagent.com/docs/bills",
	},
	// Categories are grouped across four envelope keys and addressed by
	// nominal code, so they carry no plural key. See CategoryService.
	"categories": {
		Name: "categories", Path: "categories",
		Singular: "category", Grouped: true,
		Doc: "https://dev.freeagent.com/docs/categories",
	},
	"capital_asset_types": {
		Name: "capital_asset_types", Path: "capital_asset_types",
		Singular: "capital_asset_type", Plural: "capital_asset_types",
		Doc: "https://dev.freeagent.com/docs/capital_asset_types",
	},
	// Read-only: assets are created by the expense, bill or explanation that
	// bought them.
	"capital_assets": {
		Name: "capital_assets", Path: "capital_assets",
		Singular: "capital_asset", Plural: "capital_assets",
		ReadOnly: true,
		Doc:      "https://dev.freeagent.com/docs/capital_assets",
	},
	// Cashflow sits at the API root, not under accounting/ like its
	// siblings, and needs an explicit date range.
	"cashflow": {
		Name: "cashflow", Path: "cashflow",
		Singular: "cashflow", Singleton: true, ReadOnly: true,
		Doc: "https://dev.freeagent.com/docs/cashflow",
	},
	// Envelope key is available_bands, not cis_bands. UK CIS subcontractor
	// companies only.
	"cis_bands": {
		Name: "cis_bands", Path: "cis_bands",
		Singleton: true, ReadOnly: true, CustomEnvelope: true,
		Doc: "https://dev.freeagent.com/docs/cis_bands",
	},
	"company": {
		Name: "company", Path: "company",
		Singular: "company", Singleton: true, ReadOnly: true,
		Doc: "https://dev.freeagent.com/docs/company",
	},
	"credit_note_reconciliations": {
		Name: "credit_note_reconciliations", Path: "credit_note_reconciliations",
		Singular: "credit_note_reconciliation", Plural: "credit_note_reconciliations",
		Doc: "https://dev.freeagent.com/docs/credit_note_reconciliations",
	},
	"credit_notes": {
		Name: "credit_notes", Path: "credit_notes",
		Singular: "credit_note", Plural: "credit_notes",
		Doc: "https://dev.freeagent.com/docs/credit_notes",
	},
	"contacts": {
		Name: "contacts", Path: "contacts",
		Singular: "contact", Plural: "contacts",
		Doc: "https://dev.freeagent.com/docs/contacts",
	},
	"corporation_tax_returns": {
		Name: "corporation_tax_returns", Path: "corporation_tax_returns",
		Singular: "corporation_tax_return", Plural: "corporation_tax_returns",
		Doc: "https://dev.freeagent.com/docs/corporation_tax_returns",
	},
	// An array of plain strings, not of objects.
	"email_addresses": {
		Name: "email_addresses", Path: "email_addresses",
		Plural: "email_addresses", Singleton: true, ReadOnly: true,
		Doc: "https://dev.freeagent.com/docs/email_addresses",
	},
	"estimates": {
		Name: "estimates", Path: "estimates",
		Singular: "estimate", Plural: "estimates",
		Doc: "https://dev.freeagent.com/docs/estimates",
	},
	"expenses": {
		Name: "expenses", Path: "expenses",
		Singular: "expense", Plural: "expenses",
		Doc: "https://dev.freeagent.com/docs/expenses",
	},
	// Addressed by period end date rather than a numeric id.
	"final_accounts_reports": {
		Name: "final_accounts_reports", Path: "final_accounts_reports",
		Singular: "final_accounts_report", Plural: "final_accounts_reports",
		Doc: "https://dev.freeagent.com/docs/final_accounts_reports",
	},
	// Read-only, UK companies only: created by flagging a bill as paid by
	// hire purchase.
	"hire_purchases": {
		Name: "hire_purchases", Path: "hire_purchases",
		Singular: "hire_purchase", Plural: "hire_purchases",
		ReadOnly: true,
		Doc:      "https://dev.freeagent.com/docs/hire_purchases",
	},
	// Nested under a user: /v2/users/:id/self_assessment_returns. Neither
	// /v2/income_tax_returns nor /v2/self_assessment_returns exists.
	"income_tax_returns": {
		Name: "income_tax_returns", Path: "self_assessment_returns",
		Singular: "self_assessment_return", Plural: "self_assessment_returns",
		Doc: "https://dev.freeagent.com/docs/income_tax_returns",
	},
	"invoices": {
		Name: "invoices", Path: "invoices",
		Singular: "invoice", Plural: "invoices",
		Doc: "https://dev.freeagent.com/docs/invoices",
	},
	"journal_sets": {
		Name: "journal_sets", Path: "journal_sets",
		Singular: "journal_set", Plural: "journal_sets",
		Doc: "https://dev.freeagent.com/docs/journal_sets",
	},
	// Always scoped to a contact or project; listing without one is a 400.
	"notes": {
		Name: "notes", Path: "notes",
		Singular: "note", Plural: "notes",
		Doc: "https://dev.freeagent.com/docs/notes",
	},
	// Addressed by tax year: /v2/payroll alone is a 404.
	"payroll": {
		Name: "payroll", Path: "payroll",
		ReadOnly: true, CustomEnvelope: true,
		Doc: "https://dev.freeagent.com/docs/payroll",
	},
	"payroll_profiles": {
		Name: "payroll_profiles", Path: "payroll_profiles",
		ReadOnly: true, CustomEnvelope: true,
		Doc: "https://dev.freeagent.com/docs/payroll_profiles",
	},
	"price_list_items": {
		Name: "price_list_items", Path: "price_list_items",
		Singular: "price_list_item", Plural: "price_list_items",
		Doc: "https://dev.freeagent.com/docs/price_list_items",
	},
	"profit_and_loss": {
		Name: "profit_and_loss", Path: "accounting/profit_and_loss/summary",
		Singleton: true, ReadOnly: true,
		Doc: "https://dev.freeagent.com/docs/profit_and_loss",
	},
	"projects": {
		Name: "projects", Path: "projects",
		Singular: "project", Plural: "projects",
		Doc: "https://dev.freeagent.com/docs/projects",
	},
	// UkUnincorporatedLandlord companies only; empty elsewhere.
	"properties": {
		Name: "properties", Path: "properties",
		Singular: "property", Plural: "properties",
		Doc: "https://dev.freeagent.com/docs/properties",
	},
	// Read-only: recurring schedules are created in the FreeAgent interface.
	"recurring_invoices": {
		Name: "recurring_invoices", Path: "recurring_invoices",
		Singular: "recurring_invoice", Plural: "recurring_invoices",
		ReadOnly: true,
		Doc:      "https://dev.freeagent.com/docs/recurring_invoices",
	},
	// Read-only: POST answers 404, not 405.
	// US and Universal companies only; a 404 on a UK company.
	"sales_tax_periods": {
		Name: "sales_tax_periods", Path: "sales_tax_periods",
		Singular: "sales_tax_period", Plural: "sales_tax_periods",
		Doc: "https://dev.freeagent.com/docs/sales_tax_periods",
	},
	"stock_items": {
		Name: "stock_items", Path: "stock_items",
		Singular: "stock_item", Plural: "stock_items",
		ReadOnly: true,
		Doc:      "https://dev.freeagent.com/docs/stock_items",
	},
	"tasks": {
		Name: "tasks", Path: "tasks",
		Singular: "task", Plural: "tasks",
		Doc: "https://dev.freeagent.com/docs/tasks",
	},
	"timeslips": {
		Name: "timeslips", Path: "timeslips",
		Singular: "timeslip", Plural: "timeslips",
		Doc: "https://dev.freeagent.com/docs/timeslips",
	},
	// Generated by FreeAgent from other records, so read-only. Note the
	// accounting/ prefix, and that the range may not exceed 12 months.
	"transactions": {
		Name: "transactions", Path: "accounting/transactions",
		Singular: "transaction", Plural: "transactions",
		ReadOnly: true,
		Doc:      "https://dev.freeagent.com/docs/transactions",
	},
	// Addressed by period end date. Needs a VAT-registered company.
	"vat_returns": {
		Name: "vat_returns", Path: "vat_returns",
		Singular: "vat_return", Plural: "vat_returns",
		Doc: "https://dev.freeagent.com/docs/vat_returns",
	},
	"trial_balance": {
		Name: "trial_balance", Path: "accounting/trial_balance/summary",
		Singleton: true, ReadOnly: true,
		Doc: "https://dev.freeagent.com/docs/trial_balance",
	},
	"users": {
		Name: "users", Path: "users",
		Singular: "user", Plural: "users",
		Doc: "https://dev.freeagent.com/docs/users",
	},
}

// LookupResource returns the metadata registered under name.
func LookupResource(name string) (ResourceMeta, bool) {
	meta, ok := Resources[name]
	return meta, ok
}

// ResourceNames lists the registered families in a stable order.
func ResourceNames() []string {
	names := make([]string, 0, len(Resources))
	for name := range Resources {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
