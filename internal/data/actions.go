package data

import "time"

// ActionRun is the domain view of one Gitea Actions workflow run for a repo.
// It is denormalized with RepoNameWithOwner so a future Actions section can use
// the same generic row model as pulls, issues, and notifications.
type ActionRun struct {
	ID                int64
	RunNumber         int64
	RunAttempt        int64
	DisplayTitle      string
	WorkflowName      string
	Event             string
	Status            string
	Conclusion        string
	HeadBranch        string
	HeadSHA           string
	Actor             string
	RepoNameWithOwner string
	HTMLURL           string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	StartedAt         time.Time
}

// ActionJob is the domain view of one job within an Actions workflow run.
type ActionJob struct {
	ID                int64
	RunID             int64
	Name              string
	Status            string
	Conclusion        string
	RunnerName        string
	RepoNameWithOwner string
	HTMLURL           string
	StartedAt         time.Time
	CompletedAt       time.Time
	Steps             []ActionStep
}

// ActionStep is one step within an Actions job.
type ActionStep struct {
	Number      int64
	Name        string
	Status      string
	Conclusion  string
	StartedAt   time.Time
	CompletedAt time.Time
}

func (r ActionRun) GetRepoNameWithOwner() string { return r.RepoNameWithOwner }

func (r ActionRun) GetTitle() string {
	if r.DisplayTitle != "" {
		return r.DisplayTitle
	}
	return r.WorkflowName
}

func (r ActionRun) GetNumber() int64 {
	if r.RunNumber != 0 {
		return r.RunNumber
	}
	return r.ID
}

func (r ActionRun) GetURL() string { return r.HTMLURL }

func (r ActionRun) GetUpdatedAt() time.Time {
	if !r.UpdatedAt.IsZero() {
		return r.UpdatedAt
	}
	return r.StartedAt
}
