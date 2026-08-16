package anonymise

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONReplacesIdentifyingFields(t *testing.T) {
	t.Parallel()
	raw := `{"company":{
		"url":"https://api.sandbox.freeagent.com/v2/company",
		"id":12345,
		"name":"Wilberforce Instruments Ltd",
		"subdomain":"wilberforce",
		"company_registration_number":"09876543",
		"address1":"Unit 36",
		"town":"Nether Wallop",
		"postcode":"ZZ99 9ZZ",
		"contact_email":"someone@example.org",
		"currency":"GBP",
		"type":"UkLimitedCompany",
		"company_start_date":"2024-04-12",
		"sales_tax_rates":["20.0","5.0","0.0"]
	}}`

	out, err := JSON([]byte(raw), Options{
		Forbidden: []string{"Wilberforce Instruments Ltd", "09876543", "Nether Wallop", "ZZ99 9ZZ", "wilberforce"},
	})
	if err != nil {
		t.Fatalf("JSON = %v", err)
	}

	var got struct {
		Company map[string]any `json:"company"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("scrubbed output is not valid JSON: %v", err)
	}

	// Identity gone.
	for field, unwanted := range map[string]string{
		"name":                        "Wilberforce Instruments Ltd",
		"subdomain":                   "wilberforce",
		"company_registration_number": "09876543",
		"town":                        "Nether Wallop",
		"postcode":                    "ZZ99 9ZZ",
	} {
		if got.Company[field] == unwanted {
			t.Fatalf("%s survived as %q", field, unwanted)
		}
		if got.Company[field] == "" || got.Company[field] == nil {
			t.Fatalf("%s was dropped rather than replaced", field)
		}
	}

	// Structure and parseable values preserved.
	if got.Company["currency"] != "GBP" || got.Company["type"] != "UkLimitedCompany" {
		t.Fatalf("non-identifying fields were altered: %v", got.Company)
	}
	if got.Company["company_start_date"] != "2024-04-12" {
		t.Fatalf("a date was altered: %v", got.Company["company_start_date"])
	}
	rates, ok := got.Company["sales_tax_rates"].([]any)
	if !ok || len(rates) != 3 || rates[0] != "20.0" {
		t.Fatalf("sales_tax_rates was altered: %v", got.Company["sales_tax_rates"])
	}
	// The host is normalised so a fixture does not advertise the environment.
	if url, _ := got.Company["url"].(string); !strings.Contains(url, productionHost) ||
		strings.Contains(url, sandboxHost) {
		t.Fatalf("url = %q, want the sandbox host rewritten", url)
	}
}

// Decoding through float64 would render a large id in exponent notation and
// round a money value, silently corrupting every fixture.
func TestJSONPreservesNumberLiterals(t *testing.T) {
	t.Parallel()
	raw := `{"invoice":{"id":756423123456,"total_value":"251.00","quantity":2,"rate":0.17500}}`
	out, err := JSON([]byte(raw), Options{})
	if err != nil {
		t.Fatalf("JSON = %v", err)
	}
	for _, want := range []string{"756423123456", `"251.00"`, "0.17500"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("output lost the literal %s:\n%s", want, out)
		}
	}
	if strings.Contains(string(out), "e+") {
		t.Fatalf("a number was rendered in exponent notation:\n%s", out)
	}
}

// A presigned attachment URL carries an access key and signature. Anyone
// holding it can read the file, so it is a credential, not a link.
func TestJSONScrubsPresignedURLs(t *testing.T) {
	t.Parallel()
	raw := `{"attachment":{
		"content_src":"https://s3.amazonaws.com/freeagent/1/original.pdf?AWSAccessKeyId=1K3MW21E6T8KWBY84B02&Expires=1316186571&Signature=tA4V5%2BJEE%2Fc3JTg5AiIO494m0cA%3D",
		"content_type":"application/pdf",
		"file_size":466028
	}}`
	out, err := JSON([]byte(raw), Options{Forbidden: []string{"1K3MW21E6T8KWBY84B02"}})
	if err != nil {
		t.Fatalf("JSON = %v", err)
	}
	if strings.Contains(string(out), "AWSAccessKeyId") || strings.Contains(string(out), "Signature") {
		t.Fatalf("a presigned URL survived:\n%s", out)
	}
	// The surrounding metadata is what the fixture is for, so it stays.
	if !strings.Contains(string(out), "application/pdf") || !strings.Contains(string(out), "466028") {
		t.Fatalf("attachment metadata was lost:\n%s", out)
	}
}

// The key list will always be incomplete, so the independent second pass is
// what actually makes this safe.
func TestJSONRefusesWhenAForbiddenValueSurvives(t *testing.T) {
	t.Parallel()
	// business_category is not in the placeholder list on purpose.
	raw := `{"company":{"business_category":"Wilberforce Bespoke Widgets"}}`
	_, err := JSON([]byte(raw), Options{Forbidden: []string{"Wilberforce Bespoke Widgets"}})
	if err == nil {
		t.Fatal("JSON returned a document containing a forbidden value, want an error")
	}
	if !strings.Contains(err.Error(), "survived scrubbing") {
		t.Fatalf("err = %v, want it to name the failure", err)
	}
}

func TestJSONForbiddenIsCaseInsensitive(t *testing.T) {
	t.Parallel()
	raw := `{"company":{"business_type":"WILBERFORCE trading"}}`
	if _, err := JSON([]byte(raw), Options{Forbidden: []string{"wilberforce trading"}}); err == nil {
		t.Fatal("a differently-cased leak was not caught")
	}
}

// Short forbidden values would match ordinary JSON punctuation and field
// names, making every capture fail.
func TestJSONIgnoresVeryShortForbiddenValues(t *testing.T) {
	t.Parallel()
	raw := `{"company":{"currency":"GBP"}}`
	if _, err := JSON([]byte(raw), Options{Forbidden: []string{"GBP", "", "  "}}); err != nil {
		t.Fatalf("JSON = %v, want short values ignored", err)
	}
}

func TestJSONAppliesLiteralReplacements(t *testing.T) {
	t.Parallel()
	raw := `{"invoice":{"comments":"raised by SDKTEST-20260816-114700 run","description":"SDKTEST-20260816-114700 line"}}`
	out, err := JSON([]byte(raw), Options{
		Literals:  map[string]string{"SDKTEST-20260816-114700": "SDKTEST"},
		Forbidden: []string{"20260816-114700"},
	})
	if err != nil {
		t.Fatalf("JSON = %v", err)
	}
	if strings.Contains(string(out), "20260816") {
		t.Fatalf("the run tag survived:\n%s", out)
	}
	if !strings.Contains(string(out), "SDKTEST line") {
		t.Fatalf("the replacement did not apply:\n%s", out)
	}
}

// An absent field must stay absent: inventing a value would make the fixture
// claim the API sends something it does not.
func TestJSONLeavesEmptyStringsEmpty(t *testing.T) {
	t.Parallel()
	raw := `{"contact":{"first_name":"","organisation_name":"Real Co Ltd"}}`
	out, err := JSON([]byte(raw), Options{})
	if err != nil {
		t.Fatalf("JSON = %v", err)
	}
	var got struct {
		Contact map[string]any `json:"contact"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Contact["first_name"] != "" {
		t.Fatalf("first_name = %v, want it left empty", got.Contact["first_name"])
	}
	if got.Contact["organisation_name"] == "Real Co Ltd" {
		t.Fatal("organisation_name was not scrubbed")
	}
}

func TestJSONHandlesNestedArrays(t *testing.T) {
	t.Parallel()
	raw := `{"invoices":[{"contact_name":"Real Co","invoice_items":[{"description":"work","price":"10.0"}]}]}`
	out, err := JSON([]byte(raw), Options{Forbidden: []string{"Real Co"}})
	if err != nil {
		t.Fatalf("JSON = %v", err)
	}
	if !strings.Contains(string(out), `"price": "10.0"`) {
		t.Fatalf("nested item values were lost:\n%s", out)
	}
}

func TestJSONRejectsMalformedInput(t *testing.T) {
	t.Parallel()
	if _, err := JSON([]byte(`{"not":`), Options{}); err == nil {
		t.Fatal("malformed JSON was accepted")
	}
}

// A shrinking key list is a silent regression, so pin the categories that
// must always be covered.
func TestKeysCoverTheSensitiveCategories(t *testing.T) {
	t.Parallel()
	have := map[string]bool{}
	for _, key := range Keys() {
		have[key] = true
	}
	for _, required := range []string{
		"name", "organisation_name", "subdomain",
		"company_registration_number", "sales_tax_registration_number",
		"unique_tax_reference", "ni_number",
		"address1", "town", "postcode",
		"email", "phone_number",
		"account_number", "sort_code", "iban", "bic",
		"content_src", "payment_url",
	} {
		if !have[required] {
			t.Errorf("the scrubber no longer covers %q", required)
		}
	}
}
