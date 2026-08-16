package freeagent

// Bank account type values accepted by BankAccount.Type.
const (
	BankAccountTypeStandard        = "StandardBankAccount"
	BankAccountTypeCreditCard      = "CreditCardAccount"
	BankAccountTypeEcommerce       = "EcommerceAccount"
	BankAccountTypeRentalStatement = "RentalStatementAccount"
	BankAccountTypeAmazonSeller    = "AmazonSellerBankAccount"
	BankAccountTypeGoCardless      = "GocardlessBankAccount"
	BankAccountTypePaypalClassic   = "PaypalClassicAccount"
	BankAccountTypePaypalCurrency  = "Paypal::CurrencyAccount"
	BankAccountTypeStripe          = "StripeBankAccount"
)

// BankAccount is a bank, credit card or payment-provider account.
//
// Several fields apply only to certain account types: AccountNumber, SortCode,
// IBAN and BIC to standard accounts, AccountNumber alone to credit cards, and
// Email to PayPal accounts.
//
// See https://dev.freeagent.com/docs/bank_accounts
type BankAccount struct {
	URL ResourceURL `json:"url,omitempty"`

	// Type, Name, BankName and OpeningBalance are required on create.
	Type     string `json:"type,omitempty"`
	Name     string `json:"name,omitempty"`
	BankName string `json:"bank_name,omitempty"`
	// OpeningBalance is the balance at the FreeAgent start date.
	OpeningBalance *Decimal `json:"opening_balance,omitempty"`

	// Currency is immutable once the account has transactions.
	Currency string `json:"currency,omitempty"`
	// Status is active or hidden.
	Status           string `json:"status,omitempty"`
	IsPersonal       *bool  `json:"is_personal,omitempty"`
	IsPrimary        *bool  `json:"is_primary,omitempty"`
	BankCode         string `json:"bank_code,omitempty"`
	BankGuessEnabled *bool  `json:"bank_guess_enabled,omitempty"`

	// Standard and credit card accounts.
	AccountNumber     string `json:"account_number,omitempty"`
	SortCode          string `json:"sort_code,omitempty"`
	SecondarySortCode string `json:"secondary_sort_code,omitempty"`
	IBAN              string `json:"iban,omitempty"`
	BIC               string `json:"bic,omitempty"`

	// PayPal accounts.
	Email string `json:"email,omitempty"`

	// Read-only.
	CurrentBalance     *Decimal `json:"current_balance,omitempty"`
	LatestActivityDate Date     `json:"latest_activity_date,omitzero"`
	CreatedAt          Time     `json:"created_at,omitzero"`
	UpdatedAt          Time     `json:"updated_at,omitzero"`

	// Transaction tallies. Undocumented, observed on the live API, and
	// useful for spotting an account with unexplained items waiting.
	TotalCount                    *int `json:"total_count,omitempty"`
	UnexplainedTransactionCount   *int `json:"unexplained_transaction_count,omitempty"`
	MarkedForReviewCount          *int `json:"marked_for_review_count,omitempty"`
	ManuallyAddedTransactionCount *int `json:"manually_added_transaction_count,omitempty"`
}

// Views accepted by the bank accounts list endpoint.
const (
	BankAccountViewStandard = "standard_bank_accounts"
	//nolint:gosec // G101 false positive: this is a query filter, not a credential
	BankAccountViewCreditCard = "credit_card_accounts"
	BankAccountViewPaypal     = "paypal_accounts"
)

// BankAccountService covers https://dev.freeagent.com/docs/bank_accounts
type BankAccountService struct {
	Collection[BankAccount]
}
