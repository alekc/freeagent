package freeagent

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// Task is a unit of work within a project, used to bill timeslips.
//
// See https://dev.freeagent.com/docs/tasks
type Task struct {
	URL ResourceURL `json:"url,omitempty"`

	Project  ResourceURL `json:"project,omitempty"`
	Name     string      `json:"name,omitempty"`
	Currency string      `json:"currency,omitempty"`
	// Status is Active, Completed or Hidden.
	Status     string `json:"status,omitempty"`
	IsBillable *bool  `json:"is_billable,omitempty"`

	// Needs the Contacts and Projects permission. BillingPeriod is day or
	// hour.
	BillingRate   *Decimal `json:"billing_rate,omitempty"`
	BillingPeriod string   `json:"billing_period,omitempty"`

	// Read-only.
	IsDeletable *bool `json:"is_deletable,omitempty"`
	CreatedAt   Time  `json:"created_at,omitzero"`
	UpdatedAt   Time  `json:"updated_at,omitzero"`
}

// Views accepted by the tasks list endpoint.
const (
	TaskViewAll       = "all"
	TaskViewActive    = "active"
	TaskViewCompleted = "completed"
	TaskViewHidden    = "hidden"
)

// TaskService covers https://dev.freeagent.com/docs/tasks
//
// Creating a task differs from the other collections: the parent project goes
// in the query string rather than the body, so CreateForProject replaces the
// inherited Create.
type TaskService struct {
	Collection[Task]
}

// Create is not available on tasks: the API takes the parent project as a
// query parameter, so this shadows the inherited Create rather than letting
// it silently post a task with no project. Use CreateForProject.
func (s *TaskService) Create(context.Context, *Task) (*Task, *Response, error) {
	return nil, nil, fmt.Errorf("freeagent: tasks are created under a project, use CreateForProject")
}

// CreateForProject posts a new task under the given project.
func (s *TaskService) CreateForProject(ctx context.Context, project ResourceURL, in *Task) (*Task, *Response, error) {
	if in == nil {
		return nil, nil, fmt.Errorf("freeagent: CreateForProject requires a non-nil task")
	}
	if project.IsZero() {
		return nil, nil, fmt.Errorf("freeagent: CreateForProject requires a project URL")
	}
	query := url.Values{"project": {project.String()}}
	req, err := s.client.newRequest(ctx, http.MethodPost, s.meta.Path, query, map[string]any{s.meta.Singular: in})
	if err != nil {
		return nil, nil, err
	}
	return decodeSingle[Task](s.client, req, s.meta)
}
