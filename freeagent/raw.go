package freeagent

import (
	"context"
	"net/url"
)

// Raw issues an arbitrary request against the API root and returns the
// undecoded response body. It exists so tooling can reach endpoints that have
// no typed model yet, and so a caller can inspect fields this library does
// not model. path is relative to the API root, for example "invoices/123".
func (c *Client) Raw(ctx context.Context, method, path string, query url.Values, body any) ([]byte, *Response, error) {
	req, err := c.newRequest(ctx, method, path, query, body)
	if err != nil {
		return nil, nil, err
	}
	var out []byte
	resp, err := c.do(req, &out)
	if err != nil {
		return nil, resp, err
	}
	return out, resp, nil
}

// RawURL is Raw for a payload reference. The URL must be on the client's own
// host; see pathForURL for why that is enforced.
func (c *Client) RawURL(ctx context.Context, method string, ref ResourceURL, query url.Values, body any) ([]byte, *Response, error) {
	path, err := c.pathForURL(ref)
	if err != nil {
		return nil, nil, err
	}
	return c.Raw(ctx, method, path, query, body)
}
