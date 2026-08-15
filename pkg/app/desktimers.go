package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/jesseduffield/lazygit/pkg/config"
	"github.com/jesseduffield/lazygit/pkg/desktimers"
)

// LazygitBaseVersion is the upstream tag this fork is based on. Bump when
// rebasing onto a new upstream release.
const LazygitBaseVersion = "v0.63.1"

// runDesktimersGate blocks until this machine has a DeskTimers git-client
// token (running the device flow in plain terminal mode if needed). Offline
// with a previously stored token is allowed. DESKGIT_SKIP_AUTH=1 bypasses the
// gate entirely (CI, scripted use).
func runDesktimersGate(appConfig *config.AppConfig) error {
	if os.Getenv("DESKGIT_SKIP_AUTH") == "1" {
		return nil
	}

	desktimers.SetConfiguredBaseURL(appConfig.GetUserConfig().Desktimers.ApiBaseUrl)

	_, err := desktimers.EnsureAuthenticated(os.Stdout, desktimers.OpenBrowser)
	return err
}

// loadUserConfigForCLI best-effort loads the deskgit user config for the
// terminal subcommands (login/doctor) and applies the configured API base
// URL. Returns nil when the config can't be loaded.
func loadUserConfigForCLI() *config.UserConfig {
	appConfig, err := config.NewAppConfig("deskgit", "", "", "", "cli", false, os.TempDir())
	if err != nil {
		return nil
	}
	userConfig := appConfig.GetUserConfig()
	desktimers.SetConfiguredBaseURL(userConfig.Desktimers.ApiBaseUrl)
	return userConfig
}

// runDesktimersLogin implements `deskgit login`: drop any stored token and
// run the device flow in the terminal. Also used by the TUI's re-auth path
// (run as a subprocess while the gui is suspended).
func runDesktimersLogin(w io.Writer) {
	loadUserConfigForCLI()
	_ = desktimers.DeleteToken()

	if _, err := desktimers.RunDeviceFlow(w, desktimers.OpenBrowser); err != nil {
		fmt.Fprintf(w, "Login failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(w, "Logged in.")
}

// runDesktimersDoctor implements `deskgit doctor`: prints ✓/✗ diagnostics
// and returns the process exit code (1 when any check fails).
func runDesktimersDoctor(w io.Writer, version string) int {
	failed := false
	pass := func(format string, args ...any) { fmt.Fprintf(w, "✓ "+format+"\n", args...) }
	fail := func(format string, args ...any) { failed = true; fmt.Fprintf(w, "✗ "+format+"\n", args...) }

	userConfig := loadUserConfigForCLI()

	// Version.
	if latest, err := desktimers.LatestReleasedVersion(false); err != nil {
		pass("deskgit %s (update check failed: %v)", version, err)
	} else if desktimers.IsNewerVersion(latest, version) {
		pass("deskgit %s — v%s available, run: brew upgrade deskgit", version, latest)
	} else {
		pass("deskgit %s (up to date, latest release v%s)", version, latest)
	}

	// Config dir + effective API base URL and its source.
	configDir, err := desktimers.ConfigDir()
	if err != nil {
		fail("config dir: %v", err)
	} else {
		baseURL := desktimers.ResolveBaseURL()
		source := "default"
		switch {
		case os.Getenv("DESKTIMERS_API_URL") != "":
			source = "env DESKTIMERS_API_URL"
		case userConfig != nil && userConfig.Desktimers.ApiBaseUrl != "" && userConfig.Desktimers.ApiBaseUrl != desktimers.DefaultAPIBaseURL:
			source = "config"
		}
		pass("config: %s — apiBaseUrl %s (%s)", configDir, baseURL, source)
	}

	// Token + live API probe.
	token, err := desktimers.LoadToken()
	switch {
	case err != nil:
		fail("token: %v", err)
	case token == nil:
		fail("token: not logged in — run: deskgit login")
	case !token.Valid():
		fail("token: expired — run: deskgit login")
	default:
		client := desktimers.NewClient(desktimers.ResolveBaseURL(), token.AccessToken)
		client.HTTPClient.Timeout = 3 * time.Second
		tasks, err := client.GetTasks("active")
		switch {
		case errors.Is(err, desktimers.ErrUnauthorized):
			fail("token: invalid or revoked — run: deskgit login")
		case err != nil:
			fail("API: unreachable (%v)", err)
		default:
			pass("API ok, %d task(s) visible", len(tasks))
		}
	}

	// dt-hook binary.
	if path, ok := desktimers.FindDtHookBinary(); ok {
		pass("dt-hook: %s", path)
	} else {
		fail("dt-hook: not found next to deskgit or on PATH — hooks can't run")
	}

	// Repo-local checks (only when cwd is inside a git repo).
	if isRepo, _ := isDirectoryAGitRepository("."); isRepo {
		status, err := desktimers.HooksStatus(".")
		var customPathErr *desktimers.ErrCustomHooksPath
		switch {
		case errors.As(err, &customPathErr):
			fail("hooks: repo uses a custom core.hooksPath — add dt-hook to your hook manager manually")
		case err != nil:
			fail("hooks: %v", err)
		case status == desktimers.HooksInstalled:
			pass("hooks: installed")
		case status == desktimers.HooksOutdated:
			fail("hooks: outdated — open the repo in deskgit to update them")
		default:
			fail("hooks: not installed — open the repo in deskgit to install them")
		}

		strict, source := desktimers.EffectiveStrictPush(".")
		mode := "warn-only"
		if strict {
			mode = "strict"
		}
		pass("push mode: %s (%s)", mode, source)
	}

	if failed {
		return 1
	}
	return 0
}

// runDesktimersUpdate implements `deskgit update`: fetches the release
// manifest from R2, downloads a newer binary if one exists, verifies its
// checksum, and replaces the running executable in-place. On macOS the
// replacement is ad-hoc signed so Gatekeeper lets it run. Prints progress
// to w. Returns a non-zero exit code on hard failure.
func runDesktimersUpdate(w io.Writer, currentVersion string) int {
	fmt.Fprintf(w, "Checking for updates (current: %s)…\n", currentVersion)

	manifest, err := desktimers.FetchManifest()
	if err != nil {
		fmt.Fprintf(w, "Update check failed: %v\n", err)
		return 1
	}

	if !desktimers.IsNewerVersion(manifest.Version, currentVersion) {
		fmt.Fprintf(w, "Already up to date (%s).\n", currentVersion)
		return 0
	}

	fmt.Fprintf(w, "New version available: %s → %s\n", currentVersion, manifest.Version)

	key := desktimers.PlatformKey()
	asset, ok := manifest.Assets[key]
	if !ok {
		fmt.Fprintf(w, "No binary available for %s — build from source: https://github.com/debuging-life/lazygit\n", key)
		return 1
	}

	// Find the paths of the executables to replace.
	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(w, "Cannot determine executable path: %v\n", err)
		return 1
	}
	hookPath := execPath[:len(execPath)-len("deskgit")] + "dt-hook"

	// Download and verify the main binary.
	fmt.Fprintf(w, "Downloading deskgit %s for %s…\n", manifest.Version, key)
	tmpBin, err := desktimers.DownloadAndVerify(asset.URL, asset.SHA256)
	if err != nil {
		fmt.Fprintf(w, "Download failed: %v\n", err)
		return 1
	}
	defer os.Remove(tmpBin)

	// Download and verify dt-hook.
	var tmpHook string
	if asset.HookURL != "" && asset.HookSHA256 != "" {
		fmt.Fprintf(w, "Downloading dt-hook %s…\n", manifest.Version)
		tmpHook, err = desktimers.DownloadAndVerify(asset.HookURL, asset.HookSHA256)
		if err != nil {
			fmt.Fprintf(w, "dt-hook download failed: %v\n", err)
			return 1
		}
		defer os.Remove(tmpHook)
	}

	// Replace the binaries.
	fmt.Fprintf(w, "Installing…\n")
	if err := desktimers.ReplaceExecutable(tmpBin, execPath); err != nil {
		fmt.Fprintf(w, "Failed to replace deskgit: %v\n", err)
		fmt.Fprintf(w, "Try: sudo deskgit update\n")
		return 1
	}
	if tmpHook != "" {
		if err := desktimers.ReplaceExecutable(tmpHook, hookPath); err != nil {
			// Non-fatal: the main binary was already updated.
			fmt.Fprintf(w, "Warning: failed to replace dt-hook: %v\n", err)
		}
	}

	// Ad-hoc sign on macOS so Gatekeeper doesn't kill the replacement binary.
	if runtime.GOOS == "darwin" {
		for _, path := range []string{execPath, hookPath} {
			if err := exec.Command("codesign", "--sign", "-", "--force", path).Run(); err != nil {
				fmt.Fprintf(w, "Warning: codesign failed for %s: %v\n", path, err)
			}
		}
	}

	fmt.Fprintf(w, "Updated to deskgit %s. Restart to use the new version.\n", manifest.Version)
	return 0
}

// runDesktimersLogout implements `deskgit logout`: best-effort server-side
// token revoke, then local token removal. Never exits non-zero — logout is
// idempotent.
func runDesktimersLogout(w io.Writer) {
	outcome, err := desktimers.Logout()
	if err != nil {
		fmt.Fprintf(w, "Could not remove the local login: %v\n", err)
		return
	}

	switch outcome.Result {
	case desktimers.LogoutRevoked:
		fmt.Fprintln(w, "Logged out (token revoked).")
	case desktimers.LogoutLocalOnly:
		fmt.Fprintf(w, "Logged out locally (could not reach the server to revoke: %v). You can also revoke it in DeskTimers → Settings → Git Clients.\n", outcome.RevokeErr)
	default:
		fmt.Fprintln(w, "Not logged in.")
	}
}
