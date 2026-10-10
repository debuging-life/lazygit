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

// DefaultSiteURL is the DeskTick this build talks to: the web app's address.
// The API is the same address under /api (the web app proxies it), which is
// how the desktop app reaches it too. It is the dev site while the product is
// only being tested there; a release for everybody changes this one line.
const DefaultSiteURL = "https://dev.desktick.com"

// retiredHosts are addresses deskgit used to point at. They belonged to the
// product before DeskTick and serve nothing deskgit can use, so a config or
// env value naming one is treated as unset (isRetiredBaseURL) and falls
// through to the default.
var retiredHosts = []string{
	"api-leads.loudowls.com",
	"desktimers.com",
}

func isRetiredBaseURL(url string) bool {
	for _, h := range retiredHosts {
		if strings.Contains(url, h) {
			return true
		}
	}
	return false
}

const tokenFileName = "token.json"

// Token is the stored git-client credential for a machine.
type Token struct {
	AccessToken string    `json:"accessToken"`
	TokenType   string    `json:"tokenType"`
	ExpiresAt   time.Time `json:"expiresAt"`
	// Site is the DeskTick the token was issued by. A token file without it
	// was written for the product before DeskTick: it is not valid here.
	Site string `json:"site,omitempty"`
	// Who the token is, as the sign-in said. UserID is what "my tasks" and
	// "my projects" are asked for by.
	UserID   string `json:"userId,omitempty"`
	UserName string `json:"userName,omitempty"`
}

// Valid reports whether the token can be used: non-empty and not within 60s
// of its expiry.
func (t *Token) Valid() bool {
	if t == nil || t.AccessToken == "" || t.Site == "" || t.UserID == "" {
		return false
	}
	if t.ExpiresAt.IsZero() {
		return true
	}
	return time.Now().Add(60 * time.Second).Before(t.ExpiresAt)
}

// ConfigDir returns the deskgit config directory, honoring XDG_CONFIG_HOME.
func ConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "deskgit"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".config", "deskgit"), nil
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

// ResolveBaseURL picks the DeskTick site: the DESKTICK_URL env var first,
// then the user config's desktimers.url, otherwise the built-in default.
//
// A stored token deliberately does not pin the address. It used to, and a
// token minted against an older default then kept overriding a newer one.
func ResolveBaseURL() string {
	if env := strings.TrimSuffix(os.Getenv(SiteEnvVar), "/"); env != "" && !isRetiredBaseURL(env) {
		return env
	}
	if configuredBaseURL != "" {
		return configuredBaseURL
	}
	return DefaultSiteURL
}

// SiteEnvVar overrides the site for one run, e.g. a local web app:
// DESKTICK_URL=http://localhost:5174 deskgit
const SiteEnvVar = "DESKTICK_URL"
