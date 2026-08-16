package freeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestRawReturnsBodyVerbatim(t *testing.T) {
	t.Parallel()
	const payload = `{"company":{"name":"Acme","subdomain":"acme"}}`
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	})
	body, _, err := client.Raw(context.Background(), http.MethodGet, "company", nil, nil)
	if err != nil {
		t.Fatalf("Raw = %v", err)
	}
	if string(body) != payload {
		t.Fatalf("body = %s, want it verbatim", body)
	}
}

// Raw is how tooling reaches endpoints with no typed model, including ones
// that answer with something other than JSON.
func TestRawPassesThroughNonJSON(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "plain text answer")
	})
	body, _, err := client.Raw(context.Background(), http.MethodGet, "company", nil, nil)
	if err != nil {
		t.Fatalf("Raw = %v", err)
	}
	if string(body) != "plain text answer" {
		t.Fatalf("body = %q", body)
	}
}

func TestRawSendsQueryAndBody(t *testing.T) {
	t.Parallel()
	var (
		gotQuery url.Values
		gotBody  map[string]string
	)
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		fmt.Fprint(w, `{}`)
	})

	query := url.Values{"view": {"open"}}
	if _, _, err := client.Raw(context.Background(), http.MethodPost, "invoices", query, map[string]string{"a": "b"}); err != nil {
		t.Fatalf("Raw = %v", err)
	}
	if gotQuery.Get("view") != "open" {
		t.Fatalf("query = %v, want view=open", gotQuery)
	}
	if gotBody["a"] != "b" {
		t.Fatalf("body = %v, want a=b", gotBody)
	}
}

func TestRawURL(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/invoices/5" {
			t.Errorf("path = %q, want /v2/invoices/5", r.URL.Path)
		}
		fmt.Fprint(w, `{"invoice":{}}`)
	})
	ref := ResourceURL(strings.TrimSuffix(client.BaseURL().String(), "/") + "/invoices/5")
	if _, _, err := client.RawURL(context.Background(), http.MethodGet, ref, nil, nil); err != nil {
		t.Fatalf("RawURL = %v", err)
	}
	if _, _, err := client.RawURL(context.Background(), http.MethodGet, "https://evil.example.com/v2/invoices/5", nil, nil); err == nil {
		t.Fatal("RawURL accepted a foreign host, want an error")
	}
}

// A path outside the configured API root is rejected, so a reference cannot
// walk out of /v2/ into another part of the host.
func TestPathForURLRejectsOutsideAPIRoot(t *testing.T) {
	t.Parallel()
	client, err := NewClient(WithoutAuth(), WithBaseURL("https://api.freeagent.com/v2/"))
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	if _, err := client.pathForURL("https://api.freeagent.com/admin/users/1"); err == nil {
		t.Fatal("pathForURL accepted a path outside the API root, want an error")
	}
	got, err := client.pathForURL("https://api.freeagent.com/v2/invoices/1")
	if err != nil {
		t.Fatalf("pathForURL = %v", err)
	}
	if got != "invoices/1" {
		t.Fatalf("pathForURL = %q, want invoices/1", got)
	}
}

func TestCollectionAndReaderExposeMeta(t *testing.T) {
	t.Parallel()
	client, err := NewClient(WithoutAuth())
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	collection := newCollection[widget](client, widgetMeta)
	if collection.Meta().Path != "widgets" {
		t.Fatalf("Collection.Meta().Path = %q, want widgets", collection.Meta().Path)
	}
	reader := newReader[struct{}](client, Resources["company"])
	if reader.Meta().Name != "company" {
		t.Fatalf("Reader.Meta().Name = %q, want company", reader.Meta().Name)
	}
}

// A write that answers 204 is a success with no record, not a decode failure.
func TestWriteWithEmptyResponse(t *testing.T) {
	t.Parallel()
	widgets := newWidgets(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	got, resp, err := widgets.Update(context.Background(), 1, &widget{Name: "x"})
	if err != nil {
		t.Fatalf("Update = %v", err)
	}
	if got != nil {
		t.Fatalf("record = %+v, want nil for an empty response", got)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

func TestReaderPassesParams(t *testing.T) {
	t.Parallel()
	type report struct {
		Total string `json:"total"`
	}
	var gotQuery url.Values
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		fmt.Fprint(w, `{"total":"100.0"}`)
	})
	reader := newReader[report](client, Resources["profit_and_loss"])

	got, _, err := reader.Get(context.Background(), url.Values{"from_date": {"2026-01-01"}})
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	if gotQuery.Get("from_date") != "2026-01-01" {
		t.Fatalf("query = %v, want from_date", gotQuery)
	}
	if got.Total != "100.0" {
		t.Fatalf("total = %q", got.Total)
	}
}

func TestReaderWithEnvelope(t *testing.T) {
	t.Parallel()
	type company struct {
		Name string `json:"name"`
	}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"company":{"name":"Acme"}}`)
	})
	reader := newReader[company](client, Resources["company"])
	got, _, err := reader.Get(context.Background(), nil)
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	if got.Name != "Acme" {
		t.Fatalf("Name = %q, want Acme", got.Name)
	}
}

func TestReaderReportsMissingEnvelope(t *testing.T) {
	t.Parallel()
	type company struct{}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"organisation":{}}`)
	})
	reader := newReader[company](client, Resources["company"])
	if _, _, err := reader.Get(context.Background(), nil); err == nil {
		t.Fatal("Get succeeded with the wrong envelope key, want an error")
	}
}

func TestOAuthConfigShape(t *testing.T) {
	t.Parallel()
	cfg := Sandbox.OAuthConfig("id", "secret", "http://localhost:8723/callback")
	if cfg.Endpoint.AuthURL != Sandbox.AuthorizeURL || cfg.Endpoint.TokenURL != Sandbox.TokenURL {
		t.Fatalf("endpoints = %+v, want the sandbox pair", cfg.Endpoint)
	}
	// The token endpoint wants HTTP Basic, so the style is pinned rather
	// than probed on the first call.
	if cfg.Endpoint.AuthStyle != oauth2.AuthStyleInHeader {
		t.Fatalf("AuthStyle = %v, want AuthStyleInHeader", cfg.Endpoint.AuthStyle)
	}
	if cfg.RedirectURL != "http://localhost:8723/callback" {
		t.Fatalf("RedirectURL = %q", cfg.RedirectURL)
	}
}

func TestAuthCodeURLCarriesState(t *testing.T) {
	t.Parallel()
	source, err := NewTokenSource(context.Background(), Sandbox.OAuthConfig("id", "secret", "http://localhost/cb"), NewMemoryStore(nil))
	if err != nil {
		t.Fatalf("NewTokenSource = %v", err)
	}
	got, err := url.Parse(source.AuthCodeURL("nonce-123"))
	if err != nil {
		t.Fatalf("AuthCodeURL is not a URL: %v", err)
	}
	if got.Query().Get("state") != "nonce-123" {
		t.Fatalf("state = %q, want nonce-123", got.Query().Get("state"))
	}
	if got.Query().Get("client_id") != "id" {
		t.Fatalf("client_id = %q", got.Query().Get("client_id"))
	}
}

func TestExchangeStoresToken(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm = %v", err)
		}
		if got := r.PostForm.Get("grant_type"); got != "authorization_code" {
			t.Errorf("grant_type = %q, want authorization_code", got)
		}
		if got := r.PostForm.Get("code"); got != "the-code" {
			t.Errorf("code = %q, want the-code", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-x",
			"refresh_token": "refresh-x",
			"token_type":    "bearer",
			"expires_in":    3600,
		})
	}))
	t.Cleanup(server.Close)

	store := NewMemoryStore(nil)
	source, err := NewTokenSource(context.Background(), testOAuthConfig(server.URL), store)
	if err != nil {
		t.Fatalf("NewTokenSource = %v", err)
	}
	token, err := source.Exchange(context.Background(), "the-code")
	if err != nil {
		t.Fatalf("Exchange = %v", err)
	}
	if token.AccessToken != "access-x" {
		t.Fatalf("AccessToken = %q", token.AccessToken)
	}
	stored, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load = %v", err)
	}
	if stored.RefreshToken != "refresh-x" {
		t.Fatalf("stored refresh token = %q, want refresh-x", stored.RefreshToken)
	}
	// Peek must now report the exchanged token without a network call.
	peeked, err := source.Peek()
	if err != nil {
		t.Fatalf("Peek = %v", err)
	}
	if peeked.AccessToken != "access-x" {
		t.Fatalf("Peek AccessToken = %q", peeked.AccessToken)
	}
}

func TestExchangeReportsFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid_grant"}`)
	}))
	t.Cleanup(server.Close)

	source, err := NewTokenSource(context.Background(), testOAuthConfig(server.URL), NewMemoryStore(nil))
	if err != nil {
		t.Fatalf("NewTokenSource = %v", err)
	}
	if _, err := source.Exchange(context.Background(), "bad"); err == nil {
		t.Fatal("Exchange succeeded, want an error")
	}
}

func TestDefaultTokenPath(t *testing.T) {
	t.Parallel()
	got, err := DefaultTokenPath()
	if err != nil {
		t.Fatalf("DefaultTokenPath = %v", err)
	}
	if !strings.HasSuffix(got, "freeagent/token.json") {
		t.Fatalf("DefaultTokenPath = %q, want it to end in freeagent/token.json", got)
	}
}

func TestFileStorePath(t *testing.T) {
	t.Parallel()
	store, err := NewFileStore("/tmp/x/token.json", "sandbox")
	if err != nil {
		t.Fatalf("NewFileStore = %v", err)
	}
	if store.Path() != "/tmp/x/token.json" {
		t.Fatalf("Path = %q", store.Path())
	}
}

func TestDateAndTimeConstructors(t *testing.T) {
	t.Parallel()
	if got := NewDate(2026, time.August, 16).String(); got != "2026-08-16" {
		t.Fatalf("NewDate = %q", got)
	}
	// DateOf drops the clock, which is what a calendar-date field means.
	if got := DateOf(time.Date(2026, time.August, 16, 23, 59, 59, 0, time.UTC)).String(); got != "2026-08-16" {
		t.Fatalf("DateOf = %q", got)
	}
	if got := (Date{}).String(); got != "" {
		t.Fatalf("zero Date.String = %q, want empty", got)
	}
	if got := (Time{}).String(); got != "" {
		t.Fatalf("zero Time.String = %q, want empty", got)
	}
	if _, err := ParseTime("not a time"); err == nil {
		t.Fatal("ParseTime accepted garbage, want an error")
	}
	if _, err := ParseTime(strings.Repeat("2", maxScalarLen+1)); err == nil {
		t.Fatal("ParseTime accepted an over-long string, want an error")
	}
	if got, err := ParseTime("  "); err != nil || !got.IsZero() {
		t.Fatalf("ParseTime(blank) = %v, %v, want a zero Time", got, err)
	}
}

func TestResourceURLAccessors(t *testing.T) {
	t.Parallel()
	ref := ResourceURL("https://api.freeagent.com/v2/invoices/3")
	if ref.String() != string(ref) {
		t.Fatalf("String = %q", ref.String())
	}
	if ref.IsZero() {
		t.Fatal("IsZero = true for a populated reference")
	}
	if !ResourceURL("   ").IsZero() {
		t.Fatal("IsZero = false for a blank reference")
	}
}

func TestFieldErrorString(t *testing.T) {
	t.Parallel()
	if got := (FieldError{Message: "is invalid"}).String(); got != "is invalid" {
		t.Fatalf("String = %q", got)
	}
	if got := (FieldError{Field: "dated_on", Message: "is invalid"}).String(); got != "dated_on: is invalid" {
		t.Fatalf("String = %q", got)
	}
}

func TestErrorBodyEdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "empty body", body: "", want: ""},
		{name: "errors as a bare string", body: `{"errors":"boom"}`, want: "boom"},
		{name: "nested message without wrapper", body: `{"errors":{"message":"boom"}}`, want: "boom"},
		{name: "field with a single string value", body: `{"errors":{"dated_on":"is invalid"}}`, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			msg, fields := parseErrorBody([]byte(tc.body))
			if tc.want != "" && msg != tc.want {
				t.Fatalf("message = %q, want %q", msg, tc.want)
			}
			if tc.name == "field with a single string value" {
				if len(fields) != 1 || fields[0].Field != "dated_on" {
					t.Fatalf("fields = %v, want one dated_on entry", fields)
				}
			}
		})
	}
}
