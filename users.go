package freeagent

import (
	"context"
	"fmt"
	"net/http"
)

// User is a person with access to the FreeAgent account.
//
// See https://dev.freeagent.com/docs/users
type User struct {
	URL ResourceURL `json:"url,omitempty"`

	Email     string `json:"email,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	// Role is Owner, Director, Partner, Company Secretary, Employee,
	// Shareholder or Accountant.
	Role string `json:"role,omitempty"`

	NINumber           string `json:"ni_number,omitempty"`
	UniqueTaxReference string `json:"unique_tax_reference,omitempty"`

	OpeningMileage *Decimal `json:"opening_mileage,omitempty"`
	// PermissionLevel is 0 to 8; see the access levels in the API docs.
	PermissionLevel *int `json:"permission_level,omitempty"`
	// SendInvitation asks FreeAgent to email a password invitation. Write-only.
	SendInvitation *bool `json:"send_invitation,omitempty"`

	// Hidden is undocumented but returned by the live API.
	Hidden *bool `json:"hidden,omitempty"`

	// Read-only.
	CurrentPayrollProfile *UserPayrollProfile `json:"current_payroll_profile,omitempty"`
	CreatedAt             Time                `json:"created_at,omitzero"`
	UpdatedAt             Time                `json:"updated_at,omitzero"`
}

// UserPayrollProfile is the read-only payroll summary present when a profile
// has been set for the current tax year.
type UserPayrollProfile struct {
	TotalPayInPreviousEmployment *Decimal `json:"total_pay_in_previous_employment,omitempty"`
	TotalTaxInPreviousEmployment *Decimal `json:"total_tax_in_previous_employment,omitempty"`
}

// Views accepted by the users list endpoint.
const (
	UserViewAll            = "all"
	UserViewStaff          = "staff"
	UserViewActiveStaff    = "active_staff"
	UserViewAdvisors       = "advisors"
	UserViewActiveAdvisors = "active_advisors"
)

// UserService covers https://dev.freeagent.com/docs/users
type UserService struct {
	Collection[User]
}

// Me returns the user the access token belongs to.
func (s *UserService) Me(ctx context.Context) (*User, *Response, error) {
	req, err := s.client.newRequest(ctx, http.MethodGet, s.meta.Path+"/me", nil, nil)
	if err != nil {
		return nil, nil, err
	}
	return decodeSingle[User](s.client, req, s.meta)
}

// UpdateMe updates the authenticated user.
func (s *UserService) UpdateMe(ctx context.Context, in *User) (*User, *Response, error) {
	if in == nil {
		return nil, nil, fmt.Errorf("freeagent: UpdateMe requires a non-nil user")
	}
	req, err := s.client.newRequest(ctx, http.MethodPut, s.meta.Path+"/me", nil, map[string]any{s.meta.Singular: in})
	if err != nil {
		return nil, nil, err
	}
	return decodeSingle[User](s.client, req, s.meta)
}
