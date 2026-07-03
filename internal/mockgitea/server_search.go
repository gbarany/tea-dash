package mockgitea

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// searchRoutes registers the cross-repo search and repo-scoped issue/pull
// list endpoints that internal/gitea's Search*/ListRepo* client methods
// consume (see internal/gitea/search.go's buildSearchParamsPage and
// repoIssueListOptions for the exact query-param contract these handlers
// honor).
func (s *Server) searchRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/repos/issues/search", s.handleSearch)
	mux.HandleFunc("GET /api/v1/repos/{owner}/{repo}/issues", s.handleRepoIssues)
}

// handleSearch serves the cross-repo /repos/issues/search endpoint. Me-scope
// is expressed as the boolean flags created/assigned/review_requested (the C1
// guard in internal/gitea/search.go: this endpoint has no per-login author
// filter), so plain matchPull/matchIssue happily fall through their
// created_by/assigned_by checks here — those params are never sent cross-repo.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, page := paginationParams(q)
	respondList(s, w, func() (rows []map[string]any, total int) {
		me := s.store.meLocked().Login
		if q.Get("type") == "issues" {
			return pageRows(filterIssues(s.store.allIssuesLocked(), q, me), limit, page, issueSearchRow)
		}
		return pageRows(filterPulls(s.store.allPullsLocked(), q, me), limit, page, pullSearchRow)
	})
}

// handleRepoIssues serves the repo-scoped GET /repos/{owner}/{repo}/issues
// endpoint used by ListRepoPullsPage/ListRepoIssuesPage. It doesn't go through
// respondList directly because it must 404 for an unknown repo instead of
// writing an (empty) list — the whole check-then-build-then-write sequence
// still runs as a single WithLock critical section, so no store access here
// happens outside the lock.
func (s *Server) handleRepoIssues(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	q := r.URL.Query()
	limit, page := paginationParams(q)

	s.store.WithLock(func() {
		if s.store.repoByFullNameLocked(full) == nil {
			notFound(w, r)
			return
		}
		me := s.store.meLocked().Login
		var rows []map[string]any
		var total int
		if q.Get("type") == "pulls" {
			rows, total = pageRows(filterPulls(s.store.pullsLocked(full), q, me), limit, page, pullSearchRow)
		} else {
			rows, total = pageRows(filterIssues(s.store.issuesLocked(full), q, me), limit, page, issueSearchRow)
		}
		writeList(w, total, rows)
	})
}

// filterPulls returns the pulls matching q, sorted by Updated descending
// (tea-dash merges multi-repo pages by updated time, newest first).
func filterPulls(pulls []*Pull, q url.Values, me string) []*Pull {
	var out []*Pull
	for _, p := range pulls {
		if matchPull(p, q, me) {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out
}

// filterIssues is filterPulls for issues.
func filterIssues(issues []*Issue, q url.Values, me string) []*Issue {
	var out []*Issue
	for _, i := range issues {
		if matchIssue(i, q, me) {
			out = append(out, i)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out
}

// matchPull reports whether p satisfies every filter present in q. It honors
// both the cross-repo search endpoint's me-scope booleans (created=true,
// assigned=true, review_requested=true) and the repo-scoped endpoint's
// per-login created_by/assigned_by — a given request only ever sends one set,
// so checking both here lets one matcher serve both handlers.
func matchPull(p *Pull, q url.Values, me string) bool {
	if !matchState(p.State, q.Get("state")) {
		return false
	}
	if q.Get("created") == "true" && !isAuthor(p.Author, me) {
		return false
	}
	if createdBy := q.Get("created_by"); createdBy != "" && !isAuthor(p.Author, createdBy) {
		return false
	}
	if q.Get("assigned") == "true" && !hasLogin(p.Assignees, me) {
		return false
	}
	if assignedBy := q.Get("assigned_by"); assignedBy != "" && !hasLogin(p.Assignees, assignedBy) {
		return false
	}
	if q.Get("review_requested") == "true" && !hasLogin(p.Reviewers, me) {
		return false
	}
	if !matchQuery(p.Title, q.Get("q")) {
		return false
	}
	if !matchLabels(p.Labels, q.Get("labels")) {
		return false
	}
	if !matchMilestone(p.Milestone, q.Get("milestones")) {
		return false
	}
	return true
}

// matchIssue is matchPull for issues. Issues have no requested-reviewers
// concept, so there is no review_requested check.
func matchIssue(i *Issue, q url.Values, me string) bool {
	if !matchState(i.State, q.Get("state")) {
		return false
	}
	if q.Get("created") == "true" && !isAuthor(i.Author, me) {
		return false
	}
	if createdBy := q.Get("created_by"); createdBy != "" && !isAuthor(i.Author, createdBy) {
		return false
	}
	if q.Get("assigned") == "true" && !hasLogin(i.Assignees, me) {
		return false
	}
	if assignedBy := q.Get("assigned_by"); assignedBy != "" && !hasLogin(i.Assignees, assignedBy) {
		return false
	}
	if !matchQuery(i.Title, q.Get("q")) {
		return false
	}
	if !matchLabels(i.Labels, q.Get("labels")) {
		return false
	}
	if !matchMilestone(i.Milestone, q.Get("milestones")) {
		return false
	}
	return true
}

// matchState reports whether state satisfies the "state" filter. An empty or
// "all" filter matches every state; the default applied upstream by
// config.PrIssueFilter.WithDefaults is "open".
func matchState(state, want string) bool {
	return want == "" || want == "all" || state == want
}

// matchQuery reports whether title contains q, case-insensitively. An empty q
// matches everything.
func matchQuery(title, q string) bool {
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(title), strings.ToLower(q))
}

// matchLabels reports whether labels contains every name in the comma-joined
// csv list (an AND match, matching the real search endpoint's labels param).
// An empty csv matches everything.
func matchLabels(labels []*Label, csv string) bool {
	if csv == "" {
		return true
	}
	have := make(map[string]bool, len(labels))
	for _, l := range labels {
		if l != nil {
			have[l.Name] = true
		}
	}
	for _, name := range strings.Split(csv, ",") {
		if !have[strings.TrimSpace(name)] {
			return false
		}
	}
	return true
}

// matchMilestone reports whether m's title matches the milestones filter. An
// empty filter matches everything.
func matchMilestone(m *Milestone, title string) bool {
	if title == "" {
		return true
	}
	return m != nil && m.Title == title
}

// isAuthor reports whether u is the user with the given login.
func isAuthor(u *User, login string) bool {
	return u != nil && login != "" && u.Login == login
}

// hasLogin reports whether login appears among users.
func hasLogin(users []*User, login string) bool {
	if login == "" {
		return false
	}
	for _, u := range users {
		if u != nil && u.Login == login {
			return true
		}
	}
	return false
}

// paginationParams parses the "limit"/"page" query params, defaulting a
// missing or malformed value to 0 (paginate treats <= 0 as "use the default").
func paginationParams(q url.Values) (limit, page int) {
	limit, _ = strconv.Atoi(q.Get("limit"))
	page, _ = strconv.Atoi(q.Get("page"))
	return limit, page
}

// paginate slices rows to one page. limit <= 0 defaults to 50 (matching
// internal/gitea's own default); page <= 0 defaults to 1. lo < 0 is also
// treated as out of range — a page/limit combination from an untrusted query
// string can overflow int multiplication into a negative offset.
func paginate[T any](rows []T, limit, page int) []T {
	if limit <= 0 {
		limit = 50
	}
	if page <= 0 {
		page = 1
	}
	lo := (page - 1) * limit
	if lo < 0 || lo >= len(rows) {
		return nil
	}
	hi := min(lo+limit, len(rows))
	return rows[lo:hi]
}

// pageRows pages matched down to one page and marshals each row via row,
// returning the pre-pagination total alongside it. rows is initialized to a
// non-nil empty slice so an empty page encodes as JSON "[]", matching a real
// Gitea server, instead of "null".
func pageRows[T any](matched []T, limit, page int, row func(T) map[string]any) (rows []map[string]any, total int) {
	rows = []map[string]any{}
	total = len(matched)
	for _, item := range paginate(matched, limit, page) {
		rows = append(rows, row(item))
	}
	return rows, total
}

// pullSearchRow marshals a Pull into the row shape internal/gitea's tolerant
// searchIssue decode (and the SDK's Issue type, for the repo-scoped endpoint)
// reads: pulls carry a non-nil "pull_request", plus a "repository" block both
// endpoints include.
func pullSearchRow(p *Pull) map[string]any {
	return map[string]any{
		"id":           p.ID,
		"number":       p.Number,
		"title":        p.Title,
		"body":         p.Body,
		"state":        p.State,
		"user":         p.Author,
		"labels":       p.Labels,
		"milestone":    p.Milestone,
		"assignees":    p.Assignees,
		"comments":     p.CommentCount,
		"created_at":   p.Created,
		"updated_at":   p.Updated,
		"html_url":     p.HTMLURL,
		"pull_request": map[string]any{"merged": p.Merged, "draft": p.Draft},
		"repository":   map[string]any{"full_name": p.RepoFullName},
	}
}

// issueSearchRow is pullSearchRow for issues: same shape minus "pull_request",
// which is how internal/gitea's searchIssue decode tells issues from pulls.
func issueSearchRow(i *Issue) map[string]any {
	return map[string]any{
		"id":         i.ID,
		"number":     i.Number,
		"title":      i.Title,
		"body":       i.Body,
		"state":      i.State,
		"user":       i.Author,
		"labels":     i.Labels,
		"milestone":  i.Milestone,
		"assignees":  i.Assignees,
		"comments":   i.CommentCount,
		"created_at": i.Created,
		"updated_at": i.Updated,
		"html_url":   i.HTMLURL,
		"repository": map[string]any{"full_name": i.RepoFullName},
	}
}
