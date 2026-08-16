package freeagent

import (
	"context"
	"iter"
)

// BankTransactionExplanation records what a bank transaction was for.
//
// Which fields are required depends on the kind of explanation: PaidInvoice
// for an invoice receipt, PaidBill for a bill payment, TransferBankAccount
// for a transfer, and so on. The grouped comments below follow the upstream
// documentation.
//
// See https://dev.freeagent.com/docs/bank_transaction_explanations
type BankTransactionExplanation struct {
	URL ResourceURL `json:"url,omitempty"`

	// One of BankAccount or BankTransaction is required.
	BankAccount     ResourceURL `json:"bank_account,omitempty"`
	BankTransaction ResourceURL `json:"bank_transaction,omitempty"`

	DatedOn      Date        `json:"dated_on,omitzero"`
	GrossValue   *Decimal    `json:"gross_value,omitempty"`
	Description  string      `json:"description,omitempty"`
	Category     ResourceURL `json:"category,omitempty"`
	ChequeNumber string      `json:"cheque_number,omitempty"`

	SalesTaxRate  *Decimal `json:"sales_tax_rate,omitempty"`
	SalesTaxValue *Decimal `json:"sales_tax_value,omitempty"`
	// SalesTaxStatus is TAXABLE, EXEMPT or OUT_OF_SCOPE.
	SalesTaxStatus       string   `json:"sales_tax_status,omitempty"`
	SecondSalesTaxRate   *Decimal `json:"second_sales_tax_rate,omitempty"`
	SecondSalesTaxValue  *Decimal `json:"second_sales_tax_value,omitempty"`
	SecondSalesTaxStatus string   `json:"second_sales_tax_status,omitempty"`
	// ECStatus is UK/Non-EC, EC Goods, EC Services, Reverse Charge or
	// EC VAT MOSS. PlaceOfSupply is required for EC VAT MOSS.
	ECStatus      string `json:"ec_status,omitempty"`
	PlaceOfSupply string `json:"place_of_supply,omitempty"`

	// Payments and refunds.
	Project          ResourceURL `json:"project,omitempty"`
	RebillType       string      `json:"rebill_type,omitempty"`
	RebillFactor     *Decimal    `json:"rebill_factor,omitempty"`
	ReceiptReference string      `json:"receipt_reference,omitempty"`

	// Invoice and bill settlement. ForeignCurrencyValue applies when the
	// settled document is in another currency.
	PaidInvoice          ResourceURL `json:"paid_invoice,omitempty"`
	PaidBill             ResourceURL `json:"paid_bill,omitempty"`
	ForeignCurrencyValue *Decimal    `json:"foreign_currency_value,omitempty"`

	// Money paid to or from a user.
	PaidUser ResourceURL `json:"paid_user,omitempty"`

	// Transfers between accounts.
	TransferBankAccount ResourceURL `json:"transfer_bank_account,omitempty"`

	// Stock movements.
	StockItem             ResourceURL `json:"stock_item,omitempty"`
	StockAlteringQuantity *int        `json:"stock_altering_quantity,omitempty"`

	// Capital assets. DisposedAsset is required for a disposal.
	DisposedAsset ResourceURL `json:"disposed_asset,omitempty"`

	// UK unincorporated landlords.
	Property ResourceURL `json:"property,omitempty"`
	// Opening balances against an initial debtor or creditor category.
	DirectContact ResourceURL `json:"direct_contact,omitempty"`

	Attachment *Attachment `json:"attachment,omitempty"`

	// Read-only.
	Type                      string      `json:"type,omitempty"`
	CapitalAsset              ResourceURL `json:"capital_asset,omitempty"`
	LinkedTransferExplanation ResourceURL `json:"linked_transfer_explanation,omitempty"`
	LinkedTransferAccount     ResourceURL `json:"linked_transfer_account,omitempty"`
	MarkedForReview           *bool       `json:"marked_for_review,omitempty"`
	IsMoneyIn                 *bool       `json:"is_money_in,omitempty"`
	IsMoneyOut                *bool       `json:"is_money_out,omitempty"`
	IsMoneyPaidToUser         *bool       `json:"is_money_paid_to_user,omitempty"`
	IsLocked                  *bool       `json:"is_locked,omitempty"`
	IsDeletable               *bool       `json:"is_deletable,omitempty"`
	LockedAttributes          []string    `json:"locked_attributes,omitempty"`
	LockedReason              string      `json:"locked_reason,omitempty"`
	UpdatedAt                 Time        `json:"updated_at,omitzero"`
}

// BankTransactionExplanationService covers
// https://dev.freeagent.com/docs/bank_transaction_explanations
type BankTransactionExplanationService struct {
	Collection[BankTransactionExplanation]
}

// List is unavailable without a bank account. The API requires the
// bank_account parameter, so this shadows the inherited List rather than
// letting it fail remotely. Use ListForAccount.
func (s *BankTransactionExplanationService) List(context.Context, *ListOptions) ([]BankTransactionExplanation, *Response, error) {
	return nil, nil, errBankAccountRequired("ListForAccount")
}

// All is unavailable without a bank account. Use AllForAccount.
func (s *BankTransactionExplanationService) All(context.Context, *ListOptions) iter.Seq2[BankTransactionExplanation, error] {
	return func(yield func(BankTransactionExplanation, error) bool) {
		yield(BankTransactionExplanation{}, errBankAccountRequired("AllForAccount"))
	}
}

// ListForAccount fetches one page of explanations for a bank account.
func (s *BankTransactionExplanationService) ListForAccount(ctx context.Context, account ResourceURL, opts *ListOptions) ([]BankTransactionExplanation, *Response, error) {
	scoped, err := scopeToBankAccount(opts, account)
	if err != nil {
		return nil, nil, err
	}
	return s.ReadCollection.List(ctx, scoped)
}

// AllForAccount iterates every explanation for a bank account.
func (s *BankTransactionExplanationService) AllForAccount(ctx context.Context, account ResourceURL, opts *ListOptions) iter.Seq2[BankTransactionExplanation, error] {
	scoped, err := scopeToBankAccount(opts, account)
	if err != nil {
		return func(yield func(BankTransactionExplanation, error) bool) {
			yield(BankTransactionExplanation{}, err)
		}
	}
	return s.ReadCollection.All(ctx, scoped)
}
