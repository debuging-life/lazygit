package desktimers

import (
	"strings"

	"github.com/jesseduffield/lazygit/pkg/utils"
)

// Default prefix templates; {{code}} is replaced with the task code and
// {{type}} (branch template only) with the chosen branch type.
const (
	DefaultCommitPrefixTemplate = "{{code}}/"
	DefaultBranchPrefixTemplate = "{{type}}/{{code}}-"
)

// DefaultBranchTypes is the branch-type menu, in menu order; the first entry
// is the preselected default. Overridable via desktimers.branchTypes.
var DefaultBranchTypes = []string{"feature", "bugfix", "hotfix", "release", "chore", "refactor", "docs"}

// StatusInProgress is the task status the picker floats to the top.
const StatusInProgress = "in_progress"

// pickerRank groups tasks for the picker: the timer-tracked task first, then
// in-progress, then the rest. Ordering is stable within each group.
func pickerRank(t Task) int {
	switch {
	case t.Tracking:
		return 0
	case t.Status == StatusInProgress:
		return 1
	default:
		return 2
	}
}

// OrderTasksForPicker orders tasks for the picker menus: the task being
// tracked by the running desktop timer first, then in-progress tasks, then
// the rest (stable within each group). The returned index is the item to
// preselect: the tracking task wins over the currently-selected task
// (currentCode), which wins over the first item.
func OrderTasksForPicker(tasks []Task, currentCode string) ([]Task, int) {
	ordered := make([]Task, 0, len(tasks))
	for rank := 0; rank <= 2; rank++ {
		for _, t := range tasks {
			if pickerRank(t) == rank {
				ordered = append(ordered, t)
			}
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
	for i, t := range ordered {
		if t.Tracking {
			selectedIdx = i
			break
		}
	}
	return ordered, selectedIdx
}

// PickerColumns renders a task as picker-menu columns:
// CODE, title, (project), tracking/in-progress marker.
func PickerColumns(task Task) []string {
	marker := ""
	switch {
	case task.Tracking:
		marker = "⏱ tracking"
	case task.Status == StatusInProgress:
		marker = "● in progress"
	}
	project := ""
	if task.Project != "" {
		project = "(" + task.Project + ")"
	}
	return []string{task.Code, utils.TruncateWithEllipsis(task.Title, 60), project, marker}
}

// effectiveTemplate resolves a possibly-blank user template to fallback.
func effectiveTemplate(template string, fallback string) string {
	if strings.TrimSpace(template) == "" {
		return fallback
	}
	return template
}

// ApplyPrefixTemplate expands a prefix template, replacing {{code}} with the
// task code. A blank template falls back to fallback.
func ApplyPrefixTemplate(template string, fallback string, code string) string {
	return strings.ReplaceAll(effectiveTemplate(template, fallback), "{{code}}", code)
}

// BranchTemplateUsesType reports whether the (fallback-resolved) branch
// prefix template contains {{type}} — when it doesn't, the user's template
// fully controls the prefix and the branch-type menu is skipped.
func BranchTemplateUsesType(template string) bool {
	return strings.Contains(effectiveTemplate(template, DefaultBranchPrefixTemplate), "{{type}}")
}

// ApplyBranchPrefixTemplate expands the branch prefix template with the
// chosen branch type and task code, e.g. "{{type}}/{{code}}-" →
// "bugfix/LOUD-183-".
func ApplyBranchPrefixTemplate(template string, branchType string, code string) string {
	t := effectiveTemplate(template, DefaultBranchPrefixTemplate)
	t = strings.ReplaceAll(t, "{{type}}", branchType)
	return strings.ReplaceAll(t, "{{code}}", code)
}

// FirstBranchType returns the default (first) branch type of a configured
// list, falling back to the built-in defaults.
func FirstBranchType(types []string) string {
	if len(types) == 0 {
		return DefaultBranchTypes[0]
	}
	return types[0]
}

// ComposeWithTaskPrefix prepends the readonly task prefix to what the user
// typed. The typed text wins unchanged when it's blank (so the usual
// empty-input validation still fires), when there is no prefix (no-task
// valve), or when it already carries a task code — same or different; a
// different code is an intentional override.
func ComposeWithTaskPrefix(prefix string, typed string) string {
	if prefix == "" || strings.TrimSpace(typed) == "" {
		return typed
	}
	if ExtractCode(typed) != "" {
		return typed
	}
	return prefix + typed
}

// TitleWithTaskPrefix renders a panel/prompt title carrying the readonly
// task prefix, e.g. `Commit summary — LOUD-124/`.
func TitleWithTaskPrefix(baseTitle string, prefix string) string {
	if prefix == "" {
		return baseTitle
	}
	return baseTitle + " — " + prefix
}
