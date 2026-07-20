package desktimers

import (
	"os/exec"
	"testing"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestEffectiveStrictPush(t *testing.T) {
	repo := initTestRepo(t)

	t.Run("default is warn-only", func(t *testing.T) {
		t.Setenv("DT_STRICT", "")
		strict, source := EffectiveStrictPush(repo)
		if strict || source != "default" {
			t.Errorf("got strict=%v source=%q, want false/default", strict, source)
		}
	})

	t.Run("git config turns it on", func(t *testing.T) {
		t.Setenv("DT_STRICT", "")
		if err := SyncStrictPushConfig(repo, true); err != nil {
			t.Fatal(err)
		}
		strict, source := EffectiveStrictPush(repo)
		if !strict || source != "git config" {
			t.Errorf("got strict=%v source=%q, want true/git config", strict, source)
		}
	})

	t.Run("env off beats git config on", func(t *testing.T) {
		t.Setenv("DT_STRICT", "0")
		strict, source := EffectiveStrictPush(repo)
		if strict || source != "env" {
			t.Errorf("got strict=%v source=%q, want false/env", strict, source)
		}
	})

	t.Run("env true accepted", func(t *testing.T) {
		t.Setenv("DT_STRICT", "true")
		strict, source := EffectiveStrictPush(repo)
		if !strict || source != "env" {
			t.Errorf("got strict=%v source=%q, want true/env", strict, source)
		}
	})
}

func TestSyncStrictPushConfig(t *testing.T) {
	repo := initTestRepo(t)
	t.Setenv("DT_STRICT", "")

	if err := SyncStrictPushConfig(repo, true); err != nil {
		t.Fatal(err)
	}
	if strict, _ := EffectiveStrictPush(repo); !strict {
		t.Error("sync(true) should enable strict via git config")
	}

	// Re-sync with the same value is a no-op (no error).
	if err := SyncStrictPushConfig(repo, true); err != nil {
		t.Fatal(err)
	}

	if err := SyncStrictPushConfig(repo, false); err != nil {
		t.Fatal(err)
	}
	if strict, _ := EffectiveStrictPush(repo); strict {
		t.Error("sync(false) should disable strict")
	}
}
