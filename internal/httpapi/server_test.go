package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeBackend is an in-memory Backend for handler tests. It records injected
// errors so unavailable/error paths can be exercised without a real node.
type fakeBackend struct {
	m      map[string]string
	putErr error
	delErr error
}

func newFakeBackend() *fakeBackend { return &fakeBackend{m: map[string]string{}} }

func (f *fakeBackend) Get(key string) (string, bool) { v, ok := f.m[key]; return v, ok }

func (f *fakeBackend) Put(key, value string) error {
	if f.putErr != nil {
		return f.putErr
	}
	f.m[key] = value
	return nil
}

func (f *fakeBackend) Delete(key string) (bool, error) {
	if f.delErr != nil {
		return false, f.delErr
	}
	_, ok := f.m[key]
	delete(f.m, key)
	return ok, nil
}

type fakeStatus struct {
	s Status
}

func (f fakeStatus) Status() Status { return f.s }

func newTestServer(t *testing.T, b Backend) *httptest.Server {
	t.Helper()
	status := fakeStatus{s: Status{ID: "node1", Role: "single", Leader: "node1"}}
	ts := httptest.NewServer(New(b, status).Handler())
	t.Cleanup(ts.Close)
	return ts
}

func doRequest(t *testing.T, method, url, contentType, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestPutThenGet(t *testing.T) {
	b := newFakeBackend()
	ts := newTestServer(t, b)

	if resp := doRequest(t, http.MethodPut, ts.URL+"/v1/kv/alpha", "", "one"); resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	resp := doRequest(t, http.MethodGet, ts.URL+"/v1/kv/alpha", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var kv keyValue
	if err := json.NewDecoder(resp.Body).Decode(&kv); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if kv.Key != "alpha" || kv.Value != "one" {
		t.Fatalf("GET = %+v, want {alpha one}", kv)
	}
}

func TestPutJSONBody(t *testing.T) {
	b := newFakeBackend()
	ts := newTestServer(t, b)

	resp := doRequest(t, http.MethodPut, ts.URL+"/v1/kv/k", "application/json", `{"value":"from-json"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got, _ := b.Get("k"); got != "from-json" {
		t.Fatalf("stored value = %q, want %q", got, "from-json")
	}
}

func TestPutInvalidJSON(t *testing.T) {
	ts := newTestServer(t, newFakeBackend())
	resp := doRequest(t, http.MethodPut, ts.URL+"/v1/kv/k", "application/json", "{not json")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestGetMissing(t *testing.T) {
	ts := newTestServer(t, newFakeBackend())
	resp := doRequest(t, http.MethodGet, ts.URL+"/v1/kv/missing", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestDelete(t *testing.T) {
	tests := []struct {
		name     string
		seed     bool
		wantCode int
	}{
		{name: "existing", seed: true, wantCode: http.StatusNoContent},
		{name: "missing", seed: false, wantCode: http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newFakeBackend()
			if tc.seed {
				b.m["k"] = "v"
			}
			ts := newTestServer(t, b)
			resp := doRequest(t, http.MethodDelete, ts.URL+"/v1/kv/k", "", "")
			if resp.StatusCode != tc.wantCode {
				t.Fatalf("DELETE status = %d, want %d", resp.StatusCode, tc.wantCode)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	ts := newTestServer(t, newFakeBackend())
	resp := doRequest(t, http.MethodGet, ts.URL+"/v1/status", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status endpoint = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var s Status
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if s.ID != "node1" || s.Role != "single" {
		t.Fatalf("status = %+v, want id=node1 role=single", s)
	}
}

func TestPutUnavailable(t *testing.T) {
	b := newFakeBackend()
	b.putErr = ErrUnavailable
	ts := newTestServer(t, b)
	resp := doRequest(t, http.MethodPut, ts.URL+"/v1/kv/k", "", "v")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("PUT status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

func TestPutInternalError(t *testing.T) {
	b := newFakeBackend()
	b.putErr = errors.New("boom")
	ts := newTestServer(t, b)
	resp := doRequest(t, http.MethodPut, ts.URL+"/v1/kv/k", "", "v")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("PUT status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

func TestRoot(t *testing.T) {
	ts := newTestServer(t, newFakeBackend())
	resp := doRequest(t, http.MethodGet, ts.URL+"/", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("root status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestUnknownPath(t *testing.T) {
	ts := newTestServer(t, newFakeBackend())
	resp := doRequest(t, http.MethodGet, ts.URL+"/nope", "", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown path status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}
