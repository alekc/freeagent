package freeagent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// memberPath builds the path of one record in the collection.
func (c *ReadCollection[T]) memberPath(id int64, suffix string) string {
	path := c.meta.Path + "/" + strconv.FormatInt(id, 10)
	if suffix != "" {
		path += "/" + suffix
	}
	return path
}

// action performs a non-CRUD verb on one record and decodes the singular
// envelope from the reply. Transitions, duplicate and direct debit all answer
// with the updated record, so they share this.
func (c *ReadCollection[T]) action(ctx context.Context, method string, id int64, suffix string, body any) (*T, *Response, error) {
	req, err := c.client.newRequest(ctx, method, c.memberPath(id, suffix), nil, body)
	if err != nil {
		return nil, nil, err
	}
	var envelope map[string]json.RawMessage
	resp, err := c.client.do(req, &envelope)
	if err != nil {
		return nil, resp, err
	}
	if len(envelope) == 0 {
		return nil, resp, nil
	}
	item, err := unwrapOne[T](envelope, c.meta.Singular, c.meta.Path)
	if err != nil {
		return nil, resp, err
	}
	return item, resp, nil
}

// getSub fetches a sub-resource of one record into out, without assuming the
// collection's own envelope key.
func (c *ReadCollection[T]) getSub(ctx context.Context, id int64, suffix string, params url.Values, out any) (*Response, error) {
	req, err := c.client.newRequest(ctx, http.MethodGet, c.memberPath(id, suffix), params, nil)
	if err != nil {
		return nil, err
	}
	return c.client.do(req, out)
}

// PDF is a rendered document. FreeAgent returns the file base64 encoded in a
// JSON envelope rather than as an octet stream.
type PDF struct {
	Content string `json:"content"`
}

// Bytes decodes the document. The encoded form is kept on the struct so a
// caller can hand it straight back to something that wants base64.
func (p *PDF) Bytes() ([]byte, error) {
	if p == nil || p.Content == "" {
		return nil, fmt.Errorf("freeagent: PDF has no content")
	}
	decoded, err := base64.StdEncoding.DecodeString(p.Content)
	if err != nil {
		return nil, fmt.Errorf("freeagent: decoding PDF content: %w", err)
	}
	return decoded, nil
}

// pdf fetches the base64 PDF rendering of one record.
func (c *ReadCollection[T]) pdf(ctx context.Context, id int64) (*PDF, *Response, error) {
	var envelope struct {
		PDF PDF `json:"pdf"`
	}
	resp, err := c.getSub(ctx, id, "pdf", nil, &envelope)
	if err != nil {
		return nil, resp, err
	}
	return &envelope.PDF, resp, nil
}

// EmailOptions overrides the message used when sending a document. A zero
// value tells FreeAgent to use the account's configured template.
type EmailOptions struct {
	To          string `json:"to,omitempty"`
	From        string `json:"from,omitempty"`
	Subject     string `json:"subject,omitempty"`
	Body        string `json:"body,omitempty"`
	EmailToSelf bool   `json:"email_to_self,omitempty"`
}

// sendEmail posts the send_email action for one record.
func (c *ReadCollection[T]) sendEmail(ctx context.Context, id int64, opts *EmailOptions) (*Response, error) {
	body := map[string]any{"email": opts}
	if opts == nil {
		body = map[string]any{"email": map[string]any{}}
	}
	req, err := c.client.newRequest(ctx, http.MethodPost, c.memberPath(id, "send_email"), nil, body)
	if err != nil {
		return nil, err
	}
	return c.client.do(req, nil)
}
