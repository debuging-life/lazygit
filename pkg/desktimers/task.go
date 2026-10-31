package desktimers

import (
	"regexp"
	"strings"
)

// Task is a DeskTick task as returned by the git-client API.
type Task struct {
	Code    string `json:"code"`
	Title   string `json:"title"`
	Project string `json:"project"`
	Status  string `json:"status"`
	// Tracking marks the task of the user's currently running desktop timer.
	Tracking bool `json:"tracking"`
	// URL is an absolute webapp deep link to the task; may be absent.
	URL string `json:"url"`
}

// CodeRegex matches a DeskTick task code (e.g. MOB-101) anywhere in a
// string. Codes are per-project: a 1-10 char alphanumeric project code, a
// dash, and a task number. It matches any case, as the API does when it
// reads codes off branches and commits (feature/clk-12-timer is CLK-12);
// the canonical form is uppercase.
var CodeRegex = regexp.MustCompile(`(?i)\b[A-Z][A-Z0-9]{0,9}-\d+\b`)

// writtenCodeRegex is a code as DeskTick writes one: uppercase. Whether a
// message is already tagged is asked of it alone, since ordinary text is
// full of lowercase word-number pairs (utf-8, node-18, mt-4) that would
// otherwise stop the picked task's code being added.
var writtenCodeRegex = regexp.MustCompile(`\b[A-Z][A-Z0-9]{0,9}-\d+\b`)

// ExtractCode returns the first task code found in s, uppercased, or "" if
// none is present.
func ExtractCode(s string) string {
	return strings.ToUpper(CodeRegex.FindString(s))
}

// MessageNamesCode says whether s carries a code written as DeskTick writes
// one, which is what decides whether the picked task's code is added.
func MessageNamesCode(s string) bool {
	return writtenCodeRegex.MatchString(s)
}

// BranchPrefix returns the branch name prefix for a task code, e.g. "MOB-101/".
func BranchPrefix(code string) string {
	return code + "/"
}
