package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Contract tests drive requests derived from the OpenAPI document itself
// (openAPISpec) against the real handlers, so the spec cannot drift from
// behaviour in either direction:
//
//   - every response a handler can produce must be declared, and
//   - every response the spec declares must be producible.
//
// The drift tests in openapi_test.go cover the path/method and Status-schema
// shape; these cover the observable behaviour (status codes, JSON schemas,
// headers). Adding an operation or a declared response to the spec
// automatically pulls it into these tests — and a declared response with no
// scenario to provoke it fails the suite, which is the point.

// contractKey is the sample value substituted for the spec's {key} path param.
const contractKey = "contract-key"

// contractJSONBody is a valid body for an operation that declares a requestBody.
const contractJSONBody = `{"value":"contract"}`

var pathParamRe = regexp.MustCompile(`\{[^}]+\}`)

// contractDoc is the subset of the OpenAPI document these tests read.
type contractDoc struct {
	Paths      map[string]json.RawMessage `json:"paths"`
	Components struct {
		Schemas map[string]contractSchema `json:"schemas"`
	} `json:"components"`
}

type contractSchema struct {
	Required []string `json:"required"`
}

type openAPIMedia struct {
	Schema json.RawMessage `json:"schema"`
}

type openAPIRequestBody struct {
	Content map[string]openAPIMedia `json:"content"`
}

type openAPIResponse struct {
	Headers map[string]json.RawMessage `json:"headers"`
	Content map[string]openAPIMedia    `json:"content"`
}

type openAPIOperation struct {
	RequestBody *openAPIRequestBody        `json:"requestBody"`
	Responses   map[string]openAPIResponse `json:"responses"`
}

func parseContractDoc(t *testing.T, data []byte) contractDoc {
	t.Helper()
	var doc contractDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse OpenAPI spec: %v", err)
	}
	if len(doc.Paths) == 0 {
		t.Fatal("OpenAPI spec declares no paths")
	}
	return doc
}

// eachOperation calls fn once per documented HTTP operation. The path item's
// non-method keys (e.g. a path-level "parameters" array) are skipped.
func eachOperation(t *testing.T, doc contractDoc, fn func(method, path string, op openAPIOperation)) {
	t.Helper()
	for path, raw := range doc.Paths {
		var pathItem map[string]json.RawMessage
		if err := json.Unmarshal(raw, &pathItem); err != nil {
			t.Fatalf("parse path item %s: %v", path, err)
		}
		for key, rawOp := range pathItem {
			method := strings.ToUpper(key)
			if !httpMethods[method] {
				continue
			}
			var op openAPIOperation
			if err := json.Unmarshal(rawOp, &op); err != nil {
				t.Fatalf("parse operation %s %s: %v", method, path, err)
			}
			if len(op.Responses) == 0 {
				t.Fatalf("%s %s declares no responses", method, path)
			}
			fn(method, path, op)
		}
	}
}

func substitutePathParams(path string) string {
	return pathParamRe.ReplaceAllStringFunc(path, func(m string) string {
		if strings.Trim(m, "{}") == "key" {
			return contractKey
		}
		return "sample"
	})
}

func responseCodes(op openAPIOperation) []string {
	out := make([]string, 0, len(op.Responses))
	for code := range op.Responses {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

// TestContractObservedStatusIsDeclared fails when a handler answers with a
// status the spec does not document for that operation — the "handler adds a
// status nobody documented" direction of drift.
func TestContractObservedStatusIsDeclared(t *testing.T) {
	doc := parseContractDoc(t, openAPISpec)
	eachOperation(t, doc, func(method, path string, op openAPIOperation) {
		urlPath := substitutePathParams(path)
		contentType, body := "", ""
		if op.RequestBody != nil {
			contentType, body = "application/json", contractJSONBody
		}
		for _, seeded := range []bool{true, false} {
			name := method + " " + path
			if seeded {
				name += " (seeded)"
			} else {
				name += " (empty)"
			}
			t.Run(name, func(t *testing.T) {
				b := newFakeBackend()
				if seeded {
					b.m[contractKey] = "contract"
				}
				ts := newTestServer(t, b)
				resp := doRequest(t, method, ts.URL+urlPath, contentType, body)
				if _, ok := op.Responses[strconv.Itoa(resp.StatusCode)]; !ok {
					t.Fatalf("%s %s returned %d, which is not among the declared responses %v",
						method, path, resp.StatusCode, responseCodes(op))
				}
			})
		}
	})
}

// contractScenario provokes one declared response for one operation.
type contractScenario struct {
	setup       func(*fakeBackend)
	method      string
	urlPath     string
	contentType string
	body        string
}

// contractScenarios maps "<METHOD> <path> <code>" to the request that produces
// that response. TestContractEveryDeclaredResponseIsReachable fails if a
// declared response has no entry here.
func contractScenarios() map[string]contractScenario {
	kvURL := "/v1/kv/" + contractKey
	seed := func(b *fakeBackend) { b.m[contractKey] = "contract" }

	scenarios := map[string]contractScenario{}
	add := func(key string, sc contractScenario) { scenarios[key] = sc }

	// PUT /v1/kv/{key}
	add("PUT /v1/kv/{key} 200", contractScenario{method: "PUT", urlPath: kvURL, contentType: "application/json", body: contractJSONBody})
	add("PUT /v1/kv/{key} 400", contractScenario{method: "PUT", urlPath: kvURL, contentType: "application/json", body: "{not json"})
	add("PUT /v1/kv/{key} 500", contractScenario{setup: func(b *fakeBackend) { b.putErr = errors.New("boom") }, method: "PUT", urlPath: kvURL, contentType: "application/json", body: contractJSONBody})
	add("PUT /v1/kv/{key} 503", contractScenario{setup: func(b *fakeBackend) { b.putErr = ErrUnavailable }, method: "PUT", urlPath: kvURL, contentType: "application/json", body: contractJSONBody})

	// GET /v1/kv/{key}
	add("GET /v1/kv/{key} 200", contractScenario{setup: seed, method: "GET", urlPath: kvURL})
	add("GET /v1/kv/{key} 404", contractScenario{method: "GET", urlPath: kvURL})
	add("GET /v1/kv/{key} 500", contractScenario{setup: func(b *fakeBackend) { b.getErr = errors.New("boom") }, method: "GET", urlPath: kvURL})
	add("GET /v1/kv/{key} 503", contractScenario{setup: func(b *fakeBackend) { b.getErr = ErrUnavailable }, method: "GET", urlPath: kvURL})

	// DELETE /v1/kv/{key}
	add("DELETE /v1/kv/{key} 204", contractScenario{setup: seed, method: "DELETE", urlPath: kvURL})
	add("DELETE /v1/kv/{key} 404", contractScenario{method: "DELETE", urlPath: kvURL})
	add("DELETE /v1/kv/{key} 500", contractScenario{setup: func(b *fakeBackend) { b.delErr = errors.New("boom") }, method: "DELETE", urlPath: kvURL})
	add("DELETE /v1/kv/{key} 503", contractScenario{setup: func(b *fakeBackend) { b.delErr = ErrUnavailable }, method: "DELETE", urlPath: kvURL})

	// Read-only endpoints
	add("GET /v1/status 200", contractScenario{method: "GET", urlPath: "/v1/status"})
	add("GET /v1/openapi.json 200", contractScenario{method: "GET", urlPath: "/v1/openapi.json"})

	return scenarios
}

type contractResponse struct {
	code   int
	header http.Header
	body   []byte
}

func runContractScenario(t *testing.T, sc contractScenario) contractResponse {
	t.Helper()
	b := newFakeBackend()
	if sc.setup != nil {
		sc.setup(b)
	}
	ts := newTestServer(t, b)
	resp := doRequest(t, sc.method, ts.URL+sc.urlPath, sc.contentType, sc.body)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return contractResponse{code: resp.StatusCode, header: resp.Header, body: body}
}

func scenarioStatus(t *testing.T, key string) int {
	t.Helper()
	code, err := strconv.Atoi(key[strings.LastIndex(key, " ")+1:])
	if err != nil {
		t.Fatalf("scenario key %q has no numeric status", key)
	}
	return code
}

// TestContractEveryDeclaredResponseIsReachable fails in two directions: a
// declared response with no provoking scenario (spec over-promises) and a
// scenario that does not correspond to a declared response (stale test).
func TestContractEveryDeclaredResponseIsReachable(t *testing.T) {
	doc := parseContractDoc(t, openAPISpec)
	scenarios := contractScenarios()

	declared := map[string]bool{}
	eachOperation(t, doc, func(method, path string, op openAPIOperation) {
		for code := range op.Responses {
			declared[method+" "+path+" "+code] = true
		}
	})

	for key := range declared {
		if _, ok := scenarios[key]; !ok {
			t.Errorf("declared response %q has no scenario — add one so the spec cannot over-promise", key)
		}
	}
	for key := range scenarios {
		if !declared[key] {
			t.Errorf("scenario %q does not match a declared response — remove it or fix the spec", key)
		}
	}

	for key, sc := range scenarios {
		want := scenarioStatus(t, key)
		t.Run(key, func(t *testing.T) {
			if got := runContractScenario(t, sc); got.code != want {
				t.Fatalf("status = %d, want %d", got.code, want)
			}
		})
	}
}

// TestContractJSONResponsesMatchSchemas checks that every JSON response whose
// schema is a $ref to a component exposes all of that component's required
// properties — the "handler drops a promised field" direction of drift.
func TestContractJSONResponsesMatchSchemas(t *testing.T) {
	doc := parseContractDoc(t, openAPISpec)
	scenarios := contractScenarios()

	eachOperation(t, doc, func(method, path string, op openAPIOperation) {
		for code, resp := range op.Responses {
			media, ok := resp.Content["application/json"]
			if !ok {
				continue
			}
			schema := resolveContractRef(t, doc, media.Schema)
			if schema == nil {
				continue
			}
			key := method + " " + path + " " + code
			sc, ok := scenarios[key]
			if !ok {
				t.Errorf("no scenario to verify the schema for %q", key)
				continue
			}
			t.Run(key, func(t *testing.T) {
				got := runContractScenario(t, sc)
				if want := scenarioStatus(t, key); got.code != want {
					t.Fatalf("status = %d, want %d", got.code, want)
				}
				var body map[string]json.RawMessage
				if err := json.Unmarshal(got.body, &body); err != nil {
					t.Fatalf("response body is not a JSON object: %v (%q)", err, got.body)
				}
				for _, field := range schema.Required {
					if _, ok := body[field]; !ok {
						t.Errorf("response %s is missing required field %q; body = %s", key, field, got.body)
					}
				}
			})
		}
	})
}

// TestContractDeclaredHeadersAreSent checks that a response describing headers
// (e.g. X-Raft-Leader on the 503s) actually sends them.
func TestContractDeclaredHeadersAreSent(t *testing.T) {
	doc := parseContractDoc(t, openAPISpec)
	scenarios := contractScenarios()

	eachOperation(t, doc, func(method, path string, op openAPIOperation) {
		for code, resp := range op.Responses {
			if len(resp.Headers) == 0 {
				continue
			}
			key := method + " " + path + " " + code
			sc, ok := scenarios[key]
			if !ok {
				t.Errorf("no scenario to verify the headers for %q", key)
				continue
			}
			t.Run(key, func(t *testing.T) {
				got := runContractScenario(t, sc)
				for name := range resp.Headers {
					if got.header.Get(name) == "" {
						t.Errorf("response %s is missing declared header %q; headers = %v", key, name, got.header)
					}
				}
			})
		}
	})
}

// resolveContractRef returns the component schema a $ref points at, or nil when
// the schema is inline (nothing to cross-check).
func resolveContractRef(t *testing.T, doc contractDoc, schema json.RawMessage) *contractSchema {
	t.Helper()
	if len(schema) == 0 {
		return nil
	}
	var ref struct {
		Ref string `json:"$ref"`
	}
	if err := json.Unmarshal(schema, &ref); err != nil {
		t.Fatalf("parse response schema: %v", err)
	}
	if ref.Ref == "" {
		return nil
	}
	name := ref.Ref[strings.LastIndex(ref.Ref, "/")+1:]
	s, ok := doc.Components.Schemas[name]
	if !ok {
		t.Fatalf("schema %q is referenced but not defined", ref.Ref)
	}
	return &s
}
