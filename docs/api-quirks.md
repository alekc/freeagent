# API quirks

Everything here was found by comparing the FreeAgent documentation against live responses
while building this library. It is recorded because the documentation is a starting point,
not a contract, and because most of these are invisible until they bite.

The SDK already handles all of them. This is here so you know why the models look as they do,
and what to expect if you go around the SDK with `Client.Raw`.

## Where the documentation is wrong

| Where | What the docs say | What the API does |
| --- | --- | --- |
| VAT return `box_number` | a string | a **bare JSON number**. A `string` field fails to decode a real return. |
| Payroll, single period | payslips under a `payslips` key | enveloped under `period`, payslips **nested inside**. Decoding the documented shape yields nothing, with no error. |
| Company `id` | integer, but the example quotes it | an unquoted number. The example is the stale half. |
| Tasks, delete | `DELETE /v2/users/:id` | `/v2/tasks/:id`. A copy-paste slip. |
| Payroll, mark unpaid | `GET` | `PUT`, like every other transition. |
| Stock items | write endpoints implied | read-only. `POST` answers **404**, not 405. |
| Self Assessment Returns | its own resource | the same resource as Income Tax Returns. Neither `/v2/self_assessment_returns` nor `/v2/income_tax_returns` exists; it is nested under a user. |
| Depreciation Profiles | has a documentation page | has no endpoint. It is a nested object. |
| Currencies | listed as a resource | `/v2/currencies` is a **404**. The page is a list of ISO codes. |
| Sales Tax | listed as a resource | prose. Its one endpoint is the EC VAT MOSS rate lookup. |
| Journal set `bank_accounts` | "Array" | an array of **objects** carrying their own value, not references. |
| Property `name` | absent from the attribute table | present in the same page's example, and real. |

## Fields the API sends that the documentation omits

Found by diffing live payload keys against the models. All are now modelled.

- **Company**: `locale`, `ec_vat_reporting_enabled`, `supports_auto_sales_tax_on_purchases`
- **User**: `hidden`
- **BankAccount**: `total_count`, `unexplained_transaction_count`, `marked_for_review_count`,
  `manually_added_transaction_count`
- **Payslip**: `statutory_parental_bereavement_pay_n_ireland`

Expect more in families that have never been seen populated.

## Inconsistencies to plan around

**Money is a string, except when it is a number.** Trial balance and profit and loss send
quoted strings; balance sheet and cashflow send bare numbers. Statement upload *requires* a
bare number, and sending the quoted string every other money field uses is accepted with a
200 and then imports nothing at all. `Decimal` reads both; `StatementLine` exists to write
the number form.

**Paths are not uniform.** Cashflow is at `/v2/cashflow` while the other reports are under
`accounting/`. Transactions are under `accounting/` while every other collection is at the
root. The reports are `/v2/accounting/profit_and_loss/summary`, not the obvious guess.

**Some families are addressed by a date, not an id.** Final accounts, VAT, corporation tax
and income tax returns all use the period end date, so `ResourceURL.ID()` does not apply to
them. Categories use a nominal code, and payroll uses a tax year.

**Some endpoints need a filter to work at all.** Bank transactions and their explanations
reject a list without `bank_account`. Notes need a `contact` or `project`, on create as well
as on list. The SDK shadows the unfiltered methods so they fail locally.

## Required fields the docs do not mark

- **Estimates** reject a create without an explicit `status`: `422 status is not valid`.
  Invoices do not.
- **Categories** need `category_group` (`admin_expenses`, `cost_of_sales`, `income`,
  `general`), a free `nominal_code` inside that group's range, and a `tax_reporting_name`
  from an unpublished list that is *narrower than the set already in use on the account*.
  Borrow one from an existing category in the same group and expect some to be rejected.

## Timing and lifecycle

**Statement import is asynchronous.** The 200 returns before the lines are queryable, by
anywhere from under a second to most of a minute. Poll rather than trusting the status.

**Deletion has an order.** An **Approved** estimate cannot be deleted (409), and a contact
linked to any surviving estimate cannot be deleted either (403). Return the estimate to
draft first. A **system default** capital asset type can never be deleted.

**Capital asset types reject read-only fields outright**: sending `created_at`, even as
`null`, is a `400 found unpermitted parameters`. Most endpoints silently ignore them, which
is why a Go `omitempty` bug on struct-typed dates went unnoticed everywhere else.

**The transactions date range must fall inside one accounting period**, which is stricter
than the documented 12-month limit. Derive the bounds from `Company.AnnualAccountingPeriods`
rather than counting back from today.

## Shapes that surprise

- **Categories** come back grouped under four envelope keys with no flat list, and a single
  `GET` is *also* nested under whichever group applies.
- **CIS bands** answer under `available_bands`, not `cis_bands`.
- **Email addresses** are an array of plain strings, not objects.
- **Mileage settings** are enveloped and historical: dated ranges of engine-size options and
  rates, with a scalar `basic_rate_limit` sharing an object with the per-vehicle entries.
- **The opening balances journal set** carries no date, and its bank and stock legs live in
  separate arrays, so `journal_entries` alone does not sum to zero.
- **Corporation tax** carries its payment on the return itself, where VAT and income tax use
  a dated `payments` array.
- **Trial balance** returns an array where the other reports return an object, and its
  internal `nominal_code` differs from the `display_nominal_code` shown to users.

## Gated by company type

| Family | Needs |
| --- | --- |
| Properties | `UkUnincorporatedLandlord` |
| Sales Tax Periods | a US or Universal company; a **404** on a UK one |
| Hire Purchases, CIS Bands | UK companies |
| VAT Returns | a VAT-registered company (empty otherwise) |
| Payroll, Payroll Profiles | a UK company running payroll |

A sandbox company's type is chosen at signup, so these are testable with a second free
sandbox company rather than a real business of that kind.

## Rate limits

120 requests per minute, 3600 per hour, and 15 token refreshes per minute, **per end user**.
429 carries `Retry-After`. The client keeps a budget under those caps and honours the
header. Sending `X-RateLimit-Test: true` drops the sandbox budget to 5 per minute so the
back-off path can be exercised for real.
