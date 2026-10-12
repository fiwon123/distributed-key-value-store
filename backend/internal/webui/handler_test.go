package webui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestHandler(t *testing.T, self string, peers map[string]string) http.Handler {
	t.Helper()
	h, err := NewHandler(self, peers)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func TestServeIndex(t *testing.T) {
	h := newTestHandler(t, "node1", nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<html") {
		t.Fatalf("body does not look like HTML: %q", rec.Body.String())
	}
}

func TestClusterListsPeers(t *testing.T) {
	h := newTestHandler(t, "node1", map[string]string{
		"node2": "http://127.0.0.1:8082",
		"node3": "http://127.0.0.1:8083",
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/cluster", nil))

	var got ClusterInfo
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Self != "node1" {
		t.Fatalf("self = %q, want node1", got.Self)
	}
	if len(got.Peers) != 2 || got.Peers[0] != "node2" || got.Peers[1] != "node3" {
		t.Fatalf("peers = %v, want [node2 node3]", got.Peers)
	}
}

func TestDocsPage(t *testing.T) {
	h := newTestHandler(t, "node1", nil)

	for _, path := range []string{"/docs", "/docs/"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
				t.Fatalf("Content-Type = %q, want text/html", ct)
			}
			body := rec.Body.String()
			if !strings.Contains(body, "swagger-ui") {
				t.Fatalf("docs page does not reference swagger-ui")
			}
			if !strings.Contains(body, "/v1/openapi.json") {
				t.Fatalf("docs page does not point at the spec URL")
			}
		})
	}
}

func TestProxyForwardsToPeer(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "path="+r.URL.Path)
	}))
	defer upstream.Close()

	h := newTestHandler(t, "node1", map[string]string{"node2": upstream.URL})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/node/node2/v1/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "path=/v1/status" {
		t.Fatalf("body = %q, want path=/v1/status", got)
	}
}

func TestProxyUnknownNode(t *testing.T) {
	h := newTestHandler(t, "node1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/node/ghost/v1/status", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
