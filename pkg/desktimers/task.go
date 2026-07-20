package desktimers

import (
	"regexp"
	"strings"
)

// Task is a DeskTimers task as returned by the git-client API.
type Task struct {
	Code    string `json:"code"`
	Title   string `json:"title"`
	Project string `json:"project"`
	Status  string `json:"status"`
}

// CodeRegex matches a DeskTimers task code (e.g. MOB-101) anywhere in a
// string. Codes are per-project: a 1-10 char uppercase alphanumeric project
// code, a dash, and a task number. Matching is case-insensitive; canonical
// form is uppercase.
var CodeRegex = regexp.MustCompile(`(?i)\b[A-Z][A-Z0-9]{0,9}-\d+\b`)

// ExtractCode returns the first task code found in s, uppercased, or "" if
// none is present.
func ExtractCode(s string) string {
	match := CodeRegex.FindString(s)
	if match == "" {
		return ""
	}
	return strings.ToUpper(match)
}

// BranchPrefix returns the branch name prefix for a task code, e.g. "MOB-101/".
func BranchPrefix(code string) string {
	return code + "/"
}
