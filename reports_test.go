package freeagent

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// The reports are inconsistent about money: trial balance and profit and loss
// send quoted strings, balance sheet and cashflow send bare numbers. Decimal
// takes both, and these fixtures are the evidence.
func TestWaveBReportDecoding(t *testing.T) {
	t.Parallel()

	t.Run("trial_balance", func(t *testing.T) {
		t.Parallel()
		client := goldenClient(t, "trial_balance.json")
		entries, _, err := client.Reports.TrialBalance(context.Background(), nil)
		if err != nil {
			t.Fatalf("TrialBalance = %v", err)
		}
		if len(entries) == 0 {
			t.Fatal("no entries decoded")
		}
		sum := decimal.Zero
		for _, entry := range entries {
			if entry.Total == nil || entry.Name == "" {
				t.Fatalf("entry is incomplete: %+v", entry)
			}
			sum = sum.Add(*entry.Total)
		}
		// A trial balance that does not sum to zero means a decimal was lost.
		if !sum.IsZero() {
			t.Fatalf("entries sum to %v, want zero", sum)
		}
		// The internal and display codes differ on sub-accounts and are not
		// interchangeable.
		for _, entry := range entries {
			if entry.BankAccount.IsZero() {
				continue
			}
			if entry.NominalCode == entry.DisplayNominalCode {
				continue
			}
			if !strings.HasPrefix(entry.NominalCode, "750-") {
				t.Fatalf("unexpected sub-account code %q", entry.NominalCode)
			}
		}
	})

	t.Run("profit_and_loss", func(t *testing.T) {
		t.Parallel()
		client := goldenClient(t, "profit_and_loss.json")
		report, _, err := client.Reports.ProfitAndLoss(context.Background(), nil)
		if err != nil {
			t.Fatalf("ProfitAndLoss = %v", err)
		}
		if report.From.IsZero() || report.To.IsZero() {
			t.Fatalf("period did not decode: %+v", report)
		}
		if report.Income == nil || report.OperatingProfit == nil {
			t.Fatalf("headline figures did not decode: %+v", report)
		}
		if len(report.Less) == 0 {
			t.Fatal("the less rows did not decode")
		}
		for _, row := range report.Less {
			if row.Title == "" || row.Total == nil {
				t.Fatalf("deduction row is incomplete: %+v", row)
			}
		}
	})

	t.Run("balance_sheet", func(t *testing.T) {
		t.Parallel()
		client := goldenClient(t, "balance_sheet.json")
		report, _, err := client.Reports.BalanceSheet(context.Background(), nil)
		if err != nil {
			t.Fatalf("BalanceSheet = %v", err)
		}
		if report.AsAtDate.IsZero() || report.Currency == "" {
			t.Fatalf("header did not decode: %+v", report)
		}
		// These arrive as bare JSON numbers rather than quoted strings.
		if report.NetCurrentAssets == nil || report.TotalAssets == nil ||
			report.TotalOwnersEquity == nil {
			t.Fatalf("totals did not decode: %+v", report)
		}
		if report.CurrentAssets == nil || len(report.CurrentAssets.Accounts) == 0 {
			t.Fatal("current assets did not decode")
		}
		account := report.CurrentAssets.Accounts[0]
		if account.Name == "" || account.NominalCode == "" || account.TotalDebit == nil {
			t.Fatalf("account line is incomplete: %+v", account)
		}
		if report.OwnersEquity == nil || report.OwnersEquity.RetainedProfit == nil {
			t.Fatal("owners equity did not decode")
		}
		if report.CapitalAssets == nil || report.CapitalAssets.NetBookValue == nil {
			t.Fatal("capital assets did not decode")
		}
	})

	t.Run("cashflow", func(t *testing.T) {
		t.Parallel()
		client := goldenClient(t, "cashflow.json")
		report, _, err := client.Reports.Cashflow(context.Background(),
			NewDate(2026, 5, 1), NewDate(2026, 8, 16))
		if err != nil {
			t.Fatalf("Cashflow = %v", err)
		}
		if report.Incoming == nil || report.Outgoing == nil || report.Balance == nil {
			t.Fatalf("sections did not decode: %+v", report)
		}
		if len(report.Incoming.Months) == 0 {
			t.Fatal("monthly buckets did not decode")
		}
		for _, month := range report.Incoming.Months {
			if month.Year == 0 || month.Month == 0 || month.Total == nil {
				t.Fatalf("month bucket is incomplete: %+v", month)
			}
		}
	})
}

// Cashflow sits at the API root while its siblings are under accounting/, so
// the path is worth pinning.
func TestReportPaths(t *testing.T) {
	t.Parallel()
	var seen string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path + "?" + r.URL.RawQuery
		_, _ = w.Write([]byte(`{}`))
	})
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
		want string
	}{
		{
			name: "trial balance",
			call: func() error { _, _, err := client.Reports.TrialBalance(ctx, nil); return err },
			want: "/v2/accounting/trial_balance/summary?",
		},
		{
			name: "profit and loss",
			call: func() error { _, _, err := client.Reports.ProfitAndLoss(ctx, nil); return err },
			want: "/v2/accounting/profit_and_loss/summary?",
		},
		{
			name: "balance sheet",
			call: func() error { _, _, err := client.Reports.BalanceSheet(ctx, nil); return err },
			want: "/v2/accounting/balance_sheet?",
		},
		{
			name: "cashflow",
			call: func() error {
				_, _, err := client.Reports.Cashflow(ctx, NewDate(2026, 1, 1), NewDate(2026, 3, 31))
				return err
			},
			want: "/v2/cashflow?from_date=2026-01-01&to_date=2026-03-31",
		},
	}
	for _, tc := range tests {
		if err := tc.call(); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if seen != tc.want {
			t.Fatalf("%s requested %q, want %q", tc.name, seen, tc.want)
		}
	}
}

func TestReportOptionsValues(t *testing.T) {
	t.Parallel()
	opts := &ReportOptions{
		FromDate:         NewDate(2026, 1, 1),
		ToDate:           NewDate(2026, 12, 31),
		AccountingPeriod: "current",
		Extra:            url.Values{"accounting_period": {"previous"}},
	}
	got := opts.values()
	if got.Get("from_date") != "2026-01-01" || got.Get("to_date") != "2026-12-31" {
		t.Fatalf("dates = %v", got)
	}
	// Extra wins, as it does on ListOptions.
	if got.Get("accounting_period") != "previous" {
		t.Fatalf("accounting_period = %q, want Extra to win", got.Get("accounting_period"))
	}

	var nilOpts *ReportOptions
	if len(nilOpts.values()) != 0 {
		t.Fatal("nil options produced parameters")
	}
}

// Both dates are required and that is enforced before any request.
func TestCashflowRequiresBothDates(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be sent without both dates")
	})
	ctx := context.Background()
	if _, _, err := client.Reports.Cashflow(ctx, Date{}, NewDate(2026, 3, 31)); err == nil {
		t.Fatal("missing from date was accepted")
	}
	if _, _, err := client.Reports.Cashflow(ctx, NewDate(2026, 1, 1), Date{}); err == nil {
		t.Fatal("missing to date was accepted")
	}
}
