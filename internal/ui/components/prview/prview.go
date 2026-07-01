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
	"github.com/gbarany/tea-dash/internal/ui/components/section"
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

// maxChecks / maxComments cap how many per-check and per-comment lines render
// before a "…and N more" summary line.
const (
	maxChecks   = 8
	maxComments = 10
)

var (
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	titleStyle = lipgloss.NewStyle().Bold(true)
	subtleRef  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	greenStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(colOpen))
	redStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colClosed))
	yellowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#d29922"))
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
	var extras []string
	if detail != nil {
		body = detail.Body
		extras = []string{
			renderCI(detail.CI, width),
			renderReviews(detail.Reviews),
			renderComments(detail.Comments, width),
		}
	}
	return compose(header, body, detail == nil, width, expanded, extras)
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
	var extras []string
	if detail != nil {
		body = detail.Body
		extras = []string{renderComments(detail.Comments, width)}
	}
	return compose(header, body, detail == nil, width, expanded, extras)
}

// compose joins the header block with the rendered body, then appends any
// non-empty extra sections (CI / reviews / comments) below the fold, each
// separated by a blank line so the viewport scrolls through them. When loading
// is true the body is a dim placeholder and extras are ignored.
func compose(header []string, rawBody string, loading bool, width int, expanded bool, extras []string) string {
	var body string
	if loading {
		body = dimStyle.Render("Loading…")
	} else {
		body = foldBody(markdown.Render(rawBody, width), expanded)
	}
	parts := []string{
		lipgloss.JoinVertical(lipgloss.Left, header...),
		"",
		body,
	}
	for _, e := range extras {
		if e == "" {
			continue
		}
		parts = append(parts, "", e)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
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

// renderCI renders the CI block: a colored "Checks: ✓N ✗M •K" summary line
// followed by up to maxChecks per-check lines (with a "…and N more" overflow
// line). It returns "" when no CI state was populated.
func renderCI(ci data.CIStatus, width int) string {
	if !ci.HasCI() {
		return ""
	}
	var success, failure, pending int
	for _, c := range ci.Checks {
		switch c.State {
		case data.CheckStateSuccess:
			success++
		case data.CheckStateFailure, data.CheckStateError:
			failure++
		default:
			pending++
		}
	}
	summary := "Checks: " +
		greenStyle.Render(fmt.Sprintf("✓%d", success)) + " " +
		redStyle.Render(fmt.Sprintf("✗%d", failure)) + " " +
		yellowStyle.Render(fmt.Sprintf("•%d", pending))

	lines := []string{summary}
	shown := ci.Checks
	extra := 0
	if len(shown) > maxChecks {
		extra = len(shown) - maxChecks
		shown = shown[:maxChecks]
	}
	for _, c := range shown {
		lines = append(lines, checkLine(c, width))
	}
	if extra > 0 {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("…and %d more", extra)))
	}
	return strings.Join(lines, "\n")
}

// checkLine renders one CI check as "icon Context — Description", truncated to
// width so it stays on a single line.
func checkLine(c data.Check, width int) string {
	icon, st := checkIcon(c.State)
	text := c.Context
	if c.Description != "" {
		text += " — " + c.Description
	}
	return st.Render(icon) + " " + truncateText(text, width-2)
}

// checkIcon maps a check state to its icon and color style.
func checkIcon(state data.CheckState) (string, lipgloss.Style) {
	switch state {
	case data.CheckStateSuccess:
		return "✓", greenStyle
	case data.CheckStateFailure, data.CheckStateError:
		return "✗", redStyle
	case data.CheckStateWarning:
		return "!", yellowStyle
	default:
		return "•", yellowStyle
	}
}

// renderReviews renders the "Reviews:" block: one line per review with a
// colored state badge and @author. Returns "" when there are no reviews.
func renderReviews(reviews []data.Review) string {
	if len(reviews) == 0 {
		return ""
	}
	lines := []string{titleStyle.Render("Reviews:")}
	for _, r := range reviews {
		lines = append(lines, reviewBadge(r.State)+" @"+r.Author)
	}
	return strings.Join(lines, "\n")
}

// reviewBadge renders a review's state as a colored, uppercased badge:
// APPROVED green, REQUEST_CHANGES red, anything else dim.
func reviewBadge(state data.ReviewState) string {
	label := strings.ToUpper(string(state))
	switch state {
	case data.ReviewStateApproved:
		return greenStyle.Render(label)
	case data.ReviewStateRequestChanges, "CHANGES_REQUESTED":
		return redStyle.Render(label)
	default:
		return dimStyle.Render(label)
	}
}

// renderComments renders the comments block for a PR or issue: a "N comments"
// header (singular "1 comment") followed by each comment's dim meta line and
// wrapped body, capped at maxComments with a "…and N more" overflow line.
// Returns "" when there are no comments.
func renderComments(comments []data.Comment, width int) string {
	if len(comments) == 0 {
		return ""
	}
	header := fmt.Sprintf("%d comments", len(comments))
	if len(comments) == 1 {
		header = "1 comment"
	}
	lines := []string{titleStyle.Render(header)}

	shown := comments
	extra := 0
	if len(shown) > maxComments {
		extra = len(shown) - maxComments
		shown = shown[:maxComments]
	}
	for _, c := range shown {
		meta := "@" + c.Author
		if rel := section.HumanizeTime(c.CreatedAt); rel != "" {
			meta += " · " + rel
		}
		lines = append(lines, "", dimStyle.Render(meta), commentBody(c.Body, width))
	}
	if extra > 0 {
		lines = append(lines, "", dimStyle.Render(fmt.Sprintf("…and %d more", extra)))
	}
	return strings.Join(lines, "\n")
}

// commentBody trims and wraps a comment body to width for readable display.
func commentBody(body string, width int) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	s := lipgloss.NewStyle()
	if width > 0 {
		s = s.Width(width)
	}
	return s.Render(body)
}

// truncateText shortens s to at most max runes, appending an ellipsis when it
// cuts. A non-positive max returns s unchanged.
func truncateText(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}
