package freeagent

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// PayrollPeriod is one pay run in a tax year. Every field is read-only:
// payroll is filed through the FreeAgent interface, not the API.
//
// See https://dev.freeagent.com/docs/payroll
type PayrollPeriod struct {
	URL ResourceURL `json:"url,omitempty"`

	Period    *int   `json:"period,omitempty"`
	Frequency string `json:"frequency,omitempty"`
	DatedOn   Date   `json:"dated_on,omitzero"`
	// Status is unfiled, pending, rejected, partially_filed or filed.
	Status string `json:"status,omitempty"`

	EmploymentAllowanceClaimed          *bool    `json:"employment_allowance_claimed,omitempty"`
	EmploymentAllowanceAmount           *Decimal `json:"employment_allowance_amount,omitempty"`
	ConstructionIndustrySchemeDeduction *Decimal `json:"construction_industry_scheme_deduction,omitempty"`

	// Payslips is populated when a single period is fetched, and absent from
	// the year listing.
	Payslips []Payslip `json:"payslips,omitempty"`

	CreatedAt Time `json:"created_at,omitzero"`
	UpdatedAt Time `json:"updated_at,omitzero"`
}

// PayrollYear is what a year's payroll listing returns: its periods and the
// payments due to HMRC.
type PayrollYear struct {
	Periods  []PayrollPeriod `json:"periods,omitempty"`
	Payments []FilingPayment `json:"payments,omitempty"`
}

// Payslip is one employee's pay for one period. Every field is read-only.
//
// The field list is long because RTI reporting is: statutory payments,
// pension arrangements and loan plans each need their own line.
type Payslip struct {
	User    ResourceURL `json:"user,omitempty"`
	DatedOn Date        `json:"dated_on,omitzero"`

	TaxCode   string `json:"tax_code,omitempty"`
	NILetter  string `json:"ni_letter,omitempty"`
	Frequency string `json:"frequency,omitempty"`
	// NICalcType is the National Insurance calculation basis.
	NICalcType string `json:"ni_calc_type,omitempty"`

	BasicPay    *Decimal `json:"basic_pay,omitempty"`
	Overtime    *Decimal `json:"overtime,omitempty"`
	Commission  *Decimal `json:"commission,omitempty"`
	Bonus       *Decimal `json:"bonus,omitempty"`
	Allowance   *Decimal `json:"allowance,omitempty"`
	HoursWorked *Decimal `json:"hours_worked,omitempty"`

	TaxDeducted *Decimal `json:"tax_deducted,omitempty"`
	EmployeeNI  *Decimal `json:"employee_ni,omitempty"`
	EmployerNI  *Decimal `json:"employer_ni,omitempty"`

	// Statutory payments.
	StatutorySickPay                *Decimal `json:"statutory_sick_pay,omitempty"`
	StatutoryMaternityPay           *Decimal `json:"statutory_maternity_pay,omitempty"`
	StatutoryPaternityPay           *Decimal `json:"statutory_paternity_pay,omitempty"`
	AdditionalStatutoryPaternityPay *Decimal `json:"additional_statutory_paternity_pay,omitempty"`
	StatutoryAdoptionPay            *Decimal `json:"statutory_adoption_pay,omitempty"`
	StatutoryParentalBereavementPay *Decimal `json:"statutory_parental_bereavement_pay,omitempty"`
	// Undocumented, but sent by the live API: Northern Ireland has its own
	// parental bereavement figure.
	StatutoryParentalBereavementPayNIreland *Decimal `json:"statutory_parental_bereavement_pay_n_ireland,omitempty"`
	StatutoryNeonatalCarePay                *Decimal `json:"statutory_neonatal_care_pay,omitempty"`
	AbsencePayments                         *Decimal `json:"absence_payments,omitempty"`
	OtherPayments                           *Decimal `json:"other_payments,omitempty"`

	// Pensions and salary sacrifice.
	EmployeePension                *Decimal `json:"employee_pension,omitempty"`
	EmployerPension                *Decimal `json:"employer_pension,omitempty"`
	EmployeePensionNotUnderNetPay  *Decimal `json:"employee_pension_not_under_net_pay,omitempty"`
	EmployeePensionSalarySacrifice *Decimal `json:"employee_pension_salary_sacrifice,omitempty"`
	OtherSalarySacrificeDeductions *Decimal `json:"other_salary_sacrifice_deductions,omitempty"`

	// Deductions.
	OtherDeductions                  *Decimal `json:"other_deductions,omitempty"`
	OtherDeductionsFromNetPay        *Decimal `json:"other_deductions_from_net_pay,omitempty"`
	DeductionsSubjectToNICButNotPAYE *Decimal `json:"deductions_subject_to_nic_but_not_paye,omitempty"`
	DeductionFreePay                 *Decimal `json:"deduction_free_pay,omitempty"`
	Attachments                      *Decimal `json:"attachments,omitempty"`
	PayrollGiving                    *Decimal `json:"payroll_giving,omitempty"`

	// Student and postgraduate loans.
	StudentLoanDeduction     *Decimal `json:"student_loan_deduction,omitempty"`
	DeductStudentLoan        *bool    `json:"deduct_student_loan,omitempty"`
	StudentLoanDeductionPlan string   `json:"student_loan_deductions_plan,omitempty"`
	PostgradLoanDeduction    *Decimal `json:"postgrad_loan_deduction,omitempty"`
	DeductPostgradLoan       *bool    `json:"deduct_postgrad_loan,omitempty"`

	Week1Month1Basis *bool `json:"week_1_month_1_basis,omitempty"`
	LeavingPayslip   *bool `json:"leaving_payslip,omitempty"`

	CreatedAt Time `json:"created_at,omitzero"`
	UpdatedAt Time `json:"updated_at,omitzero"`
}

// PayrollService covers https://dev.freeagent.com/docs/payroll
//
// Payroll is addressed by tax year rather than by id, and there is no
// collection at /v2/payroll: the year is part of the path. UK companies with
// payroll only.
type PayrollService struct {
	client *Client
	meta   ResourceMeta
}

// Meta returns the resource metadata.
func (s *PayrollService) Meta() ResourceMeta { return s.meta }

// Year lists the periods and HMRC payments for one tax year.
func (s *PayrollService) Year(ctx context.Context, year int) (*PayrollYear, *Response, error) {
	path, err := payrollYearPath(s.meta.Path, year)
	if err != nil {
		return nil, nil, err
	}
	req, err := s.client.newRequest(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	out := new(PayrollYear)
	resp, err := s.client.do(req, out)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// Period fetches one period of a tax year, including its payslips.
//
// The reply is enveloped under "period" and carries the payslips nested
// inside it, not under a top-level "payslips" key as the documentation's
// wording suggests.
func (s *PayrollService) Period(ctx context.Context, year, period int) (*PayrollPeriod, *Response, error) {
	path, err := payrollYearPath(s.meta.Path, year)
	if err != nil {
		return nil, nil, err
	}
	if period < 0 {
		return nil, nil, fmt.Errorf("freeagent: payroll period must not be negative, got %d", period)
	}
	path += "/" + strconv.Itoa(period)

	req, err := s.client.newRequest(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	var envelope struct {
		Period PayrollPeriod `json:"period"`
	}
	resp, err := s.client.do(req, &envelope)
	if err != nil {
		return nil, resp, err
	}
	return &envelope.Period, resp, nil
}

// Payslips returns just the payslips in one period.
func (s *PayrollService) Payslips(ctx context.Context, year, period int) ([]Payslip, *Response, error) {
	got, resp, err := s.Period(ctx, year, period)
	if err != nil {
		return nil, resp, err
	}
	return got.Payslips, resp, nil
}

// MarkPaymentAsPaid records an HMRC payment for the year as settled.
//
// The documentation lists the unpaid variant as a GET, which is a typo: both
// are PUT, matching every other mark_as_* transition in the API.
func (s *PayrollService) MarkPaymentAsPaid(ctx context.Context, year int, paymentDate Date) (*Response, error) {
	return s.paymentTransition(ctx, year, paymentDate, "mark_as_paid")
}

// MarkPaymentAsUnpaid reverses MarkPaymentAsPaid.
func (s *PayrollService) MarkPaymentAsUnpaid(ctx context.Context, year int, paymentDate Date) (*Response, error) {
	return s.paymentTransition(ctx, year, paymentDate, "mark_as_unpaid")
}

func (s *PayrollService) paymentTransition(ctx context.Context, year int, paymentDate Date, action string) (*Response, error) {
	path, err := payrollYearPath(s.meta.Path, year)
	if err != nil {
		return nil, err
	}
	if paymentDate.IsZero() {
		return nil, fmt.Errorf("freeagent: a payment date is required")
	}
	path += "/payments/" + paymentDate.String() + "/" + action

	req, err := s.client.newRequest(ctx, http.MethodPut, path, nil, nil)
	if err != nil {
		return nil, err
	}
	return s.client.do(req, nil)
}

// payrollYearPath validates the tax year before it reaches a URL.
func payrollYearPath(base string, year int) (string, error) {
	if year < 1900 || year > 2200 {
		return "", fmt.Errorf("freeagent: %d is not a plausible tax year", year)
	}
	return base + "/" + strconv.Itoa(year), nil
}

// PayrollProfile is an employee's payroll details for a tax year.
//
// See https://dev.freeagent.com/docs/payroll_profiles
type PayrollProfile struct {
	User ResourceURL `json:"user,omitempty"`

	PayrollReference string `json:"payroll_reference,omitempty"`
	Title            string `json:"title,omitempty"`
	Gender           string `json:"gender,omitempty"`
	DateOfBirth      Date   `json:"date_of_birth,omitzero"`

	// The address lines are numbered rather than named here, unlike the
	// address1..address3 used everywhere else in the API.
	AddressLine1 string `json:"address_line_1,omitempty"`
	AddressLine2 string `json:"address_line_2,omitempty"`
	AddressLine3 string `json:"address_line_3,omitempty"`
	AddressLine4 string `json:"address_line_4,omitempty"`
	Postcode     string `json:"postcode,omitempty"`
	Country      string `json:"country,omitempty"`

	TotalPayInPreviousEmployment *Decimal `json:"total_pay_in_previous_employment,omitempty"`
	TotalTaxInPreviousEmployment *Decimal `json:"total_tax_in_previous_employment,omitempty"`

	EmploymentStartsOn Date `json:"employment_starts_on,omitzero"`
	EmploymentEndsOn   Date `json:"employment_ends_on,omitzero"`

	CreatedAt Time `json:"created_at,omitzero"`
	UpdatedAt Time `json:"updated_at,omitzero"`
}

// PayrollProfileService covers
// https://dev.freeagent.com/docs/payroll_profiles
//
// Like payroll, addressed by tax year: /v2/payroll_profiles alone is a 404.
type PayrollProfileService struct {
	client *Client
	meta   ResourceMeta
}

// Meta returns the resource metadata.
func (s *PayrollProfileService) Meta() ResourceMeta { return s.meta }

// Year lists every profile for a tax year. Pass a user URL to narrow it to
// one employee, or the zero value for all of them.
func (s *PayrollProfileService) Year(ctx context.Context, year int, user ResourceURL) ([]PayrollProfile, *Response, error) {
	path, err := payrollYearPath(s.meta.Path, year)
	if err != nil {
		return nil, nil, err
	}
	var query url.Values
	if !user.IsZero() {
		if user.Kind() != "users" {
			return nil, nil, fmt.Errorf("freeagent: the user filter needs a user URL, got %q",
				truncate(user.String(), 64))
		}
		query = url.Values{"user": {user.String()}}
	}

	req, err := s.client.newRequest(ctx, http.MethodGet, path, query, nil)
	if err != nil {
		return nil, nil, err
	}
	var envelope struct {
		Profiles []PayrollProfile `json:"profiles"`
	}
	resp, err := s.client.do(req, &envelope)
	if err != nil {
		return nil, resp, err
	}
	return envelope.Profiles, resp, nil
}
