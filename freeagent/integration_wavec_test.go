//go:build integration

// Wave C live coverage: assets, stock, price list, notes and sales tax.
package freeagent

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestLiveWaveCReads(t *testing.T) {
	client := liveClient(t)

	t.Run("capital_asset_types", func(t *testing.T) {
		ctx := liveContext(t)
		types, _, err := client.CapitalAssetTypes.List(ctx, nil)
		if err != nil {
			t.Fatalf("CapitalAssetTypes.List: %v", err)
		}
		if len(types) == 0 {
			t.Fatal("no capital asset types, but FreeAgent seeds four")
		}
		defaults := 0
		for _, assetType := range types {
			if assetType.Name == "" {
				t.Fatalf("type has no name: %+v", assetType)
			}
			if assetType.SystemDefault == nil {
				t.Fatalf("system_default did not decode: %+v", assetType)
			}
			if *assetType.SystemDefault {
				defaults++
			}
		}
		if defaults == 0 {
			t.Fatal("no system default types, which should be impossible")
		}
		t.Logf("%d type(s), %d system default", len(types), defaults)
	})

	t.Run("capital_assets", func(t *testing.T) {
		ctx := liveContext(t)
		assets, _, err := client.CapitalAssets.List(ctx, nil)
		if err != nil {
			t.Fatalf("CapitalAssets.List: %v", err)
		}
		// include_history is a separate call because the history is bulky.
		withHistory, _, err := client.CapitalAssets.ListWithHistory(ctx, nil)
		if err != nil {
			t.Fatalf("CapitalAssets.ListWithHistory: %v", err)
		}
		if len(withHistory) != len(assets) {
			t.Fatalf("history variant returned %d assets, plain returned %d",
				len(withHistory), len(assets))
		}
		t.Logf("%d capital asset(s)", len(assets))
	})

	t.Run("hire_purchases", func(t *testing.T) {
		ctx := liveContext(t)
		items, _, err := client.HirePurchases.List(ctx, nil)
		if err != nil {
			t.Fatalf("HirePurchases.List: %v", err)
		}
		t.Logf("%d hire purchase(s)", len(items))
	})

	t.Run("stock_items", func(t *testing.T) {
		ctx := liveContext(t)
		items, _, err := client.StockItems.List(ctx, nil)
		if err != nil {
			t.Fatalf("StockItems.List: %v", err)
		}
		t.Logf("%d stock item(s)", len(items))
	})

	t.Run("price_list_items", func(t *testing.T) {
		ctx := liveContext(t)
		items, _, err := client.PriceListItems.List(ctx, nil)
		if err != nil {
			t.Fatalf("PriceListItems.List: %v", err)
		}
		t.Logf("%d price list item(s)", len(items))
	})

	// Properties belong to UkUnincorporatedLandlord companies. On any other
	// type the list is simply empty rather than an error, which is worth
	// pinning so a future change is noticed.
	t.Run("properties", func(t *testing.T) {
		ctx := liveContext(t)
		company, _, err := client.Company.Get(ctx, nil)
		if err != nil {
			t.Fatalf("Company.Get: %v", err)
		}
		properties, _, err := client.Properties.List(ctx, nil)
		if err != nil {
			t.Fatalf("Properties.List: %v", err)
		}
		if company.Type != "UkUnincorporatedLandlord" && len(properties) != 0 {
			t.Fatalf("a %s company reported %d properties", company.Type, len(properties))
		}
		t.Logf("company type %s, %d propert(ies)", company.Type, len(properties))
	})

	// The one endpoint on the otherwise prose sales tax page.
	t.Run("ec_moss_rates", func(t *testing.T) {
		ctx := liveContext(t)
		rates, _, err := client.SalesTax.ECMossRates(ctx, "Ireland", DateOf(time.Now()))
		if err != nil {
			t.Fatalf("SalesTax.ECMossRates: %v", err)
		}
		if len(rates.Rates) == 0 {
			t.Fatal("no MOSS rates returned for Ireland")
		}
		standard := false
		for _, rate := range rates.Rates {
			if rate.Percentage == nil || rate.Band == "" {
				t.Fatalf("rate is incomplete: %+v", rate)
			}
			if rate.Band == "Standard" {
				standard = true
			}
		}
		if !standard {
			t.Fatal("no standard band in the returned rates")
		}
		if rates.ECTaxName == "" {
			t.Fatal("ec_tax_name did not decode")
		}
		t.Logf("%d rate(s) for Ireland, tax called %q", len(rates.Rates), rates.ECTaxName)

		// Both arguments are required, and that is enforced locally.
		if _, _, err := client.SalesTax.ECMossRates(ctx, "", DateOf(time.Now())); err == nil {
			t.Fatal("a blank country was accepted")
		}
		if _, _, err := client.SalesTax.ECMossRates(ctx, "Ireland", Date{}); err == nil {
			t.Fatal("a zero date was accepted")
		}
	})
}

// Notes are scoped to a contact or project, so the unscoped forms are
// shadowed and the scoped ones carry the filter.
func TestLiveNotesAreParentScoped(t *testing.T) {
	client := writeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	tag := runTag()

	// The unscoped calls must fail locally, naming the right method.
	if _, _, err := client.Notes.List(ctx, nil); err == nil ||
		!strings.Contains(err.Error(), "ListForParent") {
		t.Fatalf("List = %v, want a pointer to ListForParent", err)
	}
	if _, _, err := client.Notes.Create(ctx, &Note{Note: "x"}); err == nil ||
		!strings.Contains(err.Error(), "CreateForParent") {
		t.Fatalf("Create = %v, want a pointer to CreateForParent", err)
	}
	// A parent that is neither a contact nor a project is rejected.
	if _, _, err := client.Notes.CreateForParent(ctx, "https://x/v2/invoices/1", &Note{Note: "x"}); err == nil {
		t.Fatal("an invoice was accepted as a note parent")
	}

	contact, _, err := client.Contacts.Create(ctx, &Contact{OrganisationName: tag + " Contact"})
	if err != nil {
		t.Fatalf("Contacts.Create: %v", err)
	}
	contactID := mustID(t, contact.URL)
	t.Cleanup(func() { deleteQuietly(t, "contact", client.Contacts.Delete, contactID) })

	note, _, err := client.Notes.CreateForParent(ctx, contact.URL, &Note{Note: tag + " note body"})
	if err != nil {
		t.Fatalf("Notes.CreateForParent: %v", err)
	}
	noteID := mustID(t, note.URL)
	t.Cleanup(func() { deleteQuietly(t, "note", client.Notes.Delete, noteID) })
	if note.Note != tag+" note body" {
		t.Fatalf("note body = %q", note.Note)
	}
	if note.ParentURL.IsZero() {
		t.Fatalf("parent_url did not come back: %+v", note)
	}
	t.Logf("created note %d on %s, author %q", noteID, note.ParentURL, note.Author)

	listed, _, err := client.Notes.ListForParent(ctx, contact.URL, nil)
	if err != nil {
		t.Fatalf("Notes.ListForParent: %v", err)
	}
	if len(listed) == 0 {
		t.Fatal("the note just created is not listed against its contact")
	}

	updated, _, err := client.Notes.Update(ctx, noteID, &Note{Note: tag + " amended"})
	if err != nil {
		t.Fatalf("Notes.Update: %v", err)
	}
	if updated.Note != tag+" amended" {
		t.Fatalf("update did not take: %q", updated.Note)
	}
}

// Stock items and price list items are the Wave C write paths.
func TestLiveWaveCWriteLifecycle(t *testing.T) {
	client := writeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	tag := runTag()

	groups, _, err := client.Categories.List(ctx, false)
	if err != nil {
		t.Fatalf("Categories.List: %v", err)
	}
	if len(groups.Income) == 0 {
		t.Fatal("need an income category")
	}
	incomeCategory := groups.Income[0].URL

	// --- Capital asset type ----------------------------------------------------
	assetType, _, err := client.CapitalAssetTypes.Create(ctx, &CapitalAssetType{
		Name: tag + " Asset Type",
	})
	if err != nil {
		t.Fatalf("CapitalAssetTypes.Create: %v", err)
	}
	assetTypeID := mustID(t, assetType.URL)
	t.Cleanup(func() {
		deleteQuietly(t, "capital asset type", client.CapitalAssetTypes.Delete, assetTypeID)
	})
	if assetType.SystemDefault == nil || *assetType.SystemDefault {
		t.Fatalf("a user-created type reported system_default=%v", assetType.SystemDefault)
	}
	t.Logf("created capital asset type %d", assetTypeID)

	renamed := tag + " Asset Type Renamed"
	updatedType, _, err := client.CapitalAssetTypes.Update(ctx, assetTypeID, &CapitalAssetType{Name: renamed})
	if err != nil {
		t.Fatalf("CapitalAssetTypes.Update: %v", err)
	}
	if updatedType.Name != renamed {
		t.Fatalf("name = %q, want %q", updatedType.Name, renamed)
	}

	// --- Stock item: read-only, so only the shape is checked -------------------
	items, _, err := client.StockItems.List(ctx, nil)
	if err != nil {
		t.Fatalf("StockItems.List: %v", err)
	}
	t.Logf("%d stock item(s); the family is read-only, POST answers 404", len(items))

	// --- Price list item ---------------------------------------------------------
	priceItem, _, err := client.PriceListItems.Create(ctx, &PriceListItem{
		Code:        tag[:12] + "-A1",
		Description: tag + " Widget",
		ItemType:    "Products",
		Quantity:    new(decimal.RequireFromString("1")),
		Price:       new(decimal.RequireFromString("19.99")),
		VATStatus:   VATStatusStandard,
		Category:    incomeCategory,
	})
	if err != nil {
		t.Fatalf("PriceListItems.Create: %v", err)
	}
	priceItemID := mustID(t, priceItem.URL)
	t.Cleanup(func() {
		deleteQuietly(t, "price list item", client.PriceListItems.Delete, priceItemID)
	})
	if priceItem.Price == nil || !priceItem.Price.Equal(decimal.RequireFromString("19.99")) {
		t.Fatalf("price = %v, want 19.99", priceItem.Price)
	}
	t.Logf("created price list item %d, code %q, price %v",
		priceItemID, priceItem.Code, priceItem.Price)

	// A system default capital asset type must not be deletable.
	types, _, err := client.CapitalAssetTypes.List(ctx, nil)
	if err != nil {
		t.Fatalf("CapitalAssetTypes.List: %v", err)
	}
	for _, candidate := range types {
		if candidate.SystemDefault == nil || !*candidate.SystemDefault {
			continue
		}
		id := mustID(t, candidate.URL)
		if _, err := client.CapitalAssetTypes.Delete(ctx, id); err == nil {
			t.Fatalf("deleted system default type %q, which should be protected", candidate.Name)
		} else if !errors.Is(err, ErrValidation) && !errors.Is(err, ErrForbidden) {
			t.Logf("system default delete refused with: %v", err)
		}
		break
	}

	capture(t, client, tag, map[string]captureTarget{
		"capital_asset_types": {Path: "capital_asset_types"},
		"stock_items":         {Path: "stock_items"},
		"price_list_items":    {Path: "price_list_items"},
		"ec_moss_rates": {Path: "ec_moss/sales_tax_rates", Query: url.Values{
			"country": {"Ireland"},
			"date":    {DateOf(time.Now()).String()},
		}},
	})
}
