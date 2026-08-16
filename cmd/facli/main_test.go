package main

import (
	"flag"
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/alekc/freeagent"
)

// The stdlib flag package stops at the first positional argument, so flags
// written after the resource name would otherwise be silently ignored.
func TestParseArgsPermutesFlagsAndPositionals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		wantPos []string
		wantEnv string
		wantAll bool
	}{
		{
			name:    "flags first",
			args:    []string{"-env", "production", "invoices"},
			wantPos: []string{"invoices"},
			wantEnv: "production",
		},
		{
			name:    "flags after the positional",
			args:    []string{"invoices", "-env", "production"},
			wantPos: []string{"invoices"},
			wantEnv: "production",
		},
		{
			name:    "flags on both sides",
			args:    []string{"-all", "invoices", "-env", "production"},
			wantPos: []string{"invoices"},
			wantEnv: "production",
			wantAll: true,
		},
		{
			name:    "two positionals with a flag between",
			args:    []string{"GET", "-env", "sandbox", "invoices/1"},
			wantPos: []string{"GET", "invoices/1"},
			wantEnv: "sandbox",
		},
		{
			name:    "no positionals",
			args:    []string{"-env", "sandbox"},
			wantPos: nil,
			wantEnv: "sandbox",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var (
				env      string
				fetchAll bool
			)
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			fs.StringVar(&env, "env", "sandbox", "")
			fs.BoolVar(&fetchAll, "all", false, "")

			got, err := parseArgs(fs, tc.args)
			if err != nil {
				t.Fatalf("parseArgs = %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tc.wantPos, ",") {
				t.Fatalf("positionals = %v, want %v", got, tc.wantPos)
			}
			if env != tc.wantEnv {
				t.Fatalf("env = %q, want %q", env, tc.wantEnv)
			}
			if fetchAll != tc.wantAll {
				t.Fatalf("all = %v, want %v", fetchAll, tc.wantAll)
			}
		})
	}
}

func TestParseArgsReportsFlagErrors(t *testing.T) {
	t.Parallel()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.String("env", "sandbox", "")
	if _, err := parseArgs(fs, []string{"invoices", "-nope"}); err == nil {
		t.Fatal("parseArgs accepted an unknown flag, want an error")
	}
}

func TestParamFlag(t *testing.T) {
	t.Parallel()
	var p paramFlag
	for _, in := range []string{"view=open", "bank_account=https://api.freeagent.com/v2/bank_accounts/1", "view=paid"} {
		if err := p.Set(in); err != nil {
			t.Fatalf("Set(%q) = %v", in, err)
		}
	}
	want := url.Values{
		"view":         {"open", "paid"},
		"bank_account": {"https://api.freeagent.com/v2/bank_accounts/1"},
	}
	if p.String() != want.Encode() {
		t.Fatalf("String() = %q, want %q", p.String(), want.Encode())
	}
	for _, bad := range []string{"noequals", "=novalue"} {
		if err := p.Set(bad); err == nil {
			t.Fatalf("Set(%q) succeeded, want an error", bad)
		}
	}
	// The zero value must render without panicking.
	var empty paramFlag
	if empty.String() != "" {
		t.Fatalf("zero paramFlag.String() = %q, want empty", empty.String())
	}
}

func TestMutatingMethods(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if !mutating(method) {
			t.Fatalf("mutating(%s) = false, want true", method)
		}
	}
	for _, method := range []string{"GET", "HEAD", "OPTIONS"} {
		if mutating(method) {
			t.Fatalf("mutating(%s) = true, want false", method)
		}
	}
}

// A write must not go out on the strength of the verb alone.
func TestConfirmMutationRequiresYes(t *testing.T) {
	t.Parallel()
	sandbox, err := freeagent.EnvironmentByName("sandbox")
	if err != nil {
		t.Fatalf("environment = %v", err)
	}
	if err := confirmMutation("GET", sandbox, false); err != nil {
		t.Fatalf("reads must not need confirmation: %v", err)
	}
	if err := confirmMutation("POST", sandbox, false); err == nil {
		t.Fatal("POST without -yes succeeded, want an error")
	}
	if err := confirmMutation("POST", sandbox, true); err != nil {
		t.Fatalf("POST with -yes against sandbox = %v, want nil", err)
	}
}

func TestRequestBody(t *testing.T) {
	t.Parallel()
	if got, err := requestBody(""); err != nil || got != nil {
		t.Fatalf("requestBody(\"\") = %v, %v, want nil, nil", got, err)
	}
	if _, err := requestBody(`{"invoice":{"status":"Sent"}}`); err != nil {
		t.Fatalf("requestBody(json) = %v", err)
	}
	// Malformed JSON must fail locally rather than on the wire.
	if _, err := requestBody(`{"invoice":`); err == nil {
		t.Fatal("requestBody accepted malformed JSON, want an error")
	}
	if _, err := requestBody("@/nonexistent/path.json"); err == nil {
		t.Fatal("requestBody accepted a missing file, want an error")
	}
}

// Not parallel: t.Setenv is incompatible with parallel tests.
func TestRedirectURIPrecedence(t *testing.T) {
	c := common{redirect: "http://localhost:9999/cb"}
	if got := c.redirectURI(); got != "http://localhost:9999/cb" {
		t.Fatalf("redirectURI = %q, want the flag value", got)
	}
	t.Setenv("FREEAGENT_REDIRECT_URI", "http://localhost:1234/env")
	var fromEnv common
	if got := fromEnv.redirectURI(); got != "http://localhost:1234/env" {
		t.Fatalf("redirectURI = %q, want the environment value", got)
	}
}

func TestIsLoopback(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"localhost", "127.0.0.1", "::1"} {
		if !isLoopback(host) {
			t.Fatalf("isLoopback(%q) = false, want true", host)
		}
	}
	for _, host := range []string{"example.com", "10.0.0.1", ""} {
		if isLoopback(host) {
			t.Fatalf("isLoopback(%q) = true, want false", host)
		}
	}
}
