//go:build integration

// Wave D live coverage: tax filings, payroll, CIS and bank feeds.
//
// This wave is read-only by design. Every write it exposes files or unfiles a
// tax return or records a payment to HMRC, none of which belongs in a test.
// The transitions are covered against a fake server in waved_test.go instead.
package freeagent

import (
	"testing"
	"time"
)

func TestLiveWaveDReads(t *testing.T) {
	client := liveClient(t)

	t.Run("vat_returns", func(t *testing.T) {
		ctx := liveContext(t)
		returns, _, err := client.VATReturns.List(ctx)
		if err != nil {
			t.Fatalf("VATReturns.List: %v", err)
		}
		if len(returns) == 0 {
			t.Skip("company is not VAT registered, so there are no returns to inspect")
		}
		first := returns[0]
		if first.PeriodEndsOn.IsZero() || first.FilingStatus == "" {
			t.Fatalf("return is incomplete: %+v", first)
		}
		// Addressed by date, so the URL is not a numeric member URL.
		if _, err := first.URL.ID(); err == nil {
			t.Fatalf("URL %q parsed as a numeric member URL", first.URL)
		}
		got, _, err := client.VATReturns.Get(ctx, first.PeriodEndsOn)
		if err != nil {
			t.Fatalf("VATReturns.Get(%s): %v", first.PeriodEndsOn, err)
		}
		if got.Breakdown != nil {
			for _, row := range got.Breakdown.Rows {
				if row.Key == "" || row.Value == nil {
					t.Fatalf("breakdown row is incomplete: %+v", row)
				}
			}
			t.Logf("breakdown %q with %d row(s)", got.Breakdown.Title, len(got.Breakdown.Rows))
		}
		t.Logf("%d VAT return(s); %s is %q with %d payment(s)",
			len(returns), first.PeriodEndsOn, first.FilingStatus, len(first.Payments))
	})

	t.Run("corporation_tax_returns", func(t *testing.T) {
		ctx := liveContext(t)
		returns, _, err := client.CorporationTaxReturns.List(ctx)
		if err != nil {
			t.Fatalf("CorporationTaxReturns.List: %v", err)
		}
		if len(returns) == 0 {
			t.Skip("no corporation tax returns on this company")
		}
		first := returns[0]
		if first.PeriodEndsOn.IsZero() || first.FilingStatus == "" {
			t.Fatalf("return is incomplete: %+v", first)
		}
		// Corporation tax carries its payment on the return itself, unlike
		// VAT and income tax which use a payments array.
		if first.AmountDue == nil {
			t.Fatalf("amount_due did not decode: %+v", first)
		}
		got, _, err := client.CorporationTaxReturns.Get(ctx, first.PeriodEndsOn)
		if err != nil {
			t.Fatalf("CorporationTaxReturns.Get(%s): %v", first.PeriodEndsOn, err)
		}
		if !got.PeriodEndsOn.Time.Equal(first.PeriodEndsOn.Time) {
			t.Fatalf("fetched %s, want %s", got.PeriodEndsOn, first.PeriodEndsOn)
		}
		t.Logf("%d return(s); %s is %q, %v due %s",
			len(returns), first.PeriodEndsOn, first.FilingStatus, first.AmountDue, first.PaymentDueOn)
	})

	// The documentation has two pages for this and neither path in their
	// titles exists: the collection is nested under a user.
	t.Run("income_tax_returns", func(t *testing.T) {
		ctx := liveContext(t)
		me, _, err := client.Users.Me(ctx)
		if err != nil {
			t.Fatalf("Users.Me: %v", err)
		}
		returns, _, err := client.IncomeTaxReturns.ListForUser(ctx, me.URL)
		if err != nil {
			t.Fatalf("IncomeTaxReturns.ListForUser: %v", err)
		}
		if len(returns) == 0 {
			t.Skip("no self assessment returns for this user")
		}
		first := returns[0]
		if first.PeriodEndsOn.IsZero() || first.FilingStatus == "" {
			t.Fatalf("return is incomplete: %+v", first)
		}
		for _, payment := range first.Payments {
			if payment.DueOn.IsZero() || payment.AmountDue == nil {
				t.Fatalf("payment is incomplete: %+v", payment)
			}
		}
		got, _, err := client.IncomeTaxReturns.GetForUser(ctx, me.URL, first.PeriodEndsOn)
		if err != nil {
			t.Fatalf("IncomeTaxReturns.GetForUser: %v", err)
		}
		if got.FilingStatus == "" {
			t.Fatal("fetched return has no filing status")
		}
		t.Logf("%d return(s) for the current user; %s is %q with %d payment(s)",
			len(returns), first.PeriodEndsOn, first.FilingStatus, len(first.Payments))

		// A non-user URL must be refused before a request is built.
		if _, _, err := client.IncomeTaxReturns.ListForUser(ctx, "https://x/v2/contacts/1"); err == nil {
			t.Fatal("a contact was accepted as the scope for income tax returns")
		}
	})

	t.Run("bank_feeds", func(t *testing.T) {
		ctx := liveContext(t)
		feeds, _, err := client.BankFeeds.List(ctx, nil)
		if err != nil {
			t.Fatalf("BankFeeds.List: %v", err)
		}
		if len(feeds) == 0 {
			t.Skip("no bank feeds connected on this company")
		}
		for _, feed := range feeds {
			if feed.BankAccount.Kind() != "bank_accounts" {
				t.Fatalf("feed points at %q, not a bank account", feed.BankAccount)
			}
			if feed.State == "" || feed.FeedType == "" {
				t.Fatalf("feed is incomplete: %+v", feed)
			}
		}
		t.Logf("%d feed(s); first is %s via %q", len(feeds), feeds[0].FeedType, feeds[0].BankServiceName)
	})

	t.Run("cis_bands", func(t *testing.T) {
		ctx := liveContext(t)
		bands, _, err := client.CISBands.List(ctx)
		if err != nil {
			t.Fatalf("CISBands.List: %v", err)
		}
		if len(bands) == 0 {
			t.Skip("company is not enrolled in CIS for subcontractors")
		}
		for _, band := range bands {
			if band.Name == "" || band.DeductionRate == nil || band.NominalCode == "" {
				t.Fatalf("band is incomplete: %+v", band)
			}
		}
		t.Logf("%d band(s); first %q at rate %v", len(bands), bands[0].Name, bands[0].DeductionRate)
	})

	t.Run("email_addresses", func(t *testing.T) {
		ctx := liveContext(t)
		addresses, _, err := client.EmailAddresses.List(ctx)
		if err != nil {
			t.Fatalf("EmailAddresses.List: %v", err)
		}
		if len(addresses) == 0 {
			t.Skip("no verified sender addresses")
		}
		// Plain strings, not objects: the shape is the assertion.
		for _, address := range addresses {
			if address == "" {
				t.Fatal("an empty sender address decoded")
			}
		}
		t.Logf("%d verified sender address(es)", len(addresses))
	})

	t.Run("payroll", func(t *testing.T) {
		ctx := liveContext(t)
		year := time.Now().Year()
		payroll, _, err := client.Payroll.Year(ctx, year)
		if err != nil {
			t.Fatalf("Payroll.Year(%d): %v", year, err)
		}
		t.Logf("%d period(s) and %d payment(s) in %d",
			len(payroll.Periods), len(payroll.Payments), year)

		if len(payroll.Periods) == 0 {
			t.Skip("no payroll periods in this tax year")
		}
		period := payroll.Periods[0]
		if period.Period == nil {
			t.Fatalf("period number did not decode: %+v", period)
		}
		payslips, _, err := client.Payroll.Payslips(ctx, year, *period.Period)
		if err != nil {
			t.Fatalf("Payroll.Payslips(%d, %d): %v", year, *period.Period, err)
		}
		for _, payslip := range payslips {
			if payslip.User.Kind() != "users" {
				t.Fatalf("payslip user = %q", payslip.User)
			}
			if payslip.BasicPay == nil {
				t.Fatalf("basic_pay did not decode: %+v", payslip)
			}
		}
		t.Logf("period %d has %d payslip(s)", *period.Period, len(payslips))
	})

	t.Run("payroll_profiles", func(t *testing.T) {
		ctx := liveContext(t)
		year := time.Now().Year()
		profiles, _, err := client.PayrollProfiles.Year(ctx, year, "")
		if err != nil {
			t.Fatalf("PayrollProfiles.Year(%d): %v", year, err)
		}
		t.Logf("%d payroll profile(s) in %d", len(profiles), year)
		if len(profiles) == 0 {
			t.Skip("no payroll profiles in this tax year")
		}
		for _, profile := range profiles {
			if profile.User.Kind() != "users" {
				t.Fatalf("profile user = %q", profile.User)
			}
		}
		// The user filter narrows the same endpoint.
		me, _, err := client.Users.Me(ctx)
		if err != nil {
			t.Fatalf("Users.Me: %v", err)
		}
		mine, _, err := client.PayrollProfiles.Year(ctx, year, me.URL)
		if err != nil {
			t.Fatalf("PayrollProfiles.Year filtered: %v", err)
		}
		if len(mine) > len(profiles) {
			t.Fatalf("the user filter widened the result: %d vs %d", len(mine), len(profiles))
		}
	})

	// US and Universal companies only. Pinning the 404 documents why this
	// family cannot be verified from a UK account.
	t.Run("sales_tax_periods", func(t *testing.T) {
		ctx := liveContext(t)
		company, _, err := client.Company.Get(ctx, nil)
		if err != nil {
			t.Fatalf("Company.Get: %v", err)
		}
		_, _, err = client.SalesTaxPeriods.List(ctx, nil)
		switch {
		case err == nil:
			t.Logf("sales tax periods are available on this %s company", company.Type)
		default:
			t.Logf("sales tax periods unavailable on a %s company, as expected: %v",
				company.Type, err)
		}
	})
}
