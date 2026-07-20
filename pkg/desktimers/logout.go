package desktimers

import (
	"fmt"
	"net/http"
)

// RevokeToken revokes this device's token server-side
// (DELETE /api/git-client/token). A 401 counts as success — the token is
// already dead as far as the server is concerned.
func (c *Client) RevokeToken() error {
	req, err := http.NewRequest(http.MethodDelete, c.BaseURL+"/api/git-client/token", nil)
	if err != nil {
		return fmt.Errorf("building revoke request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("revoking token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
		return nil
	}
	return fmt.Errorf("revoking token: unexpected status %d", resp.StatusCode)
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
	client := NewClient(ResolveBaseURL(token), token.AccessToken)
	if revokeErr := client.RevokeToken(); revokeErr != nil {
		outcome = LogoutOutcome{Result: LogoutLocalOnly, RevokeErr: revokeErr}
	}

	if err := DeleteToken(); err != nil {
		return LogoutOutcome{}, err
	}
	return outcome, nil
}
