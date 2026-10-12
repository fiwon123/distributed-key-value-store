package httpapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// apiDoc is the subset of the OpenAPI document the drift tests inspect.
type apiDoc struct {
	OpenAPI    string                                `json:"openapi"`
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Schemas map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"schemas"`
	} `json:"components"`
}

// httpMethods filters out non-method keys (e.g. a path-level "parameters"
// array) when walking a Path Item Object.
var httpMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodPut:     true,
	http.MethodPost:    true,
	http.MethodDelete:  true,
	http.MethodPatch:   true,
	http.MethodHead:    true,
	http.MethodOptions: true,
	http.MethodTrace:   true,
}

func parseSpec(t *testing.T, data []byte) apiDoc {
	t.Helper()
	var doc apiDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("spec is not valid JSON: %v", err)
	}
	return doc
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestOpenAPISpecServed checks the embedded document is served as JSON and
// declares an OpenAPI 3 version.
func TestOpenAPISpecServed(t *testing.T) {
	ts := newTestServer(t, newFakeBackend())
	resp := doRequest(t, http.MethodGet, ts.URL+"/v1/openapi.json", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var body json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if doc := parseSpec(t, body); !strings.HasPrefix(doc.OpenAPI, "3.") {
		t.Fatalf("openapi = %q, want a 3.x version", doc.OpenAPI)
	}
}

// TestOpenAPISpecMatchesRoutes fails if the documented paths+methods and the
// registered routes diverge in either direction — the guard against the spec
// silently rotting as handlers change.
func TestOpenAPISpecMatchesRoutes(t *testing.T) {
	doc := parseSpec(t, openAPISpec)

	want := map[string]bool{}
	for _, r := range New(newFakeBackend(), fakeStatus{}).routes() {
		want[r.Method+" "+r.Pattern] = true
	}

	got := map[string]bool{}
	for path, item := range doc.Paths {
		for method := range item {
			if !httpMethods[strings.ToUpper(method)] {
				continue
			}
			got[strings.ToUpper(method)+" "+path] = true
		}
	}

	if !reflect.DeepEqual(want, got) {
		t.Fatalf("spec routes != registered routes\n spec: %v\n code: %v",
			sortedKeys(got), sortedKeys(want))
	}
}

// TestOpenAPIStatusSchemaMatchesStruct keeps the Status schema honest: every
// JSON field the node emits must be described in the document.
func TestOpenAPIStatusSchemaMatchesStruct(t *testing.T) {
	doc := parseSpec(t, openAPISpec)
	props := doc.Components.Schemas["Status"].Properties
	if len(props) == 0 {
		t.Fatal("spec has no Status schema properties")
	}

	typ := reflect.TypeOf(Status{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := strings.Split(field.Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		if _, ok := props[tag]; !ok {
			t.Errorf("Status.%s (json %q) is missing from the spec schema", field.Name, tag)
		}
	}
	if len(props) != typ.NumField() {
		t.Errorf("Status schema has %d properties, Status struct has %d fields", len(props), typ.NumField())
	}
}
