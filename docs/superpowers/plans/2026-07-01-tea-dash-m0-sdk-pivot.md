# tea-dash M0 — SDK Pivot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace tea-dash's `tea api` subprocess data layer with direct Gitea SDK access, so the existing PR table renders the authed user's open PRs across all repos (me-scoped) via the Gitea REST API — auth reused from `tea`'s own config.

**Architecture:** Introduce `internal/auth` (resolve instance URL + token from `tea`'s config / overrides / env), `internal/gitea` (an SDK client wrapper + a raw me-scoped `/repos/issues/search` call), and `internal/data` (a TUI-agnostic `PullRequest` domain type). Rewire `main.go` and `internal/ui` onto them and delete `internal/teacli`. The board's default query becomes "pull requests **created by me**, open, across all accessible repos" — Gitea scopes this via a boolean query param the typed SDK cannot emit, so that one call goes through a small raw HTTP helper.

**Tech Stack:** Go 1.25, `code.gitea.io/sdk/gitea` v0.25.1 (Gitea SDK; verified compiling), Bubble Tea v2 (`charm.land/*/v2`), `gopkg.in/yaml.v3`. Tests use the standard library `testing` package + `net/http/httptest` (matching the repo's existing style — no testify).

**Scope note:** M0 is the data-layer pivot only. **Deferred to M1** (intentionally, not gaps): the cobra+fang CLI (keep the current hand-rolled `main.go`); the koanf layered config (keep `yaml.v3`); per-repo / structured-filter sections (the `repos:` config field is parsed but unused in M0); the interactive TUI auth prompt (M0 falls back to env vars + a helpful error); and **cwd git-repo resolution** (`internal/git` + `internal/gitea/remote.go`) — needed only for smart-filtering / local git ops, which M0's cross-repo me-scoped board does not require.

**Verification note:** The SDK-touching code in Tasks 4–5 was compile-checked against the real `code.gitea.io/sdk/gitea` v0.25.1 (`go build`/`go vet` clean; run against `httptest`). `NewClient` issues `GET /api/v1/version` at construction and `GetMyUserInfo` hits `GET /api/v1/user` — both are served by the test servers below.

---

## File Structure

**Create:**
- `internal/data/model.go` — TUI-agnostic domain types (`PullRequest`, `User`, `Label`) + `SplitOwnerRepo` helper.
- `internal/data/model_test.go` — tests for `SplitOwnerRepo`.
- `internal/auth/auth.go` — `Config`, `Overrides`, tea-config parsing, `Resolve`/`ResolveFromFile`.
- `internal/auth/auth_test.go` — resolution/precedence/error tests.
- `internal/gitea/client.go` — `Client` wrapper: `NewClient`, `Me`, `rawGet`.
- `internal/gitea/client_test.go` — client construction + identity via httptest.
- `internal/gitea/search.go` — `SearchMyPulls` (raw me-scoped PR search) + row mapping.
- `internal/gitea/search_test.go` — search request/response + C1 regression guard.

**Modify:**
- `go.mod` / `go.sum` — add the Gitea SDK.
- `internal/config/config.go` — add an `Instance` config block.
- `internal/config/config_test.go` — parse test for `instance:`.
- `internal/ui/app.go` — use `*gitea.Client` + `data.PullRequest`; fetch via `SearchMyPulls`.
- `internal/ui/app_test.go` — update fixtures to `data.PullRequest` and the new `New` signature.
- `internal/ui/styles.go` — remove the now-dead `warnStyle`.
- `main.go` — resolve auth + build the Gitea client + pass it to the UI.
- `README.md` — refresh the "How it works" wording (SDK-direct, not `tea api`).

**Delete:**
- `internal/teacli/client.go`, `internal/teacli/types.go`, `internal/teacli/client_test.go` (the whole `teacli` package).

**Task order** (dependency-clean): SDK dep → data → **auth** → gitea client → gitea search → config → ui → main → delete teacli.

---

## Task 1: Add the Gitea SDK dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the SDK module**

Run:
```bash
cd /Users/gaborbarany/dev/sandbox/tea-dash
go get code.gitea.io/sdk/gitea@latest
```
Expected: `go.mod` gains `require code.gitea.io/sdk/gitea v0.25.1` (or newer). The `NewClient`/`SetToken`/`GetMyUserInfo`/`ServerVersion` API used here is verified against v0.25.1. If `@latest` ever regresses, pin it: `go get code.gitea.io/sdk/gitea@v0.25.1`.

- [ ] **Step 2: Tidy and verify it builds**

Run:
```bash
go mod tidy && go build ./...
```
Expected: no output, exit 0.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "build: add code.gitea.io/sdk/gitea dependency"
```

---

## Task 2: Domain model (`internal/data`)

TUI-agnostic types the UI and the Gitea layer share, plus a small tested helper for splitting `owner/repo`.

**Files:**
- Create: `internal/data/model.go`
- Test: `internal/data/model_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/data/model_test.go`:
```go
package data

import "testing"

func TestSplitOwnerRepo(t *testing.T) {
	owner, name, ok := SplitOwnerRepo("acme/widgets")
	if !ok || owner != "acme" || name != "widgets" {
		t.Fatalf(`SplitOwnerRepo("acme/widgets") = %q, %q, %v`, owner, name, ok)
	}

	for _, bad := range []string{"", "noslash", "a/b/c", "/x", "x/"} {
		if _, _, ok := SplitOwnerRepo(bad); ok {
			t.Fatalf("SplitOwnerRepo(%q) ok = true, want false", bad)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/data/ -run TestSplitOwnerRepo -v`
Expected: FAIL — build error, `undefined: SplitOwnerRepo`.

- [ ] **Step 3: Write the implementation**

Create `internal/data/model.go`:
```go
// Package data holds tea-dash's TUI-agnostic domain types, decoupled from
// both the Gitea transport and the Bubble Tea UI.
package data

import (
	"strings"
	"time"
)

// User is a subset of a Gitea user.
type User struct {
	Login    string
	FullName string
}

// Label is a subset of a Gitea label.
type Label struct {
	Name  string
	Color string
}

// PullRequest is the domain view of a Gitea pull request, denormalized so a
// row from the cross-repo search endpoint carries its own repo.
type PullRequest struct {
	Number            int64     // per-repo index
	Title             string
	RepoNameWithOwner string    // "owner/repo"
	Author            string    // poster login
	State             string    // "open" | "closed" | "merged"
	Draft             bool
	HTMLURL           string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Labels            []Label
}

// SplitOwnerRepo splits "owner/name" into its parts. ok is false for anything
// that is not exactly one owner and one name.
func SplitOwnerRepo(full string) (owner, name string, ok bool) {
	owner, name, ok = strings.Cut(full, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", "", false
	}
	return owner, name, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/data/ -run TestSplitOwnerRepo -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/data/
git commit -m "feat(data): add TUI-agnostic PullRequest domain model"
```

---

## Task 3: Auth resolution (`internal/auth`)

Resolve instance URL + token from tea-dash overrides → env → `tea`'s own config. On macOS `tea` stores its config under `os.UserConfigDir()` (`~/Library/Application Support/tea/config.yml`), **not** `~/.config` — verified on this machine.

**Files:**
- Create: `internal/auth/auth.go`
- Test: `internal/auth/auth_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/auth/auth_test.go`:
```go
package auth

import (
	"os"
	"path/filepath"
	"testing"
)

const teaConfig = `logins:
    - name: personal
      url: https://gitea.example.org
      token: personaltoken
      default: false
      insecure: false
      user: me
    - name: work
      url: https://git.work.example
      token: worktoken
      default: true
      insecure: true
      user: me
`

func writeTeaConfig(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(p, []byte(teaConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolvePicksDefaultLogin(t *testing.T) {
	t.Setenv("TEA_DASH_URL", "")
	t.Setenv("TEA_DASH_TOKEN", "")
	got, err := ResolveFromFile(writeTeaConfig(t), Overrides{})
	if err != nil {
		t.Fatalf("ResolveFromFile: %v", err)
	}
	if got.URL != "https://git.work.example" || got.Token != "worktoken" || !got.Insecure {
		t.Fatalf("resolved = %+v, want the default (work) login", got)
	}
}

func TestResolvePicksNamedLogin(t *testing.T) {
	t.Setenv("TEA_DASH_URL", "")
	t.Setenv("TEA_DASH_TOKEN", "")
	got, err := ResolveFromFile(writeTeaConfig(t), Overrides{Login: "personal"})
	if err != nil {
		t.Fatalf("ResolveFromFile: %v", err)
	}
	if got.URL != "https://gitea.example.org" || got.Token != "personaltoken" || got.Insecure {
		t.Fatalf("resolved = %+v, want the personal login", got)
	}
}

func TestResolveOverridesAndEnvWin(t *testing.T) {
	t.Setenv("TEA_DASH_URL", "")
	t.Setenv("TEA_DASH_TOKEN", "envtoken")
	got, err := ResolveFromFile(writeTeaConfig(t), Overrides{URL: "https://override.example"})
	if err != nil {
		t.Fatalf("ResolveFromFile: %v", err)
	}
	if got.URL != "https://override.example" || got.Token != "envtoken" {
		t.Fatalf("resolved = %+v, want override URL + env token", got)
	}
}

func TestResolveMissingTokenErrors(t *testing.T) {
	t.Setenv("TEA_DASH_URL", "https://only-url.example")
	t.Setenv("TEA_DASH_TOKEN", "")
	// No tea config file at this path -> no login token available.
	_, err := ResolveFromFile(filepath.Join(t.TempDir(), "missing.yml"), Overrides{})
	if err == nil {
		t.Fatal("expected an error when no token can be resolved")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -v`
Expected: FAIL — `undefined: ResolveFromFile` / `Overrides` / `Config`.

- [ ] **Step 3: Write the implementation**

Create `internal/auth/auth.go`:
```go
// Package auth resolves the Gitea instance URL + token tea-dash connects with,
// reusing the `tea` CLI's own login config when present.
package auth

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the resolved connection credential.
type Config struct {
	URL        string
	Token      string
	Insecure   bool
	CACertPath string
}

// Overrides come from tea-dash's own config (its `instance:` block).
type Overrides struct {
	Login      string // pick a named tea login
	URL        string
	Token      string
	Insecure   bool
	CACertPath string
}

// teaLogin mirrors one entry in tea's config.yml `logins:` list.
type teaLogin struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url"`
	Token    string `yaml:"token"`
	Default  bool   `yaml:"default"`
	Insecure bool   `yaml:"insecure"`
	User     string `yaml:"user"`
}

type teaConfigFile struct {
	Logins []teaLogin `yaml:"logins"`
}

// TeaConfigPath returns the path to tea's config.yml, using the same per-OS
// config directory tea itself uses: os.UserConfigDir()/tea/config.yml
// (e.g. ~/Library/Application Support/tea on macOS, ~/.config/tea on Linux).
func TeaConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tea", "config.yml"), nil
}

// Resolve reads tea's config from its default location and resolves auth.
func Resolve(ov Overrides) (Config, error) {
	path, err := TeaConfigPath()
	if err != nil {
		return Config{}, err
	}
	return ResolveFromFile(path, ov)
}

// ResolveFromFile resolves auth against a specific tea config path (used in
// tests). A missing file is not an error: overrides/env may still suffice.
func ResolveFromFile(path string, ov Overrides) (Config, error) {
	logins := readTeaLogins(path)
	login := pickLogin(logins, ov.Login)

	url := firstNonEmpty(ov.URL, os.Getenv("TEA_DASH_URL"), loginField(login, func(l teaLogin) string { return l.URL }))
	token := firstNonEmpty(ov.Token, os.Getenv("TEA_DASH_TOKEN"), loginField(login, func(l teaLogin) string { return l.Token }))

	if url == "" {
		return Config{}, errors.New("no Gitea instance URL: run `tea login add`, or set instance.url / TEA_DASH_URL")
	}
	if token == "" {
		return Config{}, errors.New("no Gitea token: run `tea login add`, or set instance.token / TEA_DASH_TOKEN")
	}

	insecure := ov.Insecure
	if login != nil && login.Insecure {
		insecure = true
	}
	return Config{URL: url, Token: token, Insecure: insecure, CACertPath: ov.CACertPath}, nil
}

// readTeaLogins returns tea's logins, or nil if the file is absent/unreadable.
func readTeaLogins(path string) []teaLogin {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg teaConfigFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	return cfg.Logins
}

// pickLogin selects a login: by name if given, else the default, else the sole
// login, else nil.
func pickLogin(logins []teaLogin, name string) *teaLogin {
	if name != "" {
		for i := range logins {
			if logins[i].Name == name {
				return &logins[i]
			}
		}
		return nil
	}
	for i := range logins {
		if logins[i].Default {
			return &logins[i]
		}
	}
	if len(logins) == 1 {
		return &logins[0]
	}
	return nil
}

func loginField(l *teaLogin, get func(teaLogin) string) string {
	if l == nil {
		return ""
	}
	return get(*l)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/auth/ -v`
Expected: PASS (all four resolution tests).

- [ ] **Step 5: Commit**

```bash
git add internal/auth/
git commit -m "feat(auth): resolve Gitea URL+token from tea config, overrides, env"
```

---

## Task 4: Gitea client wrapper (`internal/gitea`)

Wraps the SDK client, resolves identity (`me`), and holds a raw HTTP escape hatch reused in Task 5. Depends on `internal/auth` (Task 3), which is now in place.

**Files:**
- Create: `internal/gitea/client.go`
- Test: `internal/gitea/client_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/gitea/client_test.go`:
```go
package gitea

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gbarany/tea-dash/internal/auth"
)

// fakeGitea serves the minimal endpoints NewClient touches: the version probe
// (hit at construction) and the current-user lookup.
func fakeGitea(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"version":"1.22.0"}`)
	})
	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":1,"login":"me","full_name":"Me"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestNewClientResolvesMe(t *testing.T) {
	srv := fakeGitea(t)
	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.Me() != "me" {
		t.Fatalf("Me() = %q, want %q", c.Me(), "me")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/gitea/ -run TestNewClientResolvesMe -v`
Expected: FAIL — `undefined: NewClient` (the `auth` import already resolves; only `gitea` symbols are missing).

- [ ] **Step 3: Write the implementation**

Create `internal/gitea/client.go`:
```go
// Package gitea is tea-dash's Gitea transport: a thin wrapper over the
// code.gitea.io/sdk/gitea SDK plus a raw HTTP escape hatch for endpoints the
// typed SDK cannot express (notably the me-scoped cross-repo issue search).
package gitea

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"

	sdk "code.gitea.io/sdk/gitea"

	"github.com/gbarany/tea-dash/internal/auth"
)

// Client wraps the SDK client with the resolved identity and a shared HTTP
// client used both by the SDK and by the raw escape hatch.
type Client struct {
	sdk        *sdk.Client
	baseURL    string
	token      string
	httpClient *http.Client
	me         string
}

// NewClient builds a Gitea client from resolved auth, negotiating TLS,
// pinning the shared HTTP client, and caching the current user's login.
func NewClient(ctx context.Context, cfg auth.Config) (*Client, error) {
	tlsCfg := &tls.Config{}
	switch {
	case cfg.Insecure:
		tlsCfg.InsecureSkipVerify = true
	case cfg.CACertPath != "":
		pem, err := os.ReadFile(cfg.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("reading CA cert %s: %w", cfg.CACertPath, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certificates found in %s", cfg.CACertPath)
		}
		tlsCfg.RootCAs = pool
	}
	hc := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}

	client, err := sdk.NewClient(cfg.URL,
		sdk.SetToken(cfg.Token),
		sdk.SetContext(ctx),
		sdk.SetHTTPClient(hc),
	)
	if err != nil {
		return nil, err
	}

	me, _, err := client.GetMyUserInfo()
	if err != nil {
		return nil, fmt.Errorf("resolving current user: %w", err)
	}

	return &Client{
		sdk:        client,
		baseURL:    cfg.URL,
		token:      cfg.Token,
		httpClient: hc,
		me:         me.UserName,
	}, nil
}

// Me returns the authenticated user's login.
func (c *Client) Me() string { return c.me }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/gitea/ -run TestNewClientResolvesMe -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/gitea/client.go internal/gitea/client_test.go
git commit -m "feat(gitea): add SDK client wrapper with identity resolution"
```

---

## Task 5: Me-scoped PR search (`internal/gitea`)

The default board. Gitea's `/repos/issues/search` scopes to the authed user via the **boolean** `created=true` param (not a username), which the typed SDK cannot emit — so this goes through the raw helper.

**Files:**
- Create: `internal/gitea/search.go`
- Test: `internal/gitea/search_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/gitea/search_test.go`:
```go
package gitea

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gbarany/tea-dash/internal/auth"
)

const searchJSON = `[
  {"number":7,"title":"Fix thing","state":"open",
   "html_url":"https://x/acme/widgets/pulls/7",
   "user":{"login":"me","full_name":"Me"},
   "labels":[{"name":"bug","color":"ff0000"}],
   "created_at":"2026-06-01T00:00:00Z","updated_at":"2026-06-02T00:00:00Z",
   "repository":{"full_name":"acme/widgets"},
   "pull_request":{"merged":false}}
]`

func TestSearchMyPulls(t *testing.T) {
	var gotQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"version":"1.22.0"}`)
	})
	mux.HandleFunc("/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":1,"login":"me"}`)
	})
	mux.HandleFunc("/api/v1/repos/issues/search", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		fmt.Fprint(w, searchJSON)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := NewClient(context.Background(), auth.Config{URL: srv.URL, Token: "t"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	prs, err := c.SearchMyPulls(context.Background(), "open")
	if err != nil {
		t.Fatalf("SearchMyPulls: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("got %d PRs, want 1", len(prs))
	}
	pr := prs[0]
	if pr.Number != 7 || pr.Title != "Fix thing" ||
		pr.RepoNameWithOwner != "acme/widgets" || pr.Author != "me" || pr.State != "open" {
		t.Fatalf("mapped PR = %+v", pr)
	}
	if len(pr.Labels) != 1 || pr.Labels[0].Name != "bug" {
		t.Fatalf("labels = %+v", pr.Labels)
	}

	// The me-scope MUST be the boolean `created=true` on the search endpoint,
	// and MUST NOT be the per-repo `created_by` param (which search ignores).
	// This is the C1 regression guard.
	for _, want := range []string{"type=pulls", "created=true", "state=open"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query %q missing %q", gotQuery, want)
		}
	}
	if strings.Contains(gotQuery, "created_by") {
		t.Fatalf("query %q must not use created_by on the search endpoint", gotQuery)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/gitea/ -run TestSearchMyPulls -v`
Expected: FAIL — `undefined: (*Client).SearchMyPulls`.

- [ ] **Step 3: Write the implementation**

Create `internal/gitea/search.go`:
```go
package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gbarany/tea-dash/internal/data"
)

// searchIssue is a tolerant decode of a row from GET /repos/issues/search.
// Unknown fields are ignored. Pull requests carry a non-nil "pull_request".
type searchIssue struct {
	Number  int64  `json:"number"`
	Title   string `json:"title"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
	User    *struct {
		Login    string `json:"login"`
		FullName string `json:"full_name"`
	} `json:"user"`
	Labels []struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	} `json:"labels"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Repository *struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	PullRequest *struct {
		Merged bool `json:"merged"`
		Draft  bool `json:"draft"`
	} `json:"pull_request"`
}

// SearchMyPulls returns the authenticated user's pull requests (authored by
// them) across all accessible repos, in the given state ("open"/"closed"/"all").
// It uses the cross-repo search endpoint with the me-scoping boolean created=true.
func (c *Client) SearchMyPulls(ctx context.Context, state string) ([]data.PullRequest, error) {
	if state == "" {
		state = "open"
	}
	q := url.Values{}
	q.Set("type", "pulls")
	q.Set("created", "true")
	q.Set("state", state)
	q.Set("limit", "50")

	var rows []searchIssue
	if err := c.rawGet(ctx, "/repos/issues/search?"+q.Encode(), &rows); err != nil {
		return nil, err
	}

	prs := make([]data.PullRequest, 0, len(rows))
	for _, it := range rows {
		prs = append(prs, mapSearchIssue(it))
	}
	return prs, nil
}

func mapSearchIssue(it searchIssue) data.PullRequest {
	pr := data.PullRequest{
		Number:    it.Number,
		Title:     it.Title,
		State:     it.State,
		HTMLURL:   it.HTMLURL,
		CreatedAt: it.CreatedAt,
		UpdatedAt: it.UpdatedAt,
	}
	if it.User != nil {
		pr.Author = it.User.Login
	}
	if it.Repository != nil {
		pr.RepoNameWithOwner = it.Repository.FullName
	}
	if it.PullRequest != nil {
		pr.Draft = it.PullRequest.Draft
		if it.PullRequest.Merged {
			pr.State = "merged"
		}
	}
	for _, l := range it.Labels {
		pr.Labels = append(pr.Labels, data.Label{Name: l.Name, Color: l.Color})
	}
	return pr
}

// rawGet issues an authenticated GET against {baseURL}/api/v1{path} using the
// shared HTTP client and token, decoding the JSON body into out.
func (c *Client) rawGet(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gitea GET %s: %s: %s", path, resp.Status, string(body))
	}
	return json.Unmarshal(body, out)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/gitea/ -v`
Expected: PASS (both `TestNewClientResolvesMe` and `TestSearchMyPulls`).

- [ ] **Step 5: Commit**

```bash
git add internal/gitea/search.go internal/gitea/search_test.go
git commit -m "feat(gitea): add me-scoped cross-repo PR search via raw client"
```

---

## Task 6: Config `instance:` block (`internal/config`)

Extend the existing `yaml.v3` config with an optional `instance:` block (koanf migration is deferred to M1). The `repos:` field remains parsed but unused in M0.

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:
```go
func TestUnmarshalInstance(t *testing.T) {
	const y = `
instance:
  login: work
  url: https://git.example.com
  token: abc
  insecureSkipVerify: true
  caCert: /etc/ssl/corp.pem
`
	var c Config
	if err := yaml.Unmarshal([]byte(y), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.Instance.Login != "work" || c.Instance.URL != "https://git.example.com" ||
		c.Instance.Token != "abc" || !c.Instance.Insecure || c.Instance.CACert != "/etc/ssl/corp.pem" {
		t.Fatalf("instance = %+v", c.Instance)
	}
}
```

Change the top of `internal/config/config_test.go` from `import "testing"` to a block:
```go
import (
	"testing"

	"gopkg.in/yaml.v3"
)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestUnmarshalInstance -v`
Expected: FAIL — `c.Instance undefined (type Config has no field or method Instance)`.

- [ ] **Step 3: Write the implementation**

In `internal/config/config.go`, replace the `Config` struct definition and add the `Instance` type:
```go
// Config is the user configuration for tea-dash.
type Config struct {
	// Instance overrides / selects the Gitea login (else tea's config is reused).
	Instance Instance `yaml:"instance"`
	// Login is a deprecated alias for Instance.Login (tea login profile name).
	Login string `yaml:"login"`
	// Repos lists repositories to watch. Unused in M0; per-repo sections
	// return in M1.
	Repos []string `yaml:"repos"`
}

// Instance selects and overrides the Gitea connection.
type Instance struct {
	Login    string `yaml:"login"`              // pick a named tea login
	URL      string `yaml:"url"`                // override instance URL
	Token    string `yaml:"token"`              // override token
	Insecure bool   `yaml:"insecureSkipVerify"` // disable TLS verification
	CACert   string `yaml:"caCert"`             // path to a private CA bundle
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS (existing config tests still pass).

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add instance block for Gitea connection overrides"
```

---

## Task 7: Pivot the UI onto the Gitea client (`internal/ui`)

Swap `teacli` for `gitea` + `data`: the model holds a `*gitea.Client`, fetches via `SearchMyPulls`, and renders `data.PullRequest` rows. Also removes the now-dead `warnStyle`.

**Files:**
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/styles.go`
- Test: `internal/ui/app_test.go`

- [ ] **Step 1: Update the test fixtures first (they define the new shape)**

Replace the whole file `internal/ui/app_test.go` with:
```go
package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
)

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestModelRendersLoadedPulls(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = update(t, m, pullsLoadedMsg{items: []data.PullRequest{{
		Number:            128,
		Title:             "Add wiki CLI",
		RepoNameWithOwner: "gitea/tea",
		Author:            "lunny",
		State:             "open",
		UpdatedAt:         time.Now().Add(-2 * time.Hour),
	}}})

	view := m.View().Content
	for _, want := range []string{"#128", "Add wiki CLI", "gitea/tea", "@lunny", "1 pull requests"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view is missing %q\n---\n%s", want, view)
		}
	}
}

func TestModelRendersError(t *testing.T) {
	m := New(&config.Config{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = update(t, m, errMsg{err: errors.New("boom")})

	view := m.View().Content
	if !strings.Contains(view, "Error") || !strings.Contains(view, "boom") {
		t.Fatalf("expected an error view, got:\n%s", view)
	}
}

func TestQuitKeyStopsProgram(t *testing.T) {
	m := New(&config.Config{}, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("expected a quit command, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestModelRendersLoadedPulls -v`
Expected: FAIL — build errors (`New` takes 1 arg, `pullsLoadedMsg.items` is `[]pullItem`, `data` imported but unused in app.go).

- [ ] **Step 3: Rewrite `internal/ui/app.go`**

Replace the whole file `internal/ui/app.go` with (note: **no `"strings"` import** — it is not used):
```go
// Package ui contains the Bubble Tea models that make up the tea-dash TUI.
package ui

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/gitea"
)

const loadTimeout = 30 * time.Second

type status int

const (
	statusLoading status = iota
	statusReady
	statusError
)

// Messages emitted by background commands.
type (
	pullsLoadedMsg struct{ items []data.PullRequest }
	errMsg         struct{ err error }
)

// Model is the root tea-dash model: a table of the current user's pull requests.
type Model struct {
	cfg    *config.Config
	client *gitea.Client
	keys   keyMap

	table   table.Model
	spinner spinner.Model
	items   []data.PullRequest

	status  status
	loadErr error
	width   int
	height  int
}

// New builds the root model. client may be nil in tests that drive Update
// directly (loadPulls is the only consumer of the client).
func New(cfg *config.Config, client *gitea.Client) Model {
	return Model{
		cfg:     cfg,
		client:  client,
		keys:    defaultKeyMap(),
		spinner: spinner.New(spinner.WithStyle(spinnerStyle)),
		status:  statusLoading,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.loadPulls())
}

// loadPulls fetches the authenticated user's open pull requests across all
// accessible repositories via the me-scoped search endpoint.
func (m Model) loadPulls() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
		defer cancel()

		prs, err := client.SearchMyPulls(ctx, "open")
		if err != nil {
			return errMsg{err}
		}
		return pullsLoadedMsg{items: prs}
	}
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case spinner.TickMsg:
		if m.status == statusLoading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case pullsLoadedMsg:
		m.items = msg.items
		m.status = statusReady
		m.rebuildTable()
		return m, nil

	case errMsg:
		m.status = statusError
		m.loadErr = msg.err
		return m, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Refresh):
			if m.status != statusLoading {
				m.status = statusLoading
				return m, tea.Batch(m.spinner.Tick, m.loadPulls())
			}
			return m, nil
		case key.Matches(msg, m.keys.Open):
			return m, m.openSelected()
		}
	}

	if m.status == statusReady {
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) openSelected() tea.Cmd {
	if m.status != statusReady || len(m.items) == 0 {
		return nil
	}
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.items) {
		return nil
	}
	url := m.items[idx].HTMLURL
	return func() tea.Msg {
		_ = openURL(url)
		return nil
	}
}

func (m *Model) rebuildTable() {
	rows := make([]table.Row, len(m.items))
	for i, pr := range m.items {
		author := ""
		if pr.Author != "" {
			author = "@" + pr.Author
		}
		rows[i] = table.Row{
			fmt.Sprintf("#%d", pr.Number),
			pr.Title,
			pr.RepoNameWithOwner,
			author,
			prState(pr),
			humanizeTime(pr.UpdatedAt),
		}
	}
	t := table.New(
		table.WithColumns(m.columns()),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	t.SetStyles(tableStyles())
	m.table = t
	m.layout()
}

// layout resizes the table to the current terminal dimensions.
func (m *Model) layout() {
	if m.status != statusReady || m.width == 0 || m.height == 0 {
		return
	}
	tableHeight := m.height - 6 // title, blank, status, help + padding
	if tableHeight < 3 {
		tableHeight = 3
	}
	m.table.SetHeight(tableHeight)
	m.table.SetWidth(m.width - 4)
	m.table.SetColumns(m.columns())
}

func (m Model) columns() []table.Column {
	const (
		numW     = 6
		repoW    = 22
		authorW  = 16
		stateW   = 8
		updatedW = 10
	)
	total := m.width - 4
	titleW := total - (numW + repoW + authorW + stateW + updatedW) - 6
	if titleW < 20 {
		titleW = 20
	}
	return []table.Column{
		{Title: "#", Width: numW},
		{Title: "Title", Width: titleW},
		{Title: "Repo", Width: repoW},
		{Title: "Author", Width: authorW},
		{Title: "State", Width: stateW},
		{Title: "Updated", Width: updatedW},
	}
}

// View implements tea.Model.
func (m Model) View() tea.View {
	title := titleStyle.Render("tea-dash") + dimStyle.Render("  my pull requests")

	var body string
	switch m.status {
	case statusLoading:
		body = fmt.Sprintf("\n  %s Loading pull requests…", m.spinner.View())
	case statusError:
		body = "\n" + errorStyle.Render("  Error: "+m.loadErr.Error()) + "\n\n" +
			dimStyle.Render("  Check your Gitea login (run `tea login add`) and network.")
	case statusReady:
		if len(m.items) == 0 {
			body = "\n" + m.emptyState()
		} else {
			body = m.table.View()
		}
	}

	help := helpStyle.Render("↑/↓ move · r refresh · o/enter open in browser · q quit")

	content := appStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, body, m.statusLine(), help))
	return tea.View{Content: content, AltScreen: true}
}

func (m Model) statusLine() string {
	if m.status != statusReady {
		return ""
	}
	return dimStyle.Render(fmt.Sprintf("%d pull requests", len(m.items)))
}

func (m Model) emptyState() string {
	return "  No open pull requests authored by you.\n\n" +
		dimStyle.Render("  This board shows PRs you created across all repos on your Gitea instance.")
}

func prState(pr data.PullRequest) string {
	if pr.Draft {
		return "draft"
	}
	return pr.State
}

func humanizeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
```

- [ ] **Step 4: Remove the now-dead `warnStyle`**

In `internal/ui/styles.go`, delete the `warnStyle` line from the `var (...)` block (its only consumer, the old warnings status line, is gone). The remaining vars (`appStyle`, `titleStyle`, `dimStyle`, `helpStyle`, `errorStyle`, `spinnerStyle`) are all still used. After the edit the block reads:
```go
var (
	appStyle     = lipgloss.NewStyle().Padding(1, 2)
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00ADD8"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpStyle    = lipgloss.NewStyle().MarginTop(1).Foreground(lipgloss.Color("241"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8"))
)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS (all three UI tests).

- [ ] **Step 6: Commit**

```bash
git add internal/ui/app.go internal/ui/app_test.go internal/ui/styles.go
git commit -m "feat(ui): render my-PRs board from the Gitea client"
```

---

## Task 8: Wire `main.go` to auth + the Gitea client

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Rewrite `main.go`**

Replace the whole file `main.go` with:
```go
// Command tea-dash is a terminal dashboard for Gitea.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/gbarany/tea-dash/internal/auth"
	"github.com/gbarany/tea-dash/internal/build"
	"github.com/gbarany/tea-dash/internal/config"
	"github.com/gbarany/tea-dash/internal/gitea"
	"github.com/gbarany/tea-dash/internal/ui"
)

func main() {
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-v", "--version", "version":
			fmt.Println("tea-dash", build.String())
			return
		case "-h", "--help", "help":
			fmt.Println(usage)
			return
		}
	}

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tea-dash:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ov := auth.Overrides{
		Login:      firstNonEmpty(cfg.Instance.Login, cfg.Login),
		URL:        cfg.Instance.URL,
		Token:      cfg.Instance.Token,
		Insecure:   cfg.Instance.Insecure,
		CACertPath: expandHome(cfg.Instance.CACert),
	}
	authCfg, err := auth.Resolve(ov)
	if err != nil {
		return fmt.Errorf("authentication: %w", err)
	}

	ctx := context.Background()
	client, err := gitea.NewClient(ctx, authCfg)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", authCfg.URL, err)
	}

	p := tea.NewProgram(ui.New(cfg, client))
	_, err = p.Run()
	return err
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// expandHome expands a leading ~ to the user's home directory.
func expandHome(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

const usage = `tea-dash — a terminal dashboard for Gitea

Usage:
  tea-dash            start the dashboard (your open pull requests)
  tea-dash --version  print version information
  tea-dash --help     show this help

tea-dash reuses your ` + "`tea`" + ` login (run ` + "`tea login add`" + ` once), or set
instance.url + instance.token in ~/.config/tea-dash/config.yml.`
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./...`
Expected: no output, exit 0. (`internal/teacli` is now unused by other packages but still compiles; it is removed in Task 9.)

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "feat: wire auth resolution + Gitea client into startup"
```

---

## Task 9: Remove `teacli`, refresh docs, and finalize

**Files:**
- Delete: `internal/teacli/client.go`, `internal/teacli/types.go`, `internal/teacli/client_test.go`
- Modify: `README.md`

- [ ] **Step 1: Delete the package**

Run:
```bash
git rm internal/teacli/client.go internal/teacli/types.go internal/teacli/client_test.go
```
Expected: three files removed.

- [ ] **Step 2: Refresh the README**

In `README.md`, update the "How it works" section (and the Requirements list) so it no longer claims tea-dash shells out to `tea api`. Replace the "How it works" paragraph with wording to the effect of:

> tea-dash talks to Gitea directly via the official Go SDK (`code.gitea.io/sdk/gitea`). It reuses your existing `tea` login (`~/Library/Application Support/tea/config.yml` on macOS / `~/.config/tea/config.yml` on Linux) for the instance URL and token, so you get auth for free without tea-dash handling credentials itself — but `tea` is not run at runtime.

Update the Requirements list to note that `tea` is only needed **once** to create a login (`tea login add`), not as a runtime dependency. Keep the rest of the README intact.

- [ ] **Step 3: Tidy and run the full check suite**

Run:
```bash
go mod tidy
make check
```
Expected: `make check` (fmt-check + vet + test) passes — all tests green, gofmt-clean, `go vet` clean. If `gofmt` flags anything, run `make fmt` and re-run.

- [ ] **Step 4: Verify the binary builds and prints version**

Run:
```bash
make build && ./bin/tea-dash --version
```
Expected: builds, prints `tea-dash <version> (commit …, built …)`.

- [ ] **Step 5: (Optional) Smoke test against your real instance**

If you have a `tea` login configured:
```bash
./bin/tea-dash
```
Expected: after a brief spinner, a table of your open pull requests across your repos (or the "No open pull requests authored by you." empty state). `q` quits. This is the M0 exit criterion in action.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: remove teacli; tea-dash now talks to Gitea via the SDK"
```

---

## Done — M0 Exit Criteria

- [ ] `tea-dash` launches against a real Gitea/Forgejo instance.
- [ ] Auth is read from `tea`'s config with zero prompts (or from `instance:` / env).
- [ ] The board shows **the authenticated user's** open PRs across all repos, me-scoped via the raw `/repos/issues/search?type=pulls&created=true` query (verified by the C1 guard test).
- [ ] `r` refreshes, `o`/`enter` opens in browser, `q` quits.
- [ ] `internal/teacli` is gone; no `tea api` subprocess remains.
- [ ] `make check` passes.

**Next:** M1 (core architecture — `ProgramContext`/`Section`/`BaseModel`, Issues, structured filters, preview, comment, merge, CI column, external diff) gets its own plan.
