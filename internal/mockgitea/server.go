package mockgitea

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
)

// Server serves a Store over HTTP with Gitea-shaped JSON.
type Server struct {
	store *Store
	http  *httptest.Server
}

// NewServer starts an HTTP server backed by store. Callers must Close it.
func NewServer(store *Store) *Server {
	s := &Server{store: store}
	mux := http.NewServeMux()
	s.routes(mux)
	s.http = httptest.NewServer(mux)
	return s
}

// URL returns the server's base URL (e.g. "http://127.0.0.1:PORT").
func (s *Server) URL() string { return s.http.URL }

// Close shuts down the underlying httptest.Server.
func (s *Server) Close() { s.http.Close() }

// routes registers every handler. Handlers build/marshal their response
// inside store.WithLock and use only unexported "*Locked" accessors there
// (see store.go's WithLock doc) — never mutators or exported getters, which
// self-lock and would deadlock against the non-reentrant mutex.
func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"version": "1.24.3"})
	})
	mux.HandleFunc("GET /api/v1/user", func(w http.ResponseWriter, r *http.Request) {
		s.store.WithLock(func() { writeJSON(w, s.store.meLocked()) })
	})
	mux.HandleFunc("GET /api/v1/repos/{owner}/{repo}", func(w http.ResponseWriter, r *http.Request) {
		s.store.WithLock(func() {
			repo := s.store.repoByFullNameLocked(r.PathValue("owner") + "/" + r.PathValue("repo"))
			if repo == nil {
				notFound(w, r)
				return
			}
			writeJSON(w, repo)
		})
	})
	// Catch-all LAST: unknown paths fail loudly so drift surfaces in tests.
	mux.HandleFunc("/", notFound)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// writeList writes a JSON array response with Gitea's X-Total-Count header.
// Unused until Task 3 wires up the search/list endpoints; kept here now so
// those handlers share one response-shaping helper with the rest of the
// server.
func writeList(w http.ResponseWriter, total int, v any) {
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	writeJSON(w, v)
}

func notFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "mockgitea: no handler for " + r.Method + " " + r.URL.Path,
	})
}
