# Working on freeagent-sdk

Context for anyone, human or agent, picking this repository up. Read this before changing
the library; it records decisions whose reasons are not visible in the diff.

## What this is

A Go client for the FreeAgent v2 API, plus `facli`, a small operator CLI. It is a library
only: there is deliberately no sync engine, no scheduler and no storage here. Those belong in
whatever consumes this.

Go 1.26 is the floor. It uses `new(expr)` and range-over-func iterators, and tracks the
current release rather than carrying compatibility weight.

## The read-only switch is a load-bearing contract

`WithReadOnly()` makes a client refuse every mutating verb. It exists because this library
gets pointed at real accounting data, and the sync and MCP projects downstream will do that
routinely with no human watching the request stream.

**The check lives in `Client.newRequest`, and every request in the library must keep going
through it.** That is the whole guarantee: not that each service remembers to check, but that
there is exactly one place a request can be built and it refuses writes there.

So, whenever you add anything:

- Build requests with `c.newRequest`. Never construct an `*http.Request` another way, never
  call `c.httpClient.Do` directly, and never add a bypass "just for" some endpoint.
- If you add a verb beyond GET, HEAD and OPTIONS anywhere, `readMethod` in `client.go` is the
  allowlist that decides. It is an allowlist on purpose: an unrecognised verb counts as a
  write.
- Add the new call to `TestReadOnlyClientRefusesEveryWrite` in `waved_test.go`. That test
  drives every category of write through a read-only client and asserts each is refused. A
  new write path that is not in it is untested, and the guarantee quietly weakens.

`facli` exposes the same thing as `-read-only` and honours `FREEAGENT_READ_ONLY=1`. A
consumer that only reads should build its client with `WithReadOnly()` rather than relying on
calling the right methods.

Two things it is not. It is a client-side guard, so it protects against your own bugs, not
against a leaked token: FreeAgent's OAuth has no read-only scope, and a token can do whatever
the user who approved it can. And it says nothing about rate limits, which still apply.

## Non-negotiables

**Money is never a float.** Amounts and rates arrive as JSON strings, and sometimes as bare
numbers in the reports. `Decimal` (shopspring) takes both. Do not introduce `float64` for a
monetary value, ever.

**`Date` and `Time` fields use `omitzero`, never `omitempty`.** `omitempty` has no effect on
struct types, so a zero date serialises as `null` and is sent on every write. That broke
capital asset types, which reject read-only fields outright. `TestNoDateOrTimeFieldUsesOmitempty`
walks the source and fails if it comes back.

**Records are identified by URL, not id.** Cross-references are `ResourceURL`, which offers
`ID()` and `Kind()`. Following one is restricted to the configured host, because the URLs come
out of API responses.

**Never commit anything from a real company.** Fixtures in `freeagent/testdata/` are
anonymised captures, produced by the integration suite through `internal/anonymise`, which
re-checks its own output against a denylist read from the live account and refuses to write
if a value survived. To inspect a real company, use `facli schema`, which reports field paths
and type classifications and never a value.

## Layout

```
freeagent/            the library, one file per resource family
  client.go           transport, options, retry, the read-only check
  collection.go       ReadCollection[T] and Collection[T] generics
  registry.go         ResourceMeta for every family, shared with facli
  services.go         the typed services hanging off Client
  period_reports.go   the date-addressed filing families
  testdata/           anonymised fixtures, see its own README
internal/anonymise/   scrubber for captured payloads
internal/shape/       value-free payload summariser behind facli schema
cmd/facli/            the CLI
docs/                 setup and API quirks
```

## Adding a resource family

1. Read the upstream docs, then **check them against a live response**. The documentation is
   a starting point, not a contract: this repository has found a dozen places where it is
   wrong or incomplete. `docs/api-quirks.md` lists them.
2. Write the struct with exact JSON tags. `Decimal` for money, `Date`/`Time` with `omitzero`,
   `ResourceURL` for references, pointers for optional writes.
3. Add a `ResourceMeta` to `registry.go`. Set `ReadOnly`, `NoList`, `Grouped`,
   `RequiresBankAccount` or `CustomEnvelope` where they apply; `facli` reads the same entry.
4. Embed `Collection[T]`, or `ReadCollection[T]` if the API has no per-record writes. Only
   hand-write a service when the family genuinely does not fit, and say why in its doc
   comment.
5. Wire it into `services.go`.
6. Cover it in the integration suite, and add a golden fixture if it can be captured.

If an endpoint needs a filter to work at all, shadow the inherited method so it fails locally
with a pointer to the right one, as bank transactions and notes do. Failing in the caller's
own process beats a remote 400.

## Testing

```bash
make lint test            # unit, no network
make test-integration     # live, needs a token; see docs/setup.md
```

The live suite is build-tagged `integration` and never runs in PR CI. Its read half refuses
production unless `FREEAGENT_ALLOW_PRODUCTION=1`; its write half refuses anything but
sandbox, outright.

`make test-integration` compiles to `bin/integration.test` rather than a temp path, so an
outbound firewall has one stable binary to authorise.

Write tests create records and delete them again through cleanups registered at creation, so
a failure part way through still tidies up. Keep that pattern: the sandbox should be at
baseline after every run.

Assert **shape and invariants**, not the values of one capture. A re-capture must never break
a test; a wrong tag or type always should. Where a real invariant exists, assert it: a trial
balance sums to zero, an unbalanced journal set is refused.

## Conventions

Commit messages explain **why**, not what. Sign off with `git commit -s`. No AI attribution
trailers of any kind.

Comments cap at 80 characters per line and four lines per block, and say what the code
cannot: the consequence of getting it wrong, or the upstream quirk being worked around.

No em dashes in anything the repository publishes.
