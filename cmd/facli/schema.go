package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"

	"github.com/alekc/freeagent"
	"github.com/alekc/freeagent/internal/shape"
)

// runSchema prints the field paths and types of an endpoint's response and
// nothing else.
//
// This is the safe way to model a resource against a real company: it answers
// "what fields exist and what shape are they" without reproducing a single
// value, so nothing about the business reaches the terminal, a commit or a
// support conversation.
func runSchema(ctx context.Context, args []string) error {
	var (
		c      common
		params paramFlag
		follow bool
	)
	fs := flag.NewFlagSet("schema", flag.ContinueOnError)
	c.register(fs)
	fs.Var(&params, "param", "query parameter as key=value, repeatable")
	fs.BoolVar(&follow, "follow", false,
		"follow the first record's url and report that instead, without printing it")
	fs.Usage = func() {
		_, _ = fmt.Fprint(fs.Output(),
			"Usage: facli schema <path> [flags]\n\n"+
				"Prints field paths and types, never values. Safe against a real company.\n\n"+
				"Example: facli schema vat_returns -env production\n\nFlags:\n")
		fs.PrintDefaults()
	}
	positional, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		fs.Usage()
		return errors.New("schema needs exactly one path")
	}
	path := trimAPIPrefix(positional[0])

	// Schema inspection never writes, so the client is built read-only. That
	// is belt and braces on top of only issuing a GET.
	client, env, err := c.clientReadOnly(ctx)
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	body, resp, err := client.Raw(reqCtx, http.MethodGet, path, params.values, nil)
	if err != nil {
		return err
	}
	label := path

	// Drilling into one member needs its identifier, which is a value and so
	// is never printed. Following the link keeps that promise: the URL is
	// read from the payload and used, not shown.
	if follow {
		target, ok := firstMemberURL(body)
		if !ok {
			return fmt.Errorf("no record with a url to follow in the %s response", path)
		}
		body, resp, err = client.RawURL(reqCtx, http.MethodGet, target, nil, nil)
		if err != nil {
			return err
		}
		label = path + " -> first record"
	}

	report, err := shape.Of(body)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "%s %s -> %s (%d bytes, %d record(s))\n",
		env.Name, label, resp.Status, len(body), report.Records)
	fmt.Print(report.String())
	return nil
}

// firstMemberURL finds the url of the first record in a collection envelope,
// so a caller can drill in without knowing, or revealing, the identifier.
//
// Envelope keys are tried in sorted order for determinism, and the first
// array whose leading element carries a url string wins.
func firstMemberURL(body []byte) (freeagent.ResourceURL, bool) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", false
	}
	keys := make([]string, 0, len(envelope))
	for key := range envelope {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		var records []struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(envelope[key], &records); err != nil {
			continue
		}
		if len(records) > 0 && records[0].URL != "" {
			return freeagent.ResourceURL(records[0].URL), true
		}
	}
	return "", false
}
