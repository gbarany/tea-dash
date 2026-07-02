// Package actions defines UI-level action intents. It deliberately carries no
// transport or local-git behavior; callers provide a dispatcher seam.
package actions

// Kind identifies the action the user requested.
type Kind string

const (
	KindComment       Kind = "comment"
	KindMerge         Kind = "merge"
	KindClose         Kind = "close"
	KindReopen        Kind = "reopen"
	KindReview        Kind = "review"
	KindExternalDiff  Kind = "external_diff"
	KindCheckout      Kind = "checkout"
	KindSwitchBranch  Kind = "switch_branch"
	KindRerunRun      Kind = "rerun_run"
	KindCancelRun     Kind = "cancel_run"
	KindCustomCommand Kind = "custom_command"
)

// RowKind identifies the selected row's domain type.
type RowKind string

const (
	RowKindPullRequest RowKind = "pull_request"
	RowKindIssue       RowKind = "issue"
	RowKindBranch      RowKind = "branch"
	RowKindActionRun   RowKind = "action_run"
)

// PromptMode records which kind of prompt produced a submitted value.
type PromptMode string

const (
	PromptConfirm PromptMode = "confirm"
	PromptText    PromptMode = "text"
	PromptPicker  PromptMode = "picker"
)

// Target is the immutable row/section context for an action.
type Target struct {
	SectionID      int
	SectionType    string
	RowKind        RowKind
	Repo           string
	RepositoryPath string
	Number         int64
	RunID          int64
	Title          string
	URL            string
	Author         string
	SHA            string
}

// Prompt carries the prompt payload that was submitted with the intent.
type Prompt struct {
	Mode  PromptMode
	Value string
	Label string
}

// Intent is the value passed through the app-level dispatcher seam.
type Intent struct {
	Kind    Kind
	Target  Target
	Prompt  Prompt
	Command string
	Name    string
}

// ResultStatus describes feedback returned by a dispatcher command.
type ResultStatus string

const (
	ResultStarted   ResultStatus = "start"
	ResultSucceeded ResultStatus = "success"
	ResultErrored   ResultStatus = "error"
	ResultCanceled  ResultStatus = "cancel"
)

// ResultMsg is an optional Bubble Tea message a dispatcher can return so the UI
// can show action feedback without knowing how the action was executed.
type ResultMsg struct {
	Intent  Intent
	Status  ResultStatus
	Message string
	Err     error
}
