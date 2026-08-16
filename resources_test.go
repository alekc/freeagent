package freeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// golden serves a fixture from testdata for every request.
func goldenClient(t *testing.T, fixture string, seen ...*string) *Client {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if len(seen) > 0 {
			*seen[0] = r.Method + " " + r.URL.RequestURI()
		}
		_, _ = w.Write(body)
	})
}

// Each Wave A family decodes its captured payload with the right Go types.
//
// The fixtures are real responses, anonymised (see testdata/README.md), so
// these assertions are deliberately about shape and invariants rather than
// the values of one particular capture. A re-capture must not break them;
// a wrong json tag or Go type must.
func TestWaveAGoldenDecoding(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		verify func(t *testing.T, c *Client)
	}{
		{name: "company", verify: func(t *testing.T, c *Client) {
			got, _, err := c.Company.Get(context.Background(), nil)
			if err != nil {
				t.Fatalf("Get = %v", err)
			}
			// The docs type id as an integer and their own example quotes it.
			// Int64 takes either; what matters is that it decoded at all.
			if got.ID == 0 {
				t.Fatal("id did not decode")
			}
			if got.Name == "" || got.Type == "" || got.Currency == "" {
				t.Fatalf("company is missing core fields: %+v", got)
			}
			if got.CompanyStartDate.IsZero() || got.FreeAgentStartDate.IsZero() {
				t.Fatalf("accounting dates did not decode: %+v", got)
			}
			if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
				t.Fatal("timestamps did not decode")
			}
			if len(got.AnnualAccountingPeriods) == 0 {
				t.Fatal("annual_accounting_periods did not decode")
			}
			if got.AnnualAccountingPeriods[0].StartsOn.IsZero() {
				t.Fatal("a nested accounting period date did not decode")
			}
			// Rates arrive as an array of quoted decimals.
			if len(got.SalesTaxRates) == 0 {
				t.Fatal("sales_tax_rates did not decode")
			}
			// Undocumented fields the live API sends; see the README.
			if got.Locale == "" {
				t.Fatal("locale did not decode, though the live API sends it")
			}
		}},
		{name: "users", verify: func(t *testing.T, c *Client) {
			got, _, err := c.Users.List(context.Background(), nil)
			if err != nil {
				t.Fatalf("List = %v", err)
			}
			if len(got) == 0 {
				t.Fatal("no users decoded")
			}
			user := got[0]
			if user.Role == "" {
				t.Fatalf("role did not decode: %+v", user)
			}
			if _, err := user.URL.ID(); err != nil {
				t.Fatalf("user url is not a member URL: %v", err)
			}
			if user.PermissionLevel == nil {
				t.Fatal("permission_level did not decode")
			}
		}},
		{name: "contacts", verify: func(t *testing.T, c *Client) {
			got, _, err := c.Contacts.List(context.Background(), nil)
			if err != nil {
				t.Fatalf("List = %v", err)
			}
			if len(got) == 0 {
				t.Fatal("no contacts decoded")
			}
			contact := got[0]
			if contact.OrganisationName == "" && contact.FirstName == "" {
				t.Fatalf("contact has neither an organisation nor a person name: %+v", contact)
			}
			if contact.Status == "" {
				t.Fatal("status did not decode")
			}
			if contact.CreatedAt.IsZero() || contact.UpdatedAt.IsZero() {
				t.Fatal("timestamps did not decode")
			}
			// account_balance is a quoted decimal and must survive exactly.
			if contact.AccountBalance == nil {
				t.Fatal("account_balance did not decode")
			}
		}},
		{name: "projects", verify: func(t *testing.T, c *Client) {
			got, _, err := c.Projects.List(context.Background(), nil)
			if err != nil {
				t.Fatalf("List = %v", err)
			}
			if len(got) == 0 {
				t.Fatal("no projects decoded")
			}
			project := got[0]
			if project.Name == "" || project.Status == "" {
				t.Fatalf("project is missing core fields: %+v", project)
			}
			// budget can arrive as a bare number rather than a quoted string.
			if project.Budget == nil {
				t.Fatal("budget did not decode")
			}
			if kind := project.Contact.Kind(); kind != "contacts" {
				t.Fatalf("contact reference kind = %q, want contacts", kind)
			}
		}},
		{name: "tasks", verify: func(t *testing.T, c *Client) {
			got, _, err := c.Tasks.List(context.Background(), nil)
			if err != nil {
				t.Fatalf("List = %v", err)
			}
			if len(got) == 0 {
				t.Fatal("no tasks decoded")
			}
			task := got[0]
			if task.Name == "" {
				t.Fatalf("task has no name: %+v", task)
			}
			if kind := task.Project.Kind(); kind != "projects" {
				t.Fatalf("project reference kind = %q, want projects", kind)
			}
		}},
		{name: "invoices", verify: func(t *testing.T, c *Client) {
			got, _, err := c.Invoices.List(context.Background(), nil)
			if err != nil {
				t.Fatalf("List = %v", err)
			}
			if len(got) == 0 {
				t.Fatal("no invoices decoded")
			}
			invoice := got[0]
			if invoice.Status == "" || invoice.Reference == "" {
				t.Fatalf("invoice is missing core fields: %+v", invoice)
			}
			if invoice.TotalValue == nil || invoice.NetValue == nil {
				t.Fatal("computed money values did not decode")
			}
			if invoice.DatedOn.IsZero() {
				t.Fatal("dated_on did not decode")
			}
			// The nested items are why the fixture is captured with
			// nested_invoice_items=true.
			if len(invoice.InvoiceItems) == 0 {
				t.Fatal("invoice_items did not decode")
			}
			item := invoice.InvoiceItems[0]
			if item.Description == "" || item.Price == nil || item.Quantity == nil {
				t.Fatalf("invoice item is incomplete: %+v", item)
			}
			// The server computes the total from the items; a mismatch here
			// means a decimal was mangled somewhere in the round trip.
			want := item.Price.Mul(*item.Quantity)
			if !invoice.NetValue.Equal(want) {
				t.Fatalf("net_value = %v but the single item works out to %v", invoice.NetValue, want)
			}
		}},
		{name: "estimates", verify: func(t *testing.T, c *Client) {
			got, _, err := c.Estimates.List(context.Background(), nil)
			if err != nil {
				t.Fatalf("List = %v", err)
			}
			if len(got) == 0 {
				t.Fatal("no estimates decoded")
			}
			estimate := got[0]
			if estimate.EstimateType == "" || estimate.Status == "" {
				t.Fatalf("estimate is missing core fields: %+v", estimate)
			}
			if estimate.NetValue == nil {
				t.Fatal("net_value did not decode")
			}
			if len(estimate.EstimateItems) == 0 {
				t.Fatal("estimate_items did not decode")
			}
		}},
		{name: "bills", verify: func(t *testing.T, c *Client) {
			got, _, err := c.Bills.List(context.Background(), nil)
			if err != nil {
				t.Fatalf("List = %v", err)
			}
			if len(got) == 0 {
				t.Fatal("no bills decoded")
			}
			bill := got[0]
			if bill.Status == "" || bill.TotalValue == nil {
				t.Fatalf("bill is missing core fields: %+v", bill)
			}
			if bill.DatedOn.IsZero() || bill.DueOn.IsZero() {
				t.Fatal("bill dates did not decode")
			}
			if len(bill.BillItems) == 0 {
				t.Fatal("bill_items did not decode")
			}
			if bill.BillItems[0].Category.Kind() != "categories" {
				t.Fatalf("bill item category = %q", bill.BillItems[0].Category)
			}
		}},
		{name: "expenses", verify: func(t *testing.T, c *Client) {
			got, _, err := c.Expenses.List(context.Background(), nil)
			if err != nil {
				t.Fatalf("List = %v", err)
			}
			if len(got) == 0 {
				t.Fatal("no expenses decoded")
			}
			expense := got[0]
			// Negative means a payment rather than a refund.
			if expense.GrossValue == nil || !expense.GrossValue.IsNegative() {
				t.Fatalf("gross_value = %v, want a negative payment", expense.GrossValue)
			}
			if expense.User.Kind() != "users" || expense.Category.Kind() != "categories" {
				t.Fatalf("expense references are wrong: user %q category %q", expense.User, expense.Category)
			}
			// The attachment read shape shares its key with the write shape.
			if expense.Attachment == nil {
				t.Fatal("attachment did not decode")
			}
			if expense.Attachment.FileName == "" || expense.Attachment.FileSize == 0 {
				t.Fatalf("attachment metadata is incomplete: %+v", expense.Attachment)
			}
			if expense.Attachment.Data != nil {
				t.Fatal("data should be absent on read")
			}
		}},
		{name: "bank_accounts", verify: func(t *testing.T, c *Client) {
			got, _, err := c.BankAccounts.List(context.Background(), nil)
			if err != nil {
				t.Fatalf("List = %v", err)
			}
			if len(got) == 0 {
				t.Fatal("no bank accounts decoded")
			}
			account := got[0]
			if account.Type == "" || account.Name == "" {
				t.Fatalf("bank account is missing core fields: %+v", account)
			}
			if account.CurrentBalance == nil || account.OpeningBalance == nil {
				t.Fatal("balances did not decode")
			}
			// Undocumented tallies the live API sends.
			if account.TotalCount == nil || account.UnexplainedTransactionCount == nil {
				t.Fatalf("transaction tallies did not decode: %+v", account)
			}
		}},
		{name: "bank_transactions", verify: func(t *testing.T, c *Client) {
			account := ResourceURL("https://api.freeagent.com/v2/bank_accounts/1")
			got, _, err := c.BankTransactions.ListForAccount(context.Background(), account, nil)
			if err != nil {
				t.Fatalf("ListForAccount = %v", err)
			}
			if len(got) == 0 {
				t.Fatal("no bank transactions decoded")
			}
			txn := got[0]
			if txn.Amount == nil || txn.UnexplainedAmount == nil {
				t.Fatalf("transaction money values did not decode: %+v", txn)
			}
			if txn.DatedOn.IsZero() || txn.UploadedAt.IsZero() {
				t.Fatal("transaction dates did not decode")
			}
			if txn.BankAccount.Kind() != "bank_accounts" {
				t.Fatalf("bank_account reference = %q", txn.BankAccount)
			}
		}},
		{name: "bank_transaction_explanations", verify: func(t *testing.T, c *Client) {
			account := ResourceURL("https://api.freeagent.com/v2/bank_accounts/1")
			got, _, err := c.BankTransactionExplanations.ListForAccount(context.Background(), account, nil)
			if err != nil {
				t.Fatalf("ListForAccount = %v", err)
			}
			if len(got) == 0 {
				t.Fatal("no explanations decoded")
			}
			explanation := got[0]
			if explanation.GrossValue == nil {
				t.Fatal("gross_value did not decode")
			}
			if explanation.Type == "" {
				t.Fatal("type did not decode")
			}
			// The money-direction booleans are read-only and always sent.
			if explanation.IsMoneyIn == nil || explanation.IsMoneyOut == nil {
				t.Fatalf("money direction flags did not decode: %+v", explanation)
			}
			if explanation.BankTransaction.Kind() != "bank_transactions" {
				t.Fatalf("bank_transaction reference = %q", explanation.BankTransaction)
			}
		}},
		{name: "attachments", verify: func(t *testing.T, c *Client) {
			got, _, err := c.Attachments.Get(context.Background(), 1)
			if err != nil {
				t.Fatalf("Get = %v", err)
			}
			if got.FileName == "" || got.ContentType == "" || got.FileSize == 0 {
				t.Fatalf("attachment is incomplete: %+v", got)
			}
			if got.ExpiresAt.IsZero() {
				t.Fatal("expires_at did not decode")
			}
			if got.Data != nil {
				t.Fatal("data should be absent on read")
			}
		}},
		{name: "categories", verify: func(t *testing.T, c *Client) {
			groups, _, err := c.Categories.List(context.Background(), false)
			if err != nil {
				t.Fatalf("List = %v", err)
			}
			// A real chart of accounts populates all four groups.
			if len(groups.AdminExpenses) == 0 || len(groups.CostOfSales) == 0 ||
				len(groups.Income) == 0 || len(groups.General) == 0 {
				t.Fatalf("not every group decoded: admin %d, cost %d, income %d, general %d",
					len(groups.AdminExpenses), len(groups.CostOfSales),
					len(groups.Income), len(groups.General))
			}
			flat := groups.Flatten()
			if len(flat) < 50 {
				t.Fatalf("flattened to %d categories, want a full chart of accounts", len(flat))
			}
			for _, category := range flat {
				if category.NominalCode == "" {
					t.Fatalf("category has no nominal code: %+v", category)
				}
				if category.Group == "" {
					t.Fatalf("category %s has no group set", category.NominalCode)
				}
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

func TestServicesAreWired(t *testing.T) {
	t.Parallel()
	client, err := NewClient(WithoutAuth())
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	metas := map[string]ResourceMeta{
		"attachments":                   client.Attachments.Meta(),
		"bank_accounts":                 client.BankAccounts.Meta(),
		"bank_transaction_explanations": client.BankTransactionExplanations.Meta(),
		"bank_transactions":             client.BankTransactions.Meta(),
		"bills":                         client.Bills.Meta(),
		"categories":                    client.Categories.Meta(),
		"company":                       client.Company.Meta(),
		"contacts":                      client.Contacts.Meta(),
		"estimates":                     client.Estimates.Meta(),
		"expenses":                      client.Expenses.Meta(),
		"invoices":                      client.Invoices.Meta(),
		"projects":                      client.Projects.Meta(),
		"tasks":                         client.Tasks.Meta(),
		"users":                         client.Users.Meta(),
	}
	for name, meta := range metas {
		if meta.Name != name {
			t.Errorf("service %s is wired to %q", name, meta.Name)
		}
		if meta.Path == "" {
			t.Errorf("service %s has no path", name)
		}
	}
	if len(metas) != 14 {
		t.Fatalf("checked %d services, Wave A is 14", len(metas))
	}
}

// The bank endpoints reject a list without an account, so the inherited List
// is shadowed to fail locally with a pointer to the right method.
func TestBankEndpointsRequireAnAccount(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be sent without a bank account")
	})
	ctx := context.Background()

	if _, _, err := client.BankTransactions.List(ctx, nil); err == nil ||
		!strings.Contains(err.Error(), "ListForAccount") {
		t.Fatalf("List = %v, want a pointer to ListForAccount", err)
	}
	if _, _, err := client.BankTransactionExplanations.List(ctx, nil); err == nil ||
		!strings.Contains(err.Error(), "ListForAccount") {
		t.Fatalf("List = %v, want a pointer to ListForAccount", err)
	}
	for txn, err := range client.BankTransactions.All(ctx, nil) {
		if err == nil {
			t.Fatalf("All yielded %+v, want an error", txn)
		}
		break
	}
	if _, _, err := client.BankTransactions.ListForAccount(ctx, "", nil); err == nil {
		t.Fatal("ListForAccount with an empty account succeeded, want an error")
	}
	if _, err := client.BankTransactions.UploadStatement(ctx, "", nil); err == nil {
		t.Fatal("UploadStatement with no account succeeded, want an error")
	}
}

func TestBankTransactionsScopeTheAccount(t *testing.T) {
	t.Parallel()
	var seen string
	client := goldenClient(t, "bank_transactions.json", &seen)
	account := ResourceURL("https://api.freeagent.com/v2/bank_accounts/1")

	opts := &ListOptions{FromDate: NewDate(2012, 1, 1), PerPage: 50}
	if _, _, err := client.BankTransactions.ListForAccount(context.Background(), account, opts); err != nil {
		t.Fatalf("ListForAccount = %v", err)
	}
	if !strings.Contains(seen, "bank_account=https%3A%2F%2Fapi.freeagent.com%2Fv2%2Fbank_accounts%2F1") {
		t.Fatalf("request = %s, want the bank_account filter", seen)
	}
	if !strings.Contains(seen, "from_date=2012-01-01") || !strings.Contains(seen, "per_page=50") {
		t.Fatalf("request = %s, want the caller's other options preserved", seen)
	}
	// Scoping must not mutate the caller's options.
	if opts.Extra != nil {
		t.Fatalf("caller options were mutated: Extra = %v", opts.Extra)
	}
}

// Tasks are created under a project via the query string, so the inherited
// Create would post a task with no parent.
func TestTaskCreateRequiresAProject(t *testing.T) {
	t.Parallel()
	var seen string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Method + " " + r.URL.RequestURI()
		fmt.Fprint(w, `{"task":{"url":"https://api.freeagent.com/v2/tasks/1","name":"x"}}`)
	})
	ctx := context.Background()

	if _, _, err := client.Tasks.Create(ctx, &Task{Name: "x"}); err == nil ||
		!strings.Contains(err.Error(), "CreateForProject") {
		t.Fatalf("Create = %v, want a pointer to CreateForProject", err)
	}
	if _, _, err := client.Tasks.CreateForProject(ctx, "", &Task{Name: "x"}); err == nil {
		t.Fatal("CreateForProject with no project succeeded, want an error")
	}
	if _, _, err := client.Tasks.CreateForProject(ctx, "https://x/v2/projects/1", nil); err == nil {
		t.Fatal("CreateForProject with a nil task succeeded, want an error")
	}

	project := ResourceURL("https://api.freeagent.com/v2/projects/1")
	if _, _, err := client.Tasks.CreateForProject(ctx, project, &Task{Name: "x"}); err != nil {
		t.Fatalf("CreateForProject = %v", err)
	}
	if !strings.HasPrefix(seen, "POST /v2/tasks?project=") {
		t.Fatalf("request = %s, want the project in the query string", seen)
	}
}

func TestUsersMe(t *testing.T) {
	t.Parallel()
	var seen string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Method + " " + r.URL.Path
		fmt.Fprint(w, `{"user":{"url":"https://api.freeagent.com/v2/users/1","first_name":"Me"}}`)
	})
	ctx := context.Background()

	got, _, err := client.Users.Me(ctx)
	if err != nil {
		t.Fatalf("Me = %v", err)
	}
	if got.FirstName != "Me" {
		t.Fatalf("first_name = %q", got.FirstName)
	}
	if seen != "GET /v2/users/me" {
		t.Fatalf("request = %s", seen)
	}

	if _, _, err := client.Users.UpdateMe(ctx, &User{FirstName: "New"}); err != nil {
		t.Fatalf("UpdateMe = %v", err)
	}
	if seen != "PUT /v2/users/me" {
		t.Fatalf("request = %s", seen)
	}
	if _, _, err := client.Users.UpdateMe(ctx, nil); err == nil {
		t.Fatal("UpdateMe(nil) succeeded, want an error")
	}
}

func TestInvoiceTransitionsAndPDF(t *testing.T) {
	t.Parallel()
	var seen string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Method + " " + r.URL.Path
		if strings.HasSuffix(r.URL.Path, "/pdf") {
			fmt.Fprint(w, `{"pdf":{"content":"SGVsbG8gUERG"}}`)
			return
		}
		fmt.Fprint(w, `{"invoice":{"url":"https://api.freeagent.com/v2/invoices/7","status":"Open"}}`)
	})
	ctx := context.Background()

	transitions := map[string]func() (*Invoice, *Response, error){
		"/v2/invoices/7/transitions/mark_as_sent":           func() (*Invoice, *Response, error) { return client.Invoices.MarkAsSent(ctx, 7) },
		"/v2/invoices/7/transitions/mark_as_scheduled":      func() (*Invoice, *Response, error) { return client.Invoices.MarkAsScheduled(ctx, 7) },
		"/v2/invoices/7/transitions/mark_as_draft":          func() (*Invoice, *Response, error) { return client.Invoices.MarkAsDraft(ctx, 7) },
		"/v2/invoices/7/transitions/mark_as_cancelled":      func() (*Invoice, *Response, error) { return client.Invoices.MarkAsCancelled(ctx, 7) },
		"/v2/invoices/7/transitions/convert_to_credit_note": func() (*Invoice, *Response, error) { return client.Invoices.ConvertToCreditNote(ctx, 7) },
	}
	for path, call := range transitions {
		got, _, err := call()
		if err != nil {
			t.Fatalf("%s = %v", path, err)
		}
		if got.Status != "Open" {
			t.Fatalf("%s returned %+v", path, got)
		}
		if seen != "PUT "+path {
			t.Fatalf("request = %s, want PUT %s", seen, path)
		}
	}

	if _, _, err := client.Invoices.Duplicate(ctx, 7); err != nil {
		t.Fatalf("Duplicate = %v", err)
	}
	if seen != "POST /v2/invoices/7/duplicate" {
		t.Fatalf("request = %s", seen)
	}

	pdf, _, err := client.Invoices.PDF(ctx, 7)
	if err != nil {
		t.Fatalf("PDF = %v", err)
	}
	decoded, err := pdf.Bytes()
	if err != nil {
		t.Fatalf("Bytes = %v", err)
	}
	if string(decoded) != "Hello PDF" {
		t.Fatalf("decoded PDF = %q", decoded)
	}
	if _, err := (&PDF{}).Bytes(); err == nil {
		t.Fatal("empty PDF decoded, want an error")
	}
	if _, err := (&PDF{Content: "not base64!!"}).Bytes(); err == nil {
		t.Fatal("invalid base64 decoded, want an error")
	}

	if _, err := client.Invoices.SendEmail(ctx, 7, nil); err != nil {
		t.Fatalf("SendEmail = %v", err)
	}
	if seen != "POST /v2/invoices/7/send_email" {
		t.Fatalf("request = %s", seen)
	}
}

func TestEstimateTransitions(t *testing.T) {
	t.Parallel()
	var seen string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Method + " " + r.URL.Path
		fmt.Fprint(w, `{"estimate":{"status":"Approved"}}`)
	})
	ctx := context.Background()

	if _, _, err := client.Estimates.MarkAsApproved(ctx, 3); err != nil {
		t.Fatalf("MarkAsApproved = %v", err)
	}
	if seen != "PUT /v2/estimates/3/transitions/mark_as_approved" {
		t.Fatalf("request = %s", seen)
	}
	if _, _, err := client.Estimates.ConvertToInvoice(ctx, 3); err != nil {
		t.Fatalf("ConvertToInvoice = %v", err)
	}
	if seen != "PUT /v2/estimates/3/transitions/convert_to_invoice" {
		t.Fatalf("request = %s", seen)
	}
}

func TestCompanySubResources(t *testing.T) {
	t.Parallel()
	var seen string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
		switch {
		case strings.HasSuffix(r.URL.Path, "business_categories"):
			fmt.Fprint(w, `{"business_categories":["Accountancy","Consultancy"]}`)
		default:
			fmt.Fprint(w, `{"timeline_items":[{"description":"VAT return due","nature":"vat","dated_on":"2026-09-07","amount_due":"1200.0","is_personal":false}]}`)
		}
	})
	ctx := context.Background()

	categories, _, err := client.Company.BusinessCategories(ctx)
	if err != nil {
		t.Fatalf("BusinessCategories = %v", err)
	}
	if len(categories) != 2 || categories[0] != "Accountancy" {
		t.Fatalf("business categories = %v", categories)
	}
	if seen != "/v2/company/business_categories" {
		t.Fatalf("path = %s", seen)
	}

	timeline, _, err := client.Company.TaxTimeline(ctx)
	if err != nil {
		t.Fatalf("TaxTimeline = %v", err)
	}
	if len(timeline) != 1 || timeline[0].DatedOn.String() != "2026-09-07" {
		t.Fatalf("timeline = %+v", timeline)
	}
	if timeline[0].AmountDue == nil || !timeline[0].AmountDue.Equal(decimal.RequireFromString("1200.0")) {
		t.Fatalf("amount_due = %v", timeline[0].AmountDue)
	}
}

// Categories are addressed by nominal code and answer under whichever group
// they belong to, neither of which the generic collection can do.
func TestCategoryGetResolvesItsGroup(t *testing.T) {
	t.Parallel()
	var seen string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Method + " " + r.URL.Path
		fmt.Fprint(w, `{"admin_expenses_categories":{"url":"https://api.freeagent.com/v2/categories/285","description":"Accommodation and Meals","nominal_code":"285","allowable_for_tax":true}}`)
	})
	ctx := context.Background()

	got, _, err := client.Categories.Get(ctx, "285")
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	if got.Group != CategoryGroupAdminExpenses {
		t.Fatalf("group = %q, want %q", got.Group, CategoryGroupAdminExpenses)
	}
	if got.NominalCode != "285" {
		t.Fatalf("nominal_code = %q", got.NominalCode)
	}
	if seen != "GET /v2/categories/285" {
		t.Fatalf("request = %s", seen)
	}

	// Sub-account codes contain a hyphen and must be accepted.
	if _, _, err := client.Categories.Get(ctx, "750-1"); err != nil {
		t.Fatalf("Get(750-1) = %v", err)
	}
	if seen != "GET /v2/categories/750-1" {
		t.Fatalf("request = %s", seen)
	}
}

// A nominal code goes straight into a URL path, so anything that could
// address a different endpoint is rejected before the request is built.
func TestCategoryNominalCodeIsValidated(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request should be sent for an invalid code, got %s", r.URL.Path)
	})
	ctx := context.Background()

	for _, code := range []string{"", "  ", "../company", "285/../../admin", "28 5", "285?x=1"} {
		if _, _, err := client.Categories.Get(ctx, code); err == nil {
			t.Fatalf("Get(%q) succeeded, want an error", code)
		}
		if _, err := client.Categories.Delete(ctx, code); err == nil {
			t.Fatalf("Delete(%q) succeeded, want an error", code)
		}
	}
}

func TestCategoryUnknownGroupIsReported(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"mystery_categories":{"nominal_code":"999"}}`)
	})
	if _, _, err := client.Categories.Get(context.Background(), "999"); err == nil ||
		!strings.Contains(err.Error(), "no known category group") {
		t.Fatalf("err = %v, want an unknown group error", err)
	}
}

func TestCategoryWritesRejectNil(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be sent for a nil category")
	})
	ctx := context.Background()
	if _, _, err := client.Categories.Create(ctx, nil); err == nil {
		t.Fatal("Create(nil) succeeded, want an error")
	}
	if _, _, err := client.Categories.Update(ctx, "285", nil); err == nil {
		t.Fatal("Update(nil) succeeded, want an error")
	}
}

func TestCategorySubAccountsFlag(t *testing.T) {
	t.Parallel()
	var seen string
	client := goldenClient(t, "categories.json", &seen)
	if _, _, err := client.Categories.List(context.Background(), true); err != nil {
		t.Fatalf("List = %v", err)
	}
	if !strings.Contains(seen, "sub_accounts=true") {
		t.Fatalf("request = %s, want sub_accounts=true", seen)
	}
}

// The upload limit is enforced at encode time so it applies to every resource
// that carries an attachment, without each one repeating the check.
func TestAttachmentSizeLimitIsEnforcedLocally(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("an oversized attachment must not reach the network")
	})
	oversized := &Attachment{
		Data:        make([]byte, MaxAttachmentBytes+1),
		FileName:    "huge.pdf",
		ContentType: "application/pdf",
	}
	ctx := context.Background()

	_, _, err := client.Expenses.Create(ctx, &Expense{Attachment: oversized})
	if err == nil || !strings.Contains(err.Error(), "the limit is") {
		t.Fatalf("Create = %v, want a size limit error", err)
	}
	_, _, err = client.Bills.Create(ctx, &Bill{Attachment: oversized})
	if err == nil || !strings.Contains(err.Error(), "the limit is") {
		t.Fatalf("Create = %v, want a size limit error", err)
	}
}

// An attachment within the limit is base64 encoded under the same key the
// read shape uses.
func TestAttachmentUploadEncoding(t *testing.T) {
	t.Parallel()
	var body map[string]json.RawMessage
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		fmt.Fprint(w, `{"expense":{}}`)
	})
	expense := &Expense{Attachment: &Attachment{
		Data:        []byte("receipt bytes"),
		FileName:    "receipt.txt",
		ContentType: "text/plain",
	}}
	if _, _, err := client.Expenses.Create(context.Background(), expense); err != nil {
		t.Fatalf("Create = %v", err)
	}
	var sent struct {
		Attachment struct {
			Data     string `json:"data"`
			FileName string `json:"file_name"`
		} `json:"attachment"`
	}
	if err := json.Unmarshal(body["expense"], &sent); err != nil {
		t.Fatalf("decoding sent expense: %v", err)
	}
	if sent.Attachment.Data != "cmVjZWlwdCBieXRlcw==" {
		t.Fatalf("data = %q, want base64 of the file", sent.Attachment.Data)
	}
	if sent.Attachment.FileName != "receipt.txt" {
		t.Fatalf("file_name = %q", sent.Attachment.FileName)
	}
}

func TestAttachmentServiceDelete(t *testing.T) {
	t.Parallel()
	var seen string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Method + " " + r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})
	if _, err := client.Attachments.Delete(context.Background(), 3); err != nil {
		t.Fatalf("Delete = %v", err)
	}
	if seen != "DELETE /v2/attachments/3" {
		t.Fatalf("request = %s", seen)
	}
}

func TestInt64AcceptsBothForms(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    Int64
		wantErr bool
	}{
		{in: `12345`, want: 12345},
		{in: `"12345"`, want: 12345},
		{in: `null`, want: 0},
		{in: `""`, want: 0},
		{in: `-7`, want: -7},
		{in: `"abc"`, wantErr: true},
		{in: `12.5`, wantErr: true},
		{in: `"` + strings.Repeat("1", maxScalarLen+1) + `"`, wantErr: true},
	}
	for _, tc := range tests {
		var got Int64
		err := json.Unmarshal([]byte(tc.in), &got)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("Unmarshal(%s) = %d, want an error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Unmarshal(%s) = %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("Unmarshal(%s) = %d, want %d", tc.in, got, tc.want)
		}
	}
	// Always written back as a number, never re-quoted.
	out, err := json.Marshal(Int64Of(42))
	if err != nil {
		t.Fatalf("Marshal = %v", err)
	}
	if string(out) != "42" {
		t.Fatalf("Marshal = %s, want 42", out)
	}
	if Int64Of(42).Value() != 42 {
		t.Fatalf("Value = %d", Int64Of(42).Value())
	}
}

func TestRegistryCoversWaveA(t *testing.T) {
	t.Parallel()
	for _, name := range ResourceNames() {
		meta, _ := LookupResource(name)
		switch {
		case meta.Singleton, meta.Grouped, meta.NoList, meta.CustomEnvelope:
			// These have no plural envelope by design.
		default:
			if meta.Plural == "" {
				t.Errorf("%s: collections need a plural envelope key", name)
			}
		}
		if meta.Singular == "" && !meta.Singleton && !meta.CustomEnvelope {
			t.Errorf("%s: needs a singular envelope key", name)
		}
		if meta.Doc == "" {
			t.Errorf("%s: needs a documentation URL", name)
		}
	}
}
