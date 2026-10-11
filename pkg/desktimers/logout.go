package desktimers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// RevokeToken signs this session out on the server, so the token is dead
// there and not only forgotten here. A 401 means it was already dead, which
// is the outcome asked for.
func (c *Client) RevokeToken() error {
	err := c.call(http.MethodPost, "/auth/sign-out", struct{}{}, nil)
	if err == nil || errors.Is(err, ErrUnauthorized) {
		return nil
	}
	return fmt.Errorf("signing out: %w", err)
}

// LogoutResult describes how Logout resolved.
type LogoutResult int

const (
	// LogoutNotLoggedIn: there was no stored token; nothing to do.
	LogoutNotLoggedIn LogoutResult = iota
	// LogoutRevoked: the token was revoked server-side and deleted locally.
	LogoutRevoked
	// LogoutLocalOnly: the local token was deleted but the server could not
	// be reached to revoke it; RevokeErr holds the reason.
	LogoutLocalOnly
)

// LogoutOutcome is the result of a Logout call.
type LogoutOutcome struct {
	Result    LogoutResult
	RevokeErr error // set when Result == LogoutLocalOnly
}

// Logout logs this machine out: best-effort server-side revoke, then delete
// the local token file. The returned error is only for local failures
// (deleting the token file); server unreachability degrades to
// LogoutLocalOnly instead.
func Logout() (LogoutOutcome, error) {
	token, err := LoadToken()
	if err != nil {
		// A corrupt token file is still a login artifact — clear it.
		if delErr := DeleteToken(); delErr != nil {
			return LogoutOutcome{}, delErr
		}
		return LogoutOutcome{Result: LogoutLocalOnly, RevokeErr: err}, nil
	}
	if token == nil || token.AccessToken == "" {
		return LogoutOutcome{Result: LogoutNotLoggedIn}, nil
	}

	outcome := LogoutOutcome{Result: LogoutRevoked}
	// Revoke against the API that ISSUED the token — that's where it's valid —
	// which may differ from the currently active API (e.g. after the default
	// moved). Fall back to the active URL for older tokens with no stored host.
	revokeURL := strings.TrimSuffix(token.Site, "/")
	if revokeURL == "" {
		revokeURL = ResolveBaseURL()
	}
	client := NewClient(revokeURL, token.AccessToken)
	if revokeErr := client.RevokeToken(); revokeErr != nil {
		outcome = LogoutOutcome{Result: LogoutLocalOnly, RevokeErr: revokeErr}
	}

	if err := DeleteToken(); err != nil {
		return LogoutOutcome{}, err
	}
	return outcome, nil
}
