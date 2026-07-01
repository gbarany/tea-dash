package data

import "time"

// Comment is one issue/PR comment. Populated by a later sub-plan; the field
// set is defined now so downstream code can compile against it.
type Comment struct {
	Author    string
	Body      string
	CreatedAt time.Time
}

// Review is one pull-request review (approve/request-changes/comment).
type Review struct {
	Author      string
	State       string // "APPROVED" | "REQUEST_CHANGES" | "COMMENT" | ...
	Body        string
	SubmittedAt time.Time
}

// Check is one CI status entry attached to a commit SHA.
type Check struct {
	Context     string
	State       string // "success" | "pending" | "failure" | "error" | ...
	Description string
	TargetURL   string
}

// CIStatus is the combined CI state for a pull request's head commit.
type CIStatus struct {
	State  string // rolled-up state
	SHA    string
	Total  int
	Checks []Check
}

// PullDetail is the read-only detail view of a pull request, kept separate
// from the list-row PullRequest type so the detail backend can grow without
// bloating the table rows. Comments/Reviews/CI are populated in a later
// sub-plan and are left empty in M1c-1.
type PullDetail struct {
	Body         string
	BaseRef      string
	HeadRef      string
	HeadSHA      string
	Mergeable    bool
	Merged       bool
	Additions    int
	Deletions    int
	ChangedFiles int
	Comments     []Comment
	Reviews      []Review
	CI           CIStatus
}

// IssueDetail is the read-only detail view of an issue. Comments are populated
// in a later sub-plan and left empty in M1c-1.
type IssueDetail struct {
	Body     string
	Comments []Comment
}
