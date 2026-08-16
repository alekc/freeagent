package freeagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// widget stands in for a real resource so the generic layer can be tested
// without waiting on the typed models.
type widget struct {
	URL  ResourceURL `json:"url,omitempty"`
	Name string      `json:"name"`
}

var widgetMeta = ResourceMeta{
	Name: "widgets", Path: "widgets", Singular: "widget", Plural: "widgets",
}

func newWidgets(t *testing.T, handler http.HandlerFunc, extra ...Option) Collection[widget] {
	t.Helper()
	return newCollection[widget](newTestClient(t, handler, extra...), widgetMeta)
}

func TestCollectionList(t *testing.T) {
	t.Parallel()
	widgets := newWidgets(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/widgets" {
			t.Errorf("path = %q, want /v2/widgets", r.URL.Path)
		}
		w.Header().Set("X-Total-Count", "2")
		fmt.Fprint(w, `{"widgets":[{"name":"a"},{"name":"b"}]}`)
	})

	items, resp, err := widgets.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List = %v", err)
	}
	if len(items) != 2 || items[0].Name != "a" || items[1].Name != "b" {
		t.Fatalf("items = %+v", items)
	}
	if resp.TotalCount != 2 {
		t.Fatalf("TotalCount = %d, want 2", resp.TotalCount)
	}
}

// A missing envelope key is a different failure from an empty collection and
// must not be silently reported as zero records.
func TestCollectionListMissingEnvelope(t *testing.T) {
	t.Parallel()
	widgets := newWidgets(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"gadgets":[{"name":"a"}]}`)
	})
	_, _, err := widgets.List(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), `no "widgets" key`) {
		t.Fatalf("err = %v, want a missing envelope error", err)
	}
}

func TestCollectionAllFollowsPagination(t *testing.T) {
	t.Parallel()
	var pages atomic.Int32
	widgets := newWidgets(t, func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "" {
			page = "1"
		}
		if got := r.URL.Query().Get("per_page"); got != "100" {
			t.Errorf("per_page = %q, want 100: All should request full pages", got)
		}
		pages.Add(1)
		switch page {
		case "1", "2":
			next := "2"
			if page == "2" {
				next = "3"
			}
			w.Header().Set("Link", fmt.Sprintf(`<http://example.invalid/v2/widgets?page=%s>; rel="next"`, next))
			fmt.Fprintf(w, `{"widgets":[{"name":"p%s-a"},{"name":"p%s-b"}]}`, page, page)
		default:
			fmt.Fprint(w, `{"widgets":[{"name":"p3-a"}]}`)
		}
	})

	var names []string
	for item, err := range widgets.All(context.Background(), nil) {
		if err != nil {
			t.Fatalf("All yielded %v", err)
		}
		names = append(names, item.Name)
	}
	want := []string{"p1-a", "p1-b", "p2-a", "p2-b", "p3-a"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("names = %v, want %v", names, want)
	}
	if pages.Load() != 3 {
		t.Fatalf("fetched %d pages, want 3", pages.Load())
	}
}

func TestCollectionAllStopsOnBreak(t *testing.T) {
	t.Parallel()
	var pages atomic.Int32
	widgets := newWidgets(t, func(w http.ResponseWriter, r *http.Request) {
		pages.Add(1)
		w.Header().Set("Link", `<http://example.invalid/v2/widgets?page=99>; rel="next"`)
		fmt.Fprint(w, `{"widgets":[{"name":"a"},{"name":"b"}]}`)
	})

	count := 0
	for range widgets.All(context.Background(), nil) {
		count++
		break
	}
	if count != 1 {
		t.Fatalf("yielded %d items after break, want 1", count)
	}
	if pages.Load() != 1 {
		t.Fatalf("fetched %d pages after break, want 1", pages.Load())
	}
}

func TestCollectionAllSurfacesErrors(t *testing.T) {
	t.Parallel()
	widgets := newWidgets(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"errors":{"error":{"message":"Nothing here"}}}`)
	})

	var got error
	for _, err := range widgets.All(context.Background(), nil) {
		got = err
	}
	if !errors.Is(got, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", got)
	}
}

func TestCollectionGet(t *testing.T) {
	t.Parallel()
	widgets := newWidgets(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/widgets/42" {
			t.Errorf("path = %q, want /v2/widgets/42", r.URL.Path)
		}
		fmt.Fprint(w, `{"widget":{"name":"answer"}}`)
	})
	got, _, err := widgets.Get(context.Background(), 42)
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	if got.Name != "answer" {
		t.Fatalf("Name = %q, want answer", got.Name)
	}
}

// Writes must be wrapped in the singular envelope the API expects.
func TestCollectionCreateWrapsEnvelope(t *testing.T) {
	t.Parallel()
	var sent map[string]json.RawMessage
	widgets := newWidgets(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &sent); err != nil {
			t.Errorf("request body is not JSON: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"widget":{"url":"https://example.invalid/v2/widgets/7","name":"new"}}`)
	})

	created, _, err := widgets.Create(context.Background(), &widget{Name: "new"})
	if err != nil {
		t.Fatalf("Create = %v", err)
	}
	if _, ok := sent["widget"]; !ok {
		t.Fatalf("request body keys = %v, want a widget envelope", sent)
	}
	if created.Name != "new" {
		t.Fatalf("Name = %q, want new", created.Name)
	}
	if _, err := created.URL.ID(); err != nil {
		t.Fatalf("created URL is not a member URL: %v", err)
	}
}

func TestCollectionUpdateAndDelete(t *testing.T) {
	t.Parallel()
	var seen []string
	widgets := newWidgets(t, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		fmt.Fprint(w, `{"widget":{"name":"updated"}}`)
	})

	updated, _, err := widgets.Update(context.Background(), 7, &widget{Name: "updated"})
	if err != nil {
		t.Fatalf("Update = %v", err)
	}
	if updated.Name != "updated" {
		t.Fatalf("Name = %q", updated.Name)
	}
	if _, err := widgets.Delete(context.Background(), 7); err != nil {
		t.Fatalf("Delete = %v", err)
	}
	want := "PUT /v2/widgets/7,DELETE /v2/widgets/7"
	if strings.Join(seen, ",") != want {
		t.Fatalf("requests = %v, want %s", seen, want)
	}
}

func TestCollectionWriteRejectsNil(t *testing.T) {
	t.Parallel()
	widgets := newWidgets(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be sent for a nil record")
	})
	if _, _, err := widgets.Create(context.Background(), nil); err == nil {
		t.Fatal("Create(nil) succeeded, want an error")
	}
	if _, _, err := widgets.Update(context.Background(), 1, nil); err == nil {
		t.Fatal("Update(nil) succeeded, want an error")
	}
}

// References come out of API responses, so following one to a different host
// would let an upstream payload redirect the client.
func TestGetURLRejectsForeignHost(t *testing.T) {
	t.Parallel()
	widgets := newWidgets(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be sent for a foreign host")
	})
	for _, ref := range []ResourceURL{
		"https://evil.example.com/v2/widgets/1",
		"http://127.0.0.1:1/v2/widgets/1",
		"",
		"://nonsense",
	} {
		if _, _, err := widgets.GetURL(context.Background(), ref); err == nil {
			t.Fatalf("GetURL(%q) succeeded, want an error", ref)
		}
	}
}

func TestGetURLAcceptsOwnHost(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/widgets/9" {
			t.Errorf("path = %q, want /v2/widgets/9", r.URL.Path)
		}
		fmt.Fprint(w, `{"widget":{"name":"nine"}}`)
	})
	widgets := newCollection[widget](client, widgetMeta)

	ref := ResourceURL(strings.TrimSuffix(client.BaseURL().String(), "/") + "/widgets/9")
	got, _, err := widgets.GetURL(context.Background(), ref)
	if err != nil {
		t.Fatalf("GetURL = %v", err)
	}
	if got.Name != "nine" {
		t.Fatalf("Name = %q, want nine", got.Name)
	}
}

// Reports are not enveloped, so a Reader with no singular key decodes the
// whole body into the target.
func TestReaderWithoutEnvelope(t *testing.T) {
	t.Parallel()
	type report struct {
		Rows []string `json:"rows"`
	}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/accounting/trial_balance/summary" {
			t.Errorf("path = %q", r.URL.Path)
		}
		fmt.Fprint(w, `{"rows":["a","b"]}`)
	})
	reader := newReader[report](client, Resources["trial_balance"])

	got, _, err := reader.Get(context.Background(), nil)
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	if len(got.Rows) != 2 {
		t.Fatalf("rows = %v, want 2", got.Rows)
	}
}

// Envelope-key expectations live in TestRegistryCoversWaveA, which knows
// about the irregular families.
func TestRegistryPathsAreRelative(t *testing.T) {
	t.Parallel()
	for _, name := range ResourceNames() {
		meta, _ := LookupResource(name)
		if meta.Name != name {
			t.Errorf("%s: Name = %q, want the map key", name, meta.Name)
		}
		if strings.HasPrefix(meta.Path, "/") || strings.Contains(meta.Path, "://") {
			t.Errorf("%s: Path = %q, want a path relative to the API root", name, meta.Path)
		}
	}
}
