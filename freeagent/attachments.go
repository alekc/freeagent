package freeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// MaxAttachmentBytes is the upload limit FreeAgent documents for the
// attachment field on bills, expenses and bank transaction explanations.
const MaxAttachmentBytes = 5 << 20

// Attachment is a file attached to another record.
//
// The same JSON key carries two shapes: on read the API returns the stored
// file's metadata and time-limited download URLs, and on write it expects the
// file content. Set Data, FileName and ContentType to upload; everything else
// is populated by the server.
//
// See https://dev.freeagent.com/docs/attachments
type Attachment struct {
	URL ResourceURL `json:"url,omitempty"`

	// Data is the file content, base64 encoded on the wire. Write-only.
	Data []byte `json:"data,omitempty"`

	FileName    string `json:"file_name,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Description string `json:"description,omitempty"`

	FileSize         int    `json:"file_size,omitempty"`
	ContentSrc       string `json:"content_src,omitempty"`
	ContentSrcMedium string `json:"content_src_medium,omitempty"`
	ContentSrcSmall  string `json:"content_src_small,omitempty"`
	ExpiresAt        Time   `json:"expires_at,omitzero"`
}

// MarshalJSON enforces the documented size limit at encode time, so an
// oversized upload fails locally on any resource that carries an attachment
// rather than after transferring several megabytes.
func (a Attachment) MarshalJSON() ([]byte, error) {
	if len(a.Data) > MaxAttachmentBytes {
		return nil, fmt.Errorf("freeagent: attachment %q is %d bytes, the limit is %d",
			a.FileName, len(a.Data), MaxAttachmentBytes)
	}
	type alias Attachment
	return json.Marshal(alias(a))
}

// AttachmentService reads and removes attachments. There is no list or create
// endpoint: attachments are created through the parent record's attachment
// field, so this service deliberately offers neither.
//
// See https://dev.freeagent.com/docs/attachments
type AttachmentService struct {
	client *Client
	meta   ResourceMeta
}

// Meta returns the resource metadata.
func (s *AttachmentService) Meta() ResourceMeta { return s.meta }

// Get fetches one attachment by numeric id.
func (s *AttachmentService) Get(ctx context.Context, id int64) (*Attachment, *Response, error) {
	return s.get(ctx, s.meta.Path+"/"+strconv.FormatInt(id, 10))
}

// GetURL fetches the attachment a payload reference points at.
func (s *AttachmentService) GetURL(ctx context.Context, ref ResourceURL) (*Attachment, *Response, error) {
	path, err := s.client.pathForURL(ref)
	if err != nil {
		return nil, nil, err
	}
	return s.get(ctx, path)
}

// Delete removes one attachment.
func (s *AttachmentService) Delete(ctx context.Context, id int64) (*Response, error) {
	req, err := s.client.newRequest(ctx, http.MethodDelete, s.meta.Path+"/"+strconv.FormatInt(id, 10), nil, nil)
	if err != nil {
		return nil, err
	}
	return s.client.do(req, nil)
}

func (s *AttachmentService) get(ctx context.Context, path string) (*Attachment, *Response, error) {
	req, err := s.client.newRequest(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	var envelope map[string]json.RawMessage
	resp, err := s.client.do(req, &envelope)
	if err != nil {
		return nil, resp, err
	}
	item, err := unwrapOne[Attachment](envelope, s.meta.Singular, path)
	if err != nil {
		return nil, resp, err
	}
	return item, resp, nil
}
