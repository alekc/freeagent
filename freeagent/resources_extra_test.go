package freeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// recorder captures the last request and replies with a fixed body.
func recorder(t *testing.T, body string) (*Client, *string) {
	t.Helper()
	seen := new(string)
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		*seen = r.Method + " " + r.URL.RequestURI()
		fmt.Fprint(w, body)
	})
	return client, seen
}

// The remaining transitions are one-line delegations, but a wrong suffix
// would hit the wrong endpoint silently, so each path is asserted.
func TestEstimateRemainingTransitions(t *testing.T) {
	t.Parallel()
	client, seen := recorder(t, `{"estimate":{"status":"Sent"}}`)
	ctx := context.Background()

	tests := map[string]func() (*Estimate, *Response, error){
		"PUT /v2/estimates/3/transitions/mark_as_sent":     func() (*Estimate, *Response, error) { return client.Estimates.MarkAsSent(ctx, 3) },
		"PUT /v2/estimates/3/transitions/mark_as_draft":    func() (*Estimate, *Response, error) { return client.Estimates.MarkAsDraft(ctx, 3) },
		"PUT /v2/estimates/3/transitions/mark_as_rejected": func() (*Estimate, *Response, error) { return client.Estimates.MarkAsRejected(ctx, 3) },
		"POST /v2/estimates/3/duplicate":                   func() (*Estimate, *Response, error) { return client.Estimates.Duplicate(ctx, 3) },
	}
	for want, call := range tests {
		if _, _, err := call(); err != nil {
			t.Fatalf("%s = %v", want, err)
		}
		if *seen != want {
			t.Fatalf("request = %s, want %s", *seen, want)
		}
	}
}

func TestEstimatePDFAndEmail(t *testing.T) {
	t.Parallel()
	client, seen := recorder(t, `{"pdf":{"content":"UERG"}}`)
	ctx := context.Background()

	pdf, _, err := client.Estimates.PDF(ctx, 3)
	if err != nil {
		t.Fatalf("PDF = %v", err)
	}
	if got, _ := pdf.Bytes(); string(got) != "PDF" {
		t.Fatalf("decoded = %q", got)
	}
	if *seen != "GET /v2/estimates/3/pdf" {
		t.Fatalf("request = %s", *seen)
	}

	if _, err := client.Estimates.SendEmail(ctx, 3, &EmailOptions{To: "a@example.com", Subject: "Your estimate"}); err != nil {
		t.Fatalf("SendEmail = %v", err)
	}
	if *seen != "POST /v2/estimates/3/send_email" {
		t.Fatalf("request = %s", *seen)
	}
}

// EmailOptions must arrive under the email key the API expects.
func TestSendEmailBody(t *testing.T) {
	t.Parallel()
	var body map[string]json.RawMessage
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusOK)
	})
	opts := &EmailOptions{To: "client@example.com", Subject: "Invoice 001", EmailToSelf: true}
	if _, err := client.Invoices.SendEmail(context.Background(), 1, opts); err != nil {
		t.Fatalf("SendEmail = %v", err)
	}
	raw, ok := body["email"]
	if !ok {
		t.Fatalf("request body = %v, want an email key", body)
	}
	var sent EmailOptions
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("decoding email: %v", err)
	}
	if sent.To != "client@example.com" || sent.Subject != "Invoice 001" || !sent.EmailToSelf {
		t.Fatalf("email = %+v", sent)
	}

	// A nil options value still has to produce a well-formed body.
	body = nil
	if _, err := client.Invoices.SendEmail(context.Background(), 1, nil); err != nil {
		t.Fatalf("SendEmail(nil) = %v", err)
	}
	if _, ok := body["email"]; !ok {
		t.Fatalf("request body = %v, want an email key", body)
	}
}

// Mileage settings are historical: each entry covers a date range, and the
// rates object mixes per-vehicle rates with a scalar basic_rate_limit.
func TestExpenseMileageSettings(t *testing.T) {
	t.Parallel()
	client, seen := recorder(t, `{"mileage_settings":{
		"engine_type_and_size_options":[
			{"from":"2011-06-01","to":"2019-02-28","value":{"Petrol":["Up to 1400cc","Over 2000cc"]}}
		],
		"mileage_rates":[
			{"from":"1970-01-01","to":"2011-04-05","value":{
				"Car":{"basic_rate":"0.4","additional_rate":"0.25"},
				"Bicycle":{"basic_rate":"0.2","additional_rate":"0.2"},
				"basic_rate_limit":10000
			}}
		]
	}}`)

	got, _, err := client.Expenses.MileageSettings(context.Background())
	if err != nil {
		t.Fatalf("MileageSettings = %v", err)
	}
	if *seen != "GET /v2/expenses/mileage_settings" {
		t.Fatalf("request = %s", *seen)
	}

	if len(got.EngineTypeAndSizeOptions) != 1 {
		t.Fatalf("engine options = %+v", got.EngineTypeAndSizeOptions)
	}
	period := got.EngineTypeAndSizeOptions[0]
	if period.From.String() != "2011-06-01" || period.To.String() != "2019-02-28" {
		t.Fatalf("period = %s to %s", period.From, period.To)
	}
	if sizes := period.Value["Petrol"]; len(sizes) != 2 || sizes[0] != "Up to 1400cc" {
		t.Fatalf("petrol sizes = %v", sizes)
	}

	if len(got.MileageRates) != 1 {
		t.Fatalf("mileage rates = %+v", got.MileageRates)
	}
	rate, ok := got.MileageRates[0].RatesFor("Car")
	if !ok {
		t.Fatal("no Car rate decoded")
	}
	if rate.BasicRate == nil || rate.BasicRate.String() != "0.4" {
		t.Fatalf("car basic rate = %v", rate.BasicRate)
	}
	// basic_rate_limit shares the object but is a number, not a rate.
	if _, ok := got.MileageRates[0].RatesFor("basic_rate_limit"); ok {
		t.Fatal("basic_rate_limit was decoded as a vehicle rate")
	}
	if _, ok := got.MileageRates[0].RatesFor("Spaceship"); ok {
		t.Fatal("an absent vehicle type reported a rate")
	}
}

func TestAttachmentGetURL(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/attachments/3" {
			t.Errorf("path = %q", r.URL.Path)
		}
		fmt.Fprint(w, `{"attachment":{"file_name":"receipt.pdf"}}`)
	})
	ref := ResourceURL(strings.TrimSuffix(client.BaseURL().String(), "/") + "/attachments/3")

	got, _, err := client.Attachments.GetURL(context.Background(), ref)
	if err != nil {
		t.Fatalf("GetURL = %v", err)
	}
	if got.FileName != "receipt.pdf" {
		t.Fatalf("file_name = %q", got.FileName)
	}
	// A reference on another host must not be followed.
	if _, _, err := client.Attachments.GetURL(context.Background(), "https://evil.example.com/v2/attachments/3"); err == nil {
		t.Fatal("GetURL accepted a foreign host, want an error")
	}
}

func TestCategoryWrites(t *testing.T) {
	t.Parallel()
	client, seen := recorder(t, `{"admin_expenses_categories":{"nominal_code":"286","description":"New category"}}`)
	ctx := context.Background()

	created, _, err := client.Categories.Create(ctx, &Category{Description: "New category"})
	if err != nil {
		t.Fatalf("Create = %v", err)
	}
	if created.Group != CategoryGroupAdminExpenses || created.NominalCode != "286" {
		t.Fatalf("created = %+v", created)
	}
	if *seen != "POST /v2/categories" {
		t.Fatalf("request = %s", *seen)
	}

	if _, _, err := client.Categories.Update(ctx, "286", &Category{Description: "Renamed"}); err != nil {
		t.Fatalf("Update = %v", err)
	}
	if *seen != "PUT /v2/categories/286" {
		t.Fatalf("request = %s", *seen)
	}

	if _, err := client.Categories.Delete(ctx, "286"); err != nil {
		t.Fatalf("Delete = %v", err)
	}
	if *seen != "DELETE /v2/categories/286" {
		t.Fatalf("request = %s", *seen)
	}
}

// Group is derived by this library and must never be written back.
func TestCategoryGroupIsNotSerialised(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(Category{NominalCode: "285", Group: CategoryGroupIncome})
	if err != nil {
		t.Fatalf("Marshal = %v", err)
	}
	if strings.Contains(string(encoded), "income_categories") ||
		strings.Contains(string(encoded), "Group") {
		t.Fatalf("Marshal = %s, want the group omitted", encoded)
	}
}

func TestCategoryFlattenToleratesNil(t *testing.T) {
	t.Parallel()
	var groups *CategoryGroups
	if got := groups.Flatten(); got != nil {
		t.Fatalf("Flatten on nil = %v, want nil", got)
	}
	empty := &CategoryGroups{}
	if got := empty.Flatten(); len(got) != 0 {
		t.Fatalf("Flatten on empty = %v", got)
	}
}

func TestBankTransactionExplanationsAllForAccount(t *testing.T) {
	t.Parallel()
	var seen []string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RequestURI())
		fmt.Fprint(w, `{"bank_transaction_explanations":[{"url":"https://api.freeagent.com/v2/bank_transaction_explanations/1"}]}`)
	})
	account := ResourceURL("https://api.freeagent.com/v2/bank_accounts/1")

	count := 0
	for _, err := range client.BankTransactionExplanations.AllForAccount(context.Background(), account, nil) {
		if err != nil {
			t.Fatalf("AllForAccount yielded %v", err)
		}
		count++
	}
	if count != 1 {
		t.Fatalf("yielded %d, want 1", count)
	}
	if len(seen) != 1 || !strings.Contains(seen[0], "bank_account=") {
		t.Fatalf("requests = %v", seen)
	}

	// Without an account the iterator yields the error rather than calling.
	for _, err := range client.BankTransactionExplanations.AllForAccount(context.Background(), "", nil) {
		if err == nil {
			t.Fatal("AllForAccount with no account yielded no error")
		}
		break
	}
	for _, err := range client.BankTransactionExplanations.All(context.Background(), nil) {
		if err == nil {
			t.Fatal("All yielded no error")
		}
		break
	}
}

func TestBankTransactionUploadStatement(t *testing.T) {
	t.Parallel()
	var (
		seen string
		body map[string]json.RawMessage
	)
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Method + " " + r.URL.RequestURI()
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusOK)
	})
	account := ResourceURL("https://api.freeagent.com/v2/bank_accounts/1")
	lines := []StatementLine{{
		DatedOn:     NewDate(2026, 8, 1),
		Amount:      decimal.RequireFromString("-4.25"),
		Description: "Coffee",
		FitID:       "abc-1",
	}}

	if _, err := client.BankTransactions.UploadStatement(context.Background(), account, lines); err != nil {
		t.Fatalf("UploadStatement = %v", err)
	}
	if !strings.HasPrefix(seen, "POST /v2/bank_transactions/statement?bank_account=") {
		t.Fatalf("request = %s", seen)
	}
	raw, ok := body["statement"]
	if !ok {
		t.Fatalf("request body = %v, want a statement key", body)
	}

	// The statement endpoint takes amount as a JSON number. Sending it as the
	// quoted string every other money field uses is accepted with a 200 and
	// then imports nothing, so this assertion is load bearing.
	if !strings.Contains(string(raw), `"amount":-4.25`) {
		t.Fatalf("statement = %s, want an unquoted amount", raw)
	}
	if !strings.Contains(string(raw), `"fitid":"abc-1"`) {
		t.Fatalf("statement = %s, want the fitid field", raw)
	}

	if _, err := client.BankTransactions.UploadStatement(context.Background(), account, nil); err == nil {
		t.Fatal("UploadStatement with no lines succeeded, want an error")
	}
	undated := []StatementLine{{Amount: decimal.RequireFromString("1")}}
	if _, err := client.BankTransactions.UploadStatement(context.Background(), account, undated); err == nil {
		t.Fatal("UploadStatement with an undated line succeeded, want an error")
	}
}

func TestCompanySubResourceErrors(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"errors":{"error":{"message":"Insufficient access"}}}`)
	})
	ctx := context.Background()

	if _, _, err := client.Company.BusinessCategories(ctx); err == nil {
		t.Fatal("BusinessCategories succeeded on a 403, want an error")
	}
	if _, _, err := client.Company.TaxTimeline(ctx); err == nil {
		t.Fatal("TaxTimeline succeeded on a 403, want an error")
	}
}

// An action answering with an empty body is a success with no record, not a
// decode failure.
func TestActionWithEmptyBody(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	got, resp, err := client.Invoices.MarkAsSent(context.Background(), 1)
	if err != nil {
		t.Fatalf("MarkAsSent = %v", err)
	}
	if got != nil {
		t.Fatalf("record = %+v, want nil", got)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestMustResourcePanicsOnUnknownName(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("mustResource on an unknown name did not panic")
		}
	}()
	mustResource("no_such_resource")
}
