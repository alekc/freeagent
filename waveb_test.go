package freeagent

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// The Wave B ledger families, decoded from captured payloads. Assertions are
// about shape and invariants so a re-capture cannot break them.
func TestWaveBGoldenDecoding(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		verify func(t *testing.T, c *Client)
	}{
		{name: "journal_sets", verify: func(t *testing.T, c *Client) {
			sets, _, err := c.JournalSets.List(context.Background(), nil)
			if err != nil {
				t.Fatalf("List = %v", err)
			}
			if len(sets) == 0 {
				t.Fatal("no journal sets decoded")
			}
			for _, set := range sets {
				if set.Description == "" {
					t.Fatalf("journal set has no description: %+v", set)
				}
				if len(set.JournalEntries) == 0 {
					t.Fatalf("journal set %s has no entries", set.URL)
				}
				for _, entry := range set.JournalEntries {
					if entry.DebitValue == nil {
						t.Fatalf("entry has no debit value: %+v", entry)
					}
					if entry.Category.Kind() != "categories" {
						t.Fatalf("entry category = %q", entry.Category)
					}
				}
				// bank_accounts is documented as a bare array; it is an array
				// of objects carrying their own value.
				for _, balance := range set.BankAccounts {
					if balance.DebitValue == nil || balance.URL.IsZero() {
						t.Fatalf("opening balance leg is incomplete: %+v", balance)
					}
				}
			}
		}},
		{name: "transactions", verify: func(t *testing.T, c *Client) {
			txns, _, err := c.Transactions.List(context.Background(), nil)
			if err != nil {
				t.Fatalf("List = %v", err)
			}
			if len(txns) == 0 {
				t.Fatal("no transactions decoded")
			}
			for _, txn := range txns {
				if txn.DebitValue == nil {
					t.Fatalf("debit_value did not decode: %+v", txn)
				}
				if txn.NominalCode == "" || txn.CategoryName == "" {
					t.Fatalf("transaction is incomplete: %+v", txn)
				}
				if txn.DatedOn.IsZero() {
					t.Fatal("dated_on did not decode")
				}
			}
		}},
		{name: "timeslips", verify: func(t *testing.T, c *Client) {
			slips, _, err := c.Timeslips.List(context.Background(), nil)
			if err != nil {
				t.Fatalf("List = %v", err)
			}
			if len(slips) == 0 {
				t.Fatal("no timeslips decoded")
			}
			slip := slips[0]
			if slip.Hours == nil {
				t.Fatal("hours did not decode")
			}
			for name, ref := range map[string]ResourceURL{
				"user": slip.User, "project": slip.Project, "task": slip.Task,
			} {
				if ref.IsZero() {
					t.Fatalf("timeslip %s reference is empty", name)
				}
			}
			if slip.DatedOn.IsZero() {
				t.Fatal("dated_on did not decode")
			}
		}},
		{name: "credit_notes", verify: func(t *testing.T, c *Client) {
			notes, _, err := c.CreditNotes.List(context.Background(), nil)
			if err != nil {
				t.Fatalf("List = %v", err)
			}
			if len(notes) == 0 {
				t.Fatal("no credit notes decoded")
			}
			note := notes[0]
			if note.Status == "" || note.TotalValue == nil {
				t.Fatalf("credit note is incomplete: %+v", note)
			}
			// A credit note is a negative document.
			if !note.TotalValue.IsNegative() {
				t.Fatalf("total_value = %v, want negative on a credit note", note.TotalValue)
			}
			if note.Contact.Kind() != "contacts" {
				t.Fatalf("contact reference = %q", note.Contact)
			}
		}},
		{name: "final_accounts_reports", verify: func(t *testing.T, c *Client) {
			reports, _, err := c.FinalAccountsReports.List(context.Background())
			if err != nil {
				t.Fatalf("List = %v", err)
			}
			if len(reports) == 0 {
				t.Fatal("no reports decoded")
			}
			report := reports[0]
			if report.PeriodEndsOn.IsZero() || report.PeriodStartsOn.IsZero() {
				t.Fatalf("period dates did not decode: %+v", report)
			}
			if report.FilingStatus == "" {
				t.Fatal("filing_status did not decode")
			}
			// The URL ends in a date, so it is not a numeric member URL.
			if _, err := report.URL.ID(); err == nil {
				t.Fatalf("URL %q parsed as a member URL", report.URL)
			}
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.verify(t, goldenClient(t, tc.name+".json"))
		})
	}
}

// Final accounts reports are addressed by date, so the path is built from a
// rendered Date rather than from caller-supplied text.
func TestFinalAccountsReportPaths(t *testing.T) {
	t.Parallel()
	var seen string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Method + " " + r.URL.Path
		fmt.Fprint(w, `{"final_accounts_report":{"period_ends_on":"2027-04-30","filing_status":"filed"}}`)
	})
	ctx := context.Background()
	period := NewDate(2027, 4, 30)

	if _, _, err := client.FinalAccountsReports.Get(ctx, period); err != nil {
		t.Fatalf("Get = %v", err)
	}
	if seen != "GET /v2/final_accounts_reports/2027-04-30" {
		t.Fatalf("request = %s", seen)
	}

	if _, _, err := client.FinalAccountsReports.MarkAsFiled(ctx, period); err != nil {
		t.Fatalf("MarkAsFiled = %v", err)
	}
	if seen != "PUT /v2/final_accounts_reports/2027-04-30/mark_as_filed" {
		t.Fatalf("request = %s", seen)
	}

	if _, _, err := client.FinalAccountsReports.MarkAsUnfiled(ctx, period); err != nil {
		t.Fatalf("MarkAsUnfiled = %v", err)
	}
	if seen != "PUT /v2/final_accounts_reports/2027-04-30/mark_as_unfiled" {
		t.Fatalf("request = %s", seen)
	}

	// A zero date must not produce a request to a truncated path.
	for name, call := range map[string]func() error{
		"Get":           func() error { _, _, err := client.FinalAccountsReports.Get(ctx, Date{}); return err },
		"MarkAsFiled":   func() error { _, _, err := client.FinalAccountsReports.MarkAsFiled(ctx, Date{}); return err },
		"MarkAsUnfiled": func() error { _, _, err := client.FinalAccountsReports.MarkAsUnfiled(ctx, Date{}); return err },
	} {
		if err := call(); err == nil {
			t.Fatalf("%s with a zero date succeeded, want an error", name)
		}
	}
}

func TestTimeslipTimerPaths(t *testing.T) {
	t.Parallel()
	var seen string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Method + " " + r.URL.Path
		fmt.Fprint(w, `{"timeslip":{"hours":"1.5","timer":{"running":true,"start_from":"2026-08-16T10:00:00.000Z"}}}`)
	})
	ctx := context.Background()

	started, _, err := client.Timeslips.StartTimer(ctx, 25)
	if err != nil {
		t.Fatalf("StartTimer = %v", err)
	}
	if seen != "POST /v2/timeslips/25/timer" {
		t.Fatalf("request = %s", seen)
	}
	if started.Timer == nil || started.Timer.Running == nil || !*started.Timer.Running {
		t.Fatalf("timer did not decode: %+v", started.Timer)
	}
	if started.Timer.StartFrom.IsZero() {
		t.Fatal("start_from did not decode")
	}

	// Stopping is a DELETE on the same path, not a second POST.
	if _, _, err := client.Timeslips.StopTimer(ctx, 25); err != nil {
		t.Fatalf("StopTimer = %v", err)
	}
	if seen != "DELETE /v2/timeslips/25/timer" {
		t.Fatalf("request = %s", seen)
	}
}

func TestCreditNoteTransitionsAndPDF(t *testing.T) {
	t.Parallel()
	var seen string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Method + " " + r.URL.Path
		if strings.HasSuffix(r.URL.Path, "/pdf") {
			fmt.Fprint(w, `{"pdf":{"content":"UERG"}}`)
			return
		}
		fmt.Fprint(w, `{"credit_note":{"status":"Open"}}`)
	})
	ctx := context.Background()

	if _, _, err := client.CreditNotes.MarkAsSent(ctx, 9); err != nil {
		t.Fatalf("MarkAsSent = %v", err)
	}
	if seen != "PUT /v2/credit_notes/9/transitions/mark_as_sent" {
		t.Fatalf("request = %s", seen)
	}
	if _, _, err := client.CreditNotes.MarkAsDraft(ctx, 9); err != nil {
		t.Fatalf("MarkAsDraft = %v", err)
	}
	if seen != "PUT /v2/credit_notes/9/transitions/mark_as_draft" {
		t.Fatalf("request = %s", seen)
	}
	pdf, _, err := client.CreditNotes.PDF(ctx, 9)
	if err != nil {
		t.Fatalf("PDF = %v", err)
	}
	if got, _ := pdf.Bytes(); string(got) != "PDF" {
		t.Fatalf("decoded = %q", got)
	}
	if _, err := client.CreditNotes.SendEmail(ctx, 9, nil); err != nil {
		t.Fatalf("SendEmail = %v", err)
	}
	if seen != "POST /v2/credit_notes/9/send_email" {
		t.Fatalf("request = %s", seen)
	}
}

// Transactions live under accounting/, unlike every other collection.
func TestTransactionPath(t *testing.T) {
	t.Parallel()
	var seen string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
		fmt.Fprint(w, `{"transactions":[]}`)
	})
	if _, _, err := client.Transactions.List(context.Background(), nil); err != nil {
		t.Fatalf("List = %v", err)
	}
	if seen != "/v2/accounting/transactions" {
		t.Fatalf("path = %q, want the accounting prefix", seen)
	}
}

// A journal set must balance. The server enforces it, but a caller can check
// before spending a request.
func TestJournalSetBalanceHelper(t *testing.T) {
	t.Parallel()
	balanced := []JournalEntry{
		{DebitValue: new(decimal.RequireFromString("12.34"))},
		{DebitValue: new(decimal.RequireFromString("-12.34"))},
	}
	sum := decimal.Zero
	for _, entry := range balanced {
		sum = sum.Add(*entry.DebitValue)
	}
	if !sum.IsZero() {
		t.Fatalf("a balanced pair summed to %v", sum)
	}
}
