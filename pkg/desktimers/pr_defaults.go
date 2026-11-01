package desktimers

import (
	"os"
	"path/filepath"
	"strings"
)

// pullRequestTemplatePaths are where GitHub looks for a repository's pull
// request template, in its order.
var pullRequestTemplatePaths = []string{
	".github/pull_request_template.md",
	".github/PULL_REQUEST_TEMPLATE.md",
	"pull_request_template.md",
	"PULL_REQUEST_TEMPLATE.md",
	"docs/pull_request_template.md",
	"docs/PULL_REQUEST_TEMPLATE.md",
}

// PullRequestTemplate is the repository's pull request template, or "".
func PullRequestTemplate(repoPath string) string {
	root, err := gitOutput(repoPath, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	root = strings.TrimSpace(root)
	for _, path := range pullRequestTemplatePaths {
		if data, err := os.ReadFile(filepath.Join(root, path)); err == nil {
			return strings.TrimRight(string(data), "\n")
		}
	}
	return ""
}

// PullRequestText is what the form starts with.
type PullRequestText struct {
	Title string
	Body  string
	// FromTemplate says the body is the repository's template.
	FromTemplate bool
}

// branchCommit is one commit a pull request would carry.
type branchCommit struct {
	Subject string
	Body    string
}

// commitsAhead are the commits on branch that baseRef lacks, oldest first,
// or just branch's own commit when baseRef cannot be compared with.
func commitsAhead(repoPath string, branch string, baseRef string) []branchCommit {
	format := "--format=%s%x1f%b%x1e"
	out, err := gitOutput(repoPath, "log", "--reverse", "--no-show-signature", format, baseRef+".."+branch)
	if err != nil {
		out, err = gitOutput(repoPath, "log", "-1", "--no-show-signature", format, branch)
		if err != nil {
			return nil
		}
	}
	commits := []branchCommit{}
	for _, record := range strings.Split(out, "\x1e") {
		record = strings.Trim(record, "\n")
		if record == "" {
			continue
		}
		subject, body, _ := strings.Cut(record, "\x1f")
		commits = append(commits, branchCommit{Subject: subject, Body: strings.TrimSpace(body)})
	}
	return commits
}

// DefaultPullRequestText fills in the form from what is known: the task
// (the branch's code, or the task picked), the commits the pull request
// carries, and the repository's template.
//
//   - The title is the task's ("CLK-12/Show the clock face") when the task
//     is known; otherwise the one commit's subject, given the code if it
//     lacks one; otherwise the branch's name.
//   - The body is the template when there is one; otherwise the one
//     commit's body, or a line for each commit. A link to the task goes at
//     the end.
func DefaultPullRequestText(repoPath string, branch string, baseRef string, picked *State, taskURL string) PullRequestText {
	code := ExtractCode(branch)
	if code == "" && picked != nil {
		code = picked.Code
	}
	commits := commitsAhead(repoPath, branch, baseRef)

	text := PullRequestText{}
	switch {
	case picked != nil && picked.Code == code && code != "" && picked.Title != "":
		text.Title = BranchPrefix(code) + picked.Title
	case len(commits) == 1:
		text.Title = commits[0].Subject
		if code != "" && !strings.Contains(strings.ToUpper(text.Title), code) {
			text.Title = BranchPrefix(code) + text.Title
		}
	default:
		text.Title = branch
	}

	if template := PullRequestTemplate(repoPath); template != "" {
		text.Body = template
		text.FromTemplate = true
	} else if len(commits) == 1 {
		text.Body = commits[0].Body
	} else if len(commits) > 1 {
		lines := make([]string, len(commits))
		for i, commit := range commits {
			lines[i] = "- " + commit.Subject
		}
		text.Body = strings.Join(lines, "\n")
	}
	if taskURL != "" && !strings.Contains(text.Body, taskURL) {
		if text.Body != "" {
			text.Body += "\n\n"
		}
		text.Body += "Task: " + taskURL
	}
	return text
}
