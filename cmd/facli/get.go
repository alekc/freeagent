package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/alekc/freeagent"
)

func runGet(ctx context.Context, args []string) error {
	var (
		c        common
		params   paramFlag
		opts     freeagent.ListOptions
		since    string
		from     string
		to       string
		fetchAll bool
	)
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	c.register(fs)
	fs.IntVar(&opts.Page, "page", 0, "page number, 1-based")
	fs.IntVar(&opts.PerPage, "per-page", 0, "records per page, up to 100")
	fs.StringVar(&opts.Sort, "sort", "", "sort field, prefix with - for descending")
	fs.StringVar(&opts.View, "view", "", "server-side named filter")
	fs.StringVar(&since, "updated-since", "", "only records updated at or after this timestamp")
	fs.StringVar(&from, "from-date", "", "range start, YYYY-MM-DD")
	fs.StringVar(&to, "to-date", "", "range end, YYYY-MM-DD")
	fs.BoolVar(&fetchAll, "all", false, "follow pagination and print every record")
	fs.Var(&params, "param", "extra query parameter as key=value, repeatable")
	fs.Usage = func() {
		_, _ = fmt.Fprint(fs.Output(), "Usage: facli get <resource> [flags]\n\nRun \"facli resources\" for the registered names.\n\nFlags:\n")
		fs.PrintDefaults()
	}
	positional, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		fs.Usage()
		return errors.New("get needs exactly one resource name")
	}

	name := positional[0]
	meta, ok := freeagent.LookupResource(name)
	if !ok {
		return fmt.Errorf("unknown resource %q; run \"facli resources\" for the list, or use \"facli raw GET %s\"", name, name)
	}
	if err := checkListable(meta, params.values, fetchAll); err != nil {
		return err
	}
	if err := parseDateRange(&opts, since, from, to); err != nil {
		return err
	}
	opts.Extra = params.values

	query, err := opts.Values()
	if err != nil {
		return err
	}

	client, _, err := c.client(ctx)
	if err != nil {
		return err
	}
	if meta.Singleton || !fetchAll {
		reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
		defer cancel()
		body, resp, err := client.Raw(reqCtx, http.MethodGet, meta.Path, query, nil)
		if err != nil {
			return err
		}
		reportPaging(resp)
		return printJSON(body)
	}
	return getAllPages(ctx, client, &c, meta, opts)
}

// getAllPages walks the collection and prints one merged envelope, so the
// output stays valid JSON for downstream tools.
func getAllPages(ctx context.Context, client *freeagent.Client, c *common, meta freeagent.ResourceMeta, opts freeagent.ListOptions) error {
	if opts.PerPage == 0 {
		opts.PerPage = freeagent.MaxPerPage
	}
	merged := []json.RawMessage{}
	for {
		query, err := opts.Values()
		if err != nil {
			return err
		}
		reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
		body, resp, err := client.Raw(reqCtx, http.MethodGet, meta.Path, query, nil)
		cancel()
		if err != nil {
			return err
		}
		var envelope map[string][]json.RawMessage
		if err := json.Unmarshal(body, &envelope); err != nil {
			return fmt.Errorf("decoding %s page: %w", meta.Name, err)
		}
		page, ok := envelope[meta.Plural]
		if !ok {
			return fmt.Errorf("%s response has no %q key", meta.Name, meta.Plural)
		}
		merged = append(merged, page...)
		fmt.Fprintf(os.Stderr, "page %d: %d records (%d total so far)\n", max(opts.Page, 1), len(page), len(merged))
		if resp.NextPage == 0 || len(page) == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	out, err := json.Marshal(map[string][]json.RawMessage{meta.Plural: merged})
	if err != nil {
		return fmt.Errorf("encoding merged output: %w", err)
	}
	return printJSON(out)
}

func runShow(ctx context.Context, args []string) error {
	var c common
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	c.register(fs)
	fs.Usage = func() {
		_, _ = fmt.Fprint(fs.Output(), "Usage: facli show <resource-url|resource/id>\n\nFlags:\n")
		fs.PrintDefaults()
	}
	positional, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		fs.Usage()
		return errors.New("show needs exactly one URL or resource/id")
	}

	client, _, err := c.client(ctx)
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	target := positional[0]
	var body []byte
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		body, _, err = client.RawURL(reqCtx, http.MethodGet, freeagent.ResourceURL(target), nil, nil)
	} else {
		body, _, err = client.Raw(reqCtx, http.MethodGet, target, nil, nil)
	}
	if err != nil {
		return err
	}
	return printJSON(body)
}

// checkListable rejects, locally, the combinations the API would refuse. The
// three irregular families are worth naming explicitly rather than letting
// the user decode a remote 404 or 422.
func checkListable(meta freeagent.ResourceMeta, params url.Values, fetchAll bool) error {
	if meta.NoList {
		return fmt.Errorf("%s has no collection endpoint; use \"facli show %s/<id>\"", meta.Name, meta.Name)
	}
	if meta.RequiresBankAccount && params.Get("bank_account") == "" {
		return fmt.Errorf("%s needs a bank account; add -param bank_account=<url>", meta.Name)
	}
	if meta.Grouped && fetchAll {
		return fmt.Errorf("%s returns grouped results with no single list to page through; drop -all", meta.Name)
	}
	return nil
}

func parseDateRange(opts *freeagent.ListOptions, since, from, to string) error {
	if since != "" {
		parsed, err := freeagent.ParseTime(since)
		if err != nil {
			return err
		}
		opts.UpdatedSince = parsed
	}
	if from != "" {
		parsed, err := freeagent.ParseDate(from)
		if err != nil {
			return err
		}
		opts.FromDate = parsed
	}
	if to != "" {
		parsed, err := freeagent.ParseDate(to)
		if err != nil {
			return err
		}
		opts.ToDate = parsed
	}
	return nil
}

// reportPaging writes pagination hints to stderr so stdout stays pipeable.
func reportPaging(resp *freeagent.Response) {
	if resp == nil {
		return
	}
	var notes []string
	if resp.TotalCount >= 0 {
		notes = append(notes, fmt.Sprintf("%d total", resp.TotalCount))
	}
	if resp.NextPage > 0 {
		notes = append(notes, fmt.Sprintf("next page %d", resp.NextPage))
	}
	if resp.LastPage > 0 {
		notes = append(notes, fmt.Sprintf("last page %d", resp.LastPage))
	}
	if len(notes) > 0 {
		fmt.Fprintln(os.Stderr, strings.Join(notes, ", ")+" (use -all to fetch everything)")
	}
}

func printJSON(body []byte) error {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err != nil {
		// Not JSON: print it verbatim rather than hiding what came back.
		_, err := os.Stdout.Write(body)
		return err
	}
	pretty.WriteByte('\n')
	_, err := pretty.WriteTo(os.Stdout)
	return err
}
