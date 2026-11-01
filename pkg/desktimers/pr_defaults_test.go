package desktimers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// newBranchRepo is a repository with main at one commit and the branch
// checked out on top of it.
func newBranchRepo(t *testing.T, branch string) string {
	t.Helper()
	repo := newCommitRepo(t)
	git(t, repo, "commit", "--allow-empty", "-m", "start")
	git(t, repo, "checkout", "-q", "-b", branch)
	return repo
}

func TestDefaultPullRequestTextFromTheTask(t *testing.T) {
	repo := newBranchRepo(t, "CLK-12/face")
	git(t, repo, "commit", "--allow-empty", "-m", "CLK-12/draw the hands")
	git(t, repo, "commit", "--allow-empty", "-m", "CLK-12/draw the numbers")

	text := DefaultPullRequestText(repo, "CLK-12/face", "main",
		&State{Code: "CLK-12", Title: "Show the clock face"}, "https://dev.desktick.com/t/12")

	assert.Equal(t, "CLK-12/Show the clock face", text.Title)
	assert.Equal(t, "- CLK-12/draw the hands\n- CLK-12/draw the numbers\n\nTask: https://dev.desktick.com/t/12", text.Body)
	assert.False(t, text.FromTemplate)
}

func TestDefaultPullRequestTextFromTheOneCommit(t *testing.T) {
	repo := newBranchRepo(t, "clk-12-face")
	git(t, repo, "commit", "--allow-empty", "-m", "Draw the hands\n\nThey were missing.")

	// The task picked is another one: the branch's code wins, and with no
	// title of its own the commit's subject is used, given the code.
	text := DefaultPullRequestText(repo, "clk-12-face", "main", &State{Code: "MOB-1", Title: "Other"}, "")

	assert.Equal(t, "CLK-12/Draw the hands", text.Title)
	assert.Equal(t, "They were missing.", text.Body)
}

func TestDefaultPullRequestTextWithoutATask(t *testing.T) {
	repo := newBranchRepo(t, "tidy")
	git(t, repo, "commit", "--allow-empty", "-m", "one")
	git(t, repo, "commit", "--allow-empty", "-m", "two")

	text := DefaultPullRequestText(repo, "tidy", "main", nil, "")

	assert.Equal(t, "tidy", text.Title)
	assert.Equal(t, "- one\n- two", text.Body)
}

func TestDefaultPullRequestTextUsesTheTemplate(t *testing.T) {
	repo := newBranchRepo(t, "CLK-12/face")
	write(t, repo, ".github/pull_request_template.md", "## What\n\n## Why\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "CLK-12/add template")

	text := DefaultPullRequestText(repo, "CLK-12/face", "main", nil, "https://dev.desktick.com/t/12")

	assert.Equal(t, "CLK-12/add template", text.Title)
	assert.Equal(t, "## What\n\n## Why\n\nTask: https://dev.desktick.com/t/12", text.Body)
	assert.True(t, text.FromTemplate)
}

func TestDefaultPullRequestTextWhenTheBaseIsUnknown(t *testing.T) {
	repo := newBranchRepo(t, "CLK-12/face")
	git(t, repo, "commit", "--allow-empty", "-m", "CLK-12/draw")

	text := DefaultPullRequestText(repo, "CLK-12/face", "origin/nowhere", nil, "")

	// What the branch carries is not known, so no one commit stands for it.
	assert.Equal(t, "CLK-12/face", text.Title)
	assert.Empty(t, text.Body)
}
