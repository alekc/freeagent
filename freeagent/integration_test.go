//go:build integration

// Live suite against the FreeAgent sandbox. It is build-tagged out of the
// normal run because it needs real credentials and spends the per-user rate
// limit, so it must never be wired into PR CI.
//
// Run it with:
//
//	facli auth login          # once, to obtain a token
//	make test-integration
//
// Everything here is read-only. Write coverage against a live account is a
// separate decision, because the records on the other side are real.
package freeagent

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"
)

// liveClient builds a client from the same token file facli writes. The suite
// skips rather than fails when the environment is not set up, so an
// accidental `-tags=integration` run is not a red build.
func liveClient(t *testing.T) *Client {
	t.Helper()

	id := os.Getenv("FREEAGENT_CLIENT_ID")
	secret := os.Getenv("FREEAGENT_CLIENT_SECRET")
	if id == "" || secret == "" {
		t.Skip("FREEAGENT_CLIENT_ID and FREEAGENT_CLIENT_SECRET are not set")
	}

	env := Sandbox
	if name := os.Getenv("FREEAGENT_ENV"); name != "" {
		resolved, err := EnvironmentByName(name)
		if err != nil {
			t.Fatalf("FREEAGENT_ENV: %v", err)
		}
		env = resolved
	}
	// Guard rather than trust: this suite is read-only, but pointing it at
	// production still spends a real user's rate limit.
	if env.Name == Production.Name && os.Getenv("FREEAGENT_ALLOW_PRODUCTION") != "1" {
		t.Skip("refusing to run against production without FREEAGENT_ALLOW_PRODUCTION=1")
	}

	path := os.Getenv("FREEAGENT_TOKEN_FILE")
	if path == "" {
		var err error
		path, err = DefaultTokenPath()
		if err != nil {
			t.Fatalf("DefaultTokenPath: %v", err)
		}
	}
	store, err := NewFileStore(path, env.Name)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	if _, err := store.Load(context.Background()); err != nil {
		t.Skipf("no usable token in %s for %s: %v (run: facli auth login)", path, env.Name, err)
	}

	source, err := NewTokenSource(context.Background(), env.OAuthConfig(id, secret, ""), store)
	if err != nil {
		t.Fatalf("NewTokenSource: %v", err)
	}
	client, err := NewClient(
		WithBaseURL(env.BaseURL),
		WithTokenSource(source),
		WithUserAgent("freeagent-sdk-go integration test"),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// The company record is the cheapest proof that credentials, transport and
// decoding all work end to end. Populated accounting dates also confirm the
// account finished its setup stages, which FreeAgent warns about.
func TestLiveCompany(t *testing.T) {
	client := liveClient(t)
	ctx := liveContext(t)

	company, resp, err := client.Company.Get(ctx, nil)
	if err != nil {
		t.Fatalf("Company.Get: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if company.Name == "" {
		t.Fatal("company has no name")
	}
	if company.ID == 0 {
		t.Fatal("company has no id, which would mean Int64 failed to decode it")
	}
	if company.Currency == "" || company.Type == "" {
		t.Fatalf("company is missing currency or type: %+v", company)
	}
	if company.CompanyStartDate.IsZero() || company.FreeAgentStartDate.IsZero() {
		t.Fatal("accounting dates are unset, so the account has not completed its setup stages")
	}
	if company.CreatedAt.IsZero() {
		t.Fatal("created_at did not decode")
	}
	t.Logf("company %q, type %s, currency %s, start %s",
		company.Name, company.Type, company.Currency, company.CompanyStartDate)
}

func TestLiveUsers(t *testing.T) {
	client := liveClient(t)
	ctx := liveContext(t)

	users, _, err := client.Users.List(ctx, nil)
	if err != nil {
		t.Fatalf("Users.List: %v", err)
	}
	if len(users) == 0 {
		t.Fatal("no users, which should be impossible on an account you can authenticate to")
	}
	for _, user := range users {
		if user.URL.IsZero() {
			t.Fatal("user has no url")
		}
		if _, err := user.URL.ID(); err != nil {
			t.Fatalf("user url %q is not a member URL: %v", user.URL, err)
		}
	}

	me, _, err := client.Users.Me(ctx)
	if err != nil {
		t.Fatalf("Users.Me: %v", err)
	}
	if me.URL.IsZero() || me.Email == "" {
		t.Fatalf("me is incomplete: %+v", me)
	}
	t.Logf("%d user(s); me is %s (%s)", len(users), me.Email, me.Role)
}

// Categories are the awkward shape: four envelope keys, no flat list, and
// nominal-code addressing. Every account has a seeded chart of accounts, so
// this exercises the whole path including a Get by code.
func TestLiveCategories(t *testing.T) {
	client := liveClient(t)
	ctx := liveContext(t)

	groups, _, err := client.Categories.List(ctx, false)
	if err != nil {
		t.Fatalf("Categories.List: %v", err)
	}
	flat := groups.Flatten()
	if len(flat) == 0 {
		t.Fatal("no categories, but every account has a seeded chart of accounts")
	}
	for _, category := range flat {
		if category.NominalCode == "" {
			t.Fatalf("category has no nominal code: %+v", category)
		}
		if category.Group == "" {
			t.Fatalf("category %s has no group", category.NominalCode)
		}
	}

	// Fetch one back by code and confirm the group resolves the same way.
	want := flat[0]
	got, _, err := client.Categories.Get(ctx, want.NominalCode)
	if err != nil {
		t.Fatalf("Categories.Get(%s): %v", want.NominalCode, err)
	}
	if got.NominalCode != want.NominalCode {
		t.Fatalf("nominal code = %q, want %q", got.NominalCode, want.NominalCode)
	}
	if got.Group != want.Group {
		t.Fatalf("group = %q, want %q", got.Group, want.Group)
	}
	t.Logf("%d categories across %d groups", len(flat), 4)
}

func TestLiveBankAccountsAndTransactions(t *testing.T) {
	client := liveClient(t)
	ctx := liveContext(t)

	accounts, _, err := client.BankAccounts.List(ctx, nil)
	if err != nil {
		t.Fatalf("BankAccounts.List: %v", err)
	}
	if len(accounts) == 0 {
		t.Skip("no bank accounts on this company")
	}
	account := accounts[0]
	if account.Type == "" || account.Name == "" {
		t.Fatalf("bank account is incomplete: %+v", account)
	}

	// The required bank_account filter is the interesting part here.
	txns, _, err := client.BankTransactions.ListForAccount(ctx, account.URL, &ListOptions{PerPage: 10})
	if err != nil {
		t.Fatalf("BankTransactions.ListForAccount: %v", err)
	}
	for _, txn := range txns {
		if txn.BankAccount != account.URL {
			t.Fatalf("transaction belongs to %q, want %q", txn.BankAccount, account.URL)
		}
	}
	t.Logf("account %q (%s), %d transaction(s)", account.Name, account.Type, len(txns))
}

// Every collection with a typed service must at least list without error, so
// a wrong envelope key or path shows up here rather than in a caller.
func TestLiveCollectionsList(t *testing.T) {
	client := liveClient(t)

	tests := map[string]func(context.Context) (int, error){
		"contacts": func(ctx context.Context) (int, error) {
			items, _, err := client.Contacts.List(ctx, nil)
			return len(items), err
		},
		"projects": func(ctx context.Context) (int, error) {
			items, _, err := client.Projects.List(ctx, nil)
			return len(items), err
		},
		"tasks": func(ctx context.Context) (int, error) {
			items, _, err := client.Tasks.List(ctx, nil)
			return len(items), err
		},
		"invoices": func(ctx context.Context) (int, error) {
			items, _, err := client.Invoices.List(ctx, nil)
			return len(items), err
		},
		"estimates": func(ctx context.Context) (int, error) {
			items, _, err := client.Estimates.List(ctx, nil)
			return len(items), err
		},
		"bills": func(ctx context.Context) (int, error) {
			items, _, err := client.Bills.List(ctx, nil)
			return len(items), err
		},
		"expenses": func(ctx context.Context) (int, error) {
			items, _, err := client.Expenses.List(ctx, nil)
			return len(items), err
		},
	}
	for name, list := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := liveContext(t)
			count, err := list(ctx)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			t.Logf("%s: %d record(s)", name, count)
		})
	}
}

// updated_since is the basis for any incremental read, so a far-future cursor
// must be accepted and must come back empty rather than erroring.
func TestLiveUpdatedSinceFilter(t *testing.T) {
	client := liveClient(t)
	ctx := liveContext(t)

	future := TimeOf(time.Now().Add(24 * time.Hour))
	items, _, err := client.Contacts.List(ctx, &ListOptions{UpdatedSince: future, Sort: "updated_at"})
	if err != nil {
		t.Fatalf("Contacts.List with updated_since: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("a future updated_since returned %d records, want 0", len(items))
	}
}

// The accounting reports are the Reader shape and their paths are the least
// guessable in the API, so a wrong one shows up immediately.
func TestLiveReports(t *testing.T) {
	client := liveClient(t)

	reports := map[string]string{
		"trial_balance":   "accounting/trial_balance/summary",
		"profit_and_loss": "accounting/profit_and_loss/summary",
		"balance_sheet":   "accounting/balance_sheet",
	}
	for name, path := range reports {
		t.Run(name, func(t *testing.T) {
			ctx := liveContext(t)
			body, resp, err := client.Raw(ctx, "GET", path, url.Values{}, nil)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if resp.StatusCode != 200 {
				t.Fatalf("%s: status %d", name, resp.StatusCode)
			}
			if len(body) == 0 {
				t.Fatalf("%s: empty body", name)
			}
			t.Logf("%s: %d bytes", name, len(body))
		})
	}
}

// The token on disk is normally still valid, so this drives a refresh
// explicitly and checks the rotated refresh token is persisted. Without that,
// the integration would break the first time the old token is retired.
func TestLiveTokenRefreshPersists(t *testing.T) {
	client := liveClient(t)
	ctx := liveContext(t)

	source, ok := client.tokenSource.(*TokenSource)
	if !ok {
		t.Fatal("client is not using the library TokenSource")
	}
	before, err := source.Peek()
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}

	// Expire the cached token so the next call has to refresh.
	source.mu.Lock()
	source.token.Expiry = time.Now().Add(-time.Minute)
	source.mu.Unlock()

	if _, _, err := client.Company.Get(ctx, nil); err != nil {
		t.Fatalf("call after forced expiry: %v", err)
	}

	after, err := source.Peek()
	if err != nil {
		t.Fatalf("Peek after refresh: %v", err)
	}
	if after.AccessToken == before.AccessToken {
		t.Fatal("access token did not change, so no refresh happened")
	}
	if after.RefreshToken == "" {
		t.Fatal("refresh token was lost")
	}
	if !after.Expiry.After(time.Now()) {
		t.Fatalf("refreshed token expires at %s, which is not in the future", after.Expiry)
	}

	// Re-read from disk: the rotated token must have been written back.
	reloaded := liveClient(t)
	reloadedSource, _ := reloaded.tokenSource.(*TokenSource)
	stored, err := reloadedSource.Peek()
	if err != nil {
		t.Fatalf("Peek from a fresh store: %v", err)
	}
	if stored.RefreshToken != after.RefreshToken {
		t.Fatal("the rotated refresh token was not persisted, so the next process would use a retired one")
	}
	t.Log("refresh rotated the token and the new one was persisted")
}
