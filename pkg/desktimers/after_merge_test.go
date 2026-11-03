package desktimers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommitsOnlyHere(t *testing.T) {
	remote := t.TempDir()
	runGit(t, remote, "init", "--bare")
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "commit", "--allow-empty", "-m", "first")
	runGit(t, repo, "remote", "add", "origin", remote)
	runGit(t, repo, "checkout", "-b", "feature/CLK-1-x")
	runGit(t, repo, "commit", "--allow-empty", "-m", "CLK-1/work")
	runGit(t, repo, "push", "-u", "origin", "feature/CLK-1-x")

	count, err := CommitsOnlyHere(repo, "feature/CLK-1-x", "origin/feature/CLK-1-x")
	assert.NoError(t, err)
	assert.Equal(t, 0, count)

	assert.NoError(t, os.WriteFile(filepath.Join(repo, "late"), []byte("x"), 0o644))
	runGit(t, repo, "add", "late")
	runGit(t, repo, "commit", "-m", "CLK-1/after the review")
	count, err = CommitsOnlyHere(repo, "feature/CLK-1-x", "origin/feature/CLK-1-x")
	assert.NoError(t, err)
	assert.Equal(t, 1, count)

	_, err = CommitsOnlyHere(repo, "feature/CLK-1-x", "origin/gone")
	assert.Error(t, err)
}
