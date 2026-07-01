// Package prview renders a selected pull-request or issue row (plus an optional
// loaded detail) into a preview string. It holds no Bubble Tea state — the
// entry points are pure functions the preview pane feeds into a viewport.
package prview

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/markdown"
)

// foldLines is the folded-body cap: a non-expanded preview shows at most this
// many rendered lines before the "read more" hint.
const foldLines = 12

// Pill background colors, matching the app's hardcoded palette style.
const (
	colOpen   = "#238636" // green
	colClosed = "#da3633" // red
	colMerged = "#8957e5" // purple
	colDraft  = "#6e7681" // gray
)

var (
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	titleStyle = lipgloss.NewStyle().Bold(true)
	subtleRef  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

// RenderPull renders a pull-request row and optional detail into the preview
// string. When detail is nil the body is a "Loading…" placeholder; when present
// the body is the rendered Markdown, folded to foldLines unless expanded.
func RenderPull(row data.PullRequest, detail *data.PullDetail, width int, expanded bool) string {
	var header []string
	header = append(header,
		locator(row.RepoNameWithOwner, row.Number),
		titleLine(row.Title, width),
	)

	if row.Draft {
		header = append(header, pill("DRAFT", colDraft))
	} else {
		header = append(header, pill(stateLabel(row.State), stateColor(row.State)))
	}

	if detail != nil {
		header = append(header,
			subtleRef.Render(fmt.Sprintf("%s ← %s", detail.BaseRef, detail.HeadRef)),
			dimStyle.Render(fmt.Sprintf("+%d -%d, %d files",
				detail.Additions, detail.Deletions, detail.ChangedFiles)),
		)
	}

	var body string
	if detail == nil {
		body = ""
	} else {
		body = detail.Body
	}
	return compose(header, body, detail == nil, width, expanded)
}

// RenderIssue renders an issue row and optional detail into the preview string,
// mirroring RenderPull without the PR-only base/head and diffstat lines.
func RenderIssue(row data.Issue, detail *data.IssueDetail, width int, expanded bool) string {
	header := []string{
		locator(row.RepoNameWithOwner, row.Number),
		titleLine(row.Title, width),
		pill(stateLabel(row.State), stateColor(row.State)),
	}

	var body string
	if detail != nil {
		body = detail.Body
	}
	return compose(header, body, detail == nil, width, expanded)
}

// compose joins the header block with the rendered body. When loading is true
// the body is a dim placeholder; otherwise rawBody is Markdown-rendered and
// folded unless expanded.
func compose(header []string, rawBody string, loading bool, width int, expanded bool) string {
	var body string
	if loading {
		body = dimStyle.Render("Loading…")
	} else {
		body = foldBody(markdown.Render(rawBody, width), expanded)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinVertical(lipgloss.Left, header...),
		"",
		body,
	)
}

// foldBody truncates a rendered body to foldLines (plus a hint) unless expanded
// or already short enough.
func foldBody(rendered string, expanded bool) string {
	rendered = strings.TrimRight(rendered, "\n")
	if expanded || rendered == "" {
		return rendered
	}
	lines := strings.Split(rendered, "\n")
	if len(lines) <= foldLines {
		return rendered
	}
	kept := strings.Join(lines[:foldLines], "\n")
	return kept + "\n" + dimStyle.Render("Press e to read more…")
}

// locator renders the "owner/repo · #N" line.
func locator(repo string, number int64) string {
	return repo + " · #" + strconv.FormatInt(number, 10)
}

// titleLine renders the bold title, wrapped to width.
func titleLine(title string, width int) string {
	s := titleStyle
	if width > 0 {
		s = s.Width(width)
	}
	return s.Render(title)
}

// pill renders a bold, padded, colored status chip.
func pill(text, bg string) string {
	return lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color(bg)).
		Render(text)
}

// stateLabel uppercases a row state ("open" -> "OPEN"); "" stays "".
func stateLabel(state string) string {
	return strings.ToUpper(state)
}

// stateColor maps a row state to its pill background, defaulting to open-green.
func stateColor(state string) string {
	switch strings.ToLower(state) {
	case "closed":
		return colClosed
	case "merged":
		return colMerged
	default:
		return colOpen
	}
}
