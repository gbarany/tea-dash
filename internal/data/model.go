// Package data holds tea-dash's TUI-agnostic domain types, decoupled from
// both the Gitea transport and the Bubble Tea UI.
package data

import (
	"strings"
	"time"
)

// User is a subset of a Gitea user.
type User struct {
	Login    string
	FullName string
}

// Label is a subset of a Gitea label.
type Label struct {
	Name  string
	Color string
}

// PullRequest is the domain view of a Gitea pull request, denormalized so a
// row from the cross-repo search endpoint carries its own repo.
type PullRequest struct {
	Number            int64 // per-repo index
	Title             string
	RepoNameWithOwner string // "owner/repo"
	Author            string // poster login
	State             string // "open" | "closed" | "merged"
	Draft             bool
	HTMLURL           string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Labels            []Label
}

// SplitOwnerRepo splits "owner/name" into its parts. ok is false for anything
// that is not exactly one owner and one name.
func SplitOwnerRepo(full string) (owner, name string, ok bool) {
	owner, name, ok = strings.Cut(full, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", "", false
	}
	return owner, name, true
}
