package freeagent

import "context"

// FinalAccountsReport is one accounting period's end-of-year filing state.
//
// See https://dev.freeagent.com/docs/final_accounts_reports
type FinalAccountsReport struct {
	// URL ends in the period end date rather than a numeric id, so
	// ResourceURL.ID does not apply to it. Use PeriodEndsOn.
	URL ResourceURL `json:"url,omitempty"`

	PeriodStartsOn Date `json:"period_starts_on,omitzero"`
	PeriodEndsOn   Date `json:"period_ends_on,omitzero"`
	FilingDueOn    Date `json:"filing_due_on,omitzero"`

	// FilingStatus is draft, unfiled, pending, rejected, filed or
	// marked_as_filed.
	FilingStatus   string `json:"filing_status,omitempty"`
	FiledAt        Time   `json:"filed_at,omitzero"`
	FiledReference string `json:"filed_reference,omitempty"`
}

// Filing statuses a FinalAccountsReport can hold.
const (
	FilingStatusDraft         = "draft"
	FilingStatusUnfiled       = "unfiled"
	FilingStatusPending       = "pending"
	FilingStatusRejected      = "rejected"
	FilingStatusFiled         = "filed"
	FilingStatusMarkedAsFiled = "marked_as_filed"
)

// FinalAccountsReportService covers
// https://dev.freeagent.com/docs/final_accounts_reports
//
// Reports are addressed by the period end date, not a numeric id, which is
// the shape it shares with the VAT, corporation tax and income tax families.
type FinalAccountsReportService struct {
	periodService[FinalAccountsReport]
}

// List returns every accounting period's report.
func (s *FinalAccountsReportService) List(ctx context.Context) ([]FinalAccountsReport, *Response, error) {
	return s.list(ctx, s.meta.Path)
}

// Get fetches one period's report by its end date.
func (s *FinalAccountsReportService) Get(ctx context.Context, periodEndsOn Date) (*FinalAccountsReport, *Response, error) {
	return s.get(ctx, s.meta.Path, periodEndsOn)
}

// MarkAsFiled records the period as filed outside FreeAgent. It needs Full
// Access, or Account Manager on a practice-managed account.
func (s *FinalAccountsReportService) MarkAsFiled(ctx context.Context, periodEndsOn Date) (*FinalAccountsReport, *Response, error) {
	return s.transition(ctx, s.meta.Path, periodEndsOn, "mark_as_filed")
}

// MarkAsUnfiled reverses MarkAsFiled.
func (s *FinalAccountsReportService) MarkAsUnfiled(ctx context.Context, periodEndsOn Date) (*FinalAccountsReport, *Response, error) {
	return s.transition(ctx, s.meta.Path, periodEndsOn, "mark_as_unfiled")
}
