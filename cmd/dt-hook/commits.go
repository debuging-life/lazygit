package main

import (
	"os"
	"os/exec"

	"github.com/jesseduffield/lazygit/pkg/desktimers"
)

// runPostCommit queues the commit just made for DeskTick's task cards and
// sends it in the background, so the commit returns at once and a slow or
// absent network costs it nothing. A replayed commit is not new work: the
// push that carries it is what tells DeskTick.
func runPostCommit(repoPath string) {
	if desktimers.ReplayInProgress(repoPath) {
		return
	}
	commit, err := desktimers.ReadCommit(repoPath, "HEAD")
	if err != nil {
		return
	}
	// The picked task only for a commit whose message names none: one that
	// names another task was meant for that one (prepare-commit-msg left it
	// alone for that reason), and the API reads codes from both.
	if state, err := desktimers.LoadState(repoPath); err == nil && state != nil &&
		desktimers.ExtractCode(commit.Message) == "" {
		commit.PickedCode = state.Code
	}
	if desktimers.QueueCommit(repoPath, commit) != nil {
		return
	}
	sendCommitsInBackground()
}

// sendCommitsInBackground starts `dt-hook send-commits` and does not wait
// for it. Its output goes nowhere: a program reading git's output until it
// ends (deskgit does) must not wait on it either.
var sendCommitsInBackground = func() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "send-commits")
	// Its own session (a console of its own nowhere, on Windows), so the
	// end of the terminal or pty git ran in does not end it mid-request.
	cmd.SysProcAttr = detached()
	if cmd.Start() == nil {
		_ = cmd.Process.Release()
	}
}

// runSendCommits sends what is waiting. Signed out, it waits too: deskgit
// asks the person to sign in, and sends on its next try.
func runSendCommits(repoPath string) {
	client, err := desktimers.NewClientFromToken()
	if err != nil {
		return
	}
	_ = desktimers.SendQueuedCommits(repoPath, client)
}
