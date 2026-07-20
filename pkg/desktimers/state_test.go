package desktimers

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	fullArgs := append([]string{
		"-c", "user.name=Test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main",
	}, args...)
	cmd := exec.Command("git", fullArgs...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	return dir
}

func TestStateRoundTrip(t *testing.T) {
	repo := initRepo(t)

	// no state yet
	state, err := LoadState(repo)
	if err != nil {
		t.Fatalf("LoadState on empty repo: %v", err)
	}
	if state != nil {
		t.Fatalf("expected nil state, got %+v", state)
	}

	selected := &State{Code: "MOB-101", Title: "Fix login redirect", SelectedAt: time.Now().UTC().Truncate(time.Second)}
	if err := SaveState(repo, selected); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	loaded, err := LoadState(repo)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if loaded == nil || loaded.Code != "MOB-101" || loaded.Title != "Fix login redirect" || !loaded.SelectedAt.Equal(selected.SelectedAt) {
		t.Fatalf("loaded state mismatch: %+v", loaded)
	}

	if err := ClearState(repo); err != nil {
		t.Fatalf("ClearState: %v", err)
	}
	if state, err := LoadState(repo); err != nil || state != nil {
		t.Fatalf("expected cleared state, got (%+v, %v)", state, err)
	}
	// clearing again is fine
	if err := ClearState(repo); err != nil {
		t.Fatalf("ClearState on missing file: %v", err)
	}
}

func TestStateIsPerWorktree(t *testing.T) {
	repo := initRepo(t)
	runGit(t, repo, "commit", "--allow-empty", "-m", "initial")

	worktree := filepath.Join(t.TempDir(), "wt")
	runGit(t, repo, "worktree", "add", "-b", "side", worktree)

	if err := SaveState(worktree, &State{Code: "DES-9", Title: "Side work", SelectedAt: time.Now()}); err != nil {
		t.Fatalf("SaveState in worktree: %v", err)
	}

	// the worktree's state lands in its own git dir, not the main one
	mainState, err := LoadState(repo)
	if err != nil {
		t.Fatalf("LoadState in main repo: %v", err)
	}
	if mainState != nil {
		t.Fatalf("main repo unexpectedly sees worktree state: %+v", mainState)
	}
	wtState, err := LoadState(worktree)
	if err != nil || wtState == nil || wtState.Code != "DES-9" {
		t.Fatalf("worktree state mismatch: (%+v, %v)", wtState, err)
	}

	gitDir, err := gitDir(worktree)
	if err != nil {
		t.Fatalf("gitDir: %v", err)
	}
	if !strings.Contains(gitDir, filepath.Join(".git", "worktrees")) {
		t.Fatalf("expected worktree git dir under .git/worktrees, got %s", gitDir)
	}
}
