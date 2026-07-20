package desktimers

import (
	"strings"

	"github.com/jesseduffield/lazygit/pkg/utils"
)

// StatusLineSegment renders the selected task for the status/information
// line, e.g. "⏱ MOB-101 Fix login redirect", truncated to maxWidth.
func StatusLineSegment(state *State, maxWidth int) string {
	if state == nil || state.Code == "" {
		return ""
	}
	segment := "⏱ " + state.Code
	if state.Title != "" {
		segment += " " + state.Title
	}
	return utils.TruncateWithEllipsis(segment, maxWidth)
}

// StatusLineHint renders the "no task selected" nudge for the information
// line, e.g. "⏱ no task (alt+t)", truncated to maxWidth. An empty keyLabel
// (binding disabled) yields no hint.
func StatusLineHint(keyLabel string, maxWidth int) string {
	if keyLabel == "" {
		return ""
	}
	return utils.TruncateWithEllipsis("⏱ no task ("+keyLabel+")", maxWidth)
}

// FriendlyKeyLabel turns the first key of a binding into a human-friendly
// label: "<alt+t>" → "alt+t". Returns "" for an unbound/disabled binding.
func FriendlyKeyLabel(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return strings.Trim(keys[0], "<>")
}

// MenuColumns renders a task as menu columns: CODE, title, (project).
func MenuColumns(task Task) []string {
	project := ""
	if task.Project != "" {
		project = "(" + task.Project + ")"
	}
	return []string{task.Code, utils.TruncateWithEllipsis(task.Title, 60), project}
}
