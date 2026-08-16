# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Nothing yet.

## [0.1.0] - 2026-08-16

### Added

- Transport core: client, functional options, request building, error decoding.
- Generic `Collection[T]` and `Reader[T]` providing CRUD and pagination for every
  resource family without per-resource boilerplate.
- Rate limiting sized under the published FreeAgent caps, with `Retry-After` aware
  retry on 429 and jittered backoff on 5xx.
- OAuth 2.0 token source with transparent refresh, rotated refresh-token persistence,
  and a 0600 file-backed token store.
- `facli`, a small CLI for the authorisation dance and manual API access.
- Typed models and services for the first 14 resource families: Company, Users, Categories,
  Contacts, Projects, Tasks, Invoices, Estimates, Bills, Expenses, Bank Accounts, Bank
  Transactions, Bank Transaction Explanations and Attachments.
- Invoice and estimate transitions, PDF rendering and send-email; `/users/me`; company
  business categories and tax timeline; expense mileage settings; bank statement upload.
- `ReadCollection[T]` for families with no per-record writes, and `Int64` for fields the API
  returns as either a number or a numeric string.
- `StatementLine`, the shape the statement upload endpoint actually takes. Its `amount` is a
  JSON number rather than the quoted string every other money field uses.
- The ledger and reporting families: Journal Sets with opening balances, Transactions, Credit
  Notes with transitions and PDF, Credit Note Reconciliations, Recurring Invoices, Timeslips
  with timer start and stop, and Final Accounts Reports.
- `client.Reports` for the four accounting reports: `TrialBalance`, `ProfitAndLoss`,
  `BalanceSheet` and `Cashflow`. They have no id segment and each answer with their own
  shape, so they are gathered on one service rather than forced through the collection
  generics.
- The asset and stock families: Capital Assets and Capital Asset Types, Hire Purchases, Stock
  Items, Price List Items, Properties, Notes, and the EC VAT MOSS rate lookup on
  `client.SalesTax`. The `DepreciationProfile` type, which has a documentation page but no
  endpoint of its own.
- The tax and payroll families: VAT Returns, Corporation Tax Returns, Income Tax Returns
  (which is also Self Assessment), Payroll periods and payslips, Payroll Profiles, Bank Feeds,
  CIS Bands, Sales Tax Periods and Email Addresses. The four date-addressed filing families
  share one `periodService[T]`.
- `WithReadOnly()`, which refuses every mutating verb at request construction, and its
  `facli` counterparts `-read-only` and `FREEAGENT_READ_ONLY=1`.
- `internal/anonymise`, which scrubs captured payloads so real responses can be committed as
  fixtures, with an independent second pass that refuses to emit anything still containing a
  value from the live account.
- `facli schema`, which reports a payload's field paths and type classifications and never a
  value, so a real company can be inspected without its data reaching the terminal.
- `statutory_parental_bereavement_pay_n_ireland` on `Payslip`, undocumented but sent live.
- Eight fields the live API returns but the documentation omits: `locale`,
  `ec_vat_reporting_enabled` and `supports_auto_sales_tax_on_purchases` on Company, `hidden`
  on User, and four transaction tallies on BankAccount.
- Build-tagged `integration` suite covering read and write against a live sandbox company:
  every typed collection, `updated_since`, the accounting reports, a forced token refresh
  asserting the rotated refresh token is persisted, a journal set that must balance with a
  negative test proving an unbalanced one is refused, and the full create-and-delete path for
  contacts, projects, tasks, invoices, estimates, bills and expenses. Everything created is
  deleted again.
- Contributor and user documentation: `CLAUDE.md`, `docs/setup.md`, `docs/api-quirks.md`, and
  a README carrying the per-family coverage table.

### Changed

- Requires Go 1.26. The library uses `new(expr)` and range-over-func iterators.

### Fixed

- **A VAT return's `box_number` did not decode.** The live API sends a bare number where the
  documentation types it as a string, so any real VAT return failed to unmarshal. It is now
  `Int64`, which takes either.
- **`Payroll.Payslips` always returned nothing.** A single period is enveloped under `period`
  with its payslips nested inside, not under a top-level `payslips` key, so the documented
  shape decoded to an empty slice with no error. `Payroll.Period` now returns the whole
  period and `Payslips` reads from it.
- **Every `Date` and `Time` field was serialising as `null` on writes.** `omitempty` has no
  effect on struct types, so 100 fields sent a null on every create and update. Most
  endpoints ignored them; capital asset types answer `400 found unpermitted parameters`. All
  of them now use `omitzero`, and a test fails if `omitempty` reappears on one.
- `StockItemService` was modelled with write verbs the API does not have: `POST` answers 404.
  It is now a `ReadCollection`.
- `MileageSettings` was modelled from a guess and decoded to nothing. It is enveloped and
  historical: dated ranges of engine-size options and rates.

### Notes

- Read and write paths are both verified against a live sandbox, and the fixtures in
  `testdata/` are now anonymised captures rather than doc-derived.
- The library is the module's root package, so the import path is
  `github.com/alekc/freeagent`. A first cut briefly carried the module path
  `github.com/alekc/freeagent-sdk`; that path is abandoned and should not be used.
- Estimates reject a create without an explicit `status`, unlike invoices. Category creates
  need `category_group`, a free in-range `nominal_code` and a `tax_reporting_name` from an
  unpublished list. Statement import is asynchronous. These and every other doc-versus-reality
  discrepancy are in `docs/api-quirks.md`.
- Six families have a confirmed path and envelope but were never seen populated, because no
  verification company had records: Credit Note Reconciliations, Recurring Invoices, Capital
  Assets, Hire Purchases, Stock Items, and Properties. The README coverage table marks them.
- VAT returns, bank feeds, payroll and payroll profiles were additionally verified read-only
  against a real VAT-registered company using `facli schema`, which reports field paths and
  types and never a value. No production data was captured or committed.

[Unreleased]: https://github.com/alekc/freeagent/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/alekc/freeagent/releases/tag/v0.1.0
