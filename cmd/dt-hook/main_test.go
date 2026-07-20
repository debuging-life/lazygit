package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
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

const zeroSha = "0000000000000000000000000000000000000000"

// setupRepo creates a repo with a remote configured and two commits: one
// tagged with a task code, one without. Returns repo path and head sha.
func setupRepo(t *testing.T) (string, string) {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "remote", "add", "origin", t.TempDir())
	runGit(t, repo, "commit", "--allow-empty", "-m", "MOB-101: tagged commit")
	runGit(t, repo, "commit", "--allow-empty", "-m", "untagged commit")
	head := runGit(t, repo, "rev-parse", "HEAD")
	return repo, head
}

func TestPrePushWarnsOnUnmappedCommits(t *testing.T) {
	repo, head := setupRepo(t)

	stdin := strings.NewReader("refs/heads/feature " + head + " refs/heads/feature " + zeroSha + "\n")
	stderr := &bytes.Buffer{}
	code := runPrePush([]string{"origin", "ignored-url"}, stdin, stderr, repo)

	if code != 0 {
		t.Fatalf("warn mode must exit 0, got %d", code)
	}
	out := stderr.String()
	if !strings.Contains(out, "1 commit(s)") {
		t.Errorf("expected exactly one unmapped commit in warning:\n%s", out)
	}
	if !strings.Contains(out, "untagged commit") {
		t.Errorf("warning does not list the unmapped commit:\n%s", out)
	}
	if strings.Contains(out, "tagged commit\n") && strings.Contains(out, "MOB-101: tagged commit") {
		t.Errorf("warning wrongly lists the mapped commit:\n%s", out)
	}
}

func TestPrePushBranchNameCoversCommits(t *testing.T) {
	repo, head := setupRepo(t)

	stdin := strings.NewReader("refs/heads/MOB-101/feature " + head + " refs/heads/MOB-101/feature " + zeroSha + "\n")
	stderr := &bytes.Buffer{}
	code := runPrePush([]string{"origin", "ignored-url"}, stdin, stderr, repo)

	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("task-coded branch should push silently, got code=%d stderr=%q", code, stderr.String())
	}
}

func TestPrePushStrictModeBlocks(t *testing.T) {
	repo, head := setupRepo(t)
	t.Setenv("DT_STRICT", "1")

	stdin := strings.NewReader("refs/heads/feature " + head + " refs/heads/feature " + zeroSha + "\n")
	stderr := &bytes.Buffer{}
	code := runPrePush([]string{"origin", "ignored-url"}, stdin, stderr, repo)

	if code != 1 {
		t.Fatalf("strict mode must exit 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "push blocked") {
		t.Errorf("strict mode should explain the block:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "DT_STRICT=0 git push") {
		t.Errorf("strict block should mention the escape hatch:\n%s", stderr.String())
	}
}

func TestPrePushEnvOverridesGitConfigOff(t *testing.T) {
	repo, head := setupRepo(t)
	runGit(t, repo, "config", "desktimers.strictpush", "true")
	t.Setenv("DT_STRICT", "0")

	stdin := strings.NewReader("refs/heads/feature " + head + " refs/heads/feature " + zeroSha + "\n")
	stderr := &bytes.Buffer{}
	code := runPrePush([]string{"origin", "ignored-url"}, stdin, stderr, repo)

	if code != 0 {
		t.Fatalf("DT_STRICT=0 must override git config strict, got exit %d", code)
	}
	if !strings.Contains(stderr.String(), "no DeskTimers task code") {
		t.Errorf("warning should still print in override mode:\n%s", stderr.String())
	}
}

func TestPrePushStrictViaGitConfig(t *testing.T) {
	repo, head := setupRepo(t)
	runGit(t, repo, "config", "desktimers.strictpush", "true")

	stdin := strings.NewReader("refs/heads/feature " + head + " refs/heads/feature " + zeroSha + "\n")
	code := runPrePush([]string{"origin", "ignored-url"}, stdin, &bytes.Buffer{}, repo)
	if code != 1 {
		t.Fatalf("desktimers.strictpush=true must exit 1, got %d", code)
	}
}

func TestPrePushSkipsBranchDeletion(t *testing.T) {
	repo, _ := setupRepo(t)

	stdin := strings.NewReader("(delete) " + zeroSha + " refs/heads/feature deadbeefdeadbeefdeadbeefdeadbeefdeadbeef\n")
	stderr := &bytes.Buffer{}
	code := runPrePush([]string{"origin", "ignored-url"}, stdin, stderr, repo)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("deletion should be ignored, got code=%d stderr=%q", code, stderr.String())
	}
}

func TestPrePushAllCommitsMapped(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "remote", "add", "origin", t.TempDir())
	runGit(t, repo, "commit", "--allow-empty", "-m", "MOB-101: only commit")
	head := runGit(t, repo, "rev-parse", "HEAD")

	stdin := strings.NewReader("refs/heads/feature " + head + " refs/heads/feature " + zeroSha + "\n")
	stderr := &bytes.Buffer{}
	code := runPrePush([]string{"origin", "ignored-url"}, stdin, stderr, repo)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("fully mapped push should be silent, got code=%d stderr=%q", code, stderr.String())
	}
}
