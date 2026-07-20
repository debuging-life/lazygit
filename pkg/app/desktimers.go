package app

import (
	"errors"
	"fmt"
	"io"
	"os"
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
	if latest, err := desktimers.LatestReleasedVersion(); err != nil {
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
		token, _ := desktimers.LoadToken()
		baseURL := desktimers.ResolveBaseURL(token)
		source := "default"
		switch {
		case os.Getenv("DESKTIMERS_API_URL") != "":
			source = "env DESKTIMERS_API_URL"
		case userConfig != nil && userConfig.Desktimers.ApiBaseUrl != "" && userConfig.Desktimers.ApiBaseUrl != desktimers.DefaultAPIBaseURL:
			source = "config"
		case token != nil && token.APIBaseURL != "" && token.APIBaseURL != desktimers.DefaultAPIBaseURL:
			source = "token file"
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
		client := desktimers.NewClient(desktimers.ResolveBaseURL(token), token.AccessToken)
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
