//go:build integration

// Write-path coverage against a live sandbox company. Everything created here
// is deleted again by a cleanup registered the moment it exists, so a failure
// part way through still tidies up.
//
// This file refuses to run against production outright rather than skipping,
// because the difference between the two is real accounting records.
package freeagent

import (
	"context"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// writeClient is liveClient with a hard production guard. Reads against
// production are merely wasteful; writes are not recoverable.
func writeClient(t *testing.T) *Client {
	t.Helper()
	if name := os.Getenv("FREEAGENT_ENV"); name != "" && name != Sandbox.Name {
		t.Fatalf("the write suite runs against the sandbox only, got FREEAGENT_ENV=%q", name)
	}
	return liveClient(t)
}

// runTag makes every record identifiable and unique, so a rerun cannot
// collide with leftovers from a run that failed to clean up.
func runTag() string {
	return "SDKTEST-" + time.Now().UTC().Format("20060102-150405")
}

// TestLiveWriteLifecycle walks the dependency order: a contact, then things
// that reference it, then things that reference those. Cleanup is LIFO, so
// registering as we go unwinds in the right order.
func TestLiveWriteLifecycle(t *testing.T) {
	client := writeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	tag := runTag()

	// A spending category to hang bills and expenses off. Every account has a
	// seeded chart of accounts, so this is always available.
	groups, _, err := client.Categories.List(ctx, false)
	if err != nil {
		t.Fatalf("Categories.List: %v", err)
	}
	if len(groups.AdminExpenses) == 0 {
		t.Fatal("no admin expense categories to test against")
	}
	spendCategory := groups.AdminExpenses[0].URL

	me, _, err := client.Users.Me(ctx)
	if err != nil {
		t.Fatalf("Users.Me: %v", err)
	}

	// --- Contact -------------------------------------------------------------
	contact, _, err := client.Contacts.Create(ctx, &Contact{
		OrganisationName: tag + " Contact",
		Email:            "sdk-test@example.com",
	})
	if err != nil {
		t.Fatalf("Contacts.Create: %v", err)
	}
	contactID := mustID(t, contact.URL)
	t.Cleanup(func() { deleteQuietly(t, "contact", client.Contacts.Delete, contactID) })
	if contact.OrganisationName != tag+" Contact" {
		t.Fatalf("created contact came back as %+v", contact)
	}
	t.Logf("created contact %d", contactID)

	// Update it, to prove PUT round-trips through the same envelope.
	renamed := tag + " Contact Renamed"
	updated, _, err := client.Contacts.Update(ctx, contactID, &Contact{OrganisationName: renamed})
	if err != nil {
		t.Fatalf("Contacts.Update: %v", err)
	}
	if updated.OrganisationName != renamed {
		t.Fatalf("update did not take: %q", updated.OrganisationName)
	}

	// --- Project ---------------------------------------------------------------
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
	t.Logf("created project %d", projectID)

	// --- Task, created under the project via the query string --------------------
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
	if task.Project != project.URL {
		t.Fatalf("task project = %q, want %q", task.Project, project.URL)
	}
	t.Logf("created task %d under the right project", taskID)

	// --- Invoice with nested items ------------------------------------------------
	invoice, _, err := client.Invoices.Create(ctx, &Invoice{
		Contact:            contact.URL,
		Project:            project.URL,
		DatedOn:            DateOf(time.Now()),
		PaymentTermsInDays: new(30),
		InvoiceItems: []InvoiceItem{{
			Description: tag + " line",
			ItemType:    "Hours",
			Quantity:    new(decimal.RequireFromString("2")),
			Price:       new(decimal.RequireFromString("125.5")),
		}},
	})
	if err != nil {
		t.Fatalf("Invoices.Create: %v", err)
	}
	invoiceID := mustID(t, invoice.URL)
	t.Cleanup(func() { deleteQuietly(t, "invoice", client.Invoices.Delete, invoiceID) })
	t.Logf("created invoice %d, status %q, total %v", invoiceID, invoice.Status, invoice.TotalValue)

	// The server computes the total from the items, so a non-zero total proves
	// the nested write landed rather than being silently dropped.
	if invoice.TotalValue == nil || invoice.TotalValue.IsZero() {
		t.Fatalf("invoice total is %v, want the item value to have been applied", invoice.TotalValue)
	}

	// --- Invoice transitions ----------------------------------------------------
	sent, _, err := client.Invoices.MarkAsSent(ctx, invoiceID)
	if err != nil {
		t.Fatalf("Invoices.MarkAsSent: %v", err)
	}
	if strings.EqualFold(sent.Status, "Draft") {
		t.Fatalf("status is still %q after mark_as_sent", sent.Status)
	}
	t.Logf("mark_as_sent moved the invoice to %q", sent.Status)

	// Back to draft, which is also what makes it deletable again.
	draft, _, err := client.Invoices.MarkAsDraft(ctx, invoiceID)
	if err != nil {
		t.Fatalf("Invoices.MarkAsDraft: %v", err)
	}
	if !strings.EqualFold(draft.Status, "Draft") {
		t.Fatalf("status is %q after mark_as_draft", draft.Status)
	}

	// --- Invoice PDF --------------------------------------------------------------
	pdf, _, err := client.Invoices.PDF(ctx, invoiceID)
	if err != nil {
		t.Fatalf("Invoices.PDF: %v", err)
	}
	decoded, err := pdf.Bytes()
	if err != nil {
		t.Fatalf("decoding the PDF: %v", err)
	}
	if !strings.HasPrefix(string(decoded), "%PDF") {
		t.Fatalf("decoded %d bytes but they do not start with %%PDF", len(decoded))
	}
	t.Logf("rendered a %d byte PDF", len(decoded))

	// --- Estimate --------------------------------------------------------------
	estimate, _, err := client.Estimates.Create(ctx, &Estimate{
		Contact:      contact.URL,
		Reference:    tag,
		EstimateType: "Estimate",
		// Required on create: omitting it is a 422, unlike invoices.
		Status:   "Draft",
		DatedOn:  DateOf(time.Now()),
		Currency: "GBP",
		EstimateItems: []EstimateItem{{
			Position:    new(1),
			Description: tag + " estimate line",
			ItemType:    "Services",
			Quantity:    new(decimal.RequireFromString("1")),
			Price:       new(decimal.RequireFromString("500")),
		}},
	})
	if err != nil {
		t.Fatalf("Estimates.Create: %v", err)
	}
	estimateID := mustID(t, estimate.URL)
	t.Cleanup(func() {
		// An approved estimate cannot be deleted (409), and a contact linked
		// to any surviving estimate cannot be deleted either (403). Unwind
		// the status first or the run leaks two records.
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelCleanup()
		if _, _, err := client.Estimates.MarkAsDraft(cleanupCtx, estimateID); err != nil {
			t.Logf("cleanup: could not return estimate %d to draft: %v", estimateID, err)
		}
		deleteQuietly(t, "estimate", client.Estimates.Delete, estimateID)
	})
	t.Logf("created estimate %d, status %q", estimateID, estimate.Status)

	approved, _, err := client.Estimates.MarkAsApproved(ctx, estimateID)
	if err != nil {
		t.Fatalf("Estimates.MarkAsApproved: %v", err)
	}
	t.Logf("mark_as_approved moved the estimate to %q", approved.Status)

	// --- Bill with items ---------------------------------------------------------
	bill, _, err := client.Bills.Create(ctx, &Bill{
		Contact:   contact.URL,
		Reference: tag + "-BILL",
		DatedOn:   DateOf(time.Now()),
		DueOn:     DateOf(time.Now().AddDate(0, 1, 0)),
		BillItems: []BillItem{{
			Category:    spendCategory,
			Description: tag + " bill line",
			TotalValue:  new(decimal.RequireFromString("60")),
		}},
	})
	if err != nil {
		t.Fatalf("Bills.Create: %v", err)
	}
	billID := mustID(t, bill.URL)
	t.Cleanup(func() { deleteQuietly(t, "bill", client.Bills.Delete, billID) })
	t.Logf("created bill %d, total %v", billID, bill.TotalValue)

	// --- Expense with an attachment -------------------------------------------------
	expense, _, err := client.Expenses.Create(ctx, &Expense{
		User:        me.URL,
		Category:    spendCategory,
		DatedOn:     DateOf(time.Now()),
		GrossValue:  new(decimal.RequireFromString("-12.34")),
		Description: tag + " expense",
		Attachment: &Attachment{
			Data:        []byte("%PDF-1.4\nfake receipt for " + tag + "\n"),
			FileName:    "receipt.pdf",
			ContentType: "application/pdf",
		},
	})
	if err != nil {
		t.Fatalf("Expenses.Create: %v", err)
	}
	expenseID := mustID(t, expense.URL)
	t.Cleanup(func() { deleteQuietly(t, "expense", client.Expenses.Delete, expenseID) })

	if expense.Attachment == nil || expense.Attachment.URL.IsZero() {
		t.Fatalf("expense came back with no stored attachment: %+v", expense.Attachment)
	}
	t.Logf("created expense %d with attachment %s (%d bytes stored)",
		expenseID, expense.Attachment.URL, expense.Attachment.FileSize)

	// Read the attachment back through its own service.
	fetched, _, err := client.Attachments.Get(ctx, mustID(t, expense.Attachment.URL))
	if err != nil {
		t.Fatalf("Attachments.Get: %v", err)
	}
	if fetched.ContentType == "" || fetched.FileSize == 0 {
		t.Fatalf("attachment came back incomplete: %+v", fetched)
	}

	// --- updated_since must now see what we just wrote ---------------------------
	touched, _, err := client.Contacts.List(ctx, &ListOptions{
		UpdatedSince: TimeOf(time.Now().Add(-10 * time.Minute)),
		Sort:         "-updated_at",
	})
	if err != nil {
		t.Fatalf("Contacts.List with updated_since: %v", err)
	}
	found := false
	for _, c := range touched {
		if c.URL == contact.URL {
			found = true
		}
	}
	if !found {
		t.Fatal("the contact just written is missing from an updated_since window covering it, which would break incremental reads")
	}
	t.Logf("updated_since picked up the write among %d recent contact(s)", len(touched))

	// --- Remaining document actions --------------------------------------------
	duplicate, _, err := client.Invoices.Duplicate(ctx, invoiceID)
	if err != nil {
		t.Fatalf("Invoices.Duplicate: %v", err)
	}
	duplicateID := mustID(t, duplicate.URL)
	t.Cleanup(func() { deleteQuietly(t, "duplicate invoice", client.Invoices.Delete, duplicateID) })
	if duplicateID == invoiceID {
		t.Fatal("duplicate returned the original invoice")
	}
	t.Logf("duplicated invoice %d into %d, status %q", invoiceID, duplicateID, duplicate.Status)

	estimatePDF, _, err := client.Estimates.PDF(ctx, estimateID)
	if err != nil {
		t.Fatalf("Estimates.PDF: %v", err)
	}
	estimateBytes, err := estimatePDF.Bytes()
	if err != nil {
		t.Fatalf("decoding the estimate PDF: %v", err)
	}
	if !strings.HasPrefix(string(estimateBytes), "%PDF") {
		t.Fatalf("estimate PDF is %d bytes but not a PDF", len(estimateBytes))
	}
	t.Logf("rendered a %d byte estimate PDF", len(estimateBytes))

	// --- Capture fixtures while the records still exist ---------------------------
	capture(t, client, tag, map[string]captureTarget{
		"company":     {Path: "company"},
		"users":       {Path: "users"},
		"categories":  {Path: "categories"},
		"contacts":    {Path: "contacts"},
		"projects":    {Path: "projects"},
		"tasks":       {Path: "tasks"},
		"invoices":    {Path: "invoices", Query: url.Values{"nested_invoice_items": {"true"}}},
		"estimates":   {Path: "estimates", Query: url.Values{"nested_estimate_items": {"true"}}},
		"bills":       {Path: "bills", Query: url.Values{"nested_bill_items": {"true"}}},
		"expenses":    {Path: "expenses"},
		"attachments": {Path: "attachments/" + strconv.FormatInt(mustID(t, expense.Attachment.URL), 10)},
	})
}

// The attachment cap is enforced locally, so this must never reach the API.
func TestLiveOversizedAttachmentNeverSent(t *testing.T) {
	client := writeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, _, err := client.Expenses.Create(ctx, &Expense{
		DatedOn:    DateOf(time.Now()),
		GrossValue: new(decimal.RequireFromString("-1")),
		Attachment: &Attachment{
			Data:     make([]byte, MaxAttachmentBytes+1),
			FileName: "too-big.pdf",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "the limit is") {
		t.Fatalf("err = %v, want the local size limit to have stopped it", err)
	}
}

func mustID(t *testing.T, ref ResourceURL) int64 {
	t.Helper()
	id, err := ref.ID()
	if err != nil {
		t.Fatalf("created record has an unusable URL %q: %v", ref, err)
	}
	return id
}

// deleteQuietly reports a failed cleanup without failing the test. A record
// left behind is noise in the sandbox, not a bug in the library, and masking
// the real failure with a cleanup error would be worse.
func deleteQuietly(t *testing.T, kind string, del func(context.Context, int64) (*Response, error), id int64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := del(ctx, id); err != nil {
		t.Logf("cleanup: could not delete %s %d: %v", kind, id, err)
		return
	}
	t.Logf("cleanup: deleted %s %d", kind, id)
}
