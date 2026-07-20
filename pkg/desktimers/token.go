package desktimers

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultAPIBaseURL is the production DeskTimers API.
const DefaultAPIBaseURL = "https://api.desktimers.com"

const tokenFileName = "token.json"

// Token is the stored git-client credential for a machine.
type Token struct {
	AccessToken string    `json:"accessToken"`
	TokenType   string    `json:"tokenType"`
	ExpiresAt   time.Time `json:"expiresAt"`
	Plan        string    `json:"plan"`
	APIBaseURL  string    `json:"apiBaseUrl,omitempty"`
}

// Valid reports whether the token can be used: non-empty and not within 60s
// of its expiry.
func (t *Token) Valid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	if t.ExpiresAt.IsZero() {
		return true
	}
	return time.Now().Add(60 * time.Second).Before(t.ExpiresAt)
}

// ConfigDir returns the dtgit config directory, honoring XDG_CONFIG_HOME.
func ConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "dtgit"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".config", "dtgit"), nil
}

func tokenPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, tokenFileName), nil
}

// LoadToken returns the stored token, or (nil, nil) when none is stored.
func LoadToken() (*Token, error) {
	path, err := tokenPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading token file: %w", err)
	}
	token := &Token{}
	if err := json.Unmarshal(data, token); err != nil {
		return nil, fmt.Errorf("parsing token file: %w", err)
	}
	return token, nil
}

// SaveToken stores the token with owner-only permissions.
func SaveToken(token *Token) error {
	path, err := tokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding token: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing token file: %w", err)
	}
	// os.WriteFile only applies the mode on creation; enforce it for
	// pre-existing files too.
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("setting token file permissions: %w", err)
	}
	return nil
}

// DeleteToken removes the stored token; a missing file is not an error.
func DeleteToken() error {
	path, err := tokenPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing token file: %w", err)
	}
	return nil
}

// ResolveBaseURL picks the API base URL: DESKTIMERS_API_URL env var first,
// then the user config's desktimers.apiBaseUrl, then the stored token's
// apiBaseUrl, then the production default.
func ResolveBaseURL(token *Token) string {
	if env := os.Getenv("DESKTIMERS_API_URL"); env != "" {
		return strings.TrimSuffix(env, "/")
	}
	if configuredBaseURL != "" {
		return configuredBaseURL
	}
	if token != nil && token.APIBaseURL != "" {
		return strings.TrimSuffix(token.APIBaseURL, "/")
	}
	return DefaultAPIBaseURL
}
