package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/jesseduffield/lazygit/pkg/desktimers"
)

const (
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiReset  = "\x1b[0m"
)

const maxListedCommits = 15

type unmappedCommit struct {
	sha     string
	subject string
}

// runPrePush reads the pushed ref ranges from stdin and warns about commits
// that carry no task code (or blocks them in strict mode). Internal git
// failures exit 0 — the hook must never break a push by accident.
func runPrePush(args []string, stdin io.Reader, stderr io.Writer, repoPath string) int {
	if len(args) < 1 {
		return 0
	}
	remoteName := args[0]

	unmapped := []unmappedCommit{}
	scanner := bufio.NewScanner(stdin)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 4 {
			continue
		}
		localSha, remoteRef, remoteSha := fields[1], fields[2], fields[3]
		if isZeroSha(localSha) {
			continue // branch deletion
		}
		if desktimers.ExtractCode(remoteRef) != "" {
			continue // the branch itself is mapped to a task
		}
		commits, err := newCommits(repoPath, localSha, remoteSha, remoteName)
		if err != nil {
			return 0
		}
		for _, sha := range commits {
			message, err := commitMessage(repoPath, sha)
			if err != nil {
				return 0
			}
			if desktimers.ExtractCode(message) == "" {
				unmapped = append(unmapped, unmappedCommit{
					sha:     shortSha(sha),
					subject: firstLine(message),
				})
			}
		}
	}
	if len(unmapped) == 0 {
		return 0
	}

	fmt.Fprintf(stderr, "%sdtgit: %d commit(s) being pushed have no DeskTimers task code:%s\n",
		ansiYellow, len(unmapped), ansiReset)
	for i, commit := range unmapped {
		if i == maxListedCommits {
			fmt.Fprintf(stderr, "  …and %d more\n", len(unmapped)-maxListedCommits)
			break
		}
		fmt.Fprintf(stderr, "  %s %s\n", commit.sha, commit.subject)
	}
	fmt.Fprintf(stderr, "%sSelect a task in dtgit (press 't') or include a task code like MOB-101 in the commit message.%s\n",
		ansiYellow, ansiReset)

	if strictMode(repoPath) {
		fmt.Fprintf(stderr, "%sdtgit: push blocked (strict mode): commits are missing task codes.%s\n",
			ansiRed, ansiReset)
		return 1
	}
	return 0
}

// newCommits lists the commits that would become reachable on the remote:
// everything reachable from localSha minus the remote tip (when it exists)
// and minus anything already on that remote.
func newCommits(repoPath string, localSha string, remoteSha string, remoteName string) ([]string, error) {
	args := []string{"rev-list", localSha, "--not"}
	if !isZeroSha(remoteSha) {
		args = append(args, remoteSha)
	}
	args = append(args, "--remotes="+remoteName)
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	shas := []string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			shas = append(shas, line)
		}
	}
	return shas, nil
}

func commitMessage(repoPath string, sha string) (string, error) {
	cmd := exec.Command("git", "log", "-1", "--format=%B", sha)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func strictMode(repoPath string) bool {
	if os.Getenv("DT_STRICT") == "1" {
		return true
	}
	cmd := exec.Command("git", "config", "--bool", "desktimers.strictpush")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

func isZeroSha(sha string) bool {
	for _, c := range sha {
		if c != '0' {
			return false
		}
	}
	return sha != ""
}

func shortSha(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
