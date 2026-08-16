package freeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ReadCollection is the read surface of a collection endpoint. Some families
// are genuinely read-only per record (bank transactions arrive by statement
// upload or feed, not by POST), so they embed this rather than Collection and
// the type system rules out the writes the API does not offer.
type ReadCollection[T any] struct {
	client *Client
	meta   ResourceMeta
}

// Collection adds the write verbs to ReadCollection. Resource services embed
// one of the two and add only the endpoints that do not fit the shape, which
// is what keeps 45 resource families from turning into 225 near-identical
// methods.
type Collection[T any] struct {
	ReadCollection[T]
}

func newReadCollection[T any](c *Client, meta ResourceMeta) ReadCollection[T] {
	return ReadCollection[T]{client: c, meta: meta}
}

func newCollection[T any](c *Client, meta ResourceMeta) Collection[T] {
	return Collection[T]{newReadCollection[T](c, meta)}
}

// Meta returns the resource metadata backing this collection.
func (c *ReadCollection[T]) Meta() ResourceMeta { return c.meta }

// List fetches one page. Callers that want the whole collection should use
// All, which walks the Link header for them.
func (c *ReadCollection[T]) List(ctx context.Context, opts *ListOptions) ([]T, *Response, error) {
	query, err := opts.Values()
	if err != nil {
		return nil, nil, err
	}
	req, err := c.client.newRequest(ctx, http.MethodGet, c.meta.Path, query, nil)
	if err != nil {
		return nil, nil, err
	}
	var envelope map[string]json.RawMessage
	resp, err := c.client.do(req, &envelope)
	if err != nil {
		return nil, resp, err
	}
	items, err := unwrapList[T](envelope, c.meta.Plural, c.meta.Path)
	if err != nil {
		return nil, resp, err
	}
	return items, resp, nil
}

// All iterates the entire collection, following pagination. Iteration stops
// on the first error, which is yielded alongside the zero value; breaking out
// of the range loop stops it cleanly without further requests.
func (c *ReadCollection[T]) All(ctx context.Context, opts *ListOptions) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		page := opts.clone()
		if page.PerPage == 0 {
			page.PerPage = MaxPerPage
		}
		for {
			items, resp, err := c.List(ctx, &page)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			for _, item := range items {
				if !yield(item, nil) {
					return
				}
			}
			// A missing next relation, or a short page when the server sent
			// no Link header at all, means the collection is exhausted.
			if resp.NextPage == 0 || len(items) == 0 {
				return
			}
			page.Page = resp.NextPage
		}
	}
}

// Get fetches one record by numeric id.
func (c *ReadCollection[T]) Get(ctx context.Context, id int64) (*T, *Response, error) {
	return c.get(ctx, c.meta.Path+"/"+strconv.FormatInt(id, 10), nil)
}

// GetURL fetches the record a payload reference points at. The URL must be on
// the client's own host: references come from API responses, so following one
// blindly would let an upstream response redirect the client elsewhere.
func (c *ReadCollection[T]) GetURL(ctx context.Context, ref ResourceURL) (*T, *Response, error) {
	path, err := c.client.pathForURL(ref)
	if err != nil {
		return nil, nil, err
	}
	return c.get(ctx, path, nil)
}

// Create posts a new record and returns the server's version of it.
func (c *Collection[T]) Create(ctx context.Context, in *T) (*T, *Response, error) {
	if in == nil {
		return nil, nil, fmt.Errorf("freeagent: Create(%s) requires a non-nil record", c.meta.Name)
	}
	return c.write(ctx, http.MethodPost, c.meta.Path, in)
}

// Update replaces a record by numeric id.
func (c *Collection[T]) Update(ctx context.Context, id int64, in *T) (*T, *Response, error) {
	if in == nil {
		return nil, nil, fmt.Errorf("freeagent: Update(%s) requires a non-nil record", c.meta.Name)
	}
	return c.write(ctx, http.MethodPut, c.meta.Path+"/"+strconv.FormatInt(id, 10), in)
}

// Delete removes a record by numeric id.
func (c *Collection[T]) Delete(ctx context.Context, id int64) (*Response, error) {
	req, err := c.client.newRequest(ctx, http.MethodDelete, c.meta.Path+"/"+strconv.FormatInt(id, 10), nil, nil)
	if err != nil {
		return nil, err
	}
	return c.client.do(req, nil)
}

func (c *ReadCollection[T]) get(ctx context.Context, path string, query url.Values) (*T, *Response, error) {
	req, err := c.client.newRequest(ctx, http.MethodGet, path, query, nil)
	if err != nil {
		return nil, nil, err
	}
	var envelope map[string]json.RawMessage
	resp, err := c.client.do(req, &envelope)
	if err != nil {
		return nil, resp, err
	}
	item, err := unwrapOne[T](envelope, c.meta.Singular, path)
	if err != nil {
		return nil, resp, err
	}
	return item, resp, nil
}

func (c *Collection[T]) write(ctx context.Context, method, path string, in *T) (*T, *Response, error) {
	body := map[string]any{c.meta.Singular: in}
	req, err := c.client.newRequest(ctx, method, path, nil, body)
	if err != nil {
		return nil, nil, err
	}
	var envelope map[string]json.RawMessage
	resp, err := c.client.do(req, &envelope)
	if err != nil {
		return nil, resp, err
	}
	// A 204 or an empty body is a legitimate answer to a write; report
	// success with no record rather than inventing a zero value.
	if len(envelope) == 0 {
		return nil, resp, nil
	}
	item, err := unwrapOne[T](envelope, c.meta.Singular, path)
	if err != nil {
		return nil, resp, err
	}
	return item, resp, nil
}

// Reader is the read-only surface for singletons and reports, which have no
// id segment and no write verbs.
type Reader[T any] struct {
	client *Client
	meta   ResourceMeta
}

func newReader[T any](c *Client, meta ResourceMeta) Reader[T] {
	return Reader[T]{client: c, meta: meta}
}

// Meta returns the resource metadata backing this reader.
func (r *Reader[T]) Meta() ResourceMeta { return r.meta }

// Get fetches the resource. params carries endpoint-specific filters such as
// from_date and to_date on the accounting reports.
func (r *Reader[T]) Get(ctx context.Context, params url.Values) (*T, *Response, error) {
	req, err := r.client.newRequest(ctx, http.MethodGet, r.meta.Path, params, nil)
	if err != nil {
		return nil, nil, err
	}
	// Reports are not always enveloped, so decode into the target directly
	// when the metadata declares no envelope key.
	if r.meta.Singular == "" {
		out := new(T)
		resp, err := r.client.do(req, out)
		if err != nil {
			return nil, resp, err
		}
		return out, resp, nil
	}
	var envelope map[string]json.RawMessage
	resp, err := r.client.do(req, &envelope)
	if err != nil {
		return nil, resp, err
	}
	item, err := unwrapOne[T](envelope, r.meta.Singular, r.meta.Path)
	if err != nil {
		return nil, resp, err
	}
	return item, resp, nil
}

// decodeSingle sends req and unwraps the singular envelope from the reply. An
// empty body is reported as success with no record, which is what a 204
// answer to a write means.
func decodeSingle[T any](c *Client, req *http.Request, meta ResourceMeta) (*T, *Response, error) {
	var envelope map[string]json.RawMessage
	resp, err := c.do(req, &envelope)
	if err != nil {
		return nil, resp, err
	}
	if len(envelope) == 0 {
		return nil, resp, nil
	}
	item, err := unwrapOne[T](envelope, meta.Singular, meta.Path)
	if err != nil {
		return nil, resp, err
	}
	return item, resp, nil
}

// unwrapList pulls the array out of a {"invoices": [...]} envelope. A missing
// key is reported rather than silently returning nothing, because "no such
// key" and "empty collection" are different failures.
func unwrapList[T any](envelope map[string]json.RawMessage, key, path string) ([]T, error) {
	raw, ok := envelope[key]
	if !ok {
		return nil, fmt.Errorf("freeagent: %s response has no %q key, got keys %v", path, key, envelopeKeys(envelope))
	}
	var items []T
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("freeagent: decoding %s from %s response: %w", key, path, err)
	}
	return items, nil
}

// unwrapOne pulls the object out of a {"invoice": {...}} envelope.
func unwrapOne[T any](envelope map[string]json.RawMessage, key, path string) (*T, error) {
	raw, ok := envelope[key]
	if !ok {
		return nil, fmt.Errorf("freeagent: %s response has no %q key, got keys %v", path, key, envelopeKeys(envelope))
	}
	out := new(T)
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, fmt.Errorf("freeagent: decoding %s from %s response: %w", key, path, err)
	}
	return out, nil
}

func envelopeKeys(envelope map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(envelope))
	for k := range envelope {
		keys = append(keys, k)
	}
	return keys
}

// pathForURL converts a payload reference into a path relative to the client
// base, rejecting anything pointing at a different host or API root.
func (c *Client) pathForURL(ref ResourceURL) (string, error) {
	raw := strings.TrimSpace(string(ref))
	if raw == "" {
		return "", fmt.Errorf("freeagent: empty resource URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("freeagent: invalid resource URL %q: %w", truncate(raw, 64), err)
	}
	if u.Scheme != c.baseURL.Scheme || u.Host != c.baseURL.Host {
		return "", fmt.Errorf("freeagent: resource URL %q is not on the configured host %s", truncate(raw, 64), c.baseURL.Host)
	}
	if !strings.HasPrefix(u.Path, c.baseURL.Path) {
		return "", fmt.Errorf("freeagent: resource URL %q is outside the API root %s", truncate(raw, 64), c.baseURL.Path)
	}
	return strings.TrimPrefix(u.Path, c.baseURL.Path), nil
}
