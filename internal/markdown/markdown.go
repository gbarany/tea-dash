// Package markdown renders Markdown bodies (PR/issue descriptions, comments)
// to ANSI-styled terminal text for the preview pane.
package markdown

import (
	"regexp"
	"strings"

	"charm.land/glamour/v2"
)

// htmlComment matches HTML comment blocks (including multi-line) so they can be
// stripped before rendering — Gitea/GitHub templates often embed instructional
// <!-- ... --> blocks that add nothing to a rendered preview.
var htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)

// Render turns a Markdown body into ANSI-styled terminal text wrapped to width.
// HTML comments are stripped first. On an empty body (after trimming) it
// returns "", and on any renderer error it falls back to the trimmed raw body
// so the preview is never blank. A fresh renderer is constructed per call,
// which is cheap at preview-pane scale.
func Render(body string, width int) string {
	body = htmlComment.ReplaceAllString(body, "")
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ""
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(max(20, width)),
	)
	if err != nil {
		return trimmed
	}

	out, err := r.Render(body)
	if err != nil {
		return trimmed
	}
	if strings.TrimSpace(out) == "" {
		return trimmed
	}
	return out
}
