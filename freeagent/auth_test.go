package freeagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestFileStoreRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nested", "token.json")
	store, err := NewFileStore(path, Sandbox.Name)
	if err != nil {
		t.Fatalf("NewFileStore = %v", err)
	}

	if _, err := store.Load(context.Background()); !errors.Is(err, ErrNoToken) {
		t.Fatalf("Load on a missing file = %v, want ErrNoToken", err)
	}

	want := &oauth2.Token{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour).Truncate(time.Second),
	}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatalf("Save = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file mode = %#o, want 0600", perm)
	}

	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load = %v", err)
	}
	if got.AccessToken != want.AccessToken || got.RefreshToken != want.RefreshToken {
		t.Fatalf("loaded %+v, want %+v", got, want)
	}
	if !got.Expiry.Equal(want.Expiry) {
		t.Fatalf("Expiry = %s, want %s", got.Expiry, want.Expiry)
	}
}

// Sandbox and production credentials share one file, so saving one must not
// discard the other.
func TestFileStoreKeepsEnvironmentsSeparate(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "token.json")
	sandbox, err := NewFileStore(path, Sandbox.Name)
	if err != nil {
		t.Fatalf("NewFileStore = %v", err)
	}
	production, err := NewFileStore(path, Production.Name)
	if err != nil {
		t.Fatalf("NewFileStore = %v", err)
	}

	ctx := context.Background()
	if err := sandbox.Save(ctx, &oauth2.Token{AccessToken: "s", RefreshToken: "sr"}); err != nil {
		t.Fatalf("Save sandbox = %v", err)
	}
	if err := production.Save(ctx, &oauth2.Token{AccessToken: "p", RefreshToken: "pr"}); err != nil {
		t.Fatalf("Save production = %v", err)
	}

	got, err := sandbox.Load(ctx)
	if err != nil {
		t.Fatalf("Load sandbox = %v", err)
	}
	if got.RefreshToken != "sr" {
		t.Fatalf("sandbox refresh token = %q, want sr: the production save clobbered it", got.RefreshToken)
	}
}

func TestFileStoreRejectsLoosePermissions(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "token.json")
	store, err := NewFileStore(path, Sandbox.Name)
	if err != nil {
		t.Fatalf("NewFileStore = %v", err)
	}
	if err := store.Save(context.Background(), &oauth2.Token{RefreshToken: "r"}); err != nil {
		t.Fatalf("Save = %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod = %v", err)
	}
	_, err = store.Load(context.Background())
	if err == nil || !errorContains(err, "chmod 600") {
		t.Fatalf("Load = %v, want a permissions error naming the fix", err)
	}
}

func TestFileStoreRequiresPathAndKey(t *testing.T) {
	t.Parallel()
	if _, err := NewFileStore("", "sandbox"); err == nil {
		t.Fatal("NewFileStore with no path succeeded, want an error")
	}
	if _, err := NewFileStore("/tmp/x.json", ""); err == nil {
		t.Fatal("NewFileStore with no key succeeded, want an error")
	}
}

// tokenServer is a stand-in for the FreeAgent token endpoint. It rotates the
// refresh token on every call, which is what the real one does.
func tokenServer(t *testing.T, refreshes *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm = %v", err)
		}
		if got := r.PostForm.Get("grant_type"); got != "refresh_token" {
			t.Errorf("grant_type = %q, want refresh_token", got)
		}
		// The token endpoint authenticates the app with HTTP Basic.
		if id, secret, ok := r.BasicAuth(); !ok || id != "client-id" || secret != "client-secret" {
			t.Errorf("basic auth = %q/%q ok=%v, want client-id/client-secret", id, secret, ok)
		}
		n := refreshes.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  fmt.Sprintf("access-%d", n),
			"refresh_token": fmt.Sprintf("refresh-%d", n),
			"token_type":    "bearer",
			"expires_in":    3600,
		})
	}))
	t.Cleanup(server.Close)
	return server
}

func testOAuthConfig(tokenURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		Endpoint:     oauth2.Endpoint{TokenURL: tokenURL, AuthStyle: oauth2.AuthStyleInHeader},
	}
}

// FreeAgent rotates the refresh token on every refresh. Failing to persist
// the new one strands the integration as soon as the old one is retired, so
// this is the single most important behaviour in the auth path.
func TestTokenSourcePersistsRotatedRefreshToken(t *testing.T) {
	t.Parallel()
	var refreshes atomic.Int32
	server := tokenServer(t, &refreshes)

	store := NewMemoryStore(&oauth2.Token{
		AccessToken:  "stale",
		RefreshToken: "refresh-0",
		Expiry:       time.Now().Add(-time.Minute),
	})
	source, err := NewTokenSource(context.Background(), testOAuthConfig(server.URL), store)
	if err != nil {
		t.Fatalf("NewTokenSource = %v", err)
	}

	token, err := source.Token()
	if err != nil {
		t.Fatalf("Token = %v", err)
	}
	if token.AccessToken != "access-1" {
		t.Fatalf("AccessToken = %q, want access-1", token.AccessToken)
	}
	stored, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load = %v", err)
	}
	if stored.RefreshToken != "refresh-1" {
		t.Fatalf("stored refresh token = %q, want the rotated refresh-1", stored.RefreshToken)
	}

	// A still-valid token must be reused rather than refreshed again.
	if _, err := source.Token(); err != nil {
		t.Fatalf("second Token = %v", err)
	}
	if refreshes.Load() != 1 {
		t.Fatalf("refreshes = %d, want 1", refreshes.Load())
	}
}

// FreeAgent allows only 15 refreshes a minute, so concurrent callers have to
// collapse onto a single refresh rather than each triggering their own.
func TestTokenSourceRefreshesOnceUnderConcurrency(t *testing.T) {
	t.Parallel()
	var refreshes atomic.Int32
	server := tokenServer(t, &refreshes)

	store := NewMemoryStore(&oauth2.Token{
		AccessToken:  "stale",
		RefreshToken: "refresh-0",
		Expiry:       time.Now().Add(-time.Minute),
	})
	source, err := NewTokenSource(context.Background(), testOAuthConfig(server.URL), store)
	if err != nil {
		t.Fatalf("NewTokenSource = %v", err)
	}

	const callers = 25
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		tokens = map[string]int{}
	)
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			token, err := source.Token()
			if err != nil {
				t.Errorf("Token = %v", err)
				return
			}
			mu.Lock()
			tokens[token.AccessToken]++
			mu.Unlock()
		}()
	}
	wg.Wait()

	if refreshes.Load() != 1 {
		t.Fatalf("refreshes = %d, want exactly 1", refreshes.Load())
	}
	if len(tokens) != 1 {
		t.Fatalf("callers saw %d distinct tokens, want 1: %v", len(tokens), tokens)
	}
}

// The spec allows a refresh response to omit the refresh token. Dropping it
// would strand the next refresh, so the previous one is carried forward.
func TestTokenSourceKeepsRefreshTokenWhenOmitted(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "fresh",
			"token_type":   "bearer",
			"expires_in":   3600,
		})
	}))
	t.Cleanup(server.Close)

	store := NewMemoryStore(&oauth2.Token{RefreshToken: "keep-me", Expiry: time.Now().Add(-time.Minute)})
	source, err := NewTokenSource(context.Background(), testOAuthConfig(server.URL), store)
	if err != nil {
		t.Fatalf("NewTokenSource = %v", err)
	}
	token, err := source.Token()
	if err != nil {
		t.Fatalf("Token = %v", err)
	}
	if token.RefreshToken != "keep-me" {
		t.Fatalf("RefreshToken = %q, want the previous one carried forward", token.RefreshToken)
	}
}

func TestTokenSourceReportsMissingToken(t *testing.T) {
	t.Parallel()
	source, err := NewTokenSource(context.Background(), testOAuthConfig("http://example.invalid"), NewMemoryStore(nil))
	if err != nil {
		t.Fatalf("NewTokenSource = %v", err)
	}
	if _, err := source.Token(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("Token = %v, want ErrNoToken", err)
	}
	if _, err := source.Peek(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("Peek = %v, want ErrNoToken", err)
	}
}

func TestNewTokenSourceValidatesConfig(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	good := testOAuthConfig("http://example.invalid")
	tests := map[string]struct {
		cfg   *oauth2.Config
		store TokenStore
	}{
		"nil config":    {cfg: nil, store: NewMemoryStore(nil)},
		"nil store":     {cfg: good, store: nil},
		"no client id":  {cfg: &oauth2.Config{ClientSecret: "s"}, store: NewMemoryStore(nil)},
		"no app secret": {cfg: &oauth2.Config{ClientID: "i"}, store: NewMemoryStore(nil)},
	}
	for name, tc := range tests {
		if _, err := NewTokenSource(ctx, tc.cfg, tc.store); err == nil {
			t.Fatalf("%s: NewTokenSource succeeded, want an error", name)
		}
	}
}

func TestEnvironmentByName(t *testing.T) {
	t.Parallel()
	if env, err := EnvironmentByName("sandbox"); err != nil || env.BaseURL != SandboxBaseURL {
		t.Fatalf("sandbox = %+v, %v", env, err)
	}
	if env, err := EnvironmentByName("production"); err != nil || env.BaseURL != DefaultBaseURL {
		t.Fatalf("production = %+v, %v", env, err)
	}
	if _, err := EnvironmentByName("staging"); err == nil {
		t.Fatal("EnvironmentByName(staging) succeeded, want an error")
	}
}

func TestExpiresIn(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		token *oauth2.Token
		want  string
	}{
		{name: "no expiry", token: &oauth2.Token{}, want: "unknown"},
		{name: "past", token: &oauth2.Token{Expiry: now.Add(-time.Second)}, want: "expired"},
		{name: "future", token: &oauth2.Token{Expiry: now.Add(90 * time.Second)}, want: "1m30s"},
	}
	for _, tc := range tests {
		if got := ExpiresIn(tc.token, now); got != tc.want {
			t.Fatalf("%s: ExpiresIn = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func errorContains(err error, want string) bool {
	return err != nil && strings.Contains(err.Error(), want)
}
