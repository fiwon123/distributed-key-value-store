// Package webui serves the dashboard SPA, a Swagger UI page at /docs, a
// /cluster discovery endpoint, and a reverse proxy to the configured peer nodes.
//
// Static assets are embedded (see embed.go). The SPA falls back to
// placeholder.html only when the built index.html is absent, so a binary
// compiled without running the frontend build still serves something useful.
package webui

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"sort"
	"strings"
)

// ClusterInfo is the payload of GET /cluster.
type ClusterInfo struct {
	Self  string   `json:"self"`
	Peers []string `json:"peers"`
}

type handler struct {
	self    string
	peers   []string
	dist    fs.FS
	proxies map[string]*httputil.ReverseProxy
}

// NewHandler serves the dashboard for the node selfID, proxying to peers
// (id -> base URL). It returns an error only if the embedded assets are
// malformed.
func NewHandler(selfID string, peers map[string]string) (http.Handler, error) {
	dist, err := distFS()
	if err != nil {
		return nil, err
	}
	// peers is always a slice (never nil) so /cluster encodes [] not null;
	// the browser spreads it into an array.
	peerIDs := make([]string, 0, len(peers))
	h := &handler{
		self:    selfID,
		dist:    dist,
		proxies: make(map[string]*httputil.ReverseProxy, len(peers)),
	}
	for id, raw := range peers {
		target, err := url.Parse(raw)
		if err != nil {
			return nil, err
		}
		peerIDs = append(peerIDs, id)
		h.proxies[id] = httputil.NewSingleHostReverseProxy(target)
	}
	sort.Strings(peerIDs)
	h.peers = peerIDs

	mux := http.NewServeMux()
	mux.HandleFunc("GET /cluster", h.cluster)
	// Registered before the SPA catch-all ("/"); the more specific patterns win
	// regardless of order in Go's ServeMux.
	mux.HandleFunc("GET /docs", h.docs)
	mux.HandleFunc("GET /docs/", h.docs)
	mux.HandleFunc("/node/", h.proxy)
	mux.HandleFunc("/", h.serveSPA)
	return mux, nil
}

// docs serves the Swagger UI page, so operators can exercise the HTTP API
// straight from a browser against the node they are talking to.
func (h *handler) docs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(docsHTML)
}

func (h *handler) cluster(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ClusterInfo{Self: h.self, Peers: h.peers})
}

func (h *handler) proxy(w http.ResponseWriter, r *http.Request) {
	id, sub, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/node/"), "/")
	rp, ok := h.proxies[id]
	if !ok {
		http.Error(w, "unknown node", http.StatusNotFound)
		return
	}
	out := r.Clone(r.Context())
	out.URL.Path = "/" + sub
	rp.ServeHTTP(w, out)
}

func (h *handler) serveSPA(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || !h.exists(name) {
		name = h.indexName()
	}
	http.ServeFileFS(w, r, h.dist, name)
}

func (h *handler) indexName() string {
	if h.exists("index.html") {
		return "index.html"
	}
	return "placeholder.html"
}

func (h *handler) exists(name string) bool {
	info, err := fs.Stat(h.dist, name)
	return err == nil && !info.IsDir()
}
