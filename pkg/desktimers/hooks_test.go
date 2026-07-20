package desktimers

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallHooksFresh(t *testing.T) {
	repo := initRepo(t)

	if err := InstallHooks(repo, "/usr/local/bin/dt-hook"); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}

	for _, name := range ManagedHooks {
		path := filepath.Join(repo, ".git", "hooks", name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s hook missing: %v", name, err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("%s hook is not executable", name)
		}
		content, _ := os.ReadFile(path)
		if !strings.Contains(string(content), hookMarker()) {
			t.Errorf("%s hook missing marker", name)
		}
		if !strings.Contains(string(content), `"/usr/local/bin/dt-hook"`) {
			t.Errorf("%s hook missing baked-in dt-hook path:\n%s", name, content)
		}
		if !strings.Contains(string(content), name) {
			t.Errorf("%s hook does not invoke its subcommand", name)
		}
	}

	status, err := HooksStatus(repo)
	if err != nil || status != HooksInstalled {
		t.Fatalf("HooksStatus = (%v, %v), want installed", status, err)
	}
}

func TestInstallHooksChainsExistingHook(t *testing.T) {
	repo := initRepo(t)
	hookPath := filepath.Join(repo, ".git", "hooks", "pre-push")
	original := "#!/bin/sh\necho custom hook\n"
	if err := os.WriteFile(hookPath, []byte(original), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := InstallHooks(repo, "/usr/local/bin/dt-hook"); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}

	localContent, err := os.ReadFile(hookPath + ".local")
	if err != nil {
		t.Fatalf("existing hook was not preserved as .local: %v", err)
	}
	if string(localContent) != original {
		t.Errorf("preserved hook content changed: %q", localContent)
	}

	script, _ := os.ReadFile(hookPath)
	if !strings.Contains(string(script), "pre-push.local") {
		t.Errorf("generated script does not chain the .local hook:\n%s", script)
	}
	if !strings.Contains(string(script), "dt_stdin=$(cat)") {
		t.Errorf("chained pre-push script must capture stdin once:\n%s", script)
	}
}

func TestInstallHooksCustomHooksPath(t *testing.T) {
	repo := initRepo(t)
	runGit(t, repo, "config", "core.hooksPath", ".husky")

	err := InstallHooks(repo, "/usr/local/bin/dt-hook")
	customErr := &ErrCustomHooksPath{}
	if !errors.As(err, &customErr) || customErr.Path != ".husky" {
		t.Fatalf("expected ErrCustomHooksPath(.husky), got %v", err)
	}

	if _, err := os.Stat(filepath.Join(repo, ".git", "hooks", "pre-push")); err == nil {
		t.Error("hooks were written despite custom core.hooksPath")
	}
}

func TestUninstallHooksRestoresChainedHook(t *testing.T) {
	repo := initRepo(t)
	hookPath := filepath.Join(repo, ".git", "hooks", "pre-push")
	original := "#!/bin/sh\necho custom hook\n"
	if err := os.WriteFile(hookPath, []byte(original), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := InstallHooks(repo, "/usr/local/bin/dt-hook"); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}

	if err := UninstallHooks(repo); err != nil {
		t.Fatalf("UninstallHooks: %v", err)
	}

	restored, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("original hook not restored: %v", err)
	}
	if string(restored) != original {
		t.Errorf("restored hook content changed: %q", restored)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "hooks", "prepare-commit-msg")); err == nil {
		t.Error("prepare-commit-msg hook still present after uninstall")
	}
	if _, err := os.Stat(hookPath + ".local"); err == nil {
		t.Error(".local file still present after uninstall")
	}

	status, err := HooksStatus(repo)
	if err != nil || status != HooksMissing {
		t.Fatalf("HooksStatus after uninstall = (%v, %v), want missing", status, err)
	}
}

func TestHooksStatusOutdated(t *testing.T) {
	repo := initRepo(t)
	if err := InstallHooks(repo, "/usr/local/bin/dt-hook"); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}
	stale := "#!/bin/sh\n# deskgit-hook v0\nexec dt-hook pre-push \"$@\"\n"
	if err := os.WriteFile(filepath.Join(repo, ".git", "hooks", "pre-push"), []byte(stale), 0o755); err != nil {
		t.Fatal(err)
	}

	status, err := HooksStatus(repo)
	if err != nil || status != HooksOutdated {
		t.Fatalf("HooksStatus = (%v, %v), want outdated", status, err)
	}

	// reinstall upgrades in place without inventing a .local file
	if err := InstallHooks(repo, "/usr/local/bin/dt-hook"); err != nil {
		t.Fatalf("re-InstallHooks: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "hooks", "pre-push.local")); err == nil {
		t.Error("upgrade wrongly preserved a deskgit script as .local")
	}
	status, err = HooksStatus(repo)
	if err != nil || status != HooksInstalled {
		t.Fatalf("HooksStatus after upgrade = (%v, %v), want installed", status, err)
	}
}

func TestUninstallLeavesForeignHooksAlone(t *testing.T) {
	repo := initRepo(t)
	hookPath := filepath.Join(repo, ".git", "hooks", "pre-push")
	original := "#!/bin/sh\necho custom hook\n"
	if err := os.WriteFile(hookPath, []byte(original), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := UninstallHooks(repo); err != nil {
		t.Fatalf("UninstallHooks: %v", err)
	}
	content, err := os.ReadFile(hookPath)
	if err != nil || string(content) != original {
		t.Fatalf("foreign hook was touched: (%q, %v)", content, err)
	}
}
