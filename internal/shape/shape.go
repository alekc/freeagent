// Package shape summarises a JSON payload as field paths and types, without
// reproducing any value.
//
// It exists so a response from a real company can be used to build a model
// without the company's data leaving the machine it was fetched on. Modelling
// needs to know that bills[].due_on is a date-shaped string and that
// total_value is a quoted decimal. It does not need to know the amount, the
// supplier or the reference, and those are exactly what must not be echoed
// into a terminal, a commit or a conversation.
//
// Nothing here emits a value. Strings are reduced to a classification such as
// "date" or "decimal-string"; numbers to "number" or "number(int)"; and every
// other leaf to its JSON type.
package shape

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Field is one leaf of a payload, described rather than quoted.
type Field struct {
	// Path is dotted, with [] marking an array, for example
	// "bills[].bill_items[].total_value".
	Path string
	// Type is the classification, never the value.
	Type string
	// Nullable records that at least one occurrence was null.
	Nullable bool
	// Count is how many occurrences were seen, which distinguishes a field
	// present on every record from one present on a single outlier.
	Count int
}

// Report is the summarised payload.
type Report struct {
	Fields []Field
	// Records is the length of the top-level array, when the envelope holds
	// one. Zero for a singleton.
	Records int
}

// String renders the report as aligned lines, one per field.
func (r Report) String() string {
	var b strings.Builder
	width := 0
	for _, field := range r.Fields {
		if len(field.Path) > width {
			width = len(field.Path)
		}
	}
	for _, field := range r.Fields {
		suffix := ""
		if field.Nullable {
			suffix = " (nullable)"
		}
		fmt.Fprintf(&b, "%-*s  %s%s\n", width, field.Path, field.Type, suffix)
	}
	return b.String()
}

var (
	dateRE     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	datetimeRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)
	decimalRE  = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
	urlRE      = regexp.MustCompile(`^https?://`)
	intRE      = regexp.MustCompile(`^-?\d+$`)
)

// Of summarises a payload.
func Of(raw []byte) (Report, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return Report{}, fmt.Errorf("shape: parsing payload: %w", err)
	}

	seen := map[string]*Field{}
	records := 0
	// A collection envelope holds one array; note its length so a reader can
	// tell "absent from this account" from "absent from this record".
	if envelope, ok := document.(map[string]any); ok {
		for _, value := range envelope {
			if list, ok := value.([]any); ok && len(list) > records {
				records = len(list)
			}
		}
	}
	walk(document, "", seen)

	report := Report{Records: records}
	for _, field := range seen {
		report.Fields = append(report.Fields, *field)
	}
	sort.Slice(report.Fields, func(i, j int) bool {
		return report.Fields[i].Path < report.Fields[j].Path
	})
	return report, nil
}

func walk(node any, path string, seen map[string]*Field) {
	switch typed := node.(type) {
	case map[string]any:
		for key, value := range typed {
			child := key
			if path != "" {
				child = path + "." + key
			}
			walk(value, child, seen)
		}
	case []any:
		// An array contributes its element shape, merged across elements so a
		// field present on only some records still shows up.
		for _, value := range typed {
			walk(value, path+"[]", seen)
		}
		if len(typed) == 0 {
			record(seen, path, "array(empty)", false)
		}
	default:
		kind, nullable := classify(node)
		record(seen, path, kind, nullable)
	}
}

func record(seen map[string]*Field, path, kind string, nullable bool) {
	if path == "" {
		return
	}
	existing, ok := seen[path]
	if !ok {
		seen[path] = &Field{Path: path, Type: kind, Nullable: nullable, Count: 1}
		return
	}
	existing.Count++
	existing.Nullable = existing.Nullable || nullable
	// A null occurrence must not erase a type learned from a populated one.
	if existing.Type == "null" && kind != "null" {
		existing.Type = kind
	} else if existing.Type != kind && kind != "null" {
		existing.Type = mergeKinds(existing.Type, kind)
	}
}

func mergeKinds(a, b string) string {
	if a == b {
		return a
	}
	parts := []string{a, b}
	sort.Strings(parts)
	return parts[0] + "|" + parts[1]
}

// classify describes a leaf. It returns a category, never the value.
func classify(node any) (string, bool) {
	switch typed := node.(type) {
	case nil:
		return "null", true
	case bool:
		return "bool", false
	case json.Number:
		if intRE.MatchString(typed.String()) {
			return "number(int)", false
		}
		return "number(decimal)", false
	case string:
		return classifyString(typed), false
	default:
		return fmt.Sprintf("%T", node), false
	}
}

func classifyString(value string) string {
	switch {
	case value == "":
		return "string(empty)"
	case urlRE.MatchString(value):
		return "url"
	case datetimeRE.MatchString(value):
		return "timestamp"
	case dateRE.MatchString(value):
		return "date"
	case decimalRE.MatchString(value):
		return "decimal-string"
	default:
		return "string"
	}
}
