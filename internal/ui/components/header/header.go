// Package header renders tea-dash's one-row header, embedded in the shell's
// top border line: app name, the five numbered view labels, and the
// instance host/user on the right (spec §1). It hand-draws the box-art
// corners/fill and colors them via context.Styles.BorderBlurred — border
// styles are foreground-only by design, so a plain lipgloss.Style.Border()
// can't embed the label text into the border run (see context/styles.go).
package header

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/gbarany/tea-dash/internal/ui/context"
)

// viewLabels enumerates the five top-level views' jump numbers and short
// display names, in header order (spec §1: "1 Pulls · 2 Issues · 3 Inbox ·
// 4 CI · 5 Branches"). Display names are intentionally short and distinct
// from the config-facing section identifiers (prs/issues/notifications/
// actions/branches).
var viewLabels = []struct {
	View context.ViewType
	Name string
}{
	{context.PullsView, "Pulls"},
	{context.IssuesView, "Issues"},
	{context.NotificationsView, "Inbox"},
	{context.ActionsView, "CI"},
	{context.BranchesView, "Branches"},
}

// LabelRange is the 0-based, row-relative [Start,End) column range a view's
// header label occupies in the string View(w, ...) would render, for Task
// 6's ZoneViewLabel mouse-hit registration.
type LabelRange struct {
	View       context.ViewType
	Start, End int
}

// View renders the header row into exactly w cells. Narrow widths drop the
// host/user block first, then the view-label block, always keeping the app
// name and the box-art frame.
func View(w int, active context.ViewType, host, user string, styles context.Styles) string {
	row, _ := build(w, active, host, user, styles)
	return row
}

// Labels returns the column ranges View(w, active, host, user, styles)
// would render each view label at. Returns nil when the row is too narrow
// to show the label block at all.
func Labels(w int, active context.ViewType, host, user string, styles context.Styles) []LabelRange {
	_, ranges := build(w, active, host, user, styles)
	return ranges
}

func build(w int, active context.ViewType, host, user string, styles context.Styles) (string, []LabelRange) {
	border := styles.BorderBlurred
	leftCorner := border.Render("┌")
	rightCorner := border.Render("┐")
	corners := lipgloss.Width(leftCorner) + lipgloss.Width(rightCorner)
	if w < 0 {
		w = 0
	}
	inner := w - corners
	if inner < 0 {
		inner = 0
	}

	appName := styles.PanelTitle.Render("tea-dash")
	leftBlock := " " + appName + " "
	leftW := lipgloss.Width(leftBlock)

	labelStyled, labelRanges := renderLabels(active, styles)
	// +2: a plain space on each side, so the dash fill either side of the
	// label block doesn't touch label text directly (e.g. "─1 Pulls").
	labelW := lipgloss.Width(labelStyled) + 2

	hostText := hostBlock(host, user)
	var hostStyled string
	var hostW int
	if hostText != "" {
		hostStyled = styles.DimText.Render(hostText)
		hostW = lipgloss.Width(hostStyled)
	}

	showLabels := labelW > 0
	showHost := hostW > 0

	contentWidth := func(labels, hostShown bool) int {
		gaps := 0
		width := leftW
		if labels {
			width += labelW
			gaps++
		}
		if hostShown {
			width += hostW + 1 // +1 = trailing space before the right corner
			gaps++
		}
		return width + gaps
	}

	if showHost && contentWidth(showLabels, showHost) > inner {
		showHost = false
	}
	if showLabels && contentWidth(showLabels, showHost) > inner {
		showLabels = false
		labelRanges = nil
	}
	if !showLabels {
		labelRanges = nil
	}

	numGaps := 0
	if showLabels {
		numGaps++
	}
	if showHost {
		numGaps++
	}
	used := contentWidth(showLabels, showHost)
	leftover := inner - used
	if leftover < 0 {
		leftover = 0
	}

	fill := styles.BorderBlurred
	var b strings.Builder
	b.WriteString(leftCorner)
	b.WriteString(leftBlock)
	offset := lipgloss.Width(leftCorner) + leftW

	if numGaps == 0 {
		b.WriteString(fill.Render(strings.Repeat("─", leftover)))
	} else {
		gapIdx := 0
		nextGapWidth := func() int {
			gapIdx++
			gw := 1
			if gapIdx == numGaps {
				gw += leftover
			}
			return gw
		}
		if showLabels {
			gw := nextGapWidth()
			b.WriteString(fill.Render(strings.Repeat("─", gw)))
			b.WriteString(" ")
			offset += gw + 1
			for i := range labelRanges {
				labelRanges[i].Start += offset
				labelRanges[i].End += offset
			}
			b.WriteString(labelStyled)
			b.WriteString(" ")
			offset += labelW - 1 // -1: the leading space above already counted
		}
		if showHost {
			gw := nextGapWidth()
			b.WriteString(fill.Render(strings.Repeat("─", gw)))
			offset += gw
			b.WriteString(hostStyled)
			b.WriteString(" ")
			offset += hostW + 1
		}
	}
	b.WriteString(rightCorner)

	return b.String(), labelRanges
}

// renderLabels renders the "1 Pulls · 2 Issues · ..." block (active label
// highlighted) and returns the [Start,End) column range of each label
// relative to the start of this block (the caller offsets them once the
// block's absolute position in the row is known).
func renderLabels(active context.ViewType, styles context.Styles) (string, []LabelRange) {
	var b strings.Builder
	var ranges []LabelRange
	pos := 0
	for i, vl := range viewLabels {
		if i > 0 {
			sep := styles.DimText.Render(" · ")
			b.WriteString(sep)
			pos += lipgloss.Width(sep)
		}
		text := fmt.Sprintf("%d %s", i+1, vl.Name)
		style := styles.DimText
		if vl.View == active {
			// PanelTitle (not ActiveTab): ActiveTab carries Padding(0,1) for
			// its tab-bar use, which would put stray extra spaces around
			// the label next to this header's own explicit " · "
			// separators.
			style = styles.PanelTitle
		}
		rendered := style.Render(text)
		rw := lipgloss.Width(rendered)
		ranges = append(ranges, LabelRange{View: vl.View, Start: pos, End: pos + rw})
		b.WriteString(rendered)
		pos += rw
	}
	return b.String(), ranges
}

func hostBlock(host, user string) string {
	if host == "" {
		return ""
	}
	if user == "" {
		return host
	}
	return host + " · " + user
}
