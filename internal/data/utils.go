package data

import "time"

// RowData is the minimal view a section/table needs of any listed item,
// keeping the generic table/section code decoupled from concrete types.
type RowData interface {
	GetRepoNameWithOwner() string
	GetTitle() string
	GetNumber() int64
	GetUrl() string
	GetUpdatedAt() time.Time
}

func (p PullRequest) GetRepoNameWithOwner() string { return p.RepoNameWithOwner }
func (p PullRequest) GetTitle() string             { return p.Title }
func (p PullRequest) GetNumber() int64             { return p.Number }
func (p PullRequest) GetUrl() string               { return p.HTMLURL }
func (p PullRequest) GetUpdatedAt() time.Time      { return p.UpdatedAt }

func (i Issue) GetRepoNameWithOwner() string { return i.RepoNameWithOwner }
func (i Issue) GetTitle() string             { return i.Title }
func (i Issue) GetNumber() int64             { return i.Number }
func (i Issue) GetUrl() string               { return i.HTMLURL }
func (i Issue) GetUpdatedAt() time.Time      { return i.UpdatedAt }

// Compile-time assertions that both domain row types satisfy RowData.
var (
	_ RowData = PullRequest{}
	_ RowData = Issue{}
)
