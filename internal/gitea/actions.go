package gitea

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/gbarany/tea-dash/internal/data"
)

// ActionRunListOptions are the optional filters supported by Gitea's
// repo-scoped Actions runs endpoint.
type ActionRunListOptions struct {
	Event   string
	Branch  string
	Status  string
	Actor   string
	HeadSHA string
	Limit   int
}

type rawActionRun struct {
	ID           int64     `json:"id"`
	RunNumber    int64     `json:"run_number"`
	RunAttempt   int64     `json:"run_attempt"`
	Name         string    `json:"name"`
	WorkflowName string    `json:"workflow_name"`
	DisplayTitle string    `json:"display_title"`
	Event        string    `json:"event"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	HeadBranch   string    `json:"head_branch"`
	HeadSHA      string    `json:"head_sha"`
	HTMLURL      string    `json:"html_url"`
	URL          string    `json:"url"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	RunStartedAt time.Time `json:"run_started_at"`
	StartedAt    time.Time `json:"started_at"`
	Actor        *struct {
		Login string `json:"login"`
	} `json:"actor"`
	Repository *struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

type rawActionJob struct {
	ID          int64           `json:"id"`
	RunID       int64           `json:"run_id"`
	Name        string          `json:"name"`
	Status      string          `json:"status"`
	Conclusion  string          `json:"conclusion"`
	RunnerName  string          `json:"runner_name"`
	HTMLURL     string          `json:"html_url"`
	URL         string          `json:"url"`
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt time.Time       `json:"completed_at"`
	Steps       []rawActionStep `json:"steps"`
}

type rawActionStep struct {
	Number      int64     `json:"number"`
	StepNumber  int64     `json:"step_number"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

type actionRunsEnvelope struct {
	TotalCount   int            `json:"total_count"`
	WorkflowRuns []rawActionRun `json:"workflow_runs"`
	Runs         []rawActionRun `json:"runs"`
	ActionRuns   []rawActionRun `json:"action_runs"`
	Data         []rawActionRun `json:"data"`
}

type actionJobsEnvelope struct {
	TotalCount int            `json:"total_count"`
	Jobs       []rawActionJob `json:"jobs"`
	ActionJobs []rawActionJob `json:"action_jobs"`
	Data       []rawActionJob `json:"data"`
}

// ListActionRuns returns one page of repo-scoped Actions workflow runs plus the
// server's total count when the payload exposes it. Decoding accepts both the
// documented wrapped shape and bare arrays seen on some Forgejo/Gitea variants.
func (c *Client) ListActionRuns(ctx context.Context, owner, repo string, opts ActionRunListOptions) ([]data.ActionRun, int, error) {
	path := actionRunsPath(owner, repo)
	if q := buildActionRunParams(opts); len(q) > 0 {
		path += "?" + q.Encode()
	}

	var raw json.RawMessage
	if _, err := c.rawGet(ctx, path, &raw); err != nil {
		return nil, 0, err
	}
	rows, total, err := decodeActionRuns(raw)
	if err != nil {
		return nil, 0, fmt.Errorf("decoding actions runs: %w", err)
	}

	repoName := owner + "/" + repo
	runs := make([]data.ActionRun, 0, len(rows))
	for _, row := range rows {
		runs = append(runs, mapActionRun(row, repoName))
	}
	return runs, total, nil
}

// GetActionRun fetches one repo-scoped Actions workflow run.
func (c *Client) GetActionRun(ctx context.Context, owner, repo string, runID int64) (data.ActionRun, error) {
	var raw rawActionRun
	if _, err := c.rawGet(ctx, actionRunPath(owner, repo, runID), &raw); err != nil {
		return data.ActionRun{}, err
	}
	return mapActionRun(raw, owner+"/"+repo), nil
}

// ListActionJobs returns the jobs attached to one repo-scoped Actions workflow
// run. Decoding accepts both the documented wrapped shape and bare arrays.
func (c *Client) ListActionJobs(ctx context.Context, owner, repo string, runID int64) ([]data.ActionJob, error) {
	var raw json.RawMessage
	if _, err := c.rawGet(ctx, actionJobsPath(owner, repo, runID), &raw); err != nil {
		return nil, err
	}
	rows, err := decodeActionJobs(raw)
	if err != nil {
		return nil, fmt.Errorf("decoding actions jobs: %w", err)
	}

	repoName := owner + "/" + repo
	jobs := make([]data.ActionJob, 0, len(rows))
	for _, row := range rows {
		jobs = append(jobs, mapActionJob(row, repoName, runID))
	}
	return jobs, nil
}

// RerunActionRun asks the server to rerun one repo-scoped Actions workflow run.
func (c *Client) RerunActionRun(ctx context.Context, owner, repo string, runID int64) error {
	if err := c.rawPost(ctx, actionRunControlPath(owner, repo, runID, "rerun")); err != nil {
		return fmt.Errorf("rerun action run %s/%s#%d: %w", owner, repo, runID, err)
	}
	return nil
}

// CancelActionRun asks the server to cancel one repo-scoped Actions workflow run.
func (c *Client) CancelActionRun(ctx context.Context, owner, repo string, runID int64) error {
	if err := c.rawPost(ctx, actionRunControlPath(owner, repo, runID, "cancel")); err != nil {
		return fmt.Errorf("cancel action run %s/%s#%d: %w", owner, repo, runID, err)
	}
	return nil
}

func buildActionRunParams(opts ActionRunListOptions) url.Values {
	q := url.Values{}
	if opts.Event != "" {
		q.Set("event", opts.Event)
	}
	if opts.Branch != "" {
		q.Set("branch", opts.Branch)
	}
	if opts.Status != "" {
		q.Set("status", opts.Status)
	}
	if opts.Actor != "" {
		q.Set("actor", opts.Actor)
	}
	if opts.HeadSHA != "" {
		q.Set("head_sha", opts.HeadSHA)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	return q
}

func decodeActionRuns(raw json.RawMessage) ([]rawActionRun, int, error) {
	var rows []rawActionRun
	if err := json.Unmarshal(raw, &rows); err == nil {
		return rows, len(rows), nil
	}

	var env actionRunsEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, 0, err
	}
	switch {
	case env.WorkflowRuns != nil:
		rows = env.WorkflowRuns
	case env.Runs != nil:
		rows = env.Runs
	case env.ActionRuns != nil:
		rows = env.ActionRuns
	case env.Data != nil:
		rows = env.Data
	default:
		rows = nil
	}
	total := env.TotalCount
	if total == 0 {
		total = len(rows)
	}
	return rows, total, nil
}

func decodeActionJobs(raw json.RawMessage) ([]rawActionJob, error) {
	var rows []rawActionJob
	if err := json.Unmarshal(raw, &rows); err == nil {
		return rows, nil
	}

	var env actionJobsEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	switch {
	case env.Jobs != nil:
		return env.Jobs, nil
	case env.ActionJobs != nil:
		return env.ActionJobs, nil
	case env.Data != nil:
		return env.Data, nil
	default:
		return nil, nil
	}
}

func mapActionRun(row rawActionRun, fallbackRepo string) data.ActionRun {
	workflowName := row.WorkflowName
	if workflowName == "" {
		workflowName = row.Name
	}
	htmlURL := row.HTMLURL
	if htmlURL == "" {
		htmlURL = row.URL
	}
	startedAt := row.RunStartedAt
	if startedAt.IsZero() {
		startedAt = row.StartedAt
	}
	repoName := fallbackRepo
	if row.Repository != nil && row.Repository.FullName != "" {
		repoName = row.Repository.FullName
	}

	run := data.ActionRun{
		ID:                row.ID,
		RunNumber:         row.RunNumber,
		RunAttempt:        row.RunAttempt,
		DisplayTitle:      row.DisplayTitle,
		WorkflowName:      workflowName,
		Event:             row.Event,
		Status:            row.Status,
		Conclusion:        row.Conclusion,
		HeadBranch:        row.HeadBranch,
		HeadSHA:           row.HeadSHA,
		RepoNameWithOwner: repoName,
		HTMLURL:           htmlURL,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
		StartedAt:         startedAt,
	}
	if row.Actor != nil {
		run.Actor = row.Actor.Login
	}
	return run
}

func mapActionJob(row rawActionJob, repoName string, fallbackRunID int64) data.ActionJob {
	runID := row.RunID
	if runID == 0 {
		runID = fallbackRunID
	}
	htmlURL := row.HTMLURL
	if htmlURL == "" {
		htmlURL = row.URL
	}
	job := data.ActionJob{
		ID:                row.ID,
		RunID:             runID,
		Name:              row.Name,
		Status:            row.Status,
		Conclusion:        row.Conclusion,
		RunnerName:        row.RunnerName,
		RepoNameWithOwner: repoName,
		HTMLURL:           htmlURL,
		StartedAt:         row.StartedAt,
		CompletedAt:       row.CompletedAt,
	}
	if len(row.Steps) > 0 {
		job.Steps = make([]data.ActionStep, 0, len(row.Steps))
		for _, step := range row.Steps {
			job.Steps = append(job.Steps, mapActionStep(step))
		}
	}
	return job
}

func mapActionStep(row rawActionStep) data.ActionStep {
	number := row.Number
	if number == 0 {
		number = row.StepNumber
	}
	return data.ActionStep{
		Number:      number,
		Name:        row.Name,
		Status:      row.Status,
		Conclusion:  row.Conclusion,
		StartedAt:   row.StartedAt,
		CompletedAt: row.CompletedAt,
	}
}

func actionRunsPath(owner, repo string) string {
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/actions/runs"
}

func actionRunPath(owner, repo string, runID int64) string {
	return actionRunsPath(owner, repo) + "/" + strconv.FormatInt(runID, 10)
}

func actionJobsPath(owner, repo string, runID int64) string {
	return actionRunPath(owner, repo, runID) + "/jobs"
}

func actionRunControlPath(owner, repo string, runID int64, control string) string {
	return actionRunPath(owner, repo, runID) + "/" + control
}
