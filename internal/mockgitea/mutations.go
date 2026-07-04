package mockgitea

import (
	"encoding/json"
	"net/http"
	"strings"
)

// mutationRoutes registers the comment/state/label/milestone/assignee/merge/
// review/reviewer/subscription mutation endpoints internal/gitea/mutation.go
// consumes. Every handler follows the check-then-mutate-then-respond
// template documented on handleMarkNotification: store mutators self-lock
// and are called outside WithLock, with any response body re-fetched inside
// a fresh WithLock afterward.
func (s *Server) mutationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/repos/{owner}/{repo}/issues/{index}/comments", s.handleCreateComment)
	mux.HandleFunc("PATCH /api/v1/repos/{owner}/{repo}/issues/{index}", s.handleEditIssue)
	mux.HandleFunc("PATCH /api/v1/repos/{owner}/{repo}/pulls/{index}", s.handleEditPull)
	mux.HandleFunc("POST /api/v1/repos/{owner}/{repo}/pulls/{index}/merge", s.handleMergePull)
	mux.HandleFunc("POST /api/v1/repos/{owner}/{repo}/pulls/{index}/update", s.handleUpdatePull)
	mux.HandleFunc("POST /api/v1/repos/{owner}/{repo}/pulls/{index}/requested_reviewers", s.handleAddPullReviewers)
	mux.HandleFunc("DELETE /api/v1/repos/{owner}/{repo}/pulls/{index}/requested_reviewers", s.handleRemovePullReviewers)
	mux.HandleFunc("POST /api/v1/repos/{owner}/{repo}/pulls/{index}/reviews", s.handleCreatePullReview)
	mux.HandleFunc("POST /api/v1/repos/{owner}/{repo}/issues/{index}/labels", s.handleAddIssueLabels)
	mux.HandleFunc("DELETE /api/v1/repos/{owner}/{repo}/issues/{index}/labels/{labelID}", s.handleRemoveIssueLabel)
	mux.HandleFunc("PUT /api/v1/repos/{owner}/{repo}/issues/{index}/subscriptions/{user}", s.handleSetIssueSubscription)
	mux.HandleFunc("DELETE /api/v1/repos/{owner}/{repo}/issues/{index}/subscriptions/{user}", s.handleSetIssueSubscription)
}

// decodeJSONBody decodes r's body into v, returning a decode error verbatim
// for the caller to turn into a 400 — every handler here treats a malformed
// body as a client error, distinct from an unknown resource (404).
func decodeJSONBody(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// handleCreateComment serves POST .../issues/{index}/comments, also used for
// pull request comments (Gitea shares one comment thread between issues and
// pulls). Mirrors CreateIssueCommentOption.Validate(): an empty body is a 400.
func (s *Server) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	idx, ok := parsePathInt64(r.PathValue("index"))
	if !ok {
		notFound(w, r)
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Body) == "" {
		http.Error(w, "body is empty", http.StatusBadRequest)
		return
	}

	var exists bool
	var me string
	s.store.WithLock(func() {
		exists = s.store.pullLocked(full, idx) != nil || s.store.issueLocked(full, idx) != nil
		me = s.store.meLocked().Login
	})
	if !exists {
		notFound(w, r)
		return
	}

	comment := s.store.AddComment(full, idx, me, body.Body)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, comment)
}

// editItemBody decodes the fields SetIssueState/SetItemAssignees/
// SetIssueMilestone actually send in one PATCH .../issues/{index}: State and
// Milestone are pointers because EditIssueOption declares them as pointers
// (nil means "untouched"); Assignees is a nilable slice for the same reason
// (nil means "untouched", any concrete slice — even empty — means "replace
// with this"); Title is never sent for issues in this codebase, but decoding
// it costs nothing.
type editItemBody struct {
	Title     string   `json:"title"`
	Assignees []string `json:"assignees"`
	State     *string  `json:"state"`
}

// handleEditIssue serves PATCH .../issues/{index}: a partial update of
// state/milestone/assignees. Unlike pulls, EditIssueOption's Milestone field
// is itself a pointer (*int64), so nil vs. a real ID is unambiguous without
// the "0 means untouched" convention handleEditPull needs.
func (s *Server) handleEditIssue(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	idx, ok := parsePathInt64(r.PathValue("index"))
	if !ok {
		notFound(w, r)
		return
	}
	var body struct {
		editItemBody
		Milestone *int64 `json:"milestone"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var exists bool
	s.store.WithLock(func() { exists = s.store.issueLocked(full, idx) != nil })
	if !exists {
		notFound(w, r)
		return
	}

	if body.State != nil {
		if err := s.store.SetIssueState(full, idx, *body.State); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if body.Milestone != nil {
		if err := s.store.SetItemMilestone(full, idx, *body.Milestone); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if body.Assignees != nil {
		if err := s.store.SetItemAssignees(full, idx, body.Assignees); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
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

// handleEditPull serves PATCH .../pulls/{index}: a partial update of
// state/milestone/assignees/title. Title is how the draft mechanism arrives
// here (see internal/gitea/mutation.go's setPullDraft/draftTitle) — there is
// no dedicated draft field on the wire, so a title change also recomputes
// Draft from the WIP prefix convention. EditPullRequestOption's Milestone
// field is a plain (non-pointer) int64, so 0 means "untouched" rather than
// null meaning that, unlike handleEditIssue.
func (s *Server) handleEditPull(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	idx, ok := parsePathInt64(r.PathValue("index"))
	if !ok {
		notFound(w, r)
		return
	}
	var body struct {
		editItemBody
		Milestone int64 `json:"milestone"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var exists bool
	s.store.WithLock(func() { exists = s.store.pullLocked(full, idx) != nil })
	if !exists {
		notFound(w, r)
		return
	}

	if body.State != nil {
		if err := s.store.SetPullState(full, idx, *body.State); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if body.Milestone != 0 {
		if err := s.store.SetItemMilestone(full, idx, body.Milestone); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if body.Assignees != nil {
		if err := s.store.SetItemAssignees(full, idx, body.Assignees); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if body.Title != "" {
		if err := s.store.SetPullTitle(full, idx, body.Title); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.store.SetPullDraft(full, idx, hasWIPPrefix(body.Title)); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
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

// hasWIPPrefix reports whether title carries Gitea/Forgejo's work-in-progress
// prefix, mirroring internal/gitea/mutation.go's unexported stripWIPPrefix
// (duplicated here rather than imported: that helper lives in a different
// package and isn't exported, and the check is two lines).
func hasWIPPrefix(title string) bool {
	trimmed := strings.ToLower(strings.TrimLeft(title, " \t"))
	return strings.HasPrefix(trimmed, "wip:") || strings.HasPrefix(trimmed, "[wip]")
}

// handleMergePull serves POST .../pulls/{index}/merge. MergePullRequest
// (the real client) determines success purely from the HTTP status code —
// 200 or 201 — and never inspects the response body, so no body is written.
func (s *Server) handleMergePull(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	idx, ok := parsePathInt64(r.PathValue("index"))
	if !ok {
		notFound(w, r)
		return
	}
	var body struct {
		Do                     string `json:"Do"`
		DeleteBranchAfterMerge *bool  `json:"delete_branch_after_merge"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	deleteBranch := body.DeleteBranchAfterMerge != nil && *body.DeleteBranchAfterMerge
	if err := s.store.MergePull(full, idx, body.Do, deleteBranch); err != nil {
		notFound(w, r)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleUpdatePull serves POST .../pulls/{index}/update. UpdatePullRequest
// requires exactly HTTP 200 to consider the call successful.
func (s *Server) handleUpdatePull(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	idx, ok := parsePathInt64(r.PathValue("index"))
	if !ok {
		notFound(w, r)
		return
	}
	if err := s.store.TouchPull(full, idx); err != nil {
		notFound(w, r)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// reviewersBody decodes {"reviewers": [...]}, the shared body shape for
// requesting and removing pull reviewers.
type reviewersBody struct {
	Reviewers []string `json:"reviewers"`
}

// handleAddPullReviewers serves POST .../pulls/{index}/requested_reviewers.
// doRequestWithStatusHandle only requires a 2xx; 201 matches
// mutation_test.go's fixture.
func (s *Server) handleAddPullReviewers(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	idx, ok := parsePathInt64(r.PathValue("index"))
	if !ok {
		notFound(w, r)
		return
	}
	var body reviewersBody
	if err := decodeJSONBody(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.store.AddPullReviewers(full, idx, body.Reviewers); err != nil {
		notFound(w, r)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// handleRemovePullReviewers serves DELETE .../pulls/{index}/requested_reviewers.
// 204 matches mutation_test.go's fixture; any 2xx would satisfy the client.
func (s *Server) handleRemovePullReviewers(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	idx, ok := parsePathInt64(r.PathValue("index"))
	if !ok {
		notFound(w, r)
		return
	}
	var body reviewersBody
	if err := decodeJSONBody(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.store.RemovePullReviewers(full, idx, body.Reviewers); err != nil {
		notFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCreatePullReview serves POST .../pulls/{index}/reviews. Review's own
// JSON tags already match what internal/gitea's mapReviews reads
// (id/state/body/user/submitted_at), so the created review marshals directly
// with no map builder needed. No re-fetch-inside-WithLock step is needed
// before marshaling: AddReview just created and returned this exact object,
// so unlike a re-fetch of a pre-existing row, there's no stale-read race to
// guard against (same reasoning as handleCreateComment's direct marshal of
// AddComment's return value, above).
func (s *Server) handleCreatePullReview(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	idx, ok := parsePathInt64(r.PathValue("index"))
	if !ok {
		notFound(w, r)
		return
	}
	var body struct {
		Event string `json:"event"`
		Body  string `json:"body"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var me string
	s.store.WithLock(func() { me = s.store.meLocked().Login })

	review, err := s.store.AddReview(full, idx, me, body.Event, body.Body)
	if err != nil {
		notFound(w, r)
		return
	}
	writeJSON(w, review)
}

// handleAddIssueLabels serves POST .../issues/{index}/labels {"labels":[ids]},
// also used for pull request labels (Gitea models PR labels through the
// issue-label API — see AddItemLabels's doc). Responds with the item's
// resulting labels, matching AddIssueLabels' []*Label return shape (unused
// by internal/gitea's AddLabels, which discards it, but harmless to send).
//
// The existence check runs separately from AddItemLabels' own internal one
// so the two failure modes it can return — unknown item vs. an unresolvable
// label ID — map to different statuses: 404 for the former, 400 for the
// latter (a bad reference in an otherwise well-formed body, not a missing
// primary resource).
func (s *Server) handleAddIssueLabels(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	idx, ok := parsePathInt64(r.PathValue("index"))
	if !ok {
		notFound(w, r)
		return
	}
	var body struct {
		Labels []int64 `json:"labels"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var exists bool
	s.store.WithLock(func() {
		exists = s.store.pullLocked(full, idx) != nil || s.store.issueLocked(full, idx) != nil
	})
	if !exists {
		notFound(w, r)
		return
	}
	if err := s.store.AddItemLabels(full, idx, body.Labels); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.store.WithLock(func() {
		var labels []*Label
		if p := s.store.pullLocked(full, idx); p != nil {
			labels = p.Labels
		} else if i := s.store.issueLocked(full, idx); i != nil {
			labels = i.Labels
		}
		writeJSON(w, labels)
	})
}

// handleRemoveIssueLabel serves DELETE .../issues/{index}/labels/{labelID}
// (one label ID per call — internal/gitea's RemoveLabels loops over
// DeleteIssueLabel rather than sending a batch). doRequestWithStatusHandle
// only requires a 2xx.
func (s *Server) handleRemoveIssueLabel(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	idx, ok := parsePathInt64(r.PathValue("index"))
	if !ok {
		notFound(w, r)
		return
	}
	labelID, ok := parsePathInt64(r.PathValue("labelID"))
	if !ok {
		notFound(w, r)
		return
	}
	if err := s.store.RemoveItemLabel(full, idx, labelID); err != nil {
		notFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSetIssueSubscription serves PUT/DELETE .../issues/{index}/subscriptions/{user}.
// The SDK's AddIssueSubscription/DeleteIssueSubscription both require
// exactly 201 to consider the call successful; a 200 is treated as an error
// ("already subscribed"/"already unsubscribed"). SetSubscription's
// wasSubscribed report lets this handler reproduce that: 201 only on an
// actual state change, 200 on a redundant repeat.
func (s *Server) handleSetIssueSubscription(w http.ResponseWriter, r *http.Request) {
	full := r.PathValue("owner") + "/" + r.PathValue("repo")
	idx, ok := parsePathInt64(r.PathValue("index"))
	if !ok {
		notFound(w, r)
		return
	}
	user := r.PathValue("user")
	subscribe := r.Method == http.MethodPut

	wasSubscribed, err := s.store.SetSubscription(full, idx, user, subscribe)
	if err != nil {
		notFound(w, r)
		return
	}
	if wasSubscribed == subscribe {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
