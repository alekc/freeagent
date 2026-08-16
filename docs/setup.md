# Setup

Getting credentials, registering the app, and obtaining a token.

## Three accounts, and they are not linked

This is the part that catches people out. You need up to three separate FreeAgent logins,
each on its own system:

| Account | Sign up at | What it is for |
| --- | --- | --- |
| Developer dashboard | <https://dev.freeagent.com> | Registering the app. Issues the client ID and secret. |
| Sandbox company | <https://signup.sandbox.freeagent.com/signup> | The data you develop against, and **the login you approve the app with** |
| Production company | <https://login.freeagent.com> | Your real accounts, once you are ready |

Creating a developer dashboard account does **not** create a sandbox company, and reusing the
same email address does not link them. Their passwords are independent.

If you approve the app using your developer dashboard credentials you get *"The email and
password you entered were incorrect"*, because that login does not exist on the sandbox.

## 1. Create a sandbox company

Sign up at <https://signup.sandbox.freeagent.com/signup>, then sign into it and complete the
setup stages. The upstream quick start is blunt about this: "If you don't do this, you will
receive unexpected error messages when using the API." There is no developer-specific wizard;
it means FreeAgent's ordinary new-company onboarding, such as business type and accounting
dates.

The company **type** is chosen here, and it gates several resource families. A
`UkLimitedCompany` cannot see sales tax periods, and only a `UkUnincorporatedLandlord` can
hold properties. Sandbox companies are free, so create a second one of another type if you
need to exercise those.

## 2. Register an app

Create one at <https://dev.freeagent.com/apps>.

**One app serves both environments**: the same client ID and secret work against sandbox and
production. Which one you reach is decided by the endpoint and by the account you approve
with.

- Add `http://localhost:8723/callback` to the OAuth redirect URIs, or set
  `FREEAGENT_REDIRECT_URI` to one you have registered. FreeAgent has required an exact match
  since February 2017, so the port counts and a trailing slash would be a different URI.
- Leave **Enable Accountancy Practice API** unchecked unless you are building for
  accountants; it changes the rate-limit model to per-client.

## 3. Authorise

```bash
export FREEAGENT_CLIENT_ID=...
export FREEAGENT_CLIENT_SECRET=...

make facli
./bin/facli auth login          # sandbox is the default
./bin/facli auth status
```

`auth login` prints an approval URL, listens on the loopback callback, exchanges the code,
stores the token under a per-environment key so sandbox and production never collide, and
then calls `GET /v2/company` so a stored token is one that has been proven to work.

Sign in with the **sandbox** account at the approval screen, not the dev dashboard one.

Use `-manual` if you would rather paste the `code` parameter than run the local listener.

Tokens are stored as 0600 JSON under your user config directory. `auth status` never prints
the tokens themselves. FreeAgent issues a new refresh token on every refresh and the client
persists it, so a long-lived integration keeps working.

## Production

```bash
./bin/facli auth login -env production
```

Approve with your production FreeAgent login.

**FreeAgent's OAuth has no read-only scope.** A production token can do whatever the user who
approved it can, so treat it accordingly:

- Build clients with `freeagent.WithReadOnly()` when the consumer only reads. The guard
  refuses every mutating verb before the request is constructed.
- `facli` takes `-read-only`, and honours `FREEAGENT_READ_ONLY=1` for a whole shell.
- Use `facli schema <path>` to inspect a real company's field shapes without printing a
  single value.
- Revoke the app's access from FreeAgent's settings, under connected or authorised apps, when
  you no longer need it.

## direnv

A local `.envrc` keeps the credentials out of your shell history. It is gitignored.

```bash
export FREEAGENT_CLIENT_ID="..."
export FREEAGENT_CLIENT_SECRET="..."
export FREEAGENT_REDIRECT_URI="http://localhost:8723/callback"
```

```bash
chmod 600 .envrc && direnv allow
```

## Troubleshooting

| Symptom | Cause |
| --- | --- |
| "The email and password you entered were incorrect" on the approval screen | Signing in with the developer dashboard login instead of the company one |
| "The redirect URI is invalid for this app" | The URI is not registered, or differs by port or trailing slash |
| Unexpected errors on the first API calls | The sandbox company has not completed its setup stages |
| Requests hang with no TLS handshake, on macOS | An outbound firewall such as Little Snitch is gating the freshly built binary. `make test-integration` compiles to a fixed path so you authorise once rather than every run. |
| 429 with `Retry-After` | Rate limited. The client backs off automatically; 120 requests/minute and 3600/hour, per user. |
