package desktimers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ManagedHooks are the git hooks deskgit installs.
var ManagedHooks = []string{"prepare-commit-msg", "pre-push"}

const hookMarkerPrefix = "# deskgit-hook"

func hookMarker() string {
	return fmt.Sprintf("%s v%d", hookMarkerPrefix, hookScriptVersion)
}

// HookStatus describes the install state of the deskgit hooks in a repo.
type HookStatus int

const (
	// HooksMissing: no deskgit hooks are installed.
	HooksMissing HookStatus = iota
	// HooksOutdated: deskgit hooks exist but are stale or incomplete.
	HooksOutdated
	// HooksInstalled: all deskgit hooks are installed and current.
	HooksInstalled
)

// ErrCustomHooksPath is returned when a repo sets core.hooksPath (e.g.
// Husky); deskgit never writes into a custom hooks directory.
type ErrCustomHooksPath struct {
	Path string
}

func (e *ErrCustomHooksPath) Error() string {
	return fmt.Sprintf(
		"repository uses a custom core.hooksPath (%s); add `dt-hook prepare-commit-msg` and `dt-hook pre-push` to those hooks manually",
		e.Path)
}

// customHooksPath returns the repo's core.hooksPath, or "" when unset.
func customHooksPath(repoPath string) (string, error) {
	cmd := exec.Command("git", "config", "core.hooksPath")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "", nil // unset
		}
		return "", fmt.Errorf("reading core.hooksPath: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// hooksDir resolves the directory git actually reads hooks from.
func hooksDir(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-path", "hooks")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolving hooks dir for %s: %w", repoPath, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func fileHasMarker(path string) (hasAny bool, current bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}
	content := string(data)
	if !strings.Contains(content, hookMarkerPrefix) {
		return false, false
	}
	return true, strings.Contains(content, hookMarker())
}

// HooksStatus reports whether the deskgit hooks are installed in the repo at
// repoPath. Returns *ErrCustomHooksPath when core.hooksPath is set.
func HooksStatus(repoPath string) (HookStatus, error) {
	if custom, err := customHooksPath(repoPath); err != nil {
		return HooksMissing, err
	} else if custom != "" {
		return HooksMissing, &ErrCustomHooksPath{Path: custom}
	}
	dir, err := hooksDir(repoPath)
	if err != nil {
		return HooksMissing, err
	}
	currentCount := 0
	anyCount := 0
	for _, name := range ManagedHooks {
		hasAny, isCurrent := fileHasMarker(filepath.Join(dir, name))
		if hasAny {
			anyCount++
		}
		if isCurrent {
			currentCount++
		}
	}
	switch {
	case currentCount == len(ManagedHooks):
		return HooksInstalled, nil
	case anyCount > 0:
		return HooksOutdated, nil
	default:
		return HooksMissing, nil
	}
}

// InstallHooks writes the deskgit hook scripts into the repo at repoPath,
// chaining any pre-existing hooks by renaming them to <name>.local.
// dtHookPath is the absolute path of the dt-hook binary to bake into the
// scripts.
func InstallHooks(repoPath string, dtHookPath string) error {
	if custom, err := customHooksPath(repoPath); err != nil {
		return err
	} else if custom != "" {
		return &ErrCustomHooksPath{Path: custom}
	}
	dir, err := hooksDir(repoPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating hooks dir: %w", err)
	}
	for _, name := range ManagedHooks {
		target := filepath.Join(dir, name)
		local := target + ".local"
		if hasAny, _ := fileHasMarker(target); !hasAny {
			// A pre-existing non-deskgit hook gets preserved and chained.
			if _, err := os.Stat(target); err == nil {
				if _, err := os.Stat(local); err == nil {
					return fmt.Errorf(
						"both %s and %s.local exist; move one aside before installing deskgit hooks", name, name)
				}
				if err := os.Rename(target, local); err != nil {
					return fmt.Errorf("preserving existing %s hook: %w", name, err)
				}
			}
		}
		chained := false
		if _, err := os.Stat(local); err == nil {
			chained = true
		}
		script := hookScript(name, dtHookPath, chained)
		if err := os.WriteFile(target, []byte(script), 0o755); err != nil {
			return fmt.Errorf("writing %s hook: %w", name, err)
		}
		if err := os.Chmod(target, 0o755); err != nil {
			return fmt.Errorf("marking %s hook executable: %w", name, err)
		}
	}
	return nil
}

// UninstallHooks removes the deskgit hook scripts, restoring any chained
// <name>.local hooks. Hooks not written by deskgit are left untouched.
func UninstallHooks(repoPath string) error {
	if custom, err := customHooksPath(repoPath); err != nil {
		return err
	} else if custom != "" {
		return &ErrCustomHooksPath{Path: custom}
	}
	dir, err := hooksDir(repoPath)
	if err != nil {
		return err
	}
	for _, name := range ManagedHooks {
		target := filepath.Join(dir, name)
		hasAny, _ := fileHasMarker(target)
		if !hasAny {
			continue
		}
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("removing %s hook: %w", name, err)
		}
		local := target + ".local"
		if _, err := os.Stat(local); err == nil {
			if err := os.Rename(local, target); err != nil {
				return fmt.Errorf("restoring original %s hook: %w", name, err)
			}
		}
	}
	return nil
}

// hookScript renders the shell script for a managed hook. The script must
// never break git: if dt-hook is missing it exits 0 silently.
func hookScript(name string, dtHookPath string, chained bool) string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "#!/bin/sh\n%s\n", hookMarker())
	fmt.Fprintf(b, "# Installed by deskgit (DeskTimers). Safe to delete; deskgit can reinstall it.\n\n")

	needsStdinCapture := chained && name == "pre-push"
	if needsStdinCapture {
		// pre-push receives the pushed refs on stdin; capture once so both
		// the chained hook and dt-hook see them.
		b.WriteString("dt_stdin=$(cat)\n\n")
	}

	if chained {
		b.WriteString("hook_dir=$(dirname \"$0\")\n")
		fmt.Fprintf(b, "if [ -x \"$hook_dir/%s.local\" ]; then\n", name)
		if needsStdinCapture {
			fmt.Fprintf(b, "  if [ -n \"$dt_stdin\" ]; then\n")
			fmt.Fprintf(b, "    printf '%%s\\n' \"$dt_stdin\" | \"$hook_dir/%s.local\" \"$@\" || exit $?\n", name)
			fmt.Fprintf(b, "  else\n")
			fmt.Fprintf(b, "    \"$hook_dir/%s.local\" \"$@\" </dev/null || exit $?\n", name)
			fmt.Fprintf(b, "  fi\n")
		} else {
			fmt.Fprintf(b, "  \"$hook_dir/%s.local\" \"$@\" || exit $?\n", name)
		}
		b.WriteString("fi\n\n")
	}

	fmt.Fprintf(b, "DT_HOOK=%q\n", dtHookPath)
	b.WriteString("if [ ! -x \"$DT_HOOK\" ]; then\n")
	b.WriteString("  DT_HOOK=$(command -v dt-hook 2>/dev/null) || exit 0\n")
	b.WriteString("fi\n")
	b.WriteString("[ -n \"$DT_HOOK\" ] || exit 0\n")

	if needsStdinCapture {
		fmt.Fprintf(b, "if [ -n \"$dt_stdin\" ]; then\n")
		fmt.Fprintf(b, "  printf '%%s\\n' \"$dt_stdin\" | \"$DT_HOOK\" %s \"$@\"\n", name)
		fmt.Fprintf(b, "else\n")
		fmt.Fprintf(b, "  \"$DT_HOOK\" %s \"$@\" </dev/null\n", name)
		fmt.Fprintf(b, "fi\n")
	} else {
		fmt.Fprintf(b, "exec \"$DT_HOOK\" %s \"$@\"\n", name)
	}
	return b.String()
}
