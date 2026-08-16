package freeagent

import "fmt"

// services holds the typed resource services hanging off a Client. Each wave
// of resource models adds its fields here and its wiring to initServices, so
// the Client surface grows in one predictable place.
type services struct {
	Attachments                 *AttachmentService
	BankAccounts                *BankAccountService
	BankTransactionExplanations *BankTransactionExplanationService
	BankFeeds                   *BankFeedService
	BankTransactions            *BankTransactionService
	Bills                       *BillService
	CapitalAssetTypes           *CapitalAssetTypeService
	CapitalAssets               *CapitalAssetService
	CISBands                    *CISBandService
	Categories                  *CategoryService
	Company                     *CompanyService
	Contacts                    *ContactService
	CreditNoteReconciliations   *CreditNoteReconciliationService
	CorporationTaxReturns       *CorporationTaxReturnService
	CreditNotes                 *CreditNoteService
	EmailAddresses              *EmailAddressService
	Estimates                   *EstimateService
	Expenses                    *ExpenseService
	FinalAccountsReports        *FinalAccountsReportService
	HirePurchases               *HirePurchaseService
	IncomeTaxReturns            *IncomeTaxReturnService
	Invoices                    *InvoiceService
	JournalSets                 *JournalSetService
	Notes                       *NoteService
	Payroll                     *PayrollService
	PayrollProfiles             *PayrollProfileService
	PriceListItems              *PriceListItemService
	Projects                    *ProjectService
	Properties                  *PropertyService
	RecurringInvoices           *RecurringInvoiceService
	// SalesTax exposes the EC VAT MOSS rate lookup, the only endpoint on the
	// otherwise prose sales tax documentation page.
	SalesTax        *SalesTaxService
	SalesTaxPeriods *SalesTaxPeriodService
	StockItems      *StockItemService
	// Reports gathers the accounting reports, which are read-only, have no
	// id segment, and each answer with their own shape.
	Reports      *ReportService
	Tasks        *TaskService
	Timeslips    *TimeslipService
	Transactions *TransactionService
	Users        *UserService
	VATReturns   *VATReturnService
}

// initServices wires the typed services after options have been applied.
func (c *Client) initServices() {
	c.Attachments = &AttachmentService{client: c, meta: mustResource("attachments")}
	c.BankAccounts = &BankAccountService{newCollection[BankAccount](c, mustResource("bank_accounts"))}
	c.BankTransactionExplanations = &BankTransactionExplanationService{
		newCollection[BankTransactionExplanation](c, mustResource("bank_transaction_explanations")),
	}
	c.BankTransactions = &BankTransactionService{
		newReadCollection[BankTransaction](c, mustResource("bank_transactions")),
	}
	c.Bills = &BillService{newCollection[Bill](c, mustResource("bills"))}
	c.CapitalAssetTypes = &CapitalAssetTypeService{
		newCollection[CapitalAssetType](c, mustResource("capital_asset_types")),
	}
	c.CapitalAssets = &CapitalAssetService{
		newReadCollection[CapitalAsset](c, mustResource("capital_assets")),
	}
	c.Categories = &CategoryService{client: c, meta: mustResource("categories")}
	c.Company = &CompanyService{newReader[Company](c, mustResource("company"))}
	c.Contacts = &ContactService{newCollection[Contact](c, mustResource("contacts"))}
	c.CreditNoteReconciliations = &CreditNoteReconciliationService{
		newCollection[CreditNoteReconciliation](c, mustResource("credit_note_reconciliations")),
	}
	c.CreditNotes = &CreditNoteService{newCollection[CreditNote](c, mustResource("credit_notes"))}
	c.Estimates = &EstimateService{newCollection[Estimate](c, mustResource("estimates"))}
	c.Expenses = &ExpenseService{newCollection[Expense](c, mustResource("expenses"))}
	c.FinalAccountsReports = &FinalAccountsReportService{
		periodService[FinalAccountsReport]{client: c, meta: mustResource("final_accounts_reports")},
	}
	c.HirePurchases = &HirePurchaseService{
		newReadCollection[HirePurchase](c, mustResource("hire_purchases")),
	}
	c.Invoices = &InvoiceService{newCollection[Invoice](c, mustResource("invoices"))}
	c.JournalSets = &JournalSetService{newCollection[JournalSet](c, mustResource("journal_sets"))}
	c.Notes = &NoteService{newCollection[Note](c, mustResource("notes"))}
	c.PriceListItems = &PriceListItemService{
		newCollection[PriceListItem](c, mustResource("price_list_items")),
	}
	c.Projects = &ProjectService{newCollection[Project](c, mustResource("projects"))}
	c.Properties = &PropertyService{newCollection[Property](c, mustResource("properties"))}
	c.RecurringInvoices = &RecurringInvoiceService{
		newReadCollection[RecurringInvoice](c, mustResource("recurring_invoices")),
	}
	c.Reports = &ReportService{client: c}
	c.SalesTax = &SalesTaxService{client: c}
	c.StockItems = &StockItemService{newReadCollection[StockItem](c, mustResource("stock_items"))}
	c.Tasks = &TaskService{newCollection[Task](c, mustResource("tasks"))}
	c.Timeslips = &TimeslipService{newCollection[Timeslip](c, mustResource("timeslips"))}
	c.Transactions = &TransactionService{
		newReadCollection[Transaction](c, mustResource("transactions")),
	}
	c.BankFeeds = &BankFeedService{newReadCollection[BankFeed](c, mustResource("bank_feeds"))}
	c.CISBands = &CISBandService{client: c, meta: mustResource("cis_bands")}
	c.CorporationTaxReturns = &CorporationTaxReturnService{
		periodService[CorporationTaxReturn]{client: c, meta: mustResource("corporation_tax_returns")},
	}
	c.EmailAddresses = &EmailAddressService{client: c, meta: mustResource("email_addresses")}
	c.IncomeTaxReturns = &IncomeTaxReturnService{
		periodService[IncomeTaxReturn]{client: c, meta: mustResource("income_tax_returns")},
	}
	c.Payroll = &PayrollService{client: c, meta: mustResource("payroll")}
	c.PayrollProfiles = &PayrollProfileService{client: c, meta: mustResource("payroll_profiles")}
	c.SalesTaxPeriods = &SalesTaxPeriodService{
		newCollection[SalesTaxPeriod](c, mustResource("sales_tax_periods")),
	}
	c.VATReturns = &VATReturnService{
		periodService[VATReturn]{client: c, meta: mustResource("vat_returns")},
	}
	c.Users = &UserService{newCollection[User](c, mustResource("users"))}
}

// mustResource resolves a registry entry that the wiring above depends on. A
// miss is a programming error in this package, not a runtime condition, and
// TestServicesAreWired catches it before any caller can.
func mustResource(name string) ResourceMeta {
	meta, ok := Resources[name]
	if !ok {
		panic(fmt.Sprintf("freeagent: resource %q is not registered", name))
	}
	return meta
}
