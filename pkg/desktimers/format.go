package desktimers

import "github.com/jesseduffield/lazygit/pkg/utils"

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

// MenuColumns renders a task as menu columns: CODE, title, (project).
func MenuColumns(task Task) []string {
	project := ""
	if task.Project != "" {
		project = "(" + task.Project + ")"
	}
	return []string{task.Code, utils.TruncateWithEllipsis(task.Title, 60), project}
}
