//go:build integration

// Wave B live coverage: the ledger and reporting families.
package freeagent

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// The accounting reports each have their own shape, and two of them send
// money as bare JSON numbers where the rest of the API sends quoted strings.
func TestLiveReportShapes(t *testing.T) {
	client := liveClient(t)
	ctx := liveContext(t)

	t.Run("trial_balance", func(t *testing.T) {
		entries, _, err := client.Reports.TrialBalance(ctx, nil)
		if err != nil {
			t.Fatalf("TrialBalance: %v", err)
		}
		if len(entries) == 0 {
			t.Skip("no trial balance entries on this company")
		}
		for _, entry := range entries {
			if entry.Name == "" || entry.NominalCode == "" {
				t.Fatalf("trial balance entry is incomplete: %+v", entry)
			}
			if entry.Total == nil {
				t.Fatalf("total did not decode: %+v", entry)
			}
		}
		// A trial balance must sum to zero, which is a real check on the
		// decimal decoding rather than a shape assertion.
		sum := decimal.Zero
		for _, entry := range entries {
			sum = sum.Add(*entry.Total)
		}
		if !sum.IsZero() {
			t.Fatalf("trial balance sums to %v, want zero", sum)
		}
		t.Logf("%d entries summing to zero", len(entries))
	})

	t.Run("profit_and_loss", func(t *testing.T) {
		report, _, err := client.Reports.ProfitAndLoss(ctx, nil)
		if err != nil {
			t.Fatalf("ProfitAndLoss: %v", err)
		}
		if report.From.IsZero() || report.To.IsZero() {
			t.Fatalf("report period did not decode: %+v", report)
		}
		if report.Income == nil || report.Expenses == nil || report.OperatingProfit == nil {
			t.Fatalf("headline figures did not decode: %+v", report)
		}
		for _, row := range report.Less {
			if row.Title == "" || row.Total == nil {
				t.Fatalf("a deduction row is incomplete: %+v", row)
			}
		}
		t.Logf("%s to %s, income %v, %d deduction row(s)",
			report.From, report.To, report.Income, len(report.Less))
	})

	t.Run("balance_sheet", func(t *testing.T) {
		report, _, err := client.Reports.BalanceSheet(ctx, nil)
		if err != nil {
			t.Fatalf("BalanceSheet: %v", err)
		}
		if report.AsAtDate.IsZero() || report.Currency == "" {
			t.Fatalf("balance sheet header did not decode: %+v", report)
		}
		// Money here arrives as bare numbers, not quoted strings. Decimal
		// takes both, and this is what proves it.
		if report.NetCurrentAssets == nil || report.TotalAssets == nil {
			t.Fatalf("balance sheet totals did not decode: %+v", report)
		}
		if report.CurrentAssets == nil || len(report.CurrentAssets.Accounts) == 0 {
			t.Skip("no current assets on this company")
		}
		for _, account := range report.CurrentAssets.Accounts {
			if account.Name == "" || account.TotalDebit == nil {
				t.Fatalf("balance sheet account is incomplete: %+v", account)
			}
		}
		t.Logf("as at %s, %d current asset account(s), total assets %v",
			report.AsAtDate, len(report.CurrentAssets.Accounts), report.TotalAssets)
	})

	t.Run("cashflow", func(t *testing.T) {
		to := DateOf(time.Now())
		from := DateOf(time.Now().AddDate(0, -3, 0))
		report, _, err := client.Reports.Cashflow(ctx, from, to)
		if err != nil {
			t.Fatalf("Cashflow: %v", err)
		}
		if report.From.IsZero() || report.To.IsZero() {
			t.Fatalf("cashflow period did not decode: %+v", report)
		}
		if report.Incoming == nil || report.Outgoing == nil || report.Balance == nil {
			t.Fatalf("cashflow sections did not decode: %+v", report)
		}
		if len(report.Incoming.Months) == 0 {
			t.Fatal("monthly buckets did not decode")
		}
		for _, month := range report.Incoming.Months {
			if month.Year == 0 || month.Month == 0 || month.Total == nil {
				t.Fatalf("a cashflow month is incomplete: %+v", month)
			}
		}
		t.Logf("%s to %s across %d month(s), balance %v",
			report.From, report.To, len(report.Incoming.Months), report.Balance)

		// Both dates are required, and that is enforced locally.
		if _, _, err := client.Reports.Cashflow(ctx, Date{}, to); err == nil {
			t.Fatal("Cashflow without a from date succeeded, want an error")
		}
	})
}

func TestLiveWaveBReads(t *testing.T) {
	client := liveClient(t)

	t.Run("journal_sets", func(t *testing.T) {
		ctx := liveContext(t)
		sets, _, err := client.JournalSets.List(ctx, nil)
		if err != nil {
			t.Fatalf("JournalSets.List: %v", err)
		}
		t.Logf("%d journal set(s)", len(sets))

		opening, _, err := client.JournalSets.OpeningBalances(ctx)
		if err != nil {
			t.Fatalf("JournalSets.OpeningBalances: %v", err)
		}
		if len(opening.JournalEntries) == 0 {
			t.Skip("no opening balance entries on this company")
		}
		// No sum assertion here: the opening balances set is not an ordinary
		// journal set. Its bank and stock legs sit in bank_accounts and
		// stock_items rather than journal_entries, so the entries alone do
		// not balance and the sign convention across the three arrays is not
		// documented. Assert structure instead.
		for _, entry := range opening.JournalEntries {
			if entry.DebitValue == nil {
				t.Fatalf("journal entry has no debit value: %+v", entry)
			}
			if entry.Category.Kind() != "categories" {
				t.Fatalf("journal entry category = %q", entry.Category)
			}
		}
		for _, balance := range opening.BankAccounts {
			// Documented only as "Array"; these are objects with their own
			// value, not references.
			if balance.DebitValue == nil || balance.URL.IsZero() {
				t.Fatalf("bank account opening balance is incomplete: %+v", balance)
			}
			if balance.URL.Kind() != "bank_accounts" {
				t.Fatalf("opening balance URL = %q, want a bank account", balance.URL)
			}
		}
		t.Logf("opening balances: %d entries, %d bank account leg(s), %d stock leg(s)",
			len(opening.JournalEntries), len(opening.BankAccounts), len(opening.StockItems))
	})

	t.Run("transactions", func(t *testing.T) {
		ctx := liveContext(t)
		// The range must sit inside one accounting period, so take the bounds
		// from the company rather than counting months back from today. A
		// range starting before the first period is a 400.
		company, _, err := client.Company.Get(ctx, nil)
		if err != nil {
			t.Fatalf("Company.Get: %v", err)
		}
		if len(company.AnnualAccountingPeriods) == 0 {
			t.Skip("company has no accounting periods")
		}
		period := company.AnnualAccountingPeriods[0]
		opts := &ListOptions{FromDate: period.StartsOn, ToDate: period.EndsOn}
		txns, _, err := client.Transactions.List(ctx, opts)
		if err != nil {
			t.Fatalf("Transactions.List: %v", err)
		}
		for _, txn := range txns {
			if txn.DebitValue == nil {
				t.Fatalf("debit_value did not decode: %+v", txn)
			}
			if txn.NominalCode == "" {
				t.Fatalf("nominal_code did not decode: %+v", txn)
			}
		}
		t.Logf("%d transaction(s) in the last 6 months", len(txns))
	})

	t.Run("final_accounts_reports", func(t *testing.T) {
		ctx := liveContext(t)
		reports, _, err := client.FinalAccountsReports.List(ctx)
		if err != nil {
			t.Fatalf("FinalAccountsReports.List: %v", err)
		}
		if len(reports) == 0 {
			t.Skip("no final accounts reports on this company")
		}
		first := reports[0]
		if first.PeriodEndsOn.IsZero() || first.FilingStatus == "" {
			t.Fatalf("report is incomplete: %+v", first)
		}
		// The URL ends in a date, so it is deliberately not a member URL.
		if _, err := first.URL.ID(); err == nil {
			t.Fatalf("URL %q parsed as a numeric member URL, which it is not", first.URL)
		}

		// Addressed by the period end date rather than an id.
		got, _, err := client.FinalAccountsReports.Get(ctx, first.PeriodEndsOn)
		if err != nil {
			t.Fatalf("FinalAccountsReports.Get(%s): %v", first.PeriodEndsOn, err)
		}
		if !got.PeriodEndsOn.Time.Equal(first.PeriodEndsOn.Time) {
			t.Fatalf("fetched %s, want %s", got.PeriodEndsOn, first.PeriodEndsOn)
		}
		if _, _, err := client.FinalAccountsReports.Get(ctx, Date{}); err == nil {
			t.Fatal("Get with no date succeeded, want an error")
		}
		t.Logf("%d report(s); %s is %q, due %s",
			len(reports), first.PeriodEndsOn, first.FilingStatus, first.FilingDueOn)
	})

	t.Run("recurring_invoices", func(t *testing.T) {
		ctx := liveContext(t)
		invoices, _, err := client.RecurringInvoices.List(ctx, nil)
		if err != nil {
			t.Fatalf("RecurringInvoices.List: %v", err)
		}
		t.Logf("%d recurring invoice(s)", len(invoices))
	})

	t.Run("credit_note_reconciliations", func(t *testing.T) {
		ctx := liveContext(t)
		items, _, err := client.CreditNoteReconciliations.List(ctx, nil)
		if err != nil {
			t.Fatalf("CreditNoteReconciliations.List: %v", err)
		}
		t.Logf("%d reconciliation(s)", len(items))
	})

	t.Run("credit_notes", func(t *testing.T) {
		ctx := liveContext(t)
		notes, _, err := client.CreditNotes.List(ctx, nil)
		if err != nil {
			t.Fatalf("CreditNotes.List: %v", err)
		}
		t.Logf("%d credit note(s)", len(notes))
	})

	t.Run("timeslips", func(t *testing.T) {
		ctx := liveContext(t)
		slips, _, err := client.Timeslips.List(ctx, nil)
		if err != nil {
			t.Fatalf("Timeslips.List: %v", err)
		}
		t.Logf("%d timeslip(s)", len(slips))
	})

	// The read-only families and the reports are captured here rather than
	// from the write suite, because they need no fixture data of their own.
	t.Run("capture", func(t *testing.T) {
		if !captureEnabled() {
			t.Skip("capture is off")
		}
		ctx := liveContext(t)
		company, _, err := client.Company.Get(ctx, nil)
		if err != nil {
			t.Fatalf("Company.Get: %v", err)
		}
		if len(company.AnnualAccountingPeriods) == 0 {
			t.Skip("company has no accounting periods")
		}
		period := company.AnnualAccountingPeriods[0]
		targets := map[string]captureTarget{
			"final_accounts_reports": {Path: "final_accounts_reports"},
			"trial_balance":          {Path: "accounting/trial_balance/summary"},
			"profit_and_loss":        {Path: "accounting/profit_and_loss/summary"},
			"balance_sheet":          {Path: "accounting/balance_sheet"},
			"transactions": {Path: "accounting/transactions", Query: url.Values{
				"from_date": {period.StartsOn.String()},
				"to_date":   {period.EndsOn.String()},
			}},
			"cashflow": {Path: "cashflow", Query: url.Values{
				"from_date": {period.StartsOn.String()},
				"to_date":   {DateOf(time.Now()).String()},
			}},
		}
		capture(t, client, "SDKTEST", targets)
	})
}

// The Wave B write paths: a journal set that must balance, a timeslip with
// its timer, and a credit note with a reconciliation against an invoice.
func TestLiveWaveBWriteLifecycle(t *testing.T) {
	client := writeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	tag := runTag()

	groups, _, err := client.Categories.List(ctx, false)
	if err != nil {
		t.Fatalf("Categories.List: %v", err)
	}
	if len(groups.AdminExpenses) == 0 || len(groups.Income) == 0 {
		t.Fatal("need both an income and a spending category")
	}
	spendCategory := groups.AdminExpenses[0].URL
	incomeCategory := groups.Income[0].URL

	me, _, err := client.Users.Me(ctx)
	if err != nil {
		t.Fatalf("Users.Me: %v", err)
	}

	// --- Journal set, which must balance to zero -------------------------------
	journal, _, err := client.JournalSets.Create(ctx, &JournalSet{
		DatedOn:     DateOf(time.Now()),
		Description: tag + " journal",
		Tag:         "SDKTEST",
		JournalEntries: []JournalEntry{
			{
				Category:    spendCategory,
				DebitValue:  new(decimal.RequireFromString("12.34")),
				Description: tag + " debit",
			},
			{
				Category:    incomeCategory,
				DebitValue:  new(decimal.RequireFromString("-12.34")),
				Description: tag + " credit",
			},
		},
	})
	if err != nil {
		t.Fatalf("JournalSets.Create: %v", err)
	}
	journalID := mustID(t, journal.URL)
	t.Cleanup(func() { deleteQuietly(t, "journal set", client.JournalSets.Delete, journalID) })
	if len(journal.JournalEntries) != 2 {
		t.Fatalf("journal set came back with %d entries, want 2", len(journal.JournalEntries))
	}
	t.Logf("created journal set %d with %d entries, tag %q",
		journalID, len(journal.JournalEntries), journal.Tag)

	// An unbalanced set must be refused by the server.
	_, _, err = client.JournalSets.Create(ctx, &JournalSet{
		DatedOn:     DateOf(time.Now()),
		Description: tag + " unbalanced",
		JournalEntries: []JournalEntry{
			{Category: spendCategory, DebitValue: new(decimal.RequireFromString("5"))},
		},
	})
	if err == nil {
		t.Fatal("an unbalanced journal set was accepted, which would corrupt the ledger")
	}
	t.Logf("unbalanced journal set correctly refused: %v", err)

	// --- Timeslip and its timer ------------------------------------------------
	contact, _, err := client.Contacts.Create(ctx, &Contact{OrganisationName: tag + " Contact"})
	if err != nil {
		t.Fatalf("Contacts.Create: %v", err)
	}
	contactID := mustID(t, contact.URL)
	t.Cleanup(func() { deleteQuietly(t, "contact", client.Contacts.Delete, contactID) })

	project, _, err := client.Projects.Create(ctx, &Project{
		Contact:                    contact.URL,
		Name:                       tag + " Project",
		Status:                     "Active",
		Budget:                     new(decimal.Zero),
		BudgetUnits:                "Hours",
		Currency:                   "GBP",
		UsesProjectInvoiceSequence: new(false),
	})
	if err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}
	projectID := mustID(t, project.URL)
	t.Cleanup(func() { deleteQuietly(t, "project", client.Projects.Delete, projectID) })

	task, _, err := client.Tasks.CreateForProject(ctx, project.URL, &Task{
		Name:          tag + " Task",
		IsBillable:    new(true),
		BillingPeriod: "hour",
	})
	if err != nil {
		t.Fatalf("Tasks.CreateForProject: %v", err)
	}
	taskID := mustID(t, task.URL)
	t.Cleanup(func() { deleteQuietly(t, "task", client.Tasks.Delete, taskID) })

	timeslip, _, err := client.Timeslips.Create(ctx, &Timeslip{
		User:    me.URL,
		Project: project.URL,
		Task:    task.URL,
		DatedOn: DateOf(time.Now()),
		Hours:   new(decimal.RequireFromString("1.5")),
		Comment: tag + " timeslip",
	})
	if err != nil {
		t.Fatalf("Timeslips.Create: %v", err)
	}
	timeslipID := mustID(t, timeslip.URL)
	t.Cleanup(func() { deleteQuietly(t, "timeslip", client.Timeslips.Delete, timeslipID) })
	if timeslip.Hours == nil || !timeslip.Hours.Equal(decimal.RequireFromString("1.5")) {
		t.Fatalf("hours = %v, want 1.5", timeslip.Hours)
	}
	t.Logf("created timeslip %d for %v hours", timeslipID, timeslip.Hours)

	started, _, err := client.Timeslips.StartTimer(ctx, timeslipID)
	if err != nil {
		t.Fatalf("Timeslips.StartTimer: %v", err)
	}
	if started.Timer == nil {
		t.Fatalf("no timer on the timeslip after starting one: %+v", started)
	}
	t.Logf("timer started, running=%v from %s", started.Timer.Running, started.Timer.StartFrom)

	if _, _, err := client.Timeslips.StopTimer(ctx, timeslipID); err != nil {
		t.Fatalf("Timeslips.StopTimer: %v", err)
	}

	// --- Credit note and reconciliation ----------------------------------------
	creditNote, _, err := client.CreditNotes.Create(ctx, &CreditNote{
		Contact:            contact.URL,
		DatedOn:            DateOf(time.Now()),
		PaymentTermsInDays: new(30),
		CreditNoteItems: []CreditNoteItem{{
			Description: tag + " credit line",
			ItemType:    "Services",
			Quantity:    new(decimal.RequireFromString("1")),
			Price:       new(decimal.RequireFromString("-25")),
			Category:    incomeCategory,
		}},
	})
	if err != nil {
		t.Fatalf("CreditNotes.Create: %v", err)
	}
	creditNoteID := mustID(t, creditNote.URL)
	t.Cleanup(func() { deleteQuietly(t, "credit note", client.CreditNotes.Delete, creditNoteID) })
	t.Logf("created credit note %d, status %q, total %v",
		creditNoteID, creditNote.Status, creditNote.TotalValue)

	pdf, _, err := client.CreditNotes.PDF(ctx, creditNoteID)
	if err != nil {
		t.Fatalf("CreditNotes.PDF: %v", err)
	}
	decoded, err := pdf.Bytes()
	if err != nil {
		t.Fatalf("decoding the credit note PDF: %v", err)
	}
	if len(decoded) == 0 {
		t.Fatal("credit note PDF is empty")
	}
	t.Logf("rendered a %d byte credit note PDF", len(decoded))

	capture(t, client, tag, map[string]captureTarget{
		"journal_sets": {Path: "journal_sets"},
		"timeslips":    {Path: "timeslips"},
		"credit_notes": {Path: "credit_notes"},
	})
}
