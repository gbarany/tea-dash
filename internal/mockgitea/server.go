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
// self-lock and would deadlock against the non-reentrant mutex. Route
// registration is grouped by domain: routes() wires the version/user/repo
// probes directly and delegates everything else to a per-domain
// server_<domain>.go file (e.g. searchRoutes).
func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"version": "1.24.3"})
	})
	mux.HandleFunc("GET /api/v1/user", func(w http.ResponseWriter, r *http.Request) {
		respondOr404(s, w, r, func() *User { return s.store.meLocked() })
	})
	mux.HandleFunc("GET /api/v1/repos/{owner}/{repo}", func(w http.ResponseWriter, r *http.Request) {
		respondOr404(s, w, r, func() *Repo {
			return s.store.repoByFullNameLocked(r.PathValue("owner") + "/" + r.PathValue("repo"))
		})
	})
	s.searchRoutes(mux)
	s.detailRoutes(mux)
	// Catch-all LAST: unknown paths fail loudly so drift surfaces in tests.
	mux.HandleFunc("/", notFound)
}

// respondOr404 builds a value under the store lock and writes it as JSON, or
// a loud 404 when build returns nil. It encapsulates the marshal-under-
// WithLock contract so individual handlers can't forget it. Generic (*T, not
// any) deliberately: a typed-nil pointer boxed into any is not == nil and
// would marshal "null" instead of 404ing.
func respondOr404[T any](s *Server, w http.ResponseWriter, r *http.Request, build func() *T) {
	s.store.WithLock(func() {
		if v := build(); v != nil {
			writeJSON(w, v)
			return
		}
		notFound(w, r)
	})
}

// respondList builds a filtered/paged slice plus its pre-pagination total
// under the store lock and writes it with X-Total-Count.
func respondList[T any](s *Server, w http.ResponseWriter, build func() (rows []T, total int)) {
	s.store.WithLock(func() {
		rows, total := build()
		writeList(w, total, rows)
	})
}

// respondListOr404 is respondList for endpoints scoped to a parent resource
// (a repo, a pull, an issue, ...) that must 404 loudly when that parent is
// missing rather than silently writing an empty list — an empty list looks
// identical to "found the parent, it just has zero rows." build reports ok
// alongside rows/total; !ok writes the 404 instead.
func respondListOr404[T any](s *Server, w http.ResponseWriter, r *http.Request, build func() (rows []T, total int, ok bool)) {
	s.store.WithLock(func() {
		rows, total, ok := build()
		if !ok {
			notFound(w, r)
			return
		}
		writeList(w, total, rows)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	// json.Encoder.Encode marshals to an internal buffer before writing to w
	// in one call, so it's safe to still send an error response here — no
	// partial body can have reached the client yet.
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// writeList writes a JSON array response with Gitea's X-Total-Count header.
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
