package shape

import (
	"strings"
	"testing"
)

// The whole point of this package is that no value survives. If it ever does,
// pointing it at a real company stops being safe.
func TestOfNeverEmitsAValue(t *testing.T) {
	t.Parallel()
	raw := `{"vat_returns":[{
		"url":"https://api.freeagent.com/v2/vat_returns/2026-06-30",
		"period_ends_on":"2026-06-30",
		"filing_status":"filed",
		"filed_reference":"IRMARK-SECRET-123",
		"payments":[{"label":"VAT due","amount_due":"48211.94","status":"unpaid"}],
		"breakdown":{"rows":[{"title":"VAT due on sales","value":"91002.30","box_number":"1"}]}
	}]}`

	report, err := Of([]byte(raw))
	if err != nil {
		t.Fatalf("Of = %v", err)
	}
	out := report.String()

	// Values that cannot also be a field name must not appear at all.
	for _, secret := range []string{
		"IRMARK-SECRET-123", "48211.94", "91002.30", "2026-06-30", "VAT due on sales",
	} {
		if strings.Contains(out, secret) {
			t.Fatalf("the value %q survived into the report:\n%s", secret, out)
		}
	}

	// Short enum values like "filed" are substrings of legitimate field names
	// such as filed_reference, so a plain search would false-positive. The
	// precise guarantee is that every emitted type is a classification rather
	// than anything derived from a value, so check against the closed set.
	classifications := map[string]bool{
		"url": true, "date": true, "timestamp": true, "decimal-string": true,
		"string": true, "string(empty)": true, "number(int)": true,
		"number(decimal)": true, "bool": true, "null": true, "array(empty)": true,
	}
	for _, field := range report.Fields {
		for _, part := range strings.Split(field.Type, "|") {
			if !classifications[part] {
				t.Fatalf("%s reported type %q, which is not a classification and may be a value",
					field.Path, field.Type)
			}
		}
	}
}

func TestOfDescribesStructureAndTypes(t *testing.T) {
	t.Parallel()
	raw := `{"vat_returns":[{
		"url":"https://api.freeagent.com/v2/vat_returns/2026-06-30",
		"period_ends_on":"2026-06-30",
		"filed_at":"2026-07-15T09:00:00.000Z",
		"filing_status":"filed",
		"amount":"1234.56",
		"count":7,
		"rate":0.2,
		"enabled":true,
		"missing":null,
		"payments":[{"amount_due":"10.0"}]
	}]}`

	report, err := Of([]byte(raw))
	if err != nil {
		t.Fatalf("Of = %v", err)
	}
	if report.Records != 1 {
		t.Fatalf("Records = %d, want 1", report.Records)
	}

	want := map[string]string{
		"vat_returns[].url":                   "url",
		"vat_returns[].period_ends_on":        "date",
		"vat_returns[].filed_at":              "timestamp",
		"vat_returns[].filing_status":         "string",
		"vat_returns[].amount":                "decimal-string",
		"vat_returns[].count":                 "number(int)",
		"vat_returns[].rate":                  "number(decimal)",
		"vat_returns[].enabled":               "bool",
		"vat_returns[].missing":               "null",
		"vat_returns[].payments[].amount_due": "decimal-string",
	}
	got := map[string]string{}
	for _, field := range report.Fields {
		got[field.Path] = field.Type
	}
	for path, kind := range want {
		if got[path] != kind {
			t.Errorf("%s = %q, want %q", path, got[path], kind)
		}
	}
}

// A field present on one record and absent from another must still appear,
// and a null occurrence must not erase a type learned elsewhere.
func TestOfMergesAcrossRecords(t *testing.T) {
	t.Parallel()
	raw := `{"items":[
		{"a":"2026-01-01","b":null},
		{"a":"2026-02-01","b":"1.5","c":true}
	]}`
	report, err := Of([]byte(raw))
	if err != nil {
		t.Fatalf("Of = %v", err)
	}
	got := map[string]Field{}
	for _, field := range report.Fields {
		got[field.Path] = field
	}
	if got["items[].a"].Type != "date" || got["items[].a"].Count != 2 {
		t.Fatalf("a = %+v", got["items[].a"])
	}
	if got["items[].b"].Type != "decimal-string" {
		t.Fatalf("b = %+v, want the null occurrence not to erase the type", got["items[].b"])
	}
	if !got["items[].b"].Nullable {
		t.Fatal("b should be marked nullable")
	}
	if got["items[].c"].Type != "bool" || got["items[].c"].Count != 1 {
		t.Fatalf("c = %+v, want a field present on one record only", got["items[].c"])
	}
}

func TestOfHandlesEmptyAndSingleton(t *testing.T) {
	t.Parallel()
	empty, err := Of([]byte(`{"vat_returns":[]}`))
	if err != nil {
		t.Fatalf("Of = %v", err)
	}
	if empty.Records != 0 {
		t.Fatalf("Records = %d, want 0", empty.Records)
	}
	if len(empty.Fields) != 1 || empty.Fields[0].Type != "array(empty)" {
		t.Fatalf("fields = %+v", empty.Fields)
	}

	singleton, err := Of([]byte(`{"company":{"name":"Real Co","currency":"GBP"}}`))
	if err != nil {
		t.Fatalf("Of = %v", err)
	}
	if singleton.Records != 0 {
		t.Fatalf("Records = %d, want 0 for a singleton", singleton.Records)
	}
	out := singleton.String()
	if strings.Contains(out, "Real Co") || strings.Contains(out, "GBP") {
		t.Fatalf("values leaked:\n%s", out)
	}
	if !strings.Contains(out, "company.name") {
		t.Fatalf("path missing:\n%s", out)
	}
}

// A field whose type differs between records is reported as both, rather than
// silently taking whichever was seen last.
func TestOfReportsConflictingTypes(t *testing.T) {
	t.Parallel()
	report, err := Of([]byte(`{"items":[{"id":1},{"id":"2"}]}`))
	if err != nil {
		t.Fatalf("Of = %v", err)
	}
	for _, field := range report.Fields {
		if field.Path != "items[].id" {
			continue
		}
		if !strings.Contains(field.Type, "|") {
			t.Fatalf("id = %q, want both types reported", field.Type)
		}
		return
	}
	t.Fatal("items[].id was not reported")
}

func TestOfRejectsMalformedInput(t *testing.T) {
	t.Parallel()
	if _, err := Of([]byte(`{"broken":`)); err == nil {
		t.Fatal("malformed JSON was accepted")
	}
}
