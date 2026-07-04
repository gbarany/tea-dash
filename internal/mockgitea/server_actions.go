package mockgitea

import (
	"io"
	"net/http"
	"net/url"
	"sort"
)

// actionRoutes registers the Actions runs/jobs/logs/control endpoints
// internal/gitea/actions.go consumes. That file talks to these purely
// through the raw escape hatch (rawGet/rawGetBytes/rawPost), not the SDK, so
// every shape below is cross-checked against actions.go's rawActionRun/
// rawActionJob tolerant-decode structs and actions_test.go's fixtures rather
// than an SDK type.
func (s *Server) actionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/repos/{owner}/{repo}/actions/runs", s.handleListActionRuns)
	mux.HandleFunc("GET /api/v1/repos/{owner}/{repo}/actions/runs/{id}", s.handleGetActionRun)
	mux.HandleFunc("GET /api/v1/repos/{owner}/{repo}/actions/runs/{id}/jobs", s.handleListActionJobs)
	mux.HandleFunc("GET /api/v1/repos/{owner}/{repo}/actions/jobs/{jobID}/logs", s.handleActionJobLogs)
	mux.HandleFunc("POST /api/v1/repos/{owner}/{repo}/actions/runs/{id}/rerun", s.handleRerunActionRun)
	mux.HandleFunc("POST /api/v1/repos/{owner}/{repo}/actions/runs/{id}/cancel", s.handleCancelActionRun)
}

// handleListActionRuns serves GET .../actions/runs. Unlike the search/repo
// list endpoints, this can't go through writeList/respondList: ListActionRuns
// never reads the X-Total-Count header, it decodes total exclusively from the
// response body's "total_count" field inside a {"workflow_runs": [...]}
// envelope (actions.go's decodeActionRuns/actionRunsEnvelope) — so the
// handler builds and writes that envelope object directly.
func (s *Server) handleListActionRuns(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	q := r.URL.Query()
	limit, page := paginationParams(q)

	s.store.WithLock(func() {
		if s.store.repoByFullNameLocked(full) == nil {
			notFound(w, r)
			return
		}
		matched := filterActionRuns(s.store.runsLocked(full), q)
		rows, total := pageRows(matched, limit, page, actionRunRow)
		writeJSON(w, map[string]any{
			"total_count":   total,
			"workflow_runs": rows,
		})
	})
}

// handleGetActionRun serves GET .../actions/runs/{id}: a single run object,
// decoded bare (no envelope) by GetActionRun.
func (s *Server) handleGetActionRun(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	id, ok := parsePathInt64(r.PathValue("id"))
	if !ok {
		notFound(w, r)
		return
	}
	respondOr404(s, w, r, func() *map[string]any {
		run := s.store.runByIDLocked(full, id)
		if run == nil {
			return nil
		}
		row := actionRunRow(run)
		return &row
	})
}

// handleListActionJobs serves GET .../actions/runs/{id}/jobs: a
// {"jobs": [...]} envelope (actions.go's decodeActionJobs/
// actionJobsEnvelope). 404s only when the RUN is unknown — a run with zero
// jobs still returns 200 with an empty jobs array.
func (s *Server) handleListActionJobs(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	id, ok := parsePathInt64(r.PathValue("id"))
	if !ok {
		notFound(w, r)
		return
	}
	respondOr404(s, w, r, func() *map[string]any {
		run := s.store.runByIDLocked(full, id)
		if run == nil {
			return nil
		}
		rows := make([]map[string]any, 0, len(run.Jobs))
		for _, j := range run.Jobs {
			rows = append(rows, actionJobRow(j, run.ID))
		}
		env := map[string]any{
			"total_count": len(rows),
			"jobs":        rows,
		}
		return &env
	})
}

// handleActionJobLogs serves GET .../actions/jobs/{jobID}/logs as text/plain.
// The path is repo-scoped but not run-scoped (GetActionJobLogs addresses a
// job by ID alone within a repo), so the lookup scans every run in the repo
// for a matching job — see store.go's jobByIDLocked.
func (s *Server) handleActionJobLogs(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	jobID, ok := parsePathInt64(r.PathValue("jobID"))
	if !ok {
		notFound(w, r)
		return
	}
	var logs string
	var found bool
	s.store.WithLock(func() {
		if job := s.store.jobByIDLocked(full, jobID); job != nil {
			logs, found = job.Logs, true
		}
	})
	if !found {
		notFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, logs)
}

// handleRerunActionRun and handleCancelActionRun serve the run-control
// endpoints. RerunActionRun/CancelActionRun (rawPost) accept any 2xx and
// discard the body, so there is no re-fetch-for-response step here — just
// the same check-then-mutate shape handleMarkNotification documents, minus
// its third (response-building) section.
func (s *Server) handleRerunActionRun(w http.ResponseWriter, r *http.Request) {
	s.handleActionRunControl(w, r, "running")
}

func (s *Server) handleCancelActionRun(w http.ResponseWriter, r *http.Request) {
	s.handleActionRunControl(w, r, "cancelled")
}

func (s *Server) handleActionRunControl(w http.ResponseWriter, r *http.Request, status string) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	id, ok := parsePathInt64(r.PathValue("id"))
	if !ok {
		notFound(w, r)
		return
	}
	var exists bool
	s.store.WithLock(func() {
		exists = s.store.runByIDLocked(full, id) != nil
	})
	if !exists {
		notFound(w, r)
		return
	}
	// SetRunStatus self-locks; call it outside WithLock (non-reentrant mutex).
	if err := s.store.SetRunStatus(full, id, status); err != nil {
		// Shouldn't happen: the existence check just above confirmed the run,
		// and nothing in this package ever removes a run afterward.
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// filterActionRuns returns the runs matching q's branch/head_sha/event/
// status/actor filters (buildActionRunParams in actions.go), sorted by
// Updated descending like the other list endpoints. branch and head_sha are
// the two the watch-checks flow depends on; the rest are supported for the
// same reason the search/repo-list filters are — matching the real query
// contract, not just the minimum the given tests exercise.
func filterActionRuns(runs []*ActionRun, q url.Values) []*ActionRun {
	branch := q.Get("branch")
	headSHA := q.Get("head_sha")
	event := q.Get("event")
	status := q.Get("status")
	actor := q.Get("actor")

	out := make([]*ActionRun, 0, len(runs))
	for _, run := range runs {
		switch {
		case branch != "" && run.HeadBranch != branch:
		case headSHA != "" && run.HeadSHA != headSHA:
		case event != "" && run.Event != event:
		case status != "" && run.Status != status:
		case actor != "" && (run.Actor == nil || run.Actor.Login != actor):
		default:
			out = append(out, run)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out
}

// actionRunRow marshals an ActionRun into the shape actions.go's rawActionRun
// tolerant decode reads: id/display_title/event/status/head_branch/head_sha/
// html_url/created_at/updated_at, workflow name under both "name" and
// "workflow_name" (mapActionRun falls back from the latter to the former),
// nested actor.login, and a run_started_at synthesized from Created (the
// store tracks no separate "run actually started" timestamp).
func actionRunRow(run *ActionRun) map[string]any {
	return map[string]any{
		"id":             run.ID,
		"name":           run.WorkflowName,
		"workflow_name":  run.WorkflowName,
		"display_title":  run.DisplayTitle,
		"event":          run.Event,
		"status":         run.Status,
		"head_branch":    run.HeadBranch,
		"head_sha":       run.HeadSHA,
		"html_url":       run.HTMLURL,
		"created_at":     run.Created,
		"updated_at":     run.Updated,
		"run_started_at": run.Created,
		"actor":          run.Actor,
		"repository":     map[string]any{"full_name": run.RepoFullName},
	}
}

// actionJobRow marshals an ActionJob into the shape actions.go's
// rawActionJob tolerant decode reads: id/name/status/started_at/
// completed_at, plus run_id (not tracked on the store type itself, so it's
// threaded through from the parent run at call sites).
func actionJobRow(j *ActionJob, runID int64) map[string]any {
	return map[string]any{
		"id":           j.ID,
		"run_id":       runID,
		"name":         j.Name,
		"status":       j.Status,
		"started_at":   j.Started,
		"completed_at": j.Stopped,
	}
}
