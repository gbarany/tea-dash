package data

// ItemKind identifies whether a repo item is a pull request or an issue.
type ItemKind string

const (
	ItemKindPull  ItemKind = "pull"
	ItemKindIssue ItemKind = "issue"
)

// ItemRef identifies one mutable repo item.
type ItemRef struct {
	Owner  string
	Repo   string
	Number int64
	Kind   ItemKind
}

// ItemState is the mutable open/closed state shared by issues and pulls.
type ItemState string

const (
	ItemStateOpen   ItemState = "open"
	ItemStateClosed ItemState = "closed"
)

// MergeStyle is the server-supported pull-request merge strategy.
type MergeStyle string

const (
	MergeStyleMerge           MergeStyle = "merge"
	MergeStyleRebase          MergeStyle = "rebase"
	MergeStyleRebaseMerge     MergeStyle = "rebase-merge"
	MergeStyleSquash          MergeStyle = "squash"
	MergeStyleFastForwardOnly MergeStyle = "fast-forward-only"
)

// MergeOptions carries the editable fields for merging a pull request.
type MergeOptions struct {
	Style        MergeStyle
	Title        string
	Message      string
	DeleteBranch bool
	ForceMerge   bool
	AutoMerge    bool
	HeadCommitID string
}

// PullReviewEvent is the user's review action. The transport maps these event
// names onto Gitea's submitted review states.
type PullReviewEvent string

const (
	PullReviewEventApprove        PullReviewEvent = "approve"
	PullReviewEventRequestChanges PullReviewEvent = "request-changes"
	PullReviewEventComment        PullReviewEvent = "comment"
)

const (
	PullReviewStateApproved       ReviewState = ReviewStateApproved
	PullReviewStateRequestChanges ReviewState = ReviewStateRequestChanges
	PullReviewStateComment        ReviewState = ReviewStateComment
)

// PullReviewOptions carries the submitted review action and body.
type PullReviewOptions struct {
	Event PullReviewEvent
	Body  string
}
