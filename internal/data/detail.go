package data

import "time"

// ReviewState is a submitted pull-request review state.
type ReviewState string

const (
	ReviewStateApproved       ReviewState = "APPROVED"
	ReviewStateRequestChanges ReviewState = "REQUEST_CHANGES"
	ReviewStateComment        ReviewState = "COMMENT"
)

// CheckState is one CI/check status state attached to a commit.
type CheckState string

const (
	CheckStateSuccess CheckState = "success"
	CheckStatePending CheckState = "pending"
	CheckStateFailure CheckState = "failure"
	CheckStateError   CheckState = "error"
	CheckStateWarning CheckState = "warning"
)

// CIState is the rolled-up combined status state for a commit.
type CIState string

const (
	CIStateSuccess CIState = "success"
	CIStatePending CIState = "pending"
	CIStateFailure CIState = "failure"
	CIStateError   CIState = "error"
)

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
	State       ReviewState
	Body        string
	SubmittedAt time.Time
}

// Check is one CI status entry attached to a commit SHA.
type Check struct {
	Context     string
	State       CheckState
	Description string
	TargetURL   string
}

// CIStatus is the combined CI state for a pull request's head commit. The zero
// value means CI was not fetched or was unavailable.
type CIStatus struct {
	State  CIState
	SHA    string
	Total  int
	Checks []Check
}

// HasCI reports whether a combined status was populated.
func (c CIStatus) HasCI() bool {
	return c.State != "" || c.SHA != "" || c.Total != 0 || len(c.Checks) > 0
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
