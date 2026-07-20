package config

import "testing"

// The deskgit defaults are load-bearing: 'auto' hook installation scoped to
// the work orgs, strict push on, update checks on.
func TestDesktimersConfigDefaults(t *testing.T) {
	cfg := GetDefaultConfig().Desktimers

	if cfg.AutoInstallHooks != "auto" {
		t.Errorf("autoInstallHooks default = %q, want auto", cfg.AutoInstallHooks)
	}
	wantOrgs := []string{"debuging-life", "loudowls"}
	if len(cfg.HookOrgs) != len(wantOrgs) {
		t.Fatalf("hookOrgs default = %v, want %v", cfg.HookOrgs, wantOrgs)
	}
	for i, org := range wantOrgs {
		if cfg.HookOrgs[i] != org {
			t.Errorf("hookOrgs[%d] = %q, want %q", i, cfg.HookOrgs[i], org)
		}
	}
	if !cfg.StrictPush {
		t.Error("strictPush should default to true")
	}
	if !cfg.CheckForUpdates {
		t.Error("checkForUpdates should default to true")
	}
	if !cfg.RequireTaskForCommit || !cfg.RequireTaskForBranch {
		t.Error("requireTaskForCommit/Branch should default to true")
	}
}
