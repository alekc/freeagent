package freeagent

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// ReportOptions bounds an accounting report. Most reports accept a date range;
// some also accept an accounting period reference instead.
//
// Leaving everything unset asks for the report's own default, which is
// generally the current accounting year to date.
type ReportOptions struct {
	FromDate Date
	ToDate   Date
	// AccountingPeriod selects a named period instead of a date range, for
	// the reports that support it.
	AccountingPeriod string
	// Extra carries anything not modelled here.
	Extra url.Values
}

func (o *ReportOptions) values() url.Values {
	v := url.Values{}
	if o == nil {
		return v
	}
	if !o.FromDate.IsZero() {
		v.Set("from_date", o.FromDate.String())
	}
	if !o.ToDate.IsZero() {
		v.Set("to_date", o.ToDate.String())
	}
	if o.AccountingPeriod != "" {
		v.Set("accounting_period", o.AccountingPeriod)
	}
	for key, values := range o.Extra {
		v.Del(key)
		for _, value := range values {
			v.Add(key, value)
		}
	}
	return v
}

// TrialBalanceEntry is one line of the trial balance.
type TrialBalanceEntry struct {
	Category ResourceURL `json:"category,omitempty"`
	// NominalCode is the internal code, which for sub-accounts embeds the
	// underlying record id, for example "750-47915". DisplayNominalCode is
	// the one shown to users, "750-1". They are not interchangeable.
	NominalCode        string      `json:"nominal_code,omitempty"`
	DisplayNominalCode string      `json:"display_nominal_code,omitempty"`
	Name               string      `json:"name,omitempty"`
	Total              *Decimal    `json:"total,omitempty"`
	BankAccount        ResourceURL `json:"bank_account,omitempty"`
	User               ResourceURL `json:"user,omitempty"`
	StockItem          ResourceURL `json:"stock_item,omitempty"`
}

// ProfitAndLoss is the profit and loss summary.
//
// Its money values arrive as quoted strings, unlike BalanceSheet and Cashflow
// which send bare numbers. Decimal accepts both, so the inconsistency does not
// reach callers.
type ProfitAndLoss struct {
	From                         Date               `json:"from,omitzero"`
	To                           Date               `json:"to,omitzero"`
	Income                       *Decimal           `json:"income,omitempty"`
	Expenses                     *Decimal           `json:"expenses,omitempty"`
	OperatingProfit              *Decimal           `json:"operating_profit,omitempty"`
	Less                         []ProfitAndLossRow `json:"less,omitempty"`
	RetainedProfit               *Decimal           `json:"retained_profit,omitempty"`
	RetainedProfitBroughtForward *Decimal           `json:"retained_profit_brought_forward,omitempty"`
	RetainedProfitCarriedForward *Decimal           `json:"retained_profit_carried_forward,omitempty"`
}

// ProfitAndLossRow is one deduction line under "less".
type ProfitAndLossRow struct {
	Title string   `json:"title,omitempty"`
	Total *Decimal `json:"total,omitempty"`
}

// BalanceSheet is the balance sheet as at a date.
type BalanceSheet struct {
	AccountingPeriodStartDate Date   `json:"accounting_period_start_date,omitzero"`
	AsAtDate                  Date   `json:"as_at_date,omitzero"`
	Currency                  string `json:"currency,omitempty"`

	CapitalAssets      *BalanceSheetCapitalAssets `json:"capital_assets,omitempty"`
	CurrentAssets      *BalanceSheetSection       `json:"current_assets,omitempty"`
	CurrentLiabilities *BalanceSheetSection       `json:"current_liabilities,omitempty"`
	OwnersEquity       *BalanceSheetEquity        `json:"owners_equity,omitempty"`

	NetCurrentAssets  *Decimal `json:"net_current_assets,omitempty"`
	TotalAssets       *Decimal `json:"total_assets,omitempty"`
	TotalOwnersEquity *Decimal `json:"total_owners_equity,omitempty"`
}

// BalanceSheetSection groups the accounts under one heading.
type BalanceSheetSection struct {
	Accounts []BalanceSheetAccount `json:"accounts,omitempty"`
}

// BalanceSheetAccount is one account line.
type BalanceSheetAccount struct {
	Name        string   `json:"name,omitempty"`
	NominalCode string   `json:"nominal_code,omitempty"`
	TotalDebit  *Decimal `json:"total_debit_value,omitempty"`
}

// BalanceSheetCapitalAssets summarises capital assets.
type BalanceSheetCapitalAssets struct {
	NetBookValue *Decimal `json:"net_book_value,omitempty"`
}

// BalanceSheetEquity summarises owners' equity.
type BalanceSheetEquity struct {
	RetainedProfit *Decimal `json:"retained_profit,omitempty"`
}

// Cashflow is money in and out over a period, bucketed by month.
//
// It reports history only: dates in the future come back as zero rather than
// as a forecast.
type Cashflow struct {
	From     Date             `json:"from,omitzero"`
	To       Date             `json:"to,omitzero"`
	Incoming *CashflowSection `json:"incoming,omitempty"`
	Outgoing *CashflowSection `json:"outgoing,omitempty"`
	Balance  *Decimal         `json:"balance,omitempty"`
}

// CashflowSection is one direction of the cashflow report.
type CashflowSection struct {
	Total  *Decimal        `json:"total,omitempty"`
	Months []CashflowMonth `json:"months,omitempty"`
}

// CashflowMonth is one month's bucket.
type CashflowMonth struct {
	Month int      `json:"month,omitempty"`
	Year  int      `json:"year,omitempty"`
	Total *Decimal `json:"total,omitempty"`
}

// ReportService covers the accounting reports. They are read-only, have no id
// segment, and each has its own response shape, so they are gathered here
// rather than forced through the collection generics.
type ReportService struct {
	client *Client
}

// TrialBalance returns the trial balance summary. Unlike the other reports it
// answers with an array rather than an object.
//
// See https://dev.freeagent.com/docs/trial_balance
func (s *ReportService) TrialBalance(ctx context.Context, opts *ReportOptions) ([]TrialBalanceEntry, *Response, error) {
	var envelope struct {
		Entries []TrialBalanceEntry `json:"trial_balance_summary"`
	}
	resp, err := s.get(ctx, "accounting/trial_balance/summary", opts, &envelope)
	if err != nil {
		return nil, resp, err
	}
	return envelope.Entries, resp, nil
}

// ProfitAndLoss returns the profit and loss summary.
//
// See https://dev.freeagent.com/docs/profit_and_loss
func (s *ReportService) ProfitAndLoss(ctx context.Context, opts *ReportOptions) (*ProfitAndLoss, *Response, error) {
	var envelope struct {
		Report ProfitAndLoss `json:"profit_and_loss_summary"`
	}
	resp, err := s.get(ctx, "accounting/profit_and_loss/summary", opts, &envelope)
	if err != nil {
		return nil, resp, err
	}
	return &envelope.Report, resp, nil
}

// BalanceSheet returns the balance sheet.
//
// See https://dev.freeagent.com/docs/balance_sheet
func (s *ReportService) BalanceSheet(ctx context.Context, opts *ReportOptions) (*BalanceSheet, *Response, error) {
	var envelope struct {
		Report BalanceSheet `json:"balance_sheet"`
	}
	resp, err := s.get(ctx, "accounting/balance_sheet", opts, &envelope)
	if err != nil {
		return nil, resp, err
	}
	return &envelope.Report, resp, nil
}

// Cashflow returns money in and out between two dates. Both are required.
//
// Note the path: this one sits at the API root, not under accounting/.
//
// See https://dev.freeagent.com/docs/cashflow
func (s *ReportService) Cashflow(ctx context.Context, from, to Date) (*Cashflow, *Response, error) {
	if from.IsZero() || to.IsZero() {
		return nil, nil, fmt.Errorf("freeagent: Cashflow requires both a from and a to date")
	}
	opts := &ReportOptions{FromDate: from, ToDate: to}
	var envelope struct {
		Report Cashflow `json:"cashflow"`
	}
	resp, err := s.get(ctx, "cashflow", opts, &envelope)
	if err != nil {
		return nil, resp, err
	}
	return &envelope.Report, resp, nil
}

func (s *ReportService) get(ctx context.Context, path string, opts *ReportOptions, out any) (*Response, error) {
	req, err := s.client.newRequest(ctx, http.MethodGet, path, opts.values(), nil)
	if err != nil {
		return nil, err
	}
	return s.client.do(req, out)
}
