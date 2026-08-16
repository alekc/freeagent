package freeagent

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Wave C decoding from captured payloads.
func TestWaveCGoldenDecoding(t *testing.T) {
	t.Parallel()

	t.Run("capital_asset_types", func(t *testing.T) {
		t.Parallel()
		client := goldenClient(t, "capital_asset_types.json")
		types, _, err := client.CapitalAssetTypes.List(context.Background(), nil)
		if err != nil {
			t.Fatalf("List = %v", err)
		}
		if len(types) == 0 {
			t.Fatal("no types decoded")
		}
		for _, assetType := range types {
			if assetType.Name == "" || assetType.SystemDefault == nil {
				t.Fatalf("type is incomplete: %+v", assetType)
			}
			if assetType.CreatedAt.IsZero() {
				t.Fatal("created_at did not decode")
			}
		}
	})

	t.Run("price_list_items", func(t *testing.T) {
		t.Parallel()
		client := goldenClient(t, "price_list_items.json")
		items, _, err := client.PriceListItems.List(context.Background(), nil)
		if err != nil {
			t.Fatalf("List = %v", err)
		}
		if len(items) == 0 {
			t.Fatal("no price list items decoded")
		}
		item := items[0]
		if item.Code == "" || item.ItemType == "" {
			t.Fatalf("item is incomplete: %+v", item)
		}
		if item.Price == nil || item.Quantity == nil {
			t.Fatalf("money fields did not decode: %+v", item)
		}
	})

	t.Run("ec_moss_rates", func(t *testing.T) {
		t.Parallel()
		client := goldenClient(t, "ec_moss_rates.json")
		rates, _, err := client.SalesTax.ECMossRates(context.Background(), "Ireland", NewDate(2026, 8, 16))
		if err != nil {
			t.Fatalf("ECMossRates = %v", err)
		}
		if len(rates.Rates) == 0 || rates.ECTaxName == "" {
			t.Fatalf("rates did not decode: %+v", rates)
		}
		for _, rate := range rates.Rates {
			if rate.Percentage == nil || rate.Band == "" {
				t.Fatalf("rate is incomplete: %+v", rate)
			}
		}
	})
}

// Notes take their parent as a query parameter, and only a contact or a
// project is a valid one.
func TestNoteParentScoping(t *testing.T) {
	t.Parallel()
	var seen string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Method + " " + r.URL.RequestURI()
		// A list wants the plural envelope, a write the singular one.
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"notes":[{"url":"https://api.freeagent.com/v2/notes/1","note":"hi"}]}`)
			return
		}
		fmt.Fprint(w, `{"note":{"url":"https://api.freeagent.com/v2/notes/1","note":"hi"}}`)
	})
	ctx := context.Background()

	// The unscoped forms fail locally and name the right method.
	for name, err := range map[string]error{
		"List":   errFrom(func() error { _, _, e := client.Notes.List(ctx, nil); return e }),
		"Create": errFrom(func() error { _, _, e := client.Notes.Create(ctx, &Note{}); return e }),
	} {
		if err == nil {
			t.Fatalf("%s succeeded without a parent", name)
		}
		if !strings.Contains(err.Error(), "ForParent") {
			t.Fatalf("%s error = %v, want it to name the scoped method", name, err)
		}
	}
	for note, err := range client.Notes.All(ctx, nil) {
		if err == nil {
			t.Fatalf("All yielded %+v without a parent", note)
		}
		break
	}

	contact := ResourceURL("https://api.freeagent.com/v2/contacts/7")
	if _, _, err := client.Notes.CreateForParent(ctx, contact, &Note{Note: "hi"}); err != nil {
		t.Fatalf("CreateForParent = %v", err)
	}
	if seen != "POST /v2/notes?contact=https%3A%2F%2Fapi.freeagent.com%2Fv2%2Fcontacts%2F7" {
		t.Fatalf("request = %s, want the contact filter", seen)
	}

	project := ResourceURL("https://api.freeagent.com/v2/projects/3")
	if _, _, err := client.Notes.ListForParent(ctx, project, nil); err != nil {
		t.Fatalf("ListForParent = %v", err)
	}
	if !strings.Contains(seen, "project=") {
		t.Fatalf("request = %s, want the project filter", seen)
	}

	// Anything else is refused before a request is built.
	for _, bad := range []ResourceURL{
		"https://api.freeagent.com/v2/invoices/1",
		"https://api.freeagent.com/v2/company",
		"",
	} {
		if _, _, err := client.Notes.CreateForParent(ctx, bad, &Note{Note: "x"}); err == nil {
			t.Fatalf("CreateForParent accepted %q as a parent", bad)
		}
		if _, _, err := client.Notes.ListForParent(ctx, bad, nil); err == nil {
			t.Fatalf("ListForParent accepted %q as a parent", bad)
		}
	}
	if _, _, err := client.Notes.CreateForParent(ctx, contact, nil); err == nil {
		t.Fatal("CreateForParent accepted a nil note")
	}
}

// include_history is opt-in because the history is bulky.
func TestCapitalAssetIncludeHistory(t *testing.T) {
	t.Parallel()
	var seen string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.RequestURI()
		if strings.Contains(r.URL.Path, "/capital_assets/") {
			fmt.Fprint(w, `{"capital_asset":{"description":"Laptop","capital_asset_history":[{"type":"purchase","value":"1000.0"}]}}`)
			return
		}
		fmt.Fprint(w, `{"capital_assets":[{"description":"Laptop"}]}`)
	})
	ctx := context.Background()

	if _, _, err := client.CapitalAssets.List(ctx, nil); err != nil {
		t.Fatalf("List = %v", err)
	}
	if strings.Contains(seen, "include_history") {
		t.Fatalf("plain List sent include_history: %s", seen)
	}

	opts := &ListOptions{PerPage: 25}
	if _, _, err := client.CapitalAssets.ListWithHistory(ctx, opts); err != nil {
		t.Fatalf("ListWithHistory = %v", err)
	}
	if !strings.Contains(seen, "include_history=true") || !strings.Contains(seen, "per_page=25") {
		t.Fatalf("request = %s, want the flag alongside the caller's options", seen)
	}
	// The caller's options must not be mutated.
	if opts.Extra != nil {
		t.Fatalf("caller options were mutated: %v", opts.Extra)
	}

	asset, _, err := client.CapitalAssets.GetWithHistory(ctx, 4)
	if err != nil {
		t.Fatalf("GetWithHistory = %v", err)
	}
	if !strings.Contains(seen, "include_history=true") {
		t.Fatalf("GetWithHistory request = %s", seen)
	}
	if len(asset.CapitalAssetHistory) == 0 {
		t.Fatal("history did not decode")
	}
	if asset.CapitalAssetHistory[0].Value == nil {
		t.Fatal("history value did not decode")
	}
}

// The families the API exposes read-only must not offer write verbs. This is
// a compile-time property; the test documents it and fails loudly if a future
// change swaps the embedded type.
func TestReadOnlyFamiliesHaveNoWriteVerbs(t *testing.T) {
	t.Parallel()
	client, err := NewClient(WithoutAuth())
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	readOnly := map[string]ResourceMeta{
		"capital_assets":     client.CapitalAssets.Meta(),
		"hire_purchases":     client.HirePurchases.Meta(),
		"stock_items":        client.StockItems.Meta(),
		"recurring_invoices": client.RecurringInvoices.Meta(),
		"transactions":       client.Transactions.Meta(),
		"bank_transactions":  client.BankTransactions.Meta(),
	}
	for name, meta := range readOnly {
		if !meta.ReadOnly {
			t.Errorf("%s is wired read-only but its registry entry does not say so", name)
		}
	}
}

func errFrom(call func() error) error { return call() }
