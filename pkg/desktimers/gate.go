package desktimers

import (
	"fmt"
	"io"
)

// GateResult describes how the launch auth gate resolved.
type GateResult int

const (
	// GateAuthenticated: a valid token is present (possibly just obtained).
	GateAuthenticated GateResult = iota
	// GateOffline: no valid token, but a stored (possibly expired) one
	// exists and the API is unreachable — proceed with cached data.
	GateOffline
)

// EnsureAuthenticated is the launch gate: returns GateAuthenticated when a
// valid token exists or the device flow succeeds. When the API is unreachable
// but a token file exists (even expired), it returns GateOffline so the app
// can start with cached state. Only when there is no token at all and the
// flow fails does it return an error.
func EnsureAuthenticated(w io.Writer, openBrowser func(url string) error) (GateResult, error) {
	stored, err := LoadToken()
	if err != nil {
		// A corrupt token file shouldn't brick the app; re-auth replaces it.
		fmt.Fprintf(w, "Warning: %v — re-authenticating.\n", err)
		stored = nil
	}
	if stored.Valid() {
		return GateAuthenticated, nil
	}

	if stored != nil {
		fmt.Fprintln(w, "Your DeskTimers session has expired — please re-authenticate.")
	} else {
		fmt.Fprintln(w, "Welcome to dtgit! Connect it to your DeskTimers account.")
	}

	if _, err := RunDeviceFlow(w, openBrowser); err != nil {
		if stored != nil {
			// Offline grace: keep working with the stale token's cached
			// context; task picking will fail but git operations won't.
			fmt.Fprintf(w, "Warning: could not reach DeskTimers (%v) — continuing offline.\n", err)
			return GateOffline, nil
		}
		return 0, fmt.Errorf("dtgit requires a DeskTimers login: %w", err)
	}

	fmt.Fprintln(w, "Device connected — happy shipping!")
	return GateAuthenticated, nil
}
