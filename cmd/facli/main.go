// Command facli is a small operator tool for the FreeAgent API. It runs the
// OAuth flow, reads any endpoint, and issues raw requests.
//
// It defaults to the sandbox. Production and mutating requests each need an
// explicit opt-in, because the data on the other side is accounting records.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alekc/freeagent"
)

const usageText = `facli: FreeAgent API command line

Usage:
  facli <command> [flags]

Commands:
  auth login      Run the OAuth flow and store a token
  auth status     Show the stored token and its expiry
  get             Read a registered resource collection
  show            Read a single record by URL or resource/id
  raw             Issue an arbitrary request against the API
  schema          Print an endpoint's field paths and types, never its values
  resources       List the registered resource families

Environment:
  FREEAGENT_CLIENT_ID       OAuth client id     (required)
  FREEAGENT_CLIENT_SECRET   OAuth client secret (required)

Run "facli <command> -h" for the flags of a command.
`

func main() {
	os.Exit(exec())
}

// exec exists so the signal cleanup runs before the process exits.
func exec() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "facli: %v\n", err)
		return 1
	}
	return 0
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usageText)
		return errors.New("no command given")
	}
	switch args[0] {
	case "auth":
		return runAuth(ctx, args[1:])
	case "get":
		return runGet(ctx, args[1:])
	case "show":
		return runShow(ctx, args[1:])
	case "raw":
		return runRaw(ctx, args[1:])
	case "schema":
		return runSchema(ctx, args[1:])
	case "resources":
		return runResources(args[1:])
	case "help", "-h", "--help":
		fmt.Print(usageText)
		return nil
	default:
		fmt.Fprint(os.Stderr, usageText)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// parseArgs parses flags that appear before, between, or after positional
// arguments. The stdlib flag package stops at the first non-flag argument,
// which would silently ignore "facli get invoices -env production".
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// common holds the flags every command that talks to the API accepts.
type common struct {
	env           string
	tokenFile     string
	apiVersion    string
	redirect      string
	timeout       time.Duration
	rateLimitTest bool
	readOnly      bool
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.env, "env", freeagent.Sandbox.Name, "environment: sandbox or production")
	fs.StringVar(&c.redirect, "redirect-uri", "", "OAuth redirect URI (default: $FREEAGENT_REDIRECT_URI or "+defaultRedirectURI+")")
	fs.StringVar(&c.tokenFile, "token-file", "", "token file path (default: user config dir)")
	fs.StringVar(&c.apiVersion, "api-version", "", "override the X-Api-Version header")
	fs.DurationVar(&c.timeout, "timeout", 60*time.Second, "per-request timeout")
	fs.BoolVar(&c.rateLimitTest, "rate-limit-test", false, "send X-RateLimit-Test to force sandbox throttling")
	fs.BoolVar(&c.readOnly, "read-only", false, "refuse every mutating request, whatever else is asked for")
}

func (c *common) environment() (freeagent.Environment, error) {
	return freeagent.EnvironmentByName(c.env)
}

func (c *common) store(env freeagent.Environment) (*freeagent.FileStore, error) {
	path := c.tokenFile
	if path == "" {
		var err error
		path, err = freeagent.DefaultTokenPath()
		if err != nil {
			return nil, err
		}
	}
	return freeagent.NewFileStore(path, env.Name)
}

// tokenSource builds a refreshing source for the selected environment. It
// does not require a stored token, so the login command can use it too.
func (c *common) tokenSource(ctx context.Context) (*freeagent.TokenSource, freeagent.Environment, error) {
	env, err := c.environment()
	if err != nil {
		return nil, freeagent.Environment{}, err
	}
	id, secret, err := credentials()
	if err != nil {
		return nil, env, err
	}
	store, err := c.store(env)
	if err != nil {
		return nil, env, err
	}
	source, err := freeagent.NewTokenSource(ctx, env.OAuthConfig(id, secret, c.redirectURI()), store)
	if err != nil {
		return nil, env, err
	}
	return source, env, nil
}

// clientReadOnly builds a client that cannot write, whatever it is asked to
// do. Used by commands that only ever read, so a bug in one of them cannot
// mutate an account.
func (c *common) clientReadOnly(ctx context.Context) (*freeagent.Client, freeagent.Environment, error) {
	return c.buildClient(ctx, freeagent.WithReadOnly())
}

func (c *common) client(ctx context.Context) (*freeagent.Client, freeagent.Environment, error) {
	// FREEAGENT_READ_ONLY is a blanket safety catch for a shell pointed at an
	// account that must not be touched.
	var extra []freeagent.Option
	if os.Getenv("FREEAGENT_READ_ONLY") == "1" {
		extra = append(extra, freeagent.WithReadOnly())
	}
	return c.buildClient(ctx, extra...)
}

func (c *common) buildClient(ctx context.Context, extra ...freeagent.Option) (*freeagent.Client, freeagent.Environment, error) {
	source, env, err := c.tokenSource(ctx)
	if err != nil {
		return nil, env, err
	}
	opts := []freeagent.Option{
		freeagent.WithBaseURL(env.BaseURL),
		freeagent.WithTokenSource(source),
		freeagent.WithUserAgent("facli/" + freeagent.Version + " (+https://github.com/alekc/freeagent)"),
		freeagent.WithRateLimitTest(c.rateLimitTest),
	}
	if c.apiVersion != "" {
		opts = append(opts, freeagent.WithAPIVersion(c.apiVersion))
	}
	if c.readOnly {
		opts = append(opts, freeagent.WithReadOnly())
	}
	opts = append(opts, extra...)
	client, err := freeagent.NewClient(opts...)
	if err != nil {
		return nil, env, err
	}
	return client, env, nil
}

// credentials reads the OAuth app credentials. Both are required, and an
// empty one is reported here rather than as a confusing 401 later.
func credentials() (string, string, error) {
	id := strings.TrimSpace(os.Getenv("FREEAGENT_CLIENT_ID"))
	secret := strings.TrimSpace(os.Getenv("FREEAGENT_CLIENT_SECRET"))
	var missing []string
	if id == "" {
		missing = append(missing, "FREEAGENT_CLIENT_ID")
	}
	if secret == "" {
		missing = append(missing, "FREEAGENT_CLIENT_SECRET")
	}
	if len(missing) > 0 {
		return "", "", fmt.Errorf("%s not set; create an app at https://dev.freeagent.com/apps and export both", strings.Join(missing, " and "))
	}
	return id, secret, nil
}

// redirectURI resolves the OAuth callback: flag, then environment, then the
// loopback default. It must match a URI registered on the FreeAgent app.
func (c *common) redirectURI() string {
	if v := strings.TrimSpace(c.redirect); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("FREEAGENT_REDIRECT_URI")); v != "" {
		return v
	}
	return defaultRedirectURI
}

// paramFlag collects repeated -param key=value pairs into a query string.
type paramFlag struct{ values url.Values }

func (p *paramFlag) String() string {
	if p.values == nil {
		return ""
	}
	return p.values.Encode()
}

func (p *paramFlag) Set(raw string) error {
	key, value, found := strings.Cut(raw, "=")
	if !found || key == "" {
		return fmt.Errorf("expected key=value, got %q", raw)
	}
	if p.values == nil {
		p.values = url.Values{}
	}
	p.values.Add(key, value)
	return nil
}

func runResources(args []string) error {
	fs := flag.NewFlagSet("resources", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	for _, name := range freeagent.ResourceNames() {
		meta, _ := freeagent.LookupResource(name)
		var notes []string
		if meta.Singleton {
			notes = append(notes, "singleton")
		}
		if meta.ReadOnly {
			notes = append(notes, "read-only")
		}
		if meta.Grouped {
			notes = append(notes, "grouped")
		}
		if meta.NoList {
			notes = append(notes, "no list")
		}
		if meta.RequiresBankAccount {
			notes = append(notes, "needs bank_account")
		}
		flags := ""
		if len(notes) > 0 {
			flags = " (" + strings.Join(notes, ", ") + ")"
		}
		fmt.Printf("%-30s /v2/%s%s\n", name, meta.Path, flags)
	}
	fmt.Printf("\n%d registered. Use \"facli raw GET <path>\" for endpoints not listed.\n", len(freeagent.ResourceNames()))
	return nil
}
