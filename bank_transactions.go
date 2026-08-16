package freeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"net/url"
)

// BankTransaction is a line on a bank statement or feed.
//
// Individual transactions are not created, updated or deleted through the
// API: they arrive by statement upload or bank feed, which is why this family
// exposes only reads plus UploadStatement.
//
// See https://dev.freeagent.com/docs/bank_transactions
type BankTransaction struct {
	URL ResourceURL `json:"url,omitempty"`

	BankAccount ResourceURL `json:"bank_account,omitempty"`
	// Amount is in the company's native currency.
	Amount          *Decimal `json:"amount,omitempty"`
	DatedOn         Date     `json:"dated_on,omitzero"`
	Description     string   `json:"description,omitempty"`
	FullDescription string   `json:"full_description,omitempty"`
	// TransactionID is the bank's own identifier, also known as fit_id.
	TransactionID string `json:"transaction_id,omitempty"`

	// Read-only.
	UnexplainedAmount           *Decimal                     `json:"unexplained_amount,omitempty"`
	IsManual                    *bool                        `json:"is_manual,omitempty"`
	MatchingTransactionsCount   *int                         `json:"matching_transactions_count,omitempty"`
	BankTransactionExplanations []BankTransactionExplanation `json:"bank_transaction_explanations,omitempty"`
	UploadedAt                  Time                         `json:"uploaded_at,omitzero"`
	CreatedAt                   Time                         `json:"created_at,omitzero"`
	UpdatedAt                   Time                         `json:"updated_at,omitzero"`
}

// BankTransactionService covers
// https://dev.freeagent.com/docs/bank_transactions
type BankTransactionService struct {
	ReadCollection[BankTransaction]
}

// List is unavailable without a bank account. The API requires the
// bank_account parameter, so this shadows the inherited List rather than
// letting it fail remotely. Use ListForAccount.
func (s *BankTransactionService) List(context.Context, *ListOptions) ([]BankTransaction, *Response, error) {
	return nil, nil, errBankAccountRequired("ListForAccount")
}

// All is unavailable without a bank account. Use AllForAccount.
func (s *BankTransactionService) All(context.Context, *ListOptions) iter.Seq2[BankTransaction, error] {
	return func(yield func(BankTransaction, error) bool) {
		yield(BankTransaction{}, errBankAccountRequired("AllForAccount"))
	}
}

// ListForAccount fetches one page of transactions for a bank account.
func (s *BankTransactionService) ListForAccount(ctx context.Context, account ResourceURL, opts *ListOptions) ([]BankTransaction, *Response, error) {
	scoped, err := scopeToBankAccount(opts, account)
	if err != nil {
		return nil, nil, err
	}
	return s.ReadCollection.List(ctx, scoped)
}

// AllForAccount iterates every transaction for a bank account.
func (s *BankTransactionService) AllForAccount(ctx context.Context, account ResourceURL, opts *ListOptions) iter.Seq2[BankTransaction, error] {
	scoped, err := scopeToBankAccount(opts, account)
	if err != nil {
		return func(yield func(BankTransaction, error) bool) {
			yield(BankTransaction{}, err)
		}
	}
	return s.ReadCollection.All(ctx, scoped)
}

// StatementLine is one row of a statement upload.
//
// This is deliberately not a BankTransaction: the upload takes a different and
// much smaller shape, and its amount is a JSON number where every other money
// value in the API is a quoted string. Posting a BankTransaction here is
// accepted with a 200 and then silently imports nothing.
type StatementLine struct {
	DatedOn     Date    `json:"dated_on"`
	Amount      Decimal `json:"amount"`
	Description string  `json:"description,omitempty"`
	// FitID is the bank's own identifier for the row. Supplying it lets
	// FreeAgent recognise a re-upload instead of duplicating the line.
	FitID string `json:"fitid,omitempty"`
}

// MarshalJSON writes amount unquoted. Decimal marshals as a string by design,
// which is right everywhere else and wrong here.
func (l StatementLine) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		DatedOn     Date            `json:"dated_on"`
		Amount      json.RawMessage `json:"amount"`
		Description string          `json:"description,omitempty"`
		FitID       string          `json:"fitid,omitempty"`
	}{
		DatedOn:     l.DatedOn,
		Amount:      json.RawMessage(l.Amount.String()),
		Description: l.Description,
		FitID:       l.FitID,
	})
}

// UploadStatement posts statement lines to a bank account. This is the only
// way transactions enter FreeAgent through the API.
//
// Import is asynchronous. The call returns before the lines are queryable,
// and the delay has been observed to range from under a second to most of a
// minute, so a read straight afterwards will often come back empty. Poll
// ListForAccount until the lines appear rather than treating the 200 as
// confirmation that they exist.
func (s *BankTransactionService) UploadStatement(ctx context.Context, account ResourceURL, lines []StatementLine) (*Response, error) {
	if account.IsZero() {
		return nil, errBankAccountRequired("UploadStatement")
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("freeagent: UploadStatement requires at least one line")
	}
	for i, line := range lines {
		if line.DatedOn.IsZero() {
			return nil, fmt.Errorf("freeagent: statement line %d has no date", i)
		}
	}
	query := url.Values{"bank_account": {account.String()}}
	body := map[string]any{"statement": lines}
	req, err := s.client.newRequest(ctx, http.MethodPost, s.meta.Path+"/statement", query, body)
	if err != nil {
		return nil, err
	}
	return s.client.do(req, nil)
}

// scopeToBankAccount copies opts and pins the required bank_account filter.
func scopeToBankAccount(opts *ListOptions, account ResourceURL) (*ListOptions, error) {
	if account.IsZero() {
		return nil, fmt.Errorf("freeagent: a bank account URL is required")
	}
	scoped := opts.clone()
	extra := url.Values{}
	for key, values := range scoped.Extra {
		extra[key] = values
	}
	extra.Set("bank_account", account.String())
	scoped.Extra = extra
	return &scoped, nil
}

func errBankAccountRequired(alternative string) error {
	return fmt.Errorf("freeagent: this endpoint requires a bank account, use %s", alternative)
}
