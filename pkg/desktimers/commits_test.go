package desktimers

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{
		"-c", "user.name=Ada Lovelace", "-c", "user.email=ada@example.com",
		"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main",
	}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if !assert.NoError(t, err, "git %v\n%s", args, out) {
		t.FailNow()
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir string, name string, content string) {
	t.Helper()
	if !assert.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)) {
		t.FailNow()
	}
	if !assert.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)) {
		t.FailNow()
	}
}

func newCommitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init")
	git(t, repo, "remote", "add", "origin", "git@github.com:acme/clock.git")
	return repo
}

func ints(n int) *int { return &n }

func TestReadCommit(t *testing.T) {
	repo := newCommitRepo(t)
	write(t, repo, "kept.txt", "one\ntwo\n")
	write(t, repo, "moved.txt", "a\nb\nc\nd\ne\n")
	write(t, repo, "gone.txt", "bye\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "start")

	write(t, repo, "kept.txt", "one\nthree\nfour\n")
	if !assert.NoError(t, os.Mkdir(filepath.Join(repo, "dir"), 0o755)) {
		t.FailNow()
	}
	git(t, repo, "mv", "moved.txt", "dir/moved.txt")
	git(t, repo, "rm", "-q", "gone.txt")
	write(t, repo, "new.bin", "\x00\x01\x02")
	git(t, repo, "add", ".")
	git(t, repo, "checkout", "-q", "-b", "CLK-12/clock-face")
	git(t, repo, "commit", "-m", "CLK-12/move the face\n\nAnd tidy up.")

	commit, err := ReadCommit(repo, "HEAD")
	if !assert.NoError(t, err) {
		t.FailNow()
	}

	assert.Equal(t, "acme/clock", commit.Repo)
	assert.Equal(t, git(t, repo, "rev-parse", "HEAD"), commit.Sha)
	assert.Len(t, commit.Sha, 40)
	if !assert.NotNil(t, commit.Branch) {
		t.FailNow()
	}
	assert.Equal(t, "CLK-12/clock-face", *commit.Branch)
	assert.Equal(t, "CLK-12/move the face\n\nAnd tidy up.", commit.Message)
	assert.Equal(t, "Ada Lovelace", commit.AuthorName)
	assert.Equal(t, "ada@example.com", commit.AuthorEmail)
	assert.Regexp(t, `^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d([+-]\d\d:\d\d|Z)$`, commit.AuthoredAt)
	assert.False(t, commit.IsMerge)
	assert.Zero(t, commit.FilesTotal)
	assert.ElementsMatch(t, []CommitFile{
		{Path: "kept.txt", Status: "M", Additions: ints(2), Deletions: ints(1)},
		{Path: "dir/moved.txt", PreviousPath: "moved.txt", Status: "R", Additions: ints(0), Deletions: ints(0)},
		{Path: "gone.txt", Status: "D", Additions: ints(0), Deletions: ints(1)},
		{Path: "new.bin", Status: "A"},
	}, commit.Files)
}

func TestReadCommitRootDetachedAndMerge(t *testing.T) {
	repo := newCommitRepo(t)
	write(t, repo, "a.txt", "a\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "root")

	root, err := ReadCommit(repo, "HEAD")
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	assert.Equal(t, []CommitFile{{Path: "a.txt", Status: "A", Additions: ints(1), Deletions: ints(0)}}, root.Files)

	git(t, repo, "checkout", "-q", "-b", "side")
	write(t, repo, "b.txt", "b\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "side")
	git(t, repo, "checkout", "-q", "main")
	write(t, repo, "c.txt", "c\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "main")
	git(t, repo, "merge", "-q", "--no-edit", "side")

	merge, err := ReadCommit(repo, "HEAD")
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	assert.True(t, merge.IsMerge)
	assert.Equal(t, []CommitFile{}, merge.Files)

	git(t, repo, "checkout", "-q", "--detach")
	detached, err := ReadCommit(repo, "HEAD")
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	assert.Nil(t, detached.Branch)
}

func TestReadCommitSendsAtMostThreeHundredFiles(t *testing.T) {
	repo := newCommitRepo(t)
	for i := range 301 {
		write(t, repo, fmt.Sprintf("f%03d.txt", i), "x\n")
	}
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "lots")

	commit, err := ReadCommit(repo, "HEAD")
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	assert.Len(t, commit.Files, 300)
	assert.Equal(t, 301, commit.FilesTotal)
}

func TestReadCommitNeedsAnOrigin(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init")
	git(t, repo, "commit", "--allow-empty", "-m", "x")
	_, err := ReadCommit(repo, "HEAD")
	assert.Error(t, err)
}

// queueCommits puts count made-up commits in a repo's queue.
func queueCommits(t *testing.T, repo string, count int) {
	t.Helper()
	dir, err := commitQueuePath(repo)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	for i := range count {
		err := queueCommitIn(dir, Commit{Repo: "acme/clock", Sha: fmt.Sprintf("%040x", i+1), Files: []CommitFile{}})
		if !assert.NoError(t, err) {
			t.FailNow()
		}
	}
}

func TestQueueCommitKeepsACommitOnce(t *testing.T) {
	repo := newCommitRepo(t)
	queueCommits(t, repo, 1)
	queueCommits(t, repo, 1)
	assert.Len(t, QueuedCommits(repo), 1)
}

func TestQueueCommitKeepsTheNewest(t *testing.T) {
	repo := newCommitRepo(t)
	queueCommits(t, repo, queuedCommitsKept+3)
	waiting := QueuedCommits(repo)
	if !assert.Len(t, waiting, queuedCommitsKept) {
		t.FailNow()
	}
	assert.Equal(t, fmt.Sprintf("%040x", 4), waiting[0].Sha)
}

func TestSendQueuedCommits(t *testing.T) {
	for _, test := range []struct {
		name string
		// answer is the status a batch of commits gets.
		answer      func(commits []any) int
		wantErr     error
		wantBatches []int
		wantLeft    int
	}{
		{
			name:        "taken: forgotten, fifty at a time",
			wantBatches: []int{50, 50, 20},
		},
		{
			name:        "signed out: kept",
			answer:      func([]any) int { return http.StatusUnauthorized },
			wantErr:     ErrUnauthorized,
			wantBatches: nil, // a 401 is answered before the body is read
			wantLeft:    120,
		},
		{
			name: "one malformed: split until it is alone, the rest sent",
			answer: func(commits []any) int {
				for _, commit := range commits {
					if commit.(map[string]any)["sha"] == fmt.Sprintf("%040x", 7) {
						return http.StatusBadRequest
					}
				}
				return 200
			},
			wantBatches: []int{50, 25, 12, 6, 6, 3, 1, 2, 3, 13, 25, 50, 20},
		},
		{
			name: "too big: split until it fits",
			answer: func(commits []any) int {
				if len(commits) > 20 {
					return http.StatusRequestEntityTooLarge
				}
				return 200
			},
			wantBatches: []int{50, 25, 12, 13, 25, 12, 13, 50, 25, 12, 13, 25, 12, 13, 20},
		},
		{
			name:        "down: kept, and nothing more tried",
			answer:      func([]any) int { return http.StatusBadGateway },
			wantBatches: []int{50},
			wantLeft:    120,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			api := newFakeDeskTick(t)
			api.commitAnswer = test.answer
			client := api.client()
			if test.wantErr == ErrUnauthorized {
				client.AccessToken = "ended"
			}
			repo := newCommitRepo(t)
			queueCommits(t, repo, 120)

			err := SendQueuedCommits(repo, client)

			if test.wantErr != nil {
				assert.ErrorIs(t, err, test.wantErr)
			}
			batches := []int{}
			for _, batch := range api.commitBatches {
				batches = append(batches, len(batch))
			}
			if test.wantBatches == nil {
				test.wantBatches = []int{}
			}
			assert.Equal(t, test.wantBatches, batches)
			assert.Len(t, QueuedCommits(repo), test.wantLeft)
		})
	}
}

func TestSendQueuedCommitsSendsWhatWasRead(t *testing.T) {
	api := newFakeDeskTick(t)
	repo := newCommitRepo(t)
	write(t, repo, "a.txt", "a\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "CLK-12/start")
	commit, err := ReadCommit(repo, "HEAD")
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	commit.PickedCode = "CLK-12"
	if !assert.NoError(t, QueueCommit(repo, commit)) {
		t.FailNow()
	}

	if !assert.NoError(t, SendQueuedCommits(repo, api.client())) {
		t.FailNow()
	}

	if !assert.Len(t, api.commitBatches, 1) {
		t.FailNow()
	}
	sent := api.commitBatches[0][0].(map[string]any)
	assert.Equal(t, "acme/clock", sent["repo"])
	assert.Equal(t, commit.Sha, sent["sha"])
	assert.Equal(t, "main", sent["branch"])
	assert.Equal(t, "CLK-12/start", sent["message"])
	assert.Equal(t, "CLK-12", sent["pickedCode"])
	assert.Equal(t, "ada@example.com", sent["authorEmail"])
	assert.NotContains(t, sent, "isMerge")
	assert.Equal(t, []any{map[string]any{
		"path": "a.txt", "status": "A", "additions": float64(1), "deletions": float64(0),
	}}, sent["files"])
	assert.Empty(t, QueuedCommits(repo))
}

func TestReadCommitOfASignedCommitWithShowSignature(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen is needed to sign a commit")
	}
	repo := newCommitRepo(t)
	key := filepath.Join(t.TempDir(), "key")
	out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput()
	if !assert.NoError(t, err, "%s", out) {
		t.FailNow()
	}
	git(t, repo, "config", "gpg.format", "ssh")
	git(t, repo, "config", "user.signingkey", key+".pub")
	git(t, repo, "config", "log.showSignature", "true")
	write(t, repo, "a.txt", "a\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-S", "-m", "signed")

	commit, err := ReadCommit(repo, "HEAD")
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	assert.Equal(t, git(t, repo, "rev-parse", "HEAD"), commit.Sha)
	assert.Equal(t, "signed", commit.Message)
}

func TestReadCommitRefusesASha256Repository(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "--object-format=sha256")
	git(t, repo, "remote", "add", "origin", "git@github.com:acme/clock.git")
	git(t, repo, "commit", "--allow-empty", "-m", "x")
	_, err := ReadCommit(repo, "HEAD")
	assert.Error(t, err)
}

func TestTruncateUTF8(t *testing.T) {
	assert.Equal(t, "ab", truncateUTF8("ab", 5))
	// "é" is two bytes: a cut through it drops it whole.
	assert.Equal(t, "ab", truncateUTF8("abé", 3))
	assert.Equal(t, "abé", truncateUTF8("abéz", 4))
	// A byte that is not UTF-8 early on is kept, and so is what follows it.
	assert.Equal(t, "a\xe9bcd", truncateUTF8("a\xe9bcdef", 5))
}

func TestQueueIsSharedByEveryWorktree(t *testing.T) {
	repo := newCommitRepo(t)
	git(t, repo, "commit", "--allow-empty", "-m", "start")
	other := filepath.Join(t.TempDir(), "other")
	git(t, repo, "worktree", "add", "-q", "-b", "side", other)

	queueCommits(t, other, 1)

	assert.Len(t, QueuedCommits(repo), 1)
}
