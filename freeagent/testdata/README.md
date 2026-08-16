# Golden fixtures

These are **real responses from a live FreeAgent sandbox company, anonymised**. They are not
hand-written, and not derived from the published examples: only a real response carries the
fields the documentation omits, which is a category of bug we have already hit eight times.

## How they are produced

They are written by the integration suite, while the records it creates still exist:

```bash
FREEAGENT_CAPTURE=1 make test-integration
```

Capture is off by default, so an ordinary integration run never rewrites them.

## How they are made safe

Every payload passes through `internal/anonymise` before it reaches disk. That does two
things:

1. Replaces the values of known-identifying fields (company name, registration number,
   address, contact details, bank details, tax references) with placeholders that keep the
   format, and rewrites the sandbox host to the production one.
2. Re-scans the scrubbed output for a denylist read from the live account itself, and
   **refuses to write anything that still contains one of those values**.

The second pass is the one that matters. Any list of sensitive field names will be
incomplete, so the safety net has to be independent of it. Presigned attachment URLs are
scrubbed too: they carry an access key and a signature, so they are credentials rather than
links.

## Writing tests against them

Assert shape and invariants, not the values of one particular capture. A re-capture should
never break a test; a wrong `json` tag or Go type always should. See
`TestWaveAGoldenDecoding`.
