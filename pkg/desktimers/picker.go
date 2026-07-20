package desktimers

import (
	"strings"

	"github.com/jesseduffield/lazygit/pkg/utils"
)

// Default prefix templates; {{code}} is replaced with the task code.
const (
	DefaultCommitPrefixTemplate = "{{code}}/"
	DefaultBranchPrefixTemplate = "feature/{{code}}-"
)

// StatusInProgress is the task status the picker floats to the top.
const StatusInProgress = "in_progress"

// OrderTasksForPicker orders tasks for the pick-before-commit menu:
// in-progress tasks first (stable within each group), and returns the index
// of the currently-selected task (by code) in the ordered slice so the menu
// can preselect it. Index is 0 when currentCode is absent or empty.
func OrderTasksForPicker(tasks []Task, currentCode string) ([]Task, int) {
	ordered := make([]Task, 0, len(tasks))
	for _, t := range tasks {
		if t.Status == StatusInProgress {
			ordered = append(ordered, t)
		}
	}
	for _, t := range tasks {
		if t.Status != StatusInProgress {
			ordered = append(ordered, t)
		}
	}

	selectedIdx := 0
	if currentCode != "" {
		for i, t := range ordered {
			if t.Code == currentCode {
				selectedIdx = i
				break
			}
		}
	}
	return ordered, selectedIdx
}

// PickerColumns renders a task as picker-menu columns:
// CODE, title, (project), in-progress marker.
func PickerColumns(task Task) []string {
	marker := ""
	if task.Status == StatusInProgress {
		marker = "● in progress"
	}
	project := ""
	if task.Project != "" {
		project = "(" + task.Project + ")"
	}
	return []string{task.Code, utils.TruncateWithEllipsis(task.Title, 60), project, marker}
}

// ApplyPrefixTemplate expands a prefix template, replacing {{code}} with the
// task code. A blank template falls back to fallback.
func ApplyPrefixTemplate(template string, fallback string, code string) string {
	if strings.TrimSpace(template) == "" {
		template = fallback
	}
	return strings.ReplaceAll(template, "{{code}}", code)
}
