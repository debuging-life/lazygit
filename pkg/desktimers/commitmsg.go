package desktimers

import "strings"

// commitSourcesToSkip are prepare-commit-msg sources that must never be
// rewritten: merges, squashes, and -c/-C/--amend reuse an existing message.
var commitSourcesToSkip = map[string]bool{
	"merge":  true,
	"squash": true,
	"commit": true,
}

// PrefixCommitMessage prepends "<code>/" to a commit message file's content
// when appropriate (matching the picker's commit format, e.g.
// "LOUD-124/fix images"). It returns the (possibly rewritten) content and
// whether it changed. The message is left alone when code is empty, the
// commit source reuses an existing message, or the message already carries a
// task code.
func PrefixCommitMessage(content string, code string, source string) (string, bool) {
	if code == "" || commitSourcesToSkip[source] {
		return content, false
	}
	if messageHasCode(content) {
		return content, false
	}
	return ApplyPrefixTemplate(DefaultCommitPrefixTemplate, DefaultCommitPrefixTemplate, code) + content, true
}

// messageHasCode reports whether the non-comment portion of a commit message
// already contains a task code.
func messageHasCode(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if CodeRegex.MatchString(line) {
			return true
		}
	}
	return false
}
