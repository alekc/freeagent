package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/alekc/freeagent"
)

func runRaw(ctx context.Context, args []string) error {
	var (
		c      common
		params paramFlag
		data   string
		yes    bool
	)
	fs := flag.NewFlagSet("raw", flag.ContinueOnError)
	c.register(fs)
	fs.StringVar(&data, "data", "", "request body: inline JSON, @file to read a file, or - for stdin")
	fs.BoolVar(&yes, "yes", false, "confirm a mutating request")
	fs.Var(&params, "param", "query parameter as key=value, repeatable")
	fs.Usage = func() {
		_, _ = fmt.Fprint(fs.Output(), "Usage: facli raw <METHOD> <path> [flags]\n\nExample: facli raw GET invoices/123\n\nFlags:\n")
		fs.PrintDefaults()
	}
	positional, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 2 {
		fs.Usage()
		return errors.New("raw needs a method and a path")
	}

	method := strings.ToUpper(positional[0])
	path := trimAPIPrefix(positional[1])

	env, err := c.environment()
	if err != nil {
		return err
	}
	if err := confirmMutation(method, env, yes); err != nil {
		return err
	}

	body, err := requestBody(data)
	if err != nil {
		return err
	}

	client, _, err := c.client(ctx)
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	out, resp, err := client.Raw(reqCtx, method, path, params.values, body)
	if err != nil {
		return err
	}
	if resp != nil {
		fmt.Fprintf(os.Stderr, "%s %s -> %s\n", method, path, resp.Status)
	}
	return printJSON(out)
}

// confirmMutation gates writes. A mutating verb needs -yes anywhere, and
// against production it also needs the environment name typed back, because
// the records on the other side are real accounts.
func confirmMutation(method string, env freeagent.Environment, yes bool) error {
	if !mutating(method) {
		return nil
	}
	if !yes {
		return fmt.Errorf("%s is a mutating request; re-run with -yes to confirm", method)
	}
	if env.Name != freeagent.Production.Name {
		return nil
	}
	fmt.Fprintf(os.Stderr, "About to send a %s to PRODUCTION.\nType %q to continue: ", method, env.Name)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading confirmation: %w", err)
	}
	if strings.TrimSpace(line) != env.Name {
		return errors.New("confirmation did not match, aborting")
	}
	return nil
}

func mutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// requestBody resolves the -data flag and validates that the result is JSON
// before it reaches the network, so a typo fails locally.
func requestBody(data string) (any, error) {
	if data == "" {
		return nil, nil
	}
	var raw []byte
	switch {
	case data == "-":
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("reading body from stdin: %w", err)
		}
		raw = b
	case strings.HasPrefix(data, "@"):
		b, err := os.ReadFile(data[1:])
		if err != nil {
			return nil, fmt.Errorf("reading body file: %w", err)
		}
		raw = b
	default:
		raw = []byte(data)
	}
	var parsed json.RawMessage
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("request body is not valid JSON: %w", err)
	}
	return parsed, nil
}

// trimAPIPrefix accepts a path written either way, with or without the /v2/
// the client already supplies.
func trimAPIPrefix(path string) string {
	return strings.TrimPrefix(strings.TrimPrefix(path, "/"), "v2/")
}
