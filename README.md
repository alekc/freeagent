# freeagent-sdk

A Go client for the [FreeAgent v2 API](https://dev.freeagent.com), covering every resource
family, plus `facli`, a small operator CLI.

```go
invoices, _, err := client.Invoices.List(ctx, &freeagent.ListOptions{
	View: freeagent.InvoiceViewOpenOrOverdue,
})
```

FreeAgent publishes no OpenAPI specification and there was no existing Go client, so this is
hand-written and checked against a live account rather than transcribed. That distinction
matters: comparing the documentation to real responses turned up a dozen places where it is
wrong, and two that would break any UK user on their first call. See
[docs/api-quirks.md](docs/api-quirks.md).

## Install

```bash
go get github.com/alekc/freeagent-sdk
```

```go
import "github.com/alekc/freeagent-sdk/freeagent"
```

The package sits one directory down from the module, so the import path carries `freeagent`
twice. Requires Go 1.26.

## Getting started

Credentials, app registration and the OAuth flow are in **[docs/setup.md](docs/setup.md)**.
The short version: register one app at `dev.freeagent.com`, create a free sandbox company,
then

```bash
export FREEAGENT_CLIENT_ID=... FREEAGENT_CLIENT_SECRET=...
make facli && ./bin/facli auth login
```

## Design

**Money is never a float.** Amounts and rates arrive as JSON strings (`"-90.0"`), and as bare
numbers in some reports. Both decode to
[`shopspring/decimal`](https://github.com/shopspring/decimal) and round-trip exactly.

**Records are identified by URL, not id.** Every cross-reference in a payload is a full URL.
`ResourceURL` carries them and offers `ID()` and `Kind()`.

**The API version is pinned.** Sending no `X-Api-Version` opts into pre-versioning behaviour,
which drifts from current documentation, so a date is always sent.

**Rate limits are respected before the server enforces them.** A client-side budget sits
under the published caps, and 429s are retried per `Retry-After`.

**Read-only is enforceable.** `WithReadOnly()` refuses every mutating verb before the request
is built, which matters when pointing the library at real accounting data.

## Examples

### Reading

```go
client, err := freeagent.NewClient(
	freeagent.WithBaseURL(freeagent.Sandbox.BaseURL),
	freeagent.WithTokenSource(source),
	freeagent.WithUserAgent("my-app/1.0"),
)

company, _, err := client.Company.Get(ctx, nil)
me, _, err := client.Users.Me(ctx)
contacts, _, err := client.Contacts.List(ctx, nil)
```

### Paginating

`All` walks the `Link` header and stops cleanly when you break out:

```go
for invoice, err := range client.Invoices.All(ctx, nil) {
	if err != nil {
		return err
	}
	fmt.Println(invoice.Reference, invoice.TotalValue)
}
```

### Reading incrementally

```go
opts := &freeagent.ListOptions{
	UpdatedSince: freeagent.TimeOf(cursor),
	Sort:         "updated_at",
}
for invoice, err := range client.Invoices.All(ctx, opts) {
	// ...
}
```

There is **no deletions feed**: `updated_since` never reports a removed record, and
`/docs/changes` is a human changelog rather than a delta API. Anything mirroring FreeAgent
needs a periodic full-key reconcile to notice deletions.

### Writing

```go
contact, _, err := client.Contacts.Create(ctx, &freeagent.Contact{
	OrganisationName: "Acme Ltd",
	Email:            "billing@acme.example",
})

invoice, _, err := client.Invoices.Create(ctx, &freeagent.Invoice{
	Contact:            contact.URL,
	DatedOn:            freeagent.DateOf(time.Now()),
	PaymentTermsInDays: new(30),
	InvoiceItems: []freeagent.InvoiceItem{{
		Description: "Consultancy",
		ItemType:    "Hours",
		Quantity:    new(decimal.RequireFromString("2")),
		Price:       new(decimal.RequireFromString("125.50")),
	}},
})

sent, _, err := client.Invoices.MarkAsSent(ctx, mustID(invoice.URL))
pdf, _, err := client.Invoices.PDF(ctx, mustID(invoice.URL))
data, err := pdf.Bytes()
```

### Read-only clients

```go
client, err := freeagent.NewClient(
	freeagent.WithBaseURL(freeagent.Production.BaseURL),
	freeagent.WithTokenSource(source),
	freeagent.WithReadOnly(),
)

_, _, err = client.Contacts.Create(ctx, &freeagent.Contact{})
// err is ErrReadOnly; nothing was sent
```

The check sits in request construction, so no typed service, transition or `Raw` call can
write through such a client. It is a client-side guard though: FreeAgent's OAuth has no
read-only scope, so the token itself can still do whatever the user who approved it can.

### Errors

```go
if errors.Is(err, freeagent.ErrValidation) {
	var apiErr *freeagent.APIError
	errors.As(err, &apiErr)
	for _, fieldErr := range apiErr.Errors {
		fmt.Println(fieldErr.Field, fieldErr.Message)
	}
}
```

Sentinels: `ErrUnauthorized`, `ErrForbidden`, `ErrNotFound`, `ErrValidation`,
`ErrRateLimited`, `ErrServer`, `ErrReadOnly`, `ErrNoToken`, `ErrNotAMember`.

### Anything not modelled

```go
body, resp, err := client.Raw(ctx, http.MethodGet, "accounting/transactions", query, nil)
```

## Coverage

Every family is modelled, and every path confirmed against a live account. **Live** means a
populated response has been seen and matched.

### Sales and purchases

| Family | Service | Access | Live |
| --- | --- | --- | --- |
| Contacts | `Contacts` | read/write | yes |
| Projects | `Projects` | read/write | yes |
| Tasks | `Tasks` | read/write (create under a project) | yes |
| Invoices | `Invoices` | read/write, transitions, PDF, duplicate, email | yes |
| Estimates | `Estimates` | read/write, transitions, PDF, duplicate, email | yes |
| Credit Notes | `CreditNotes` | read/write, transitions, PDF, email | yes |
| Credit Note Reconciliations | `CreditNoteReconciliations` | read/write | endpoint only |
| Recurring Invoices | `RecurringInvoices` | read-only | endpoint only |
| Bills | `Bills` | read/write | yes |
| Expenses | `Expenses` | read/write, mileage settings | yes |
| Timeslips | `Timeslips` | read/write, timer start and stop | yes |
| Price List Items | `PriceListItems` | read/write | yes |
| Attachments | `Attachments` | read and delete only | yes |

### Banking

| Family | Service | Access | Live |
| --- | --- | --- | --- |
| Bank Accounts | `BankAccounts` | read/write | yes |
| Bank Transactions | `BankTransactions` | read-only, statement upload | yes |
| Bank Transaction Explanations | `BankTransactionExplanations` | read/write | yes |
| Bank Feeds | `BankFeeds` | read-only | yes |

### Accounting

| Family | Service | Access | Live |
| --- | --- | --- | --- |
| Company | `Company` | read-only, business categories, tax timeline | yes |
| Users | `Users` | read/write, `/users/me` | yes |
| Categories | `Categories` | read/write, grouped, by nominal code | yes |
| Journal Sets | `JournalSets` | read/write, opening balances | yes |
| Transactions | `Transactions` | read-only | yes |
| Trial Balance | `Reports.TrialBalance` | read-only | yes |
| Profit and Loss | `Reports.ProfitAndLoss` | read-only | yes |
| Balance Sheet | `Reports.BalanceSheet` | read-only | yes |
| Cashflow | `Reports.Cashflow` | read-only | yes |
| Final Accounts Reports | `FinalAccountsReports` | read, mark filed | yes |
| Notes | `Notes` | read/write, scoped to a contact or project | yes |

### Assets and stock

| Family | Service | Access | Live |
| --- | --- | --- | --- |
| Capital Assets | `CapitalAssets` | read-only, optional history | endpoint only |
| Capital Asset Types | `CapitalAssetTypes` | read/write | yes |
| Depreciation Profiles | `DepreciationProfile` type | nested, no endpoint | n/a |
| Hire Purchases | `HirePurchases` | read-only | endpoint only |
| Stock Items | `StockItems` | read-only | endpoint only |
| Properties | `Properties` | read/write | needs a landlord company |

### Tax and payroll

| Family | Service | Access | Live |
| --- | --- | --- | --- |
| VAT Returns | `VATReturns` | read, mark filed and paid | yes |
| Corporation Tax Returns | `CorporationTaxReturns` | read, mark filed and paid | yes |
| Income Tax / Self Assessment | `IncomeTaxReturns` | read, mark filed and paid, per user | yes |
| Payroll | `Payroll` | read-only, periods and payslips | yes |
| Payroll Profiles | `PayrollProfiles` | read-only, by tax year | yes |
| CIS Bands | `CISBands` | read-only | needs CIS enrolment |
| Sales Tax (EC MOSS rates) | `SalesTax.ECMossRates` | read-only | yes |
| Sales Tax Periods | `SalesTaxPeriods` | read/write | needs a US company |
| Email Addresses | `EmailAddresses` | read-only | yes |

**endpoint only** means the path, envelope and auth are confirmed but the verification
company had no records, so a populated response has not been seen. Those models come from the
documentation, which this project has repeatedly found to be incomplete: treat their
rarely-used fields as unconfirmed.

The last three rows are gated on company features rather than on anything in this library. A
sandbox company's type is chosen at signup, so the landlord and US cases are closable with a
second free sandbox company.

## facli

```bash
./bin/facli auth login                  # sandbox by default
./bin/facli resources                   # every registered family
./bin/facli get invoices -view open
./bin/facli get contacts -all           # follows pagination
./bin/facli show https://api.sandbox.freeagent.com/v2/invoices/1
./bin/facli raw GET accounting/profit_and_loss/summary -param from_date=2026-01-01
./bin/facli schema vat_returns -follow  # field paths and types, never values
```

`schema` is how to inspect a real company safely: it reports field paths and type
classifications and never reproduces a value, so nothing about the business reaches the
terminal. `-follow` drills into the first record by reading its URL from the payload rather
than printing it.

### Write safety

The tool is deliberately awkward about writes, because the records on the other side are
accounting data:

- It defaults to **sandbox**. Production needs `-env production`.
- Any mutating verb needs `-yes`.
- A mutating verb against production also requires typing the environment name back.
- `-read-only`, or `FREEAGENT_READ_ONLY=1`, refuses writes outright.

Enforced in code, not documented as a warning.

## Testing

```bash
make lint test          # unit tests, no network
make cover
make test-integration   # live, needs a token
```

The unit suite runs against `httptest` and never touches the network. Fixtures in
`freeagent/testdata/` are anonymised captures of real responses; see that directory's README
for what is scrubbed and why the safety net is independent of the field list.

The live suite is build-tagged `integration` and never runs in PR CI. Its read half refuses
production unless `FREEAGENT_ALLOW_PRODUCTION=1`; its write half refuses anything but
sandbox. Writes create records and delete them again, so the sandbox is at baseline after a
run.

`FREEAGENT_CAPTURE=1 make test-integration` additionally rewrites the fixtures from live
responses.

## Documentation

- **[docs/setup.md](docs/setup.md)**: accounts, app registration, OAuth, production notes,
  troubleshooting.
- **[docs/api-quirks.md](docs/api-quirks.md)**: every place the FreeAgent documentation
  disagrees with the API, and the shapes that surprise.
- **[CLAUDE.md](CLAUDE.md)**: contributor context, including the read-only contract.

## Licence

MIT. See [LICENSE](LICENSE).
