package app

import (
	"os"

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
