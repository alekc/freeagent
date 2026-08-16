package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/alekc/freeagent"
)

// defaultRedirectURI is a loopback callback. It has to be registered on the
// FreeAgent app before the flow will accept it.
const defaultRedirectURI = "http://localhost:8723/callback"

// callbackTimeout bounds how long login waits for the browser round trip.
const callbackTimeout = 5 * time.Minute

func runAuth(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("auth needs a subcommand: login or status")
	}
	switch args[0] {
	case "login":
		return runAuthLogin(ctx, args[1:])
	case "status":
		return runAuthStatus(ctx, args[1:])
	default:
		return fmt.Errorf("unknown auth subcommand %q, want login or status", args[0])
	}
}

func runAuthLogin(ctx context.Context, args []string) error {
	var (
		c      common
		manual bool
	)
	fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
	c.register(fs)
	fs.BoolVar(&manual, "manual", false, "paste the authorisation code instead of listening for the callback")
	if err := fs.Parse(args); err != nil {
		return err
	}

	source, env, err := c.tokenSource(ctx)
	if err != nil {
		return err
	}
	state, err := randomState()
	if err != nil {
		return err
	}

	redirect := c.redirectURI()
	var (
		listener  net.Listener
		codeCh    chan callbackResult
		serverErr chan error
	)
	if !manual {
		listener, codeCh, serverErr, err = startCallbackServer(ctx, redirect, state)
		if err != nil {
			return fmt.Errorf("%w\nuse -manual to paste the code instead", err)
		}
		defer func() { _ = listener.Close() }()
	}

	fmt.Printf("Environment: %s\nRedirect URI: %s\n\nOpen this URL and approve the app:\n\n%s\n\n", env.Name, redirect, source.AuthCodeURL(state))

	var code string
	if manual {
		code, err = readCode()
	} else {
		fmt.Printf("Waiting up to %s for the callback on %s ...\n", callbackTimeout, redirect)
		code, err = waitForCallback(ctx, codeCh, serverErr)
	}
	if err != nil {
		return err
	}

	token, err := source.Exchange(ctx, code)
	if err != nil {
		return err
	}

	store, err := c.store(env)
	if err != nil {
		return err
	}
	fmt.Printf("\nToken stored in %s under %q.\nAccess token expires in %s.\n", store.Path(), env.Name, freeagent.ExpiresIn(token, time.Now()))

	// Verify the credential end to end rather than trusting the exchange.
	client, _, err := c.client(ctx)
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	body, _, err := client.Raw(reqCtx, http.MethodGet, "company", nil, nil)
	if err != nil {
		return fmt.Errorf("token stored but the verification call failed: %w", err)
	}
	fmt.Printf("Verified: GET /v2/company returned %d bytes.\n", len(body))
	return nil
}

func runAuthStatus(ctx context.Context, args []string) error {
	var c common
	fs := flag.NewFlagSet("auth status", flag.ContinueOnError)
	c.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	source, env, err := c.tokenSource(ctx)
	if err != nil {
		return err
	}
	store, err := c.store(env)
	if err != nil {
		return err
	}

	token, err := source.Peek()
	if errors.Is(err, freeagent.ErrNoToken) {
		fmt.Printf("Environment: %s\nToken file:  %s\nStatus:      no token stored, run \"facli auth login -env %s\"\n", env.Name, store.Path(), env.Name)
		return nil
	}
	if err != nil {
		return err
	}
	// Never print the tokens themselves, only whether they are present.
	fmt.Printf("Environment:   %s\nToken file:    %s\nAccess token:  present, expires in %s\nRefresh token: %s\n",
		env.Name, store.Path(), freeagent.ExpiresIn(token, time.Now()), presence(token.RefreshToken))
	return nil
}

func presence(v string) string {
	if v == "" {
		return "absent"
	}
	return "present"
}

type callbackResult struct {
	code string
	err  error
}

// startCallbackServer listens on the loopback address in the redirect URI and
// resolves once the browser arrives with a matching state.
func startCallbackServer(ctx context.Context, redirect, state string) (net.Listener, chan callbackResult, chan error, error) {
	u, err := url.Parse(redirect)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid redirect URI %q: %w", redirect, err)
	}
	if u.Scheme != "http" || !isLoopback(u.Hostname()) {
		return nil, nil, nil, fmt.Errorf("redirect URI %q is not a loopback http address, so facli cannot listen for it", redirect)
	}
	var config net.ListenConfig
	listener, err := config.Listen(ctx, "tcp", u.Host)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("listening on %s: %w", u.Host, err)
	}

	results := make(chan callbackResult, 1)
	errs := make(chan error, 1)
	path := u.Path
	if path == "" {
		path = "/"
	}

	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case query.Get("error") != "":
			err := fmt.Errorf("authorisation denied: %s %s", query.Get("error"), query.Get("error_description"))
			http.Error(w, err.Error(), http.StatusBadRequest)
			results <- callbackResult{err: err}
		case query.Get("state") != state:
			// A mismatch means the response is not the one this process
			// started, so the code is not trustworthy.
			err := errors.New("state mismatch in the OAuth callback")
			http.Error(w, err.Error(), http.StatusBadRequest)
			results <- callbackResult{err: err}
		case query.Get("code") == "":
			err := errors.New("callback carried no authorisation code")
			http.Error(w, err.Error(), http.StatusBadRequest)
			results <- callbackResult{err: err}
		default:
			_, _ = fmt.Fprint(w, "Authorisation received. You can close this tab and return to the terminal.")
			results <- callbackResult{code: query.Get("code")}
		}
	})

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()
	return listener, results, errs, nil
}

func waitForCallback(ctx context.Context, results chan callbackResult, errs chan error) (string, error) {
	timer := time.NewTimer(callbackTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case err := <-errs:
		return "", fmt.Errorf("callback server: %w", err)
	case res := <-results:
		return res.code, res.err
	case <-timer.C:
		return "", fmt.Errorf("no callback within %s", callbackTimeout)
	}
}

func readCode() (string, error) {
	fmt.Print("Paste the code parameter from the redirect URL: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading the authorisation code: %w", err)
	}
	code := strings.TrimSpace(line)
	if code == "" {
		return "", errors.New("no authorisation code given")
	}
	return code, nil
}

func randomState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating OAuth state: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
