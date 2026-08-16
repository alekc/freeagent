//go:build integration

// Fixture capture. Real responses make the best fixtures, because only a real
// response carries the fields the documentation forgot, but a real response
// also carries the company's identity and this repository is public.
//
// Capture therefore runs inside the write suite, while the records it created
// still exist, and every payload goes through the scrubber before it reaches
// disk. Enable it with FREEAGENT_CAPTURE=1; it is off by default so an
// ordinary integration run never rewrites committed fixtures.
package freeagent

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekc/freeagent-sdk/internal/anonymise"
)

// captureTarget is one endpoint to snapshot into testdata.
type captureTarget struct {
	Path  string
	Query url.Values
}

func captureEnabled() bool { return os.Getenv("FREEAGENT_CAPTURE") == "1" }

// forbiddenValues collects the strings that must not survive scrubbing. They
// are read from the live account rather than hardcoded, so the check adapts to
// whichever company the suite is pointed at.
func forbiddenValues(t *testing.T, client *Client) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	company, _, err := client.Company.Get(ctx, nil)
	if err != nil {
		t.Fatalf("capture: reading the company to build the denylist: %v", err)
	}
	me, _, err := client.Users.Me(ctx)
	if err != nil {
		t.Fatalf("capture: reading the current user to build the denylist: %v", err)
	}
	return []string{
		company.Name,
		company.Subdomain,
		company.CompanyRegistrationNumber,
		company.SalesTaxRegistrationNumber,
		company.Address1, company.Address2, company.Address3,
		company.Town, company.Region, company.Postcode,
		company.ContactEmail, company.ContactPhone, company.Website,
		me.Email, me.FirstName, me.LastName,
		me.NINumber, me.UniqueTaxReference,
	}
}

// capture snapshots each target, scrubs it, and writes it to testdata. A
// scrub that fails its own safety check aborts the test rather than writing a
// document that still identifies the account.
func capture(t *testing.T, client *Client, tag string, targets map[string]captureTarget) {
	t.Helper()
	if !captureEnabled() {
		return
	}
	forbidden := forbiddenValues(t, client)
	opts := anonymise.Options{
		// The per-run tag is embedded in descriptions and references, and
		// would otherwise make every capture a diff.
		Literals:  map[string]string{tag: "SDKTEST"},
		Forbidden: forbidden,
	}

	for name, target := range targets {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		body, _, err := client.Raw(ctx, http.MethodGet, target.Path, target.Query, nil)
		cancel()
		if err != nil {
			t.Fatalf("capture %s: %v", name, err)
		}
		scrubbed, err := anonymise.JSON(body, opts)
		if err != nil {
			// Not a warning: a leak here would be committed and published.
			t.Fatalf("capture %s: %v", name, err)
		}
		path := filepath.Join("testdata", name+".json")
		if err := os.WriteFile(path, scrubbed, 0o644); err != nil {
			t.Fatalf("capture %s: writing %s: %v", name, path, err)
		}
		t.Logf("captured %s -> %s (%d bytes)", target.Path, path, len(scrubbed))
	}
}
