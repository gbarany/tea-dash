// Package prview renders a selected pull-request or issue row (plus an optional
// loaded detail) into a preview string. It holds no Bubble Tea state — the
// entry points are pure functions the preview pane feeds into a viewport.
package prview

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/data"
	"github.com/gbarany/tea-dash/internal/markdown"
	"github.com/gbarany/tea-dash/internal/ui/components/section"
	appctx "github.com/gbarany/tea-dash/internal/ui/context"
	"github.com/gbarany/tea-dash/internal/ui/icons"
)

// foldLines is the folded-body cap: a non-expanded preview shows at most this
// many rendered lines before the "read more" hint.
const foldLines = 12

// maxChecks / maxComments cap how many per-check and per-comment lines render
// before a "…and N more" summary line.
const (
	maxChecks      = 8
	maxComments    = 10
	maxActionJobs  = 12
	maxActionSteps = 12
)

// Tab is one rendered preview tab. The UI adapts this to the stateful sidebar
// component so this pure rendering package stays Bubble Tea-free.
type Tab struct {
	Title   string
	Content string
}

var (
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	titleStyle = lipgloss.NewStyle().Bold(true)
	subtleRef  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	// greenStyle/redStyle are kept for reviewBadge (APPROVED/REQUEST_CHANGES),
	// a review-state — not a row/CI/notification/branch state icons.State
	// covers — so it stays outside the Task 9 glyph+theme-color system.
	greenStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#238636"))
	redStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#da3633"))
)

// RenderPull renders a pull-request row and optional detail into the preview
// string. When detail is nil the body is a "Loading…" placeholder; when present
// the body is the rendered Markdown, folded to foldLines unless expanded.
// styles/set are ctx.Styles/ctx.Icons (Task 9): the header pill and CI check
// lines are colored from styles.StateColors (theme.colors.state.*, the same
// source list state cells read) and iconed from set, instead of a parallel
// hardcoded palette.
func RenderPull(row data.PullRequest, detail *data.PullDetail, width int, expanded bool, styles appctx.Styles, set icons.Set) string {
	header := pullHeader(row, detail, width, styles, set)
	var body string
	var extras []string
	if detail != nil {
		body = detail.Body
		extras = []string{
			renderCI(detail.CI, width, styles, set),
			renderReviews(detail.Reviews),
			renderComments(detail.Comments, width),
		}
	}
	return compose(header, body, detail == nil, width, expanded, extras)
}

// RenderPullTabs renders a pull request preview as gh-dash-style sidebar tabs:
// overview, checks, reviews, and comments. Loading rows stay as one Overview
// tab so tab navigation does not cycle through empty placeholders.
func RenderPullTabs(row data.PullRequest, detail *data.PullDetail, width int, expanded bool, styles appctx.Styles, set icons.Set) []Tab {
	if detail == nil {
		return []Tab{{Title: "Overview", Content: RenderPull(row, nil, width, expanded, styles, set)}}
	}
	header := pullHeader(row, detail, width, styles, set)
	checks := renderCI(detail.CI, width, styles, set)
	if checks == "" {
		checks = dimStyle.Render("No checks reported.")
	}
	reviews := renderReviews(detail.Reviews)
	if reviews == "" {
		reviews = dimStyle.Render("No reviews yet.")
	}
	comments := renderComments(detail.Comments, width)
	if comments == "" {
		comments = dimStyle.Render("No comments yet.")
	}
	return []Tab{
		{Title: "Overview", Content: compose(header, detail.Body, false, width, expanded, nil)},
		{Title: "Checks", Content: composePreformatted(header, checks)},
		{Title: "Reviews", Content: composePreformatted(header, reviews)},
		{Title: "Comments", Content: composePreformatted(header, comments)},
	}
}

// RenderIssue renders an issue row and optional detail into the preview string,
// mirroring RenderPull without the PR-only base/head and diffstat lines.
func RenderIssue(row data.Issue, detail *data.IssueDetail, width int, expanded bool, styles appctx.Styles, set icons.Set) string {
	header := issueHeader(row, width, styles, set)
	var body string
	var extras []string
	if detail != nil {
		body = detail.Body
		extras = []string{renderComments(detail.Comments, width)}
	}
	return compose(header, body, detail == nil, width, expanded, extras)
}

// RenderIssueTabs renders an issue preview as overview/comments tabs. Loading
// rows stay as one Overview tab.
func RenderIssueTabs(row data.Issue, detail *data.IssueDetail, width int, expanded bool, styles appctx.Styles, set icons.Set) []Tab {
	if detail == nil {
		return []Tab{{Title: "Overview", Content: RenderIssue(row, nil, width, expanded, styles, set)}}
	}
	header := issueHeader(row, width, styles, set)
	comments := renderComments(detail.Comments, width)
	if comments == "" {
		comments = dimStyle.Render("No comments yet.")
	}
	return []Tab{
		{Title: "Overview", Content: compose(header, detail.Body, false, width, expanded, nil)},
		{Title: "Comments", Content: composePreformatted(header, comments)},
	}
}

// RenderNotification renders a notification row into the preview. Notifications
// are list records, not detail records; this view gives enough context and lets
// open-in-browser handle the full thread.
func RenderNotification(row data.Notification, width int, styles appctx.Styles, set icons.Set) string {
	number := fmt.Sprintf("#%d", row.Number)
	if row.Number == 0 {
		number = fmt.Sprintf("notification %d", row.ID)
	}
	header := []string{
		row.RepoNameWithOwner + " · " + number,
		titleLine(row.SubjectTitle, width),
		notificationPill(row, styles, set),
	}
	if row.SubjectType != "" {
		header = append(header, subtleRef.Render(row.SubjectType))
	}
	body := "Open this notification to read the full thread in Gitea."
	if row.LatestCommentURL != "" {
		body += "\nLatest comment available."
	}
	return compose(header, body, false, width, true, nil)
}

// RenderAction renders a repo-scoped Actions workflow run into the preview.
// When detail is present it appends the loaded job and step statuses.
func RenderAction(row data.ActionRun, detail *data.ActionRunDetail, width int, styles appctx.Styles, set icons.Set) string {
	run := row
	if detail != nil {
		run = mergeActionRun(row, detail.Run)
	}
	header := actionHeader(run, width, styles, set)
	body := actionBody(run)
	if len(body) == 0 {
		body = append(body, "Open this action run in Gitea to inspect jobs and logs.")
	}
	var extras []string
	if detail == nil {
		body = append(body, "", "Jobs: Loading...")
	} else {
		extras = []string{renderActionJobs(detail.Jobs, width, styles, set)}
	}
	return compose(header, strings.Join(body, "\n"), false, width, true, extras)
}

// RenderActionTabs renders an action run preview as overview/jobs tabs.
func RenderActionTabs(row data.ActionRun, detail *data.ActionRunDetail, width int, styles appctx.Styles, set icons.Set) []Tab {
	if detail == nil {
		return []Tab{{Title: "Overview", Content: RenderAction(row, nil, width, styles, set)}}
	}
	run := mergeActionRun(row, detail.Run)
	header := actionHeader(run, width, styles, set)
	body := actionBody(run)
	if len(body) == 0 {
		body = append(body, "Open this action run in Gitea to inspect jobs and logs.")
	}
	jobs := renderActionJobs(detail.Jobs, width, styles, set)
	if jobs == "" {
		jobs = dimStyle.Render("No jobs reported.")
	}
	return []Tab{
		{Title: "Overview", Content: compose(header, strings.Join(body, "\n"), false, width, true, nil)},
		{Title: "Jobs", Content: composePreformatted(header, jobs)},
	}
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

func composePreformatted(header []string, body string) string {
	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinVertical(lipgloss.Left, header...),
		"",
		body,
	)
}

func pullHeader(row data.PullRequest, detail *data.PullDetail, width int, styles appctx.Styles, set icons.Set) []string {
	header := []string{
		locator(row.RepoNameWithOwner, row.Number),
		titleLine(row.Title, width),
	}
	if row.Draft {
		header = append(header, statePill(icons.Draft, "DRAFT", styles, set))
	} else {
		header = append(header, statePill(rowState(row.State), stateLabel(row.State), styles, set))
	}
	if detail != nil {
		header = append(header,
			subtleRef.Render(fmt.Sprintf("%s ← %s", detail.BaseRef, detail.HeadRef)),
			dimStyle.Render(fmt.Sprintf("+%d -%d, %d files",
				detail.Additions, detail.Deletions, detail.ChangedFiles)),
		)
	}
	return header
}

func issueHeader(row data.Issue, width int, styles appctx.Styles, set icons.Set) []string {
	return []string{
		locator(row.RepoNameWithOwner, row.Number),
		titleLine(row.Title, width),
		statePill(rowState(row.State), stateLabel(row.State), styles, set),
	}
}

func actionHeader(run data.ActionRun, width int, styles appctx.Styles, set icons.Set) []string {
	header := []string{
		locator(run.RepoNameWithOwner, run.GetNumber()),
		titleLine(run.GetTitle(), width),
	}
	if status := actionRunStatus(run); status != "" {
		header = append(header, statePill(actionRunState(run), strings.ToUpper(status), styles, set))
	}
	return header
}

func actionBody(run data.ActionRun) []string {
	var body []string
	if run.WorkflowName != "" {
		body = append(body, "Workflow: "+run.WorkflowName)
	}
	if status := actionRunStatus(run); status != "" {
		body = append(body, "Status: "+status)
	}
	if run.Event != "" {
		body = append(body, "Event: "+run.Event)
	}
	if run.Actor != "" {
		body = append(body, "Actor: @"+run.Actor)
	}
	if run.HeadBranch != "" {
		body = append(body, "Branch: "+run.HeadBranch)
	}
	if run.HeadSHA != "" {
		body = append(body, "SHA: "+shortSHA(run.HeadSHA))
	}
	if rel := section.HumanizeTime(run.GetUpdatedAt()); rel != "" {
		body = append(body, "Updated: "+rel)
	}
	return body
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

// pill renders a bold, padded, colored status chip with a white foreground
// over bg.
func pill(text string, bg color.Color) string {
	return lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1).
		Foreground(lipgloss.Color("15")).
		Background(bg).
		Render(text)
}

// statePill renders a "glyph LABEL" pill for st, colored from
// styles.StateColors[st] (theme.colors.state.*) — the same color source
// list state cells (section.StateCell) read, so a preview header and its
// row match. st kinds with no StateColors entry (icons.Unread — the only
// one statePill's callers ever pass, since PR/issue/action states are all
// config-backed) fall back to styles.ActionButton's primary foreground,
// mirroring section.StateCell's explicit-style path for the same reason
// (T2/T9 review note).
func statePill(st icons.State, label string, styles appctx.Styles, set icons.Set) string {
	bg := styles.ActionButton.GetForeground()
	if s, ok := styles.StateColors[st]; ok {
		bg = s.GetForeground()
	}
	return pill(icons.Glyph(set, st)+" "+label, bg)
}

// stateLabel uppercases a row state ("open" -> "OPEN"); "" stays "".
func stateLabel(state string) string {
	return strings.ToUpper(state)
}

// rowState classifies a PR/issue state word into its icons.State, defaulting
// to Open for anything unclassified (blank/unexpected backend values render
// as the open-green pill, matching the pre-Task-9 stateColor default).
func rowState(state string) icons.State {
	if st, ok := section.ClassifyState(state); ok {
		return st
	}
	return icons.Open
}

// actionRunState classifies an Actions run's "status/conclusion" pair (see
// actionRunStatus) into its icons.State, defaulting to Neutral (gray/dim)
// for unclassified/in-flight combinations that don't otherwise map.
func actionRunState(run data.ActionRun) icons.State {
	if st, ok := section.ClassifyState(actionRunStatus(run)); ok {
		return st
	}
	return icons.Neutral
}

func notificationPill(row data.Notification, styles appctx.Styles, set icons.Set) string {
	switch {
	case row.Unread:
		return statePill(icons.Unread, "UNREAD", styles, set)
	case row.Pinned:
		// No dedicated Pinned icons.State exists (theme.colors.state has no
		// "pinned" knob); Neutral matches how section.StateCell already
		// colors "pinned" list rows.
		return statePill(icons.Neutral, "PINNED", styles, set)
	case row.SubjectState != "":
		return statePill(rowState(row.SubjectState), strings.ToUpper(row.SubjectState), styles, set)
	default:
		return statePill(icons.Neutral, "READ", styles, set)
	}
}

func actionRunStatus(row data.ActionRun) string {
	switch {
	case row.Status != "" && row.Conclusion != "":
		return row.Status + "/" + row.Conclusion
	case row.Conclusion != "":
		return row.Conclusion
	default:
		return row.Status
	}
}

func mergeActionRun(base, detail data.ActionRun) data.ActionRun {
	if detail.ID == 0 {
		return base
	}
	if detail.RunNumber == 0 {
		detail.RunNumber = base.RunNumber
	}
	if detail.RunAttempt == 0 {
		detail.RunAttempt = base.RunAttempt
	}
	if detail.DisplayTitle == "" {
		detail.DisplayTitle = base.DisplayTitle
	}
	if detail.WorkflowName == "" {
		detail.WorkflowName = base.WorkflowName
	}
	if detail.Event == "" {
		detail.Event = base.Event
	}
	if detail.Status == "" {
		detail.Status = base.Status
	}
	if detail.Conclusion == "" {
		detail.Conclusion = base.Conclusion
	}
	if detail.HeadBranch == "" {
		detail.HeadBranch = base.HeadBranch
	}
	if detail.HeadSHA == "" {
		detail.HeadSHA = base.HeadSHA
	}
	if detail.Actor == "" {
		detail.Actor = base.Actor
	}
	if detail.RepoNameWithOwner == "" {
		detail.RepoNameWithOwner = base.RepoNameWithOwner
	}
	if detail.HTMLURL == "" {
		detail.HTMLURL = base.HTMLURL
	}
	if detail.CreatedAt.IsZero() {
		detail.CreatedAt = base.CreatedAt
	}
	if detail.UpdatedAt.IsZero() {
		detail.UpdatedAt = base.UpdatedAt
	}
	if detail.StartedAt.IsZero() {
		detail.StartedAt = base.StartedAt
	}
	return detail
}

func renderActionJobs(jobs []data.ActionJob, width int, styles appctx.Styles, set icons.Set) string {
	lines := []string{titleStyle.Render("Jobs:")}
	if len(jobs) == 0 {
		return strings.Join(append(lines, dimStyle.Render("No jobs reported.")), "\n")
	}

	shown := jobs
	extra := 0
	if len(shown) > maxActionJobs {
		extra = len(shown) - maxActionJobs
		shown = shown[:maxActionJobs]
	}
	for _, job := range shown {
		lines = append(lines, actionJobLine(job, width, styles, set))
		steps := job.Steps
		stepExtra := 0
		if len(steps) > maxActionSteps {
			stepExtra = len(steps) - maxActionSteps
			steps = steps[:maxActionSteps]
		}
		for _, step := range steps {
			lines = append(lines, actionStepLine(step, width, styles, set))
		}
		if stepExtra > 0 {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  …and %d more steps", stepExtra)))
		}
	}
	if extra > 0 {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("…and %d more jobs", extra)))
	}
	return strings.Join(lines, "\n")
}

func actionJobLine(job data.ActionJob, width int, styles appctx.Styles, set icons.Set) string {
	icon, st := actionItemIcon(job.Status, job.Conclusion, styles, set)
	text := job.Name
	if status := actionItemStatus(job.Status, job.Conclusion); status != "" {
		text += " · " + status
	}
	if job.RunnerName != "" {
		text += " · " + job.RunnerName
	}
	return st.Render(icon) + " " + truncateText(text, width-2)
}

func actionStepLine(step data.ActionStep, width int, styles appctx.Styles, set icons.Set) string {
	icon, st := actionItemIcon(step.Status, step.Conclusion, styles, set)
	text := step.Name
	if step.Number != 0 {
		text = fmt.Sprintf("%d. %s", step.Number, text)
	}
	if status := actionItemStatus(step.Status, step.Conclusion); status != "" {
		text += " · " + status
	}
	return "  " + st.Render(icon) + " " + truncateText(text, width-4)
}

func actionItemStatus(status, conclusion string) string {
	switch {
	case status != "" && conclusion != "":
		return status + "/" + conclusion
	case conclusion != "":
		return conclusion
	default:
		return status
	}
}

// actionItemIcon classifies a job/step's status+conclusion pair (via the
// same section.ClassifyState composite logic actionRunState/section.StateCell
// use) into a glyph+color pair, defaulting to Running (amber/in-flight) for
// unclassified/in-flight combinations.
func actionItemIcon(status, conclusion string, styles appctx.Styles, set icons.Set) (string, lipgloss.Style) {
	st, ok := section.ClassifyState(actionItemStatus(status, conclusion))
	if !ok {
		st = icons.Running
	}
	return icons.Glyph(set, st), styles.StateColors[st]
}

func shortSHA(sha string) string {
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12]
}

// renderCI renders the CI block: a colored "Checks: ✓N ✗M •K" summary line
// (glyphs from set, colors from styles.StateColors — Task 9) followed by up
// to maxChecks per-check lines (with a "…and N more" overflow line). It
// returns "" when no CI state was populated.
func renderCI(ci data.CIStatus, width int, styles appctx.Styles, set icons.Set) string {
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
		styles.StateColors[icons.Success].Render(fmt.Sprintf("%s%d", icons.Glyph(set, icons.Success), success)) + " " +
		styles.StateColors[icons.Failure].Render(fmt.Sprintf("%s%d", icons.Glyph(set, icons.Failure), failure)) + " " +
		styles.StateColors[icons.Running].Render(fmt.Sprintf("%s%d", icons.Glyph(set, icons.Running), pending))

	lines := []string{summary}
	shown := ci.Checks
	extra := 0
	if len(shown) > maxChecks {
		extra = len(shown) - maxChecks
		shown = shown[:maxChecks]
	}
	for _, c := range shown {
		lines = append(lines, checkLine(c, width, styles, set))
	}
	if extra > 0 {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("…and %d more", extra)))
	}
	return strings.Join(lines, "\n")
}

// checkLine renders one CI check as "icon Context — Description", truncated to
// width so it stays on a single line.
func checkLine(c data.Check, width int, styles appctx.Styles, set icons.Set) string {
	icon, st := checkIcon(c.State, styles, set)
	text := c.Context
	if c.Description != "" {
		text += " — " + c.Description
	}
	return st.Render(icon) + " " + truncateText(text, width-2)
}

// checkIcon maps a check state to its glyph (from set) and color (from
// styles.StateColors) — Task 9: previously hardcoded "✓"/"✗"/"!"/"•" and a
// fixed green/red/yellow palette regardless of the configured theme.
func checkIcon(state data.CheckState, styles appctx.Styles, set icons.Set) (string, lipgloss.Style) {
	switch state {
	case data.CheckStateSuccess:
		return icons.Glyph(set, icons.Success), styles.StateColors[icons.Success]
	case data.CheckStateFailure, data.CheckStateError:
		return icons.Glyph(set, icons.Failure), styles.StateColors[icons.Failure]
	default:
		// Pending/Warning: no dedicated icons.State for "warning" — Running's
		// in-flight amber/spinner glyph is the closest existing signal.
		return icons.Glyph(set, icons.Running), styles.StateColors[icons.Running]
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
// APPROVED green, REQUEST_CHANGES red, anything else dim. Review states are
// a different domain from icons.State (row/CI/notification/branch) and
// aren't part of Task 9's theme-color/glyph system.
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
