package desktimers

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const stateFileName = "desktimers-task"

// State records the task currently selected for a repository. It is stored
// inside the git dir, so it is per-worktree and never committed.
type State struct {
	Code       string    `json:"code"`
	Title      string    `json:"title"`
	SelectedAt time.Time `json:"selectedAt"`
}

// gitDir resolves the absolute git dir for the repo at repoPath. Using
// --absolute-git-dir means each worktree gets its own state file.
func gitDir(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--absolute-git-dir")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolving git dir for %s: %w", repoPath, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func statePath(repoPath string) (string, error) {
	dir, err := gitDir(repoPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, stateFileName), nil
}

// LoadState returns the selected-task state for the repo at repoPath, or
// (nil, nil) when no task has been selected.
func LoadState(repoPath string) (*State, error) {
	path, err := statePath(repoPath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading state file: %w", err)
	}
	state := &State{}
	if err := json.Unmarshal(data, state); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}
	return state, nil
}

// SaveState writes the selected-task state for the repo at repoPath.
func SaveState(repoPath string, state *State) error {
	path, err := statePath(repoPath)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing state file: %w", err)
	}
	return nil
}

// ClearState removes the selected-task state; a missing file is not an error.
func ClearState(repoPath string) error {
	path, err := statePath(repoPath)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing state file: %w", err)
	}
	return nil
}
