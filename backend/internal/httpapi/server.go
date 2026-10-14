// Package httpapi implements the node's HTTP API.
//
// Endpoints:
//
//	PUT    /v1/kv/{key}   set a key (raw body, or JSON {"value":"..."})
//	GET    /v1/kv/{key}   read a key (404 if absent)
//	DELETE /v1/kv/{key}   delete a key (204, or 404 if absent)
//	GET    /v1/kv         list all keys
//	GET    /v1/status     node/cluster status
//
// The API talks to a Backend, so the storage layer can change without touching
// the handlers. Phase 1 backs it with the in-memory store; later phases back it
// with the Raft log.
package httpapi

import (
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// openAPISpec is the API contract served at GET /v1/openapi.json. It must live
// inside this package because go:embed cannot reference parent directories. The
// drift test in openapi_test.go keeps it in sync with routes().
//
//go:embed openapi.json
var openAPISpec []byte

// Backend is the read/write interface the HTTP API drives.
//
// Put, Delete, and Get return an error when the operation cannot be served. A
// non-leader node returns ErrUnavailable, which the API maps to 503 with the
// current leader in the X-Raft-Leader header (empty if unknown).
type Backend interface {
	Get(key string) (string, bool, error)
	Put(key, value string) error
	Delete(key string) (bool, error)
	List() ([]string, error)
}

// ErrUnavailable signals that a write cannot be accepted right now, for example
// because the node is not the leader or a quorum is unreachable. The API maps
// it to 503 Service Unavailable.
var ErrUnavailable = errors.New("write unavailable")

// Status is the JSON body served by GET /v1/status.
type Status struct {
	ID          string `json:"id"`
	Role        string `json:"role"`
	Term        uint64 `json:"term"`
	Leader      string `json:"leader"`
	CommitIndex uint64 `json:"commitIndex"`
	LastApplied uint64 `json:"lastApplied"`
	Peers       int    `json:"peers"`
}

// StatusProvider supplies the current node/cluster status.
type StatusProvider interface {
	Status() Status
}

// keyValue is the JSON shape returned by the key endpoints.
type keyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// keyList is the JSON shape returned by GET /v1/kv.
type keyList struct {
	Keys []string `json:"keys"`
}

// errorBody is the JSON shape returned on errors.
type errorBody struct {
	Error string `json:"error"`
}

// Server routes HTTP requests to the backend and status provider.
type Server struct {
	backend Backend
	status  StatusProvider
	mux     *http.ServeMux
}

// routeSpec ties an HTTP route to the handler that serves it. routes() is the
// single source of truth for both registration and the OpenAPI drift test, so a
// new endpoint is added in one place.
type routeSpec struct {
	Method  string
	Pattern string
	Handler http.HandlerFunc
}

// routes lists the public API endpoints. The root ping is deliberately absent:
// it is not part of the documented API (and is shadowed by the webui in the
// assembled node), so it is registered separately in New.
func (s *Server) routes() []routeSpec {
	return []routeSpec{
		{http.MethodPut, "/v1/kv/{key}", s.handlePut},
		{http.MethodGet, "/v1/kv/{key}", s.handleGet},
		{http.MethodDelete, "/v1/kv/{key}", s.handleDelete},
		{http.MethodGet, "/v1/kv", s.handleList},
		{http.MethodGet, "/v1/status", s.handleStatus},
		{http.MethodGet, "/v1/openapi.json", s.handleOpenAPI},
	}
}

// New builds a Server and registers its routes.
func New(backend Backend, status StatusProvider) *Server {
	s := &Server{backend: backend, status: status}
	mux := http.NewServeMux()
	for _, r := range s.routes() {
		mux.HandleFunc(r.Method+" "+r.Pattern, r.Handler)
	}
	mux.HandleFunc("GET /", s.handleRoot)
	s.mux = mux
	return s
}

// Handler returns the server's http.Handler for use with http.Server or tests.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) handlePut(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	value, err := readValue(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.backend.Put(key, value); err != nil {
		s.writeBackendError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, keyValue{Key: key, Value: value})
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	value, ok, err := s.backend.Get(key)
	if err != nil {
		s.writeBackendError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}
	writeJSON(w, http.StatusOK, keyValue{Key: key, Value: value})
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	deleted, err := s.backend.Delete(key)
	if err != nil {
		s.writeBackendError(w, err)
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleList(w http.ResponseWriter, _ *http.Request) {
	keys, err := s.backend.List()
	if err != nil {
		s.writeBackendError(w, err)
		return
	}
	if keys == nil {
		keys = []string{}
	}
	writeJSON(w, http.StatusOK, keyList{Keys: keys})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.status.Status())
}

// handleOpenAPI serves the embedded OpenAPI document so tooling (and the /docs
// Swagger UI page) can read the contract from a running node.
func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openAPISpec)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	// Only the exact root path; anything else is a 404.
	if r.URL.Path != "/" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "distributed-key-value-store\nsee /v1/status\n")
}

// readValue extracts the value from a PUT body. A JSON body of the form
// {"value":"..."} is decoded; any other body is used verbatim.
func readValue(r *http.Request) (string, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return "", errors.New("failed to read request body")
	}
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var kv keyValue
		if err := json.Unmarshal(body, &kv); err != nil {
			return "", errors.New("invalid JSON body")
		}
		return kv.Value, nil
	}
	return string(body), nil
}

// writeBackendError maps a backend error to a response. Unavailable (e.g. this
// node is not the leader) becomes 503 with the known leader in X-Raft-Leader so
// clients can retry against it.
func (s *Server) writeBackendError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrUnavailable) {
		if leader := s.status.Status().Leader; leader != "" {
			w.Header().Set("X-Raft-Leader", leader)
		}
		writeError(w, http.StatusServiceUnavailable, "write unavailable")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal error")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}
