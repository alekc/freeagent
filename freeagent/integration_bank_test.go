//go:build integration

package freeagent

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// Bank accounts are the last untested corner of Wave A: a statement upload is
// the only way transactions enter the system, and both transactions and
// explanations refuse to list without a bank_account filter.
func TestLiveBankStatementAndExplanations(t *testing.T) {
	client := writeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	tag := runTag()

	groups, _, err := client.Categories.List(ctx, false)
	if err != nil {
		t.Fatalf("Categories.List: %v", err)
	}
	if len(groups.AdminExpenses) == 0 {
		t.Fatal("no admin expense categories to explain against")
	}
	spendCategory := groups.AdminExpenses[0].URL

	account, _, err := client.BankAccounts.Create(ctx, &BankAccount{
		Type:           BankAccountTypeStandard,
		Name:           tag + " Account",
		BankName:       "Example Bank",
		Currency:       "GBP",
		OpeningBalance: new(decimal.Zero),
	})
	if err != nil {
		t.Fatalf("BankAccounts.Create: %v", err)
	}
	accountID := mustID(t, account.URL)
	t.Cleanup(func() { deleteQuietly(t, "bank account", client.BankAccounts.Delete, accountID) })
	t.Logf("created bank account %d (%s)", accountID, account.Type)

	// --- Statement upload -----------------------------------------------------
	statement := []StatementLine{
		{
			DatedOn:     DateOf(time.Now().AddDate(0, 0, -2)),
			Amount:      decimal.RequireFromString("-42.50"),
			Description: tag + " card payment",
			FitID:       tag + "-1",
		},
		{
			DatedOn:     DateOf(time.Now().AddDate(0, 0, -1)),
			Amount:      decimal.RequireFromString("1000.00"),
			Description: tag + " customer receipt",
			FitID:       tag + "-2",
		},
	}
	if _, err := client.BankTransactions.UploadStatement(ctx, account.URL, statement); err != nil {
		t.Fatalf("BankTransactions.UploadStatement: %v", err)
	}

	// Import is asynchronous. A read straight after the 200 usually comes back
	// empty; the delay has been seen anywhere from under a second to most of a
	// minute, so poll instead of asserting immediately.
	txns := waitForTransactions(t, ctx, client, account.URL, len(statement))
	for _, txn := range txns {
		if txn.BankAccount != account.URL {
			t.Fatalf("transaction is on %q, want %q", txn.BankAccount, account.URL)
		}
		if txn.Amount == nil || txn.Amount.IsZero() {
			t.Fatalf("transaction has no amount: %+v", txn)
		}
		if txn.UnexplainedAmount == nil {
			t.Fatalf("unexplained_amount did not decode: %+v", txn)
		}
	}
	t.Logf("uploaded and read back %d transaction(s)", len(txns))

	// Pick the money-out line to explain.
	var outgoing BankTransaction
	for _, txn := range txns {
		if txn.Amount.IsNegative() {
			outgoing = txn
			break
		}
	}
	if outgoing.URL.IsZero() {
		t.Fatal("no money-out transaction to explain")
	}

	// --- Explanation ----------------------------------------------------------
	explanation, _, err := client.BankTransactionExplanations.Create(ctx, &BankTransactionExplanation{
		BankTransaction: outgoing.URL,
		BankAccount:     account.URL,
		DatedOn:         outgoing.DatedOn,
		GrossValue:      outgoing.Amount,
		Category:        spendCategory,
		Description:     tag + " explained",
	})
	if err != nil {
		t.Fatalf("BankTransactionExplanations.Create: %v", err)
	}
	explanationID := mustID(t, explanation.URL)
	t.Cleanup(func() {
		deleteQuietly(t, "explanation", client.BankTransactionExplanations.Delete, explanationID)
	})
	if explanation.IsMoneyOut == nil || !*explanation.IsMoneyOut {
		t.Fatalf("is_money_out = %v, want true for a negative amount", explanation.IsMoneyOut)
	}
	t.Logf("created explanation %d, type %q", explanationID, explanation.Type)

	// The required bank_account filter again, on a different endpoint.
	explanations, _, err := client.BankTransactionExplanations.ListForAccount(ctx, account.URL, nil)
	if err != nil {
		t.Fatalf("BankTransactionExplanations.ListForAccount: %v", err)
	}
	if len(explanations) == 0 {
		t.Fatal("the explanation just created is not listed against its account")
	}

	// Explaining should have consumed the unexplained amount on that line.
	after, _, err := client.BankTransactions.GetURL(ctx, outgoing.URL)
	if err != nil {
		t.Fatalf("BankTransactions.GetURL: %v", err)
	}
	if after.UnexplainedAmount != nil && !after.UnexplainedAmount.IsZero() {
		t.Logf("note: unexplained amount is still %v after explaining", after.UnexplainedAmount)
	}
	if len(after.BankTransactionExplanations) == 0 {
		t.Log("note: the nested explanations array came back empty on a single fetch")
	}

	// These two need the bank_account filter, so they can only be captured
	// from here, where an account with data exists.
	scope := url.Values{"bank_account": {account.URL.String()}}
	capture(t, client, tag, map[string]captureTarget{
		"bank_accounts":                 {Path: "bank_accounts"},
		"bank_transactions":             {Path: "bank_transactions", Query: scope},
		"bank_transaction_explanations": {Path: "bank_transaction_explanations", Query: scope},
	})
}

// Categories are the irregular family: nominal-code addressing and a grouped
// envelope on every response including writes.
func TestLiveCategoryLifecycle(t *testing.T) {
	client := writeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	tag := runTag()

	// A create needs a free nominal code inside the group's range and a
	// tax_reporting_name drawn from a fixed list, neither of which the docs
	// mention. Both are borrowed from the seeded chart of accounts.
	groups, _, err := client.Categories.List(ctx, false)
	if err != nil {
		t.Fatalf("Categories.List: %v", err)
	}
	used := map[string]bool{}
	for _, category := range groups.Flatten() {
		used[category.NominalCode] = true
	}
	code := ""
	for candidate := 200; candidate <= 399; candidate++ {
		if next := strconv.Itoa(candidate); !used[next] {
			code = next
			break
		}
	}
	if code == "" {
		t.Skip("no free nominal code in the admin expenses range")
	}

	// The set of tax_reporting_name values accepted on create is narrower
	// than the set already in use, and is not published, so try the existing
	// ones until one is accepted. A rejected create does not consume the
	// nominal code, so the same one can be reused each attempt.
	var candidates []string
	seen := map[string]bool{}
	for _, category := range groups.AdminExpenses {
		if name := category.TaxReportingName; name != "" && !seen[name] {
			seen[name] = true
			candidates = append(candidates, name)
		}
	}
	if len(candidates) == 0 {
		t.Skip("no tax reporting names to borrow from the chart of accounts")
	}

	var created *Category
	var lastErr error
	for _, name := range candidates {
		created, _, lastErr = client.Categories.Create(ctx, &Category{
			Description:     tag + " Category",
			AllowableForTax: new(true),
			// Required on create and absent from the documented attribute
			// list, which describes group_description instead. Omitting it
			// is a 422.
			CategoryGroup:    "admin_expenses",
			NominalCode:      code,
			TaxReportingName: name,
		})
		if lastErr == nil {
			t.Logf("created with tax_reporting_name %q after %d rejected candidate(s)",
				name, indexOfString(candidates, name))
			break
		}
		if !errors.Is(lastErr, ErrValidation) {
			t.Fatalf("Categories.Create: %v", lastErr)
		}
	}
	if lastErr != nil {
		t.Fatalf("Categories.Create: every candidate tax reporting name was rejected, last: %v", lastErr)
	}
	if created.NominalCode == "" {
		t.Fatalf("created category has no nominal code: %+v", created)
	}
	if created.Group == "" {
		t.Fatalf("created category has no group resolved: %+v", created)
	}
	code = created.NominalCode
	t.Cleanup(func() {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelCleanup()
		if _, err := client.Categories.Delete(cleanupCtx, code); err != nil {
			t.Logf("cleanup: could not delete category %s: %v", code, err)
			return
		}
		t.Logf("cleanup: deleted category %s", code)
	})
	t.Logf("created category %s in group %s", code, created.Group)

	// Read it back by code, which is the path the generic collection cannot do.
	fetched, _, err := client.Categories.Get(ctx, code)
	if err != nil {
		t.Fatalf("Categories.Get(%s): %v", code, err)
	}
	if fetched.NominalCode != code {
		t.Fatalf("nominal code = %q, want %q", fetched.NominalCode, code)
	}

	renamed := tag + " Category Renamed"
	updated, _, err := client.Categories.Update(ctx, code, &Category{Description: renamed})
	if err != nil {
		t.Fatalf("Categories.Update: %v", err)
	}
	if updated.Description != renamed {
		t.Fatalf("description = %q, want %q", updated.Description, renamed)
	}
}

// Read-only endpoints that hang off a resource rather than being collections.
func TestLiveSubResources(t *testing.T) {
	client := liveClient(t)
	ctx := liveContext(t)

	categories, _, err := client.Company.BusinessCategories(ctx)
	if err != nil {
		t.Fatalf("Company.BusinessCategories: %v", err)
	}
	if len(categories) == 0 {
		t.Fatal("no business categories returned")
	}
	t.Logf("%d business categories, first %q", len(categories), categories[0])

	timeline, _, err := client.Company.TaxTimeline(ctx)
	if err != nil {
		t.Fatalf("Company.TaxTimeline: %v", err)
	}
	t.Logf("%d tax timeline item(s)", len(timeline))

	settings, _, err := client.Expenses.MileageSettings(ctx)
	if err != nil {
		t.Fatalf("Expenses.MileageSettings: %v", err)
	}
	if len(settings.MileageRates) == 0 || len(settings.EngineTypeAndSizeOptions) == 0 {
		t.Fatalf("mileage settings came back empty: %+v", settings)
	}
	latest := settings.MileageRates[len(settings.MileageRates)-1]
	if rate, ok := latest.RatesFor("Car"); ok {
		t.Logf("mileage: %d rate period(s), latest car basic rate %v", len(settings.MileageRates), rate.BasicRate)
	} else {
		t.Fatalf("no Car rate in the latest period: %+v", latest)
	}
}

// indexOfString reports the position of value in list, or -1.
func indexOfString(list []string, value string) int {
	for i, item := range list {
		if item == value {
			return i
		}
	}
	return -1
}

// waitForTransactions polls until the expected number of statement lines are
// queryable. Statement import is asynchronous, so the alternative is a test
// that passes or fails depending on how busy the server is.
func waitForTransactions(t *testing.T, ctx context.Context, client *Client, account ResourceURL, want int) []BankTransaction {
	t.Helper()
	const (
		timeout = 90 * time.Second
		every   = 2 * time.Second
	)
	deadline := time.Now().Add(timeout)
	for attempt := 1; ; attempt++ {
		txns, _, err := client.BankTransactions.ListForAccount(ctx, account, nil)
		if err != nil {
			t.Fatalf("BankTransactions.ListForAccount: %v", err)
		}
		if len(txns) >= want {
			t.Logf("statement import settled after %d poll(s)", attempt)
			return txns
		}
		if time.Now().After(deadline) {
			t.Fatalf("uploaded %d lines but only %d appeared within %s", want, len(txns), timeout)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context finished while waiting for the statement import: %v", ctx.Err())
		case <-time.After(every):
		}
	}
}
