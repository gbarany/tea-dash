package mockgitea

import (
	"io"
	"net/http"
	"strconv"
	"strings"
)

// detailRoutes registers the pull/issue detail, diff, status, reviewers, and
// label/milestone-definition endpoints internal/gitea's GetPullDetail,
// GetIssueDetail, GetPullDiff, ListReviewers, and label/milestone name->id
// resolution (used by Task 7's mutation handlers) consume. Shapes are
// cross-checked against internal/gitea/detail.go's mapPullDetail/
// mapIssueDetail/mapCombinedStatus and the Gitea SDK's PullRequest/Issue/
// CombinedStatus/Status decode structs.
func (s *Server) detailRoutes(mux *http.ServeMux) {
	// A route pattern can't spell "{index}.diff": net/http's ServeMux rejects
	// any text after a wildcard's closing '}' within the same segment as a
	// malformed pattern ("bad wildcard segment (must end with '}')"), panicking
	// at registration time — so one handler on the plain {index} pattern
	// dispatches on the suffix itself.
	mux.HandleFunc("GET /api/v1/repos/{owner}/{repo}/pulls/{index}", s.handlePullOrDiff)
	mux.HandleFunc("GET /api/v1/repos/{owner}/{repo}/pulls/{index}/reviews", s.handlePullReviews)
	mux.HandleFunc("GET /api/v1/repos/{owner}/{repo}/issues/{index}", s.handleIssueDetail)
	mux.HandleFunc("GET /api/v1/repos/{owner}/{repo}/issues/{index}/comments", s.handleIssueComments)
	mux.HandleFunc("GET /api/v1/repos/{owner}/{repo}/commits/{sha}/status", s.handleCommitStatus)
	mux.HandleFunc("GET /api/v1/repos/{owner}/{repo}/reviewers", s.handleReviewers)
	mux.HandleFunc("GET /api/v1/repos/{owner}/{repo}/labels", s.handleLabels)
	mux.HandleFunc("GET /api/v1/repos/{owner}/{repo}/milestones", s.handleMilestones)
}

// handlePullOrDiff serves both GET .../pulls/{index} (detail) and
// GET .../pulls/{index}.diff (raw unified diff): the SDK requests the latter
// at that literal suffix (internal/gitea/diff.go's GetPullDiff, once the mock
// reports a server version >= 1.13.0 as it does via GET /version), and the
// suffix isn't separable into its own route pattern (see detailRoutes).
func (s *Server) handlePullOrDiff(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	raw := r.PathValue("index")
	if idxStr, ok := strings.CutSuffix(raw, ".diff"); ok {
		idx, ok := parsePathInt64(idxStr)
		if !ok {
			notFound(w, r)
			return
		}
		s.handlePullDiff(w, r, full, idx)
		return
	}
	idx, ok := parsePathInt64(raw)
	if !ok {
		notFound(w, r)
		return
	}
	respondOr404(s, w, r, func() *map[string]any {
		p := s.store.pullLocked(full, idx)
		if p == nil {
			return nil
		}
		row := pullDetailRow(p)
		return &row
	})
}

// handlePullDiff writes the pull's raw diff as text/plain, or a loud 404 if
// the pull is unknown.
func (s *Server) handlePullDiff(w http.ResponseWriter, r *http.Request, full string, idx int64) {
	var diff string
	var found bool
	s.store.WithLock(func() {
		if p := s.store.pullLocked(full, idx); p != nil {
			diff, found = p.Diff, true
		}
	})
	if !found {
		notFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, diff)
}

// handlePullReviews serves GET .../pulls/{index}/reviews. The Review type's
// JSON tags already match what internal/gitea's ListPullReviews decodes
// (id/state/body/user/submitted_at), so reviews marshal directly with no map
// builder needed.
func (s *Server) handlePullReviews(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	idx, ok := parsePathInt64(r.PathValue("index"))
	if !ok {
		notFound(w, r)
		return
	}
	respondListOr404(s, w, r, func() (rows []*Review, total int, ok bool) {
		p := s.store.pullLocked(full, idx)
		if p == nil {
			return nil, 0, false
		}
		rows = p.Reviews
		if rows == nil {
			rows = []*Review{}
		}
		return rows, len(rows), true
	})
}

// handleIssueDetail serves GET .../issues/{index}. issueSearchRow already
// produces the shape the SDK's Issue decode reads (Task 3), so it's reused
// as-is; mapIssueDetail only reads Body out of it.
func (s *Server) handleIssueDetail(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	idx, ok := parsePathInt64(r.PathValue("index"))
	if !ok {
		notFound(w, r)
		return
	}
	respondOr404(s, w, r, func() *map[string]any {
		i := s.store.issueLocked(full, idx)
		if i == nil {
			return nil
		}
		row := issueSearchRow(i)
		return &row
	})
}

// handleIssueComments serves GET .../issues/{index}/comments, which
// internal/gitea also calls for a PR's comments (Gitea treats PRs as issues
// for the comment thread) — hence checking both pullLocked and issueLocked
// for existence before 404ing.
func (s *Server) handleIssueComments(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	idx, ok := parsePathInt64(r.PathValue("index"))
	if !ok {
		notFound(w, r)
		return
	}
	s.store.WithLock(func() {
		if s.store.pullLocked(full, idx) == nil && s.store.issueLocked(full, idx) == nil {
			notFound(w, r)
			return
		}
		comments := s.store.commentsLocked(full, idx)
		if comments == nil {
			comments = []*Comment{}
		}
		writeJSON(w, comments)
	})
}

// handleCommitStatus serves GET .../commits/{sha}/status: the combined status
// for the pull in this repo whose HeadSHA matches {sha}. 404s loudly for an
// unknown repo or a sha that doesn't match any pull's head — the mock has no
// independent notion of commits to fall back on.
func (s *Server) handleCommitStatus(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	sha := r.PathValue("sha")
	respondOr404(s, w, r, func() *map[string]any {
		if sha == "" || s.store.repoByFullNameLocked(full) == nil {
			return nil
		}
		for _, p := range s.store.pullsLocked(full) {
			if p.HeadSHA == sha {
				row := combinedStatusRow(sha, p.Statuses)
				return &row
			}
		}
		return nil
	})
}

// handleReviewers serves GET .../reviewers: every registered user except the
// authenticated one.
func (s *Server) handleReviewers(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	respondRepoList(s, w, r, full, func() (rows []*User, total int) {
		rows = []*User{}
		me := s.store.meLocked()
		for _, u := range s.store.usersLocked() {
			if u != nil && (me == nil || u.Login != me.Login) {
				rows = append(rows, u)
			}
		}
		return rows, len(rows)
	})
}

// handleLabels serves GET .../labels: the repo's label definitions. Also
// backs Task 7's label name->id resolution.
func (s *Server) handleLabels(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	respondRepoList(s, w, r, full, func() (rows []*Label, total int) {
		rows = s.store.labelDefsLocked(full)
		if rows == nil {
			rows = []*Label{}
		}
		return rows, len(rows)
	})
}

// handleMilestones serves GET .../milestones: the repo's milestone
// definitions. Also backs Task 7's milestone title->id resolution.
func (s *Server) handleMilestones(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	respondRepoList(s, w, r, full, func() (rows []*Milestone, total int) {
		rows = s.store.milestoneDefsLocked(full)
		if rows == nil {
			rows = []*Milestone{}
		}
		return rows, len(rows)
	})
}

// respondRepoList is respondListOr404 for the repo-scoped list endpoints
// (reviewers/labels/milestones) that have no other way to signal "this repo
// doesn't exist" — an empty list would look identical to a real repo with
// zero reviewers/labels/milestones.
func respondRepoList[T any](s *Server, w http.ResponseWriter, r *http.Request, full string, build func() (rows []T, total int)) {
	respondListOr404(s, w, r, func() (rows []T, total int, ok bool) {
		if s.store.repoByFullNameLocked(full) == nil {
			return nil, 0, false
		}
		rows, total = build()
		return rows, total, true
	})
}

// parsePathInt64 parses a path segment as a positive base-10 int64, the shape
// every {index} path value in this file must have.
func parsePathInt64(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil
}

// pullDetailRow marshals a Pull into the shape internal/gitea's GetPullDetail
// reads off the SDK's PullRequest type: top-level draft/mergeable/merged (NOT
// nested under "pull_request" the way search rows nest them — see
// pullSearchRow), plus "base"/"head" ref objects mapPullDetail reads BaseRef/
// HeadRef/HeadSHA from.
func pullDetailRow(p *Pull) map[string]any {
	return map[string]any{
		"id":                  p.ID,
		"number":              p.Number,
		"title":               p.Title,
		"body":                p.Body,
		"state":               p.State,
		"draft":               p.Draft,
		"mergeable":           p.Mergeable,
		"merged":              p.Merged,
		"user":                p.Author,
		"labels":              p.Labels,
		"milestone":           p.Milestone,
		"assignees":           p.Assignees,
		"requested_reviewers": p.Reviewers,
		"comments":            p.CommentCount,
		"created_at":          p.Created,
		"updated_at":          p.Updated,
		"html_url":            p.HTMLURL,
		"base":                map[string]any{"ref": p.BaseRef},
		"head":                map[string]any{"ref": p.HeadRef, "sha": p.HeadSHA},
	}
}

// combinedStatusRow marshals a sha and its statuses into the shape
// internal/gitea's mapCombinedStatus reads: state/sha/total_count/statuses.
// CommitStatus's own JSON tags (status/context/target_url/description)
// already match each entry's expected shape, so statuses marshals as-is.
func combinedStatusRow(sha string, statuses []*CommitStatus) map[string]any {
	return map[string]any{
		"state":       worstStatus(statuses),
		"sha":         sha,
		"total_count": len(statuses),
		"statuses":    statuses,
	}
}

// statusSeverity ranks CommitStatus.Status values so worstStatus can pick the
// most severe one to roll up into the combined state, matching Gitea's own
// commit-status precedence: error outranks failure, which outranks pending,
// which outranks success, which outranks warning (warning is the LEAST
// severe of the five — it means "succeeded, but flagged," not "still running"
// or "broken").
var statusSeverity = map[string]int{
	"warning": 0,
	"success": 1,
	"pending": 2,
	"failure": 3,
	"error":   4,
}

// worstStatus returns the most severe status among statuses, defaulting to
// "success" for an empty list (an unknown status string ranks alongside
// "warning", the lowest severity, rather than panicking or erroring).
func worstStatus(statuses []*CommitStatus) string {
	worst := "success"
	worstRank := -1
	for _, st := range statuses {
		if st == nil {
			continue
		}
		rank := statusSeverity[st.Status]
		if rank > worstRank {
			worstRank = rank
			worst = st.Status
		}
	}
	return worst
}
