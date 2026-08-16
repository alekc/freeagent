package freeagent_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/alekc/freeagent"
)

// Build a client backed by a file-stored token that refreshes itself. This is
// the shape the README documents; keeping it here means it cannot drift.
func Example_newClient() {
	ctx := context.Background()

	path, err := freeagent.DefaultTokenPath()
	if err != nil {
		log.Fatal(err)
	}
	store, err := freeagent.NewFileStore(path, freeagent.Sandbox.Name)
	if err != nil {
		log.Fatal(err)
	}
	config := freeagent.Sandbox.OAuthConfig(
		os.Getenv("FREEAGENT_CLIENT_ID"),
		os.Getenv("FREEAGENT_CLIENT_SECRET"),
		"http://localhost:8723/callback",
	)
	source, err := freeagent.NewTokenSource(ctx, config, store)
	if err != nil {
		log.Fatal(err)
	}

	client, err := freeagent.NewClient(
		freeagent.WithBaseURL(freeagent.Sandbox.BaseURL),
		freeagent.WithTokenSource(source),
		freeagent.WithUserAgent("my-app/1.0"),
	)
	if err != nil {
		log.Fatal(err)
	}

	body, _, err := client.Raw(ctx, http.MethodGet, "company", nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(body))
}

// Classify a failure without matching on message text.
func ExampleAPIError() {
	err := error(&freeagent.APIError{
		StatusCode: http.StatusUnprocessableEntity,
		Method:     http.MethodPost,
		URL:        "/v2/invoices",
		Errors: []freeagent.FieldError{
			{Field: "dated_on", Message: "can't be blank"},
		},
	})

	if errors.Is(err, freeagent.ErrValidation) {
		var apiErr *freeagent.APIError
		if errors.As(err, &apiErr) {
			for _, fieldErr := range apiErr.Errors {
				fmt.Printf("%s: %s\n", fieldErr.Field, fieldErr.Message)
			}
		}
	}
	// Output: dated_on: can't be blank
}

// ListOptions renders the filters an incremental read depends on.
func ExampleListOptions() {
	cursor := time.Date(2026, time.August, 1, 9, 30, 0, 0, time.UTC)
	opts := &freeagent.ListOptions{
		UpdatedSince: freeagent.TimeOf(cursor),
		Sort:         "updated_at",
		PerPage:      freeagent.MaxPerPage,
	}
	values, err := opts.Values()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(values.Encode())
	// Output: per_page=100&sort=updated_at&updated_since=2026-08-01T09%3A30%3A00.000Z
}

// Payload cross-references are URLs, not ids.
func ExampleResourceURL() {
	ref := freeagent.ResourceURL("https://api.freeagent.com/v2/bank_accounts/1")

	id, err := ref.ID()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(id, ref.Kind())

	// A singleton is not a collection member, and Kind says so too.
	company := freeagent.ResourceURL("https://api.freeagent.com/v2/company")
	_, err = company.ID()
	fmt.Println(errors.Is(err, freeagent.ErrNotAMember), company.Kind() == "")
	// Output:
	// 1 bank_accounts
	// true true
}
