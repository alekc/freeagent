// Package anonymise scrubs identifying values out of captured FreeAgent
// payloads so they can be committed as test fixtures.
//
// Fixtures are most useful when they are real: only a real response carries
// the fields the documentation forgot. But a real response also carries the
// company's name, registration number, trading address and the signed URLs
// that grant access to its attachments, and this repository is public.
//
// The scrubber replaces those values while preserving the field set, the JSON
// types and the value formats, which is what the fixtures actually test. It
// then re-checks the output against a caller-supplied list of values that must
// not survive, and refuses to return anything that still contains one. That
// second pass is the point: the key list will always be incomplete, so the
// safety net has to be independent of it.
package anonymise

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// placeholders map a JSON field name to a stand-in that keeps the shape of a
// plausible value. Format matters where the SDK or a reader parses it, so a
// postcode still looks like a postcode.
var placeholders = map[string]string{
	// Identity.
	"name":                "Example Name",
	"organisation_name":   "Example Organisation Ltd",
	"first_name":          "Example",
	"last_name":           "Person",
	"contact_name":        "Example Organisation Ltd",
	"client_contact_name": "Example Person",
	"subdomain":           "example",

	// Registration and tax identifiers.
	"company_registration_number":       "00000000",
	"sales_tax_registration_number":     "GB000000000",
	"unique_tax_reference":              "0000000000",
	"ni_number":                         "AA000000A",
	"subcontractor_verification_number": "V0000000000",

	// Postal address.
	"address1": "1 Example Street",
	"address2": "Example District",
	"address3": "",
	"town":     "Exampleton",
	"region":   "Exampleshire",
	"postcode": "EX1 1EX",

	// Contact details.
	"email":         "user@example.com",
	"billing_email": "billing@example.com",
	"contact_email": "user@example.com",
	"phone_number":  "+44 20 7000 0000",
	"mobile":        "+44 7700 900000",
	"contact_phone": "+44 20 7000 0000",
	"website":       "https://example.com",

	// Banking.
	"bank_name":           "Example Bank",
	"account_number":      "00000000",
	"sort_code":           "00-00-00",
	"secondary_sort_code": "00-00-00",
	"iban":                "GB00EXMP00000000000000",
	"bic":                 "EXMPGB00",

	// References embed timestamps, so pinning them keeps fixtures stable.
	"reference": "001",

	// Presigned URLs. These are credentials: they carry an access key and a
	// signature that grant anyone who has them read access to the file.
	"content_src":        "https://example.invalid/attachment",
	"content_src_medium": "https://example.invalid/attachment-medium",
	"content_src_small":  "https://example.invalid/attachment-small",
	"payment_url":        "https://example.invalid/pay",
}

// sandboxHost is rewritten so fixtures read as canonical API responses and
// carry no hint of which environment produced them.
const (
	sandboxHost    = "api.sandbox.freeagent.com"
	productionHost = "api.freeagent.com"
)

// Options configures a scrub.
type Options struct {
	// Literals are replaced wherever they appear inside any string value.
	// Used for things no key list can catch, such as a per-run test tag
	// embedded in free-text descriptions.
	Literals map[string]string
	// Forbidden values must not appear anywhere in the output. JSON returns
	// an error rather than a scrubbed document if one survives.
	Forbidden []string
}

// JSON scrubs a captured payload and re-encodes it indented.
func JSON(raw []byte, opts Options) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	// Keep numbers as written: decoding into float64 would turn a large id
	// into exponent notation and a money value into a rounded float.
	decoder.UseNumber()

	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("anonymise: parsing captured payload: %w", err)
	}

	scrubbed := scrub(document, "", opts)

	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetIndent("", "  ")
	// Struct-free documents contain no HTML, and escaping would corrupt the
	// URLs the fixtures carry.
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(scrubbed); err != nil {
		return nil, fmt.Errorf("anonymise: encoding scrubbed payload: %w", err)
	}

	if leaked := survivors(out.Bytes(), opts.Forbidden); len(leaked) > 0 {
		return nil, fmt.Errorf("anonymise: refusing to emit, these values survived scrubbing: %s",
			strings.Join(leaked, ", "))
	}
	return out.Bytes(), nil
}

func scrub(node any, key string, opts Options) any {
	switch typed := node.(type) {
	case map[string]any:
		for field, value := range typed {
			typed[field] = scrub(value, field, opts)
		}
		return typed
	case []any:
		for i, value := range typed {
			// Array elements inherit nothing: their own keys decide.
			typed[i] = scrub(value, "", opts)
		}
		return typed
	case string:
		return scrubString(typed, key, opts)
	default:
		return node
	}
}

func scrubString(value, key string, opts Options) string {
	if replacement, ok := placeholders[key]; ok {
		// An absent value stays absent: replacing "" with a placeholder would
		// invent a field the API did not send.
		if value == "" {
			return value
		}
		return replacement
	}
	for from, to := range opts.Literals {
		if from != "" {
			value = strings.ReplaceAll(value, from, to)
		}
	}
	return strings.ReplaceAll(value, sandboxHost, productionHost)
}

// survivors reports which forbidden values are still present, case
// insensitively, so a differently-cased leak is still caught.
func survivors(out []byte, forbidden []string) []string {
	haystack := strings.ToLower(string(out))
	var found []string
	seen := map[string]bool{}
	for _, value := range forbidden {
		trimmed := strings.TrimSpace(value)
		// Very short values produce false positives against ordinary JSON.
		if len(trimmed) < 4 || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		if strings.Contains(haystack, strings.ToLower(trimmed)) {
			found = append(found, trimmed)
		}
	}
	sort.Strings(found)
	return found
}

// Keys lists the field names the scrubber replaces. Exposed so a test can
// assert the list has not silently shrunk.
func Keys() []string {
	keys := make([]string, 0, len(placeholders))
	for key := range placeholders {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
