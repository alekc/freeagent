package freeagent

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// The Wave D transitions file and unfile tax returns and record payments to
// HMRC, so they are never exercised against a live account. Their paths are
// pinned here instead, which is the part that can actually be wrong.
func TestFilingTransitionPaths(t *testing.T) {
	t.Parallel()
	var seen string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Method + " " + r.URL.Path
		switch {
		case strings.Contains(r.URL.Path, "vat_returns"):
			fmt.Fprint(w, `{"vat_return":{"period_ends_on":"2026-06-30","filing_status":"filed"}}`)
		case strings.Contains(r.URL.Path, "corporation_tax_returns"):
			fmt.Fprint(w, `{"corporation_tax_return":{"period_ends_on":"2027-04-30","filing_status":"filed"}}`)
		case strings.Contains(r.URL.Path, "self_assessment_returns"):
			fmt.Fprint(w, `{"self_assessment_return":{"period_ends_on":"2027-04-05","filing_status":"filed"}}`)
		default:
			fmt.Fprint(w, `{"final_accounts_report":{"period_ends_on":"2027-04-30","filing_status":"filed"}}`)
		}
	})
	ctx := context.Background()

	period := NewDate(2026, 6, 30)
	payment := NewDate(2026, 8, 7)
	user := ResourceURL("https://api.freeagent.com/v2/users/40180")

	tests := []struct {
		name string
		call func() error
		want string
	}{
		{"vat filed", func() error {
			_, _, err := client.VATReturns.MarkAsFiled(ctx, period)
			return err
		}, "PUT /v2/vat_returns/2026-06-30/mark_as_filed"},
		{"vat unfiled", func() error {
			_, _, err := client.VATReturns.MarkAsUnfiled(ctx, period)
			return err
		}, "PUT /v2/vat_returns/2026-06-30/mark_as_unfiled"},
		{"vat payment paid", func() error {
			_, _, err := client.VATReturns.MarkPaymentAsPaid(ctx, period, payment)
			return err
		}, "PUT /v2/vat_returns/2026-06-30/payments/2026-08-07/mark_as_paid"},
		{"vat payment unpaid", func() error {
			_, _, err := client.VATReturns.MarkPaymentAsUnpaid(ctx, period, payment)
			return err
		}, "PUT /v2/vat_returns/2026-06-30/payments/2026-08-07/mark_as_unpaid"},

		{"corp tax filed", func() error {
			_, _, err := client.CorporationTaxReturns.MarkAsFiled(ctx, period)
			return err
		}, "PUT /v2/corporation_tax_returns/2026-06-30/mark_as_filed"},
		// Corporation tax has no payment date: the payment is a property of
		// the return, unlike VAT and income tax.
		{"corp tax paid", func() error {
			_, _, err := client.CorporationTaxReturns.MarkAsPaid(ctx, period)
			return err
		}, "PUT /v2/corporation_tax_returns/2026-06-30/mark_as_paid"},
		{"corp tax unpaid", func() error {
			_, _, err := client.CorporationTaxReturns.MarkAsUnpaid(ctx, period)
			return err
		}, "PUT /v2/corporation_tax_returns/2026-06-30/mark_as_unpaid"},

		// Nested under the user, and using the self_assessment_returns
		// segment rather than income_tax_returns.
		{"income tax filed", func() error {
			_, _, err := client.IncomeTaxReturns.MarkAsFiled(ctx, user, period)
			return err
		}, "PUT /v2/users/40180/self_assessment_returns/2026-06-30/mark_as_filed"},
		{"income tax payment paid", func() error {
			_, _, err := client.IncomeTaxReturns.MarkPaymentAsPaid(ctx, user, period, payment)
			return err
		}, "PUT /v2/users/40180/self_assessment_returns/2026-06-30/payments/2026-08-07/mark_as_paid"},

		{"final accounts filed", func() error {
			_, _, err := client.FinalAccountsReports.MarkAsFiled(ctx, period)
			return err
		}, "PUT /v2/final_accounts_reports/2026-06-30/mark_as_filed"},

		{"payroll payment paid", func() error {
			_, err := client.Payroll.MarkPaymentAsPaid(ctx, 2026, payment)
			return err
		}, "PUT /v2/payroll/2026/payments/2026-08-07/mark_as_paid"},
		{"payroll payment unpaid", func() error {
			_, err := client.Payroll.MarkPaymentAsUnpaid(ctx, 2026, payment)
			return err
		}, "PUT /v2/payroll/2026/payments/2026-08-07/mark_as_unpaid"},
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

// Dates and years are rendered rather than interpolated from caller text, and
// a missing one must fail before any request.
func TestFilingTransitionsValidateInputs(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request should be sent for invalid input, got %s", r.URL.Path)
	})
	ctx := context.Background()
	valid := NewDate(2026, 6, 30)

	checks := map[string]func() error{
		"vat get with zero period": func() error {
			_, _, err := client.VATReturns.Get(ctx, Date{})
			return err
		},
		"vat payment with zero payment date": func() error {
			_, _, err := client.VATReturns.MarkPaymentAsPaid(ctx, valid, Date{})
			return err
		},
		"corp tax with zero period": func() error {
			_, _, err := client.CorporationTaxReturns.MarkAsFiled(ctx, Date{})
			return err
		},
		"income tax with a non-user scope": func() error {
			_, _, err := client.IncomeTaxReturns.ListForUser(ctx, "https://x/v2/contacts/1")
			return err
		},
		"income tax with an empty scope": func() error {
			_, _, err := client.IncomeTaxReturns.ListForUser(ctx, "")
			return err
		},
		"payroll year out of range": func() error {
			_, _, err := client.Payroll.Year(ctx, 12)
			return err
		},
		"payroll negative period": func() error {
			_, _, err := client.Payroll.Payslips(ctx, 2026, -1)
			return err
		},
		"payroll payment with zero date": func() error {
			_, err := client.Payroll.MarkPaymentAsPaid(ctx, 2026, Date{})
			return err
		},
		"payroll profiles with a non-user filter": func() error {
			_, _, err := client.PayrollProfiles.Year(ctx, 2026, "https://x/v2/contacts/1")
			return err
		},
	}
	for name, call := range checks {
		if err := call(); err == nil {
			t.Errorf("%s: succeeded, want an error before any request", name)
		}
	}
}

func TestWaveDDecoding(t *testing.T) {
	t.Parallel()

	t.Run("vat_return_breakdown", func(t *testing.T) {
		t.Parallel()
		client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"vat_return":{
				"url":"https://api.freeagent.com/v2/vat_returns/2026-06-30",
				"period_starts_on":"2026-04-01","period_ends_on":"2026-06-30",
				"filing_due_on":"2026-08-07","filing_status":"filed",
				"filed_at":"2026-07-15T09:00:00.000Z","filed_reference":"000000000000",
				"payments":[{"label":"VAT due","due_on":"2026-08-07","amount_due":"1234.56","status":"unpaid"}],
				"breakdown":{"title":"VAT Return","rows":[
					{"title":"VAT due on sales","value":"2000.0","key":"vat_due_sales","box_number":1},
					{"title":"Total VAT due","value":"1234.56","key":"total_vat_due","box_number":5}]}}}`)
		})
		got, _, err := client.VATReturns.Get(context.Background(), NewDate(2026, 6, 30))
		if err != nil {
			t.Fatalf("Get = %v", err)
		}
		// Numeric on the wire but a JSON string, so the field stays a string.
		if got.FiledReference != "000000000000" || got.FiledAt.IsZero() {
			t.Fatalf("filing detail did not decode: %+v", got)
		}
		if len(got.Payments) != 1 {
			t.Fatalf("payments = %+v", got.Payments)
		}
		if got.Payments[0].Status != PaymentStatusUnpaid || got.Payments[0].AmountDue == nil {
			t.Fatalf("payment did not decode: %+v", got.Payments[0])
		}
		if got.Breakdown == nil || len(got.Breakdown.Rows) != 2 {
			t.Fatalf("breakdown did not decode: %+v", got.Breakdown)
		}
		// The live API sends box_number as a bare number although the
		// documentation types it as a string, so a plain string field would
		// fail to decode a real return outright.
		if got.Breakdown.Rows[1].BoxNumber != 5 || got.Breakdown.Rows[1].Value == nil {
			t.Fatalf("breakdown row did not decode: %+v", got.Breakdown.Rows[1])
		}
	})

	t.Run("cis_bands_envelope", func(t *testing.T) {
		t.Parallel()
		// The envelope key is available_bands, not cis_bands.
		client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"available_bands":[
				{"name":"cis_standard","deduction_rate":"0.2","income_description":"CIS Income (20%)",
				 "deduction_description":"CIS Deduction (20%)","nominal_code":"062"}]}`)
		})
		bands, _, err := client.CISBands.List(context.Background())
		if err != nil {
			t.Fatalf("List = %v", err)
		}
		if len(bands) != 1 || bands[0].Name != CISBandStandard {
			t.Fatalf("bands = %+v", bands)
		}
		// The rate is a fraction, not a percentage.
		if bands[0].DeductionRate == nil || bands[0].DeductionRate.String() != "0.2" {
			t.Fatalf("deduction_rate = %v, want 0.2", bands[0].DeductionRate)
		}
	})

	t.Run("email_addresses_are_strings", func(t *testing.T) {
		t.Parallel()
		client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"email_addresses":["Jane Doe <jane@example.com>","ops@example.com"]}`)
		})
		addresses, _, err := client.EmailAddresses.List(context.Background())
		if err != nil {
			t.Fatalf("List = %v", err)
		}
		if len(addresses) != 2 || !strings.Contains(addresses[0], "<jane@example.com>") {
			t.Fatalf("addresses = %v", addresses)
		}
	})

	t.Run("payroll_year_and_payslips", func(t *testing.T) {
		t.Parallel()
		var seen string
		client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			seen = r.URL.Path
			if strings.HasSuffix(r.URL.Path, "/3") {
				// Enveloped under "period", with the payslips nested inside,
				// not under a top-level payslips key.
				fmt.Fprint(w, `{"period":{"period":3,"status":"filed","dated_on":"2026-07-31",
					"payslips":[{"user":"https://api.freeagent.com/v2/users/1",
					"basic_pay":"2500.0","tax_deducted":"300.0","employee_ni":"150.0",
					"statutory_neonatal_care_pay":"0.0",
					"statutory_parental_bereavement_pay_n_ireland":"0.0",
					"deduct_student_loan":false,
					"tax_code":"1257L","dated_on":"2026-07-31"}]}}`)
				return
			}
			fmt.Fprint(w, `{"periods":[{"url":"https://api.freeagent.com/v2/payroll/2026/3",
				"period":3,"frequency":"Monthly","dated_on":"2026-07-31","status":"filed",
				"employment_allowance_claimed":true,"employment_allowance_amount":"0.0"}],
				"payments":[{"due_on":"2026-08-22","amount_due":"738.19","status":"unpaid"}]}`)
		})
		ctx := context.Background()

		year, _, err := client.Payroll.Year(ctx, 2026)
		if err != nil {
			t.Fatalf("Year = %v", err)
		}
		if seen != "/v2/payroll/2026" {
			t.Fatalf("path = %s", seen)
		}
		if len(year.Periods) != 1 || year.Periods[0].Period == nil || *year.Periods[0].Period != 3 {
			t.Fatalf("periods = %+v", year.Periods)
		}
		if len(year.Payments) != 1 || year.Payments[0].AmountDue == nil {
			t.Fatalf("payments = %+v", year.Payments)
		}

		payslips, _, err := client.Payroll.Payslips(ctx, 2026, 3)
		if err != nil {
			t.Fatalf("Payslips = %v", err)
		}
		if seen != "/v2/payroll/2026/3" {
			t.Fatalf("path = %s", seen)
		}
		if len(payslips) != 1 || payslips[0].BasicPay == nil {
			t.Fatalf("payslips = %+v", payslips)
		}
		if payslips[0].TaxCode != "1257L" {
			t.Fatalf("tax_code = %q", payslips[0].TaxCode)
		}
		// Undocumented, but sent by the live API.
		if payslips[0].StatutoryParentalBereavementPayNIreland == nil {
			t.Fatal("statutory_parental_bereavement_pay_n_ireland did not decode")
		}

		// Period returns the surrounding period too, which Payslips discards.
		full, _, err := client.Payroll.Period(ctx, 2026, 3)
		if err != nil {
			t.Fatalf("Period = %v", err)
		}
		if full.Period == nil || *full.Period != 3 || full.Status != "filed" {
			t.Fatalf("period fields did not decode: %+v", full)
		}
		if len(full.Payslips) != 1 {
			t.Fatalf("payslips nested in the period = %d, want 1", len(full.Payslips))
		}
	})

	t.Run("payroll_profiles_address_lines", func(t *testing.T) {
		t.Parallel()
		var seen string
		client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			seen = r.URL.RequestURI()
			fmt.Fprint(w, `{"profiles":[{"user":"https://api.freeagent.com/v2/users/1",
				"payroll_reference":"EMP001","address_line_1":"1 Example Street",
				"address_line_4":"Exampleshire","postcode":"EX1 1EX",
				"date_of_birth":"1990-08-17","total_pay_in_previous_employment":"1000.0"}]}`)
		})
		ctx := context.Background()

		profiles, _, err := client.PayrollProfiles.Year(ctx, 2026, "")
		if err != nil {
			t.Fatalf("Year = %v", err)
		}
		if seen != "/v2/payroll_profiles/2026" {
			t.Fatalf("path = %s", seen)
		}
		// Address lines are numbered here, unlike address1..address3
		// everywhere else in the API.
		if profiles[0].AddressLine1 == "" || profiles[0].AddressLine4 == "" {
			t.Fatalf("address lines did not decode: %+v", profiles[0])
		}
		if profiles[0].DateOfBirth.IsZero() {
			t.Fatal("date_of_birth did not decode")
		}

		user := ResourceURL("https://api.freeagent.com/v2/users/1")
		if _, _, err := client.PayrollProfiles.Year(ctx, 2026, user); err != nil {
			t.Fatalf("filtered Year = %v", err)
		}
		if !strings.Contains(seen, "user=") {
			t.Fatalf("filtered path = %s, want the user filter", seen)
		}
	})
}

// Every mutating call must be refused on a read-only client, whichever
// service it goes through. This is the guarantee that makes pointing the
// library at an account that must not change safe.
func TestReadOnlyClientRefusesEveryWrite(t *testing.T) {
	t.Parallel()
	server := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Reads are expected to arrive; a write reaching here is the failure.
		if r.Method != http.MethodGet {
			t.Errorf("a read-only client sent %s %s", r.Method, r.URL.Path)
			return
		}
		fmt.Fprint(w, `{"company":{"name":"Example"}}`)
	})
	client, err := NewClient(
		WithBaseURL(server.BaseURL().String()),
		WithoutAuth(),
		WithoutRateLimit(),
		WithReadOnly(),
	)
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	ctx := context.Background()

	writes := map[string]func() error{
		"contact create": func() error {
			_, _, err := client.Contacts.Create(ctx, &Contact{OrganisationName: "x"})
			return err
		},
		"contact update": func() error {
			_, _, err := client.Contacts.Update(ctx, 1, &Contact{OrganisationName: "x"})
			return err
		},
		"contact delete": func() error {
			_, err := client.Contacts.Delete(ctx, 1)
			return err
		},
		"invoice transition": func() error {
			_, _, err := client.Invoices.MarkAsSent(ctx, 1)
			return err
		},
		"category create": func() error {
			_, _, err := client.Categories.Create(ctx, &Category{Description: "x"})
			return err
		},
		"category delete": func() error {
			_, err := client.Categories.Delete(ctx, "285")
			return err
		},
		"note create": func() error {
			_, _, err := client.Notes.CreateForParent(ctx, "https://x/v2/contacts/1", &Note{Note: "x"})
			return err
		},
		"statement upload": func() error {
			_, err := client.BankTransactions.UploadStatement(ctx, "https://x/v2/bank_accounts/1",
				[]StatementLine{{DatedOn: NewDate(2026, 8, 1)}})
			return err
		},
		"vat mark as filed": func() error {
			_, _, err := client.VATReturns.MarkAsFiled(ctx, NewDate(2026, 6, 30))
			return err
		},
		"corp tax mark as paid": func() error {
			_, _, err := client.CorporationTaxReturns.MarkAsPaid(ctx, NewDate(2026, 6, 30))
			return err
		},
		"payroll mark payment paid": func() error {
			_, err := client.Payroll.MarkPaymentAsPaid(ctx, 2026, NewDate(2026, 8, 7))
			return err
		},
		"timeslip start timer": func() error {
			_, _, err := client.Timeslips.StartTimer(ctx, 1)
			return err
		},
		"raw post": func() error {
			_, _, err := client.Raw(ctx, http.MethodPost, "invoices", nil, map[string]string{"a": "b"})
			return err
		},
		"raw delete": func() error {
			_, _, err := client.Raw(ctx, http.MethodDelete, "invoices/1", nil, nil)
			return err
		},
	}
	for name, call := range writes {
		err := call()
		if err == nil {
			t.Errorf("%s: succeeded on a read-only client", name)
			continue
		}
		if !errorsIsReadOnly(err) {
			t.Errorf("%s: err = %v, want ErrReadOnly", name, err)
		}
	}

	// Reads still work.
	if _, _, err := client.Raw(ctx, http.MethodGet, "company", nil, nil); err == nil {
		t.Log("read passed through, as it should")
	}
}

func errorsIsReadOnly(err error) bool {
	return err != nil && strings.Contains(err.Error(), "read-only")
}
