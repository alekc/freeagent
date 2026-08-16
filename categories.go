package freeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Category is an accounting category, identified by its nominal code rather
// than a numeric id.
//
// See https://dev.freeagent.com/docs/categories
type Category struct {
	URL ResourceURL `json:"url,omitempty"`

	Description string `json:"description,omitempty"`
	// NominalCode identifies the category. On create it must be free and
	// inside the range the group allows: admin expenses are 200 to 399.
	NominalCode string `json:"nominal_code,omitempty"`
	// GroupDescription is present on income and spending categories. It is
	// read-only in practice: writes use CategoryGroup instead.
	GroupDescription string `json:"group_description,omitempty"`
	// CategoryGroup selects the group on create and is required there, even
	// though the documented attribute list does not mention it. Omitting it
	// returns 422 "'' is not a valid category_group". Values are the group
	// names without the _categories suffix: admin_expenses, cost_of_sales,
	// income, general.
	CategoryGroup string `json:"category_group,omitempty"`
	// AllowableForTax appears on spending categories only.
	AllowableForTax *bool `json:"allowable_for_tax,omitempty"`
	// TaxReportingName is where the category lands in statutory accounts. It
	// is required on create and only accepts values from a fixed list, which
	// is not published: read one off an existing category in the same group.
	TaxReportingName string `json:"tax_reporting_name,omitempty"`
	// AutoSalesTaxRate is Outside scope, Zero rate, Reduced rate,
	// Standard rate or Exempt.
	AutoSalesTaxRate string `json:"auto_sales_tax_rate,omitempty"`

	// Sub-account links. Each is present only on the matching code range.
	BankAccount      ResourceURL `json:"bank_account,omitempty"`
	CapitalAssetType ResourceURL `json:"capital_asset_type,omitempty"`
	StockItem        ResourceURL `json:"stock_item,omitempty"`
	HirePurchase     ResourceURL `json:"hire_purchase,omitempty"`
	User             ResourceURL `json:"user,omitempty"`

	// Group is the envelope the category arrived in. It is filled in by this
	// library rather than sent by the API, and is never written back.
	Group string `json:"-"`
}

// The four envelope keys the categories endpoint groups results under.
const (
	CategoryGroupAdminExpenses = "admin_expenses_categories"
	CategoryGroupCostOfSales   = "cost_of_sales_categories"
	CategoryGroupIncome        = "income_categories"
	CategoryGroupGeneral       = "general_categories"
)

// categoryGroups is the order groups are reported in.
var categoryGroups = []string{
	CategoryGroupAdminExpenses,
	CategoryGroupCostOfSales,
	CategoryGroupIncome,
	CategoryGroupGeneral,
}

// CategoryGroups is the grouped response the categories endpoint returns.
// Unlike every other collection there is no flat list, so this mirrors the
// envelope rather than pretending otherwise.
type CategoryGroups struct {
	AdminExpenses []Category `json:"admin_expenses_categories,omitempty"`
	CostOfSales   []Category `json:"cost_of_sales_categories,omitempty"`
	Income        []Category `json:"income_categories,omitempty"`
	General       []Category `json:"general_categories,omitempty"`
}

// Flatten returns every category with Group set, for callers that want one
// list rather than four.
func (g *CategoryGroups) Flatten() []Category {
	if g == nil {
		return nil
	}
	out := make([]Category, 0, len(g.AdminExpenses)+len(g.CostOfSales)+len(g.Income)+len(g.General))
	for group, items := range map[string][]Category{
		CategoryGroupAdminExpenses: g.AdminExpenses,
		CategoryGroupCostOfSales:   g.CostOfSales,
		CategoryGroupIncome:        g.Income,
		CategoryGroupGeneral:       g.General,
	} {
		for _, item := range items {
			item.Group = group
			out = append(out, item)
		}
	}
	return out
}

// CategoryService covers https://dev.freeagent.com/docs/categories
//
// Categories do not fit the generic collection: results come back grouped
// under four keys with no flat list, and records are addressed by nominal
// code rather than a numeric id.
type CategoryService struct {
	client *Client
	meta   ResourceMeta
}

// Meta returns the resource metadata.
func (s *CategoryService) Meta() ResourceMeta { return s.meta }

// List returns the categories grouped as the API reports them. Set
// subAccounts to fetch sub-accounts in place of their parent accounts.
func (s *CategoryService) List(ctx context.Context, subAccounts bool) (*CategoryGroups, *Response, error) {
	var query url.Values
	if subAccounts {
		query = url.Values{"sub_accounts": {"true"}}
	}
	req, err := s.client.newRequest(ctx, http.MethodGet, s.meta.Path, query, nil)
	if err != nil {
		return nil, nil, err
	}
	out := new(CategoryGroups)
	resp, err := s.client.do(req, out)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// Get fetches one category by nominal code. The reply is nested under
// whichever group the category belongs to, so the group is resolved here and
// recorded on the result.
func (s *CategoryService) Get(ctx context.Context, nominalCode string) (*Category, *Response, error) {
	path, err := s.memberPath(nominalCode)
	if err != nil {
		return nil, nil, err
	}
	req, err := s.client.newRequest(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	return s.decodeGrouped(req, path)
}

// Create adds a user-defined category.
func (s *CategoryService) Create(ctx context.Context, in *Category) (*Category, *Response, error) {
	if in == nil {
		return nil, nil, fmt.Errorf("freeagent: Create(categories) requires a non-nil category")
	}
	req, err := s.client.newRequest(ctx, http.MethodPost, s.meta.Path, nil, map[string]any{"category": in})
	if err != nil {
		return nil, nil, err
	}
	return s.decodeGrouped(req, s.meta.Path)
}

// Update changes a category. Only categories with no items can be updated.
func (s *CategoryService) Update(ctx context.Context, nominalCode string, in *Category) (*Category, *Response, error) {
	if in == nil {
		return nil, nil, fmt.Errorf("freeagent: Update(categories) requires a non-nil category")
	}
	path, err := s.memberPath(nominalCode)
	if err != nil {
		return nil, nil, err
	}
	req, err := s.client.newRequest(ctx, http.MethodPut, path, nil, map[string]any{"category": in})
	if err != nil {
		return nil, nil, err
	}
	return s.decodeGrouped(req, path)
}

// Delete removes a category. Only user-created categories with no items can
// be deleted.
func (s *CategoryService) Delete(ctx context.Context, nominalCode string) (*Response, error) {
	path, err := s.memberPath(nominalCode)
	if err != nil {
		return nil, err
	}
	req, err := s.client.newRequest(ctx, http.MethodDelete, path, nil, nil)
	if err != nil {
		return nil, err
	}
	return s.client.do(req, nil)
}

// memberPath validates the nominal code before it reaches a URL. Codes are
// short alphanumeric strings such as "285" or "750-1"; rejecting anything
// else here keeps a stray slash from addressing a different endpoint.
func (s *CategoryService) memberPath(nominalCode string) (string, error) {
	code := strings.TrimSpace(nominalCode)
	if code == "" {
		return "", fmt.Errorf("freeagent: a nominal code is required")
	}
	for _, r := range code {
		if (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '-' && r != '_' {
			return "", fmt.Errorf("freeagent: %q is not a valid nominal code", truncate(code, 32))
		}
	}
	return s.meta.Path + "/" + code, nil
}

// decodeGrouped unwraps a single category from whichever group key the API
// chose. A single-key envelope that is not one of the four is reported rather
// than guessed at.
func (s *CategoryService) decodeGrouped(req *http.Request, path string) (*Category, *Response, error) {
	var envelope map[string]json.RawMessage
	resp, err := s.client.do(req, &envelope)
	if err != nil {
		return nil, resp, err
	}
	for _, group := range categoryGroups {
		raw, ok := envelope[group]
		if !ok {
			continue
		}
		out := new(Category)
		if err := json.Unmarshal(raw, out); err != nil {
			return nil, resp, fmt.Errorf("freeagent: decoding %s from %s response: %w", group, path, err)
		}
		out.Group = group
		return out, resp, nil
	}
	return nil, resp, fmt.Errorf("freeagent: %s response carried no known category group, got keys %v", path, envelopeKeys(envelope))
}
