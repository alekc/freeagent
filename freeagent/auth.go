package freeagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// ErrNoToken is returned by a TokenStore that holds no token yet. It is the
// signal for tooling to run the authorisation flow.
var ErrNoToken = errors.New("freeagent: no stored token")

// Environment bundles the endpoints for one FreeAgent deployment.
//
// One app registered at dev.freeagent.com serves both, so the client ID and
// secret are shared. What differs is the user account you approve with, and
// therefore the token, which is why tokens are stored under separate keys.
type Environment struct {
	Name         string
	BaseURL      string
	AuthorizeURL string
	TokenURL     string
}

// The two deployments FreeAgent operates.
//
//nolint:gosec // G101 false positive: a token endpoint URL is public, not a credential
var (
	Production = Environment{
		Name:         "production",
		BaseURL:      DefaultBaseURL,
		AuthorizeURL: "https://api.freeagent.com/v2/approve_app",
		TokenURL:     "https://api.freeagent.com/v2/token_endpoint",
	}
	Sandbox = Environment{
		Name:         "sandbox",
		BaseURL:      SandboxBaseURL,
		AuthorizeURL: "https://api.sandbox.freeagent.com/v2/approve_app",
		TokenURL:     "https://api.sandbox.freeagent.com/v2/token_endpoint",
	}
)

// EnvironmentByName resolves "production" or "sandbox".
func EnvironmentByName(name string) (Environment, error) {
	switch name {
	case Production.Name:
		return Production, nil
	case Sandbox.Name:
		return Sandbox, nil
	default:
		return Environment{}, fmt.Errorf("freeagent: unknown environment %q, want %q or %q", name, Production.Name, Sandbox.Name)
	}
}

// OAuthConfig builds the OAuth configuration for an environment. The token
// endpoint authenticates the app with HTTP Basic, so the auth style is pinned
// rather than probed.
func (e Environment) OAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:   e.AuthorizeURL,
			TokenURL:  e.TokenURL,
			AuthStyle: oauth2.AuthStyleInHeader,
		},
	}
}

// TokenStore persists OAuth tokens between processes. Load returns ErrNoToken
// when nothing has been stored yet.
type TokenStore interface {
	Load(ctx context.Context) (*oauth2.Token, error)
	Save(ctx context.Context, token *oauth2.Token) error
}

// MemoryStore keeps a token for the life of the process. Useful in tests and
// for short-lived jobs that receive a token by other means.
type MemoryStore struct {
	mu    sync.Mutex
	token *oauth2.Token
}

// NewMemoryStore seeds a store, optionally with an existing token.
func NewMemoryStore(token *oauth2.Token) *MemoryStore {
	return &MemoryStore{token: token}
}

// Load implements TokenStore.
func (m *MemoryStore) Load(context.Context) (*oauth2.Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.token == nil {
		return nil, ErrNoToken
	}
	copied := *m.token
	return &copied, nil
}

// Save implements TokenStore.
func (m *MemoryStore) Save(_ context.Context, token *oauth2.Token) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := *token
	m.token = &copied
	return nil
}

// fileStoreVersion guards the on-disk layout against a future change.
const fileStoreVersion = 1

type tokenFile struct {
	Version int                      `json:"version"`
	Tokens  map[string]*oauth2.Token `json:"tokens"`
}

// FileStore persists tokens as 0600 JSON. One file holds one entry per
// environment, so sandbox and production credentials coexist without the
// caller juggling paths.
type FileStore struct {
	path string
	key  string
	mu   sync.Mutex
}

// NewFileStore stores tokens for one environment at path. Pass the
// environment name as key.
func NewFileStore(path, key string) (*FileStore, error) {
	if path == "" {
		return nil, errors.New("freeagent: FileStore requires a path")
	}
	if key == "" {
		return nil, errors.New("freeagent: FileStore requires a key, use the environment name")
	}
	return &FileStore{path: path, key: key}, nil
}

// DefaultTokenPath is where facli keeps credentials when not told otherwise.
func DefaultTokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("freeagent: locating the user config directory: %w", err)
	}
	return filepath.Join(dir, "freeagent", "token.json"), nil
}

// Path returns the file backing the store.
func (f *FileStore) Path() string { return f.path }

// Load implements TokenStore.
func (f *FileStore) Load(context.Context) (*oauth2.Token, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	info, err := os.Stat(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoToken
	}
	if err != nil {
		return nil, fmt.Errorf("freeagent: reading token file: %w", err)
	}
	// The file holds a long-lived refresh token. Refuse to use one that
	// other local accounts can read rather than quietly widening exposure.
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		return nil, fmt.Errorf("freeagent: token file %s has permissions %#o, run: chmod 600 %s", f.path, mode, f.path)
	}

	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, fmt.Errorf("freeagent: reading token file: %w", err)
	}
	var stored tokenFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("freeagent: parsing token file %s: %w", f.path, err)
	}
	token, ok := stored.Tokens[f.key]
	if !ok || token == nil || token.RefreshToken == "" {
		return nil, ErrNoToken
	}
	return token, nil
}

// Save implements TokenStore. The write is atomic so an interrupted save
// cannot leave a truncated credential file behind.
func (f *FileStore) Save(_ context.Context, token *oauth2.Token) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	stored := tokenFile{Version: fileStoreVersion, Tokens: map[string]*oauth2.Token{}}
	if data, err := os.ReadFile(f.path); err == nil {
		if err := json.Unmarshal(data, &stored); err != nil || stored.Tokens == nil {
			stored = tokenFile{Version: fileStoreVersion, Tokens: map[string]*oauth2.Token{}}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("freeagent: reading token file before save: %w", err)
	}
	stored.Version = fileStoreVersion
	stored.Tokens[f.key] = token

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("freeagent: encoding token file: %w", err)
	}
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("freeagent: creating %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".token-*.json")
	if err != nil {
		return fmt.Errorf("freeagent: creating temporary token file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("freeagent: securing temporary token file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("freeagent: writing temporary token file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("freeagent: closing temporary token file: %w", err)
	}
	if err := os.Rename(tmpName, f.path); err != nil {
		return fmt.Errorf("freeagent: replacing token file: %w", err)
	}
	return nil
}

// TokenSource refreshes access tokens and persists the rotated refresh token.
// FreeAgent issues a new refresh token on every refresh, so a source that
// does not write the new one back loses access as soon as the old one is
// retired. It satisfies oauth2.TokenSource and is safe for concurrent use.
type TokenSource struct {
	ctx   context.Context
	cfg   *oauth2.Config
	store TokenStore

	mu    sync.Mutex
	token *oauth2.Token
}

// NewTokenSource builds a refreshing source over a store. ctx governs the
// refresh requests and may carry an oauth2.HTTPClient override.
func NewTokenSource(ctx context.Context, cfg *oauth2.Config, store TokenStore) (*TokenSource, error) {
	if ctx == nil {
		return nil, errors.New("freeagent: NewTokenSource requires a context")
	}
	if cfg == nil {
		return nil, errors.New("freeagent: NewTokenSource requires an OAuth config")
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("freeagent: OAuth config needs both a client ID and a client secret")
	}
	if store == nil {
		return nil, errors.New("freeagent: NewTokenSource requires a token store")
	}
	return &TokenSource{ctx: ctx, cfg: cfg, store: store}, nil
}

// Token returns a valid access token, refreshing and persisting when needed.
// The lock is deliberately held across the refresh: FreeAgent allows only 15
// refreshes per minute, so serialising concurrent callers onto one refresh is
// the correct behaviour rather than a bottleneck to optimise away.
func (s *TokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.token == nil {
		stored, err := s.store.Load(s.ctx)
		if err != nil {
			return nil, err
		}
		s.token = stored
	}
	if s.token.Valid() {
		return s.token, nil
	}

	refreshed, err := s.cfg.TokenSource(s.ctx, s.token).Token()
	if err != nil {
		return nil, fmt.Errorf("freeagent: refreshing access token: %w", err)
	}
	// Defensive: the spec allows omitting the refresh token on a refresh, and
	// dropping it here would strand the next refresh.
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = s.token.RefreshToken
	}
	s.token = refreshed
	if err := s.store.Save(s.ctx, refreshed); err != nil {
		return nil, fmt.Errorf("freeagent: persisting refreshed token: %w", err)
	}
	return refreshed, nil
}

// Exchange trades an authorisation code for a token and stores it. It is the
// final step of the interactive login flow.
func (s *TokenSource) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	token, err := s.cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("freeagent: exchanging authorisation code: %w", err)
	}
	if err := s.store.Save(ctx, token); err != nil {
		return nil, fmt.Errorf("freeagent: persisting token: %w", err)
	}
	s.mu.Lock()
	s.token = token
	s.mu.Unlock()
	return token, nil
}

// AuthCodeURL builds the URL the user visits to approve the app.
func (s *TokenSource) AuthCodeURL(state string) string {
	return s.cfg.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// Peek returns the cached token without triggering a refresh, loading from
// the store on first use. Tooling uses it to report expiry.
func (s *TokenSource) Peek() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token == nil {
		stored, err := s.store.Load(s.ctx)
		if err != nil {
			return nil, err
		}
		s.token = stored
	}
	copied := *s.token
	return &copied, nil
}

// ExpiresIn renders a human-readable remaining lifetime for tooling.
func ExpiresIn(t *oauth2.Token, now time.Time) string {
	if t.Expiry.IsZero() {
		return "unknown"
	}
	d := t.Expiry.Sub(now).Round(time.Second)
	if d <= 0 {
		return "expired"
	}
	return d.String()
}
