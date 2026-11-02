package desktimers

import (
	"bytes"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func setupConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

func writeStoredToken(t *testing.T, token *Token) {
	t.Helper()
	if err := SaveToken(token); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureAuthenticatedWithValidToken(t *testing.T) {
	setupConfigDir(t)
	writeStoredToken(t, &Token{
		AccessToken: "dtg_abc",
		Site:        "https://dev.desktick.com",
		UserID:      "u1",
		ExpiresAt:   time.Now().Add(time.Hour),
	})

	var out bytes.Buffer
	result, err := EnsureAuthenticated(&out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != GateAuthenticated {
		t.Errorf("expected GateAuthenticated, got %v", result)
	}
	if out.Len() != 0 {
		t.Errorf("expected silent pass-through, got output: %s", out.String())
	}
}

// noBrowser is a sign-in nobody answers: the person is offline, or closed
// the tab.
func noBrowser(t *testing.T) {
	t.Helper()
	previous := signInWaitOverride
	signInWaitOverride = 50 * time.Millisecond
	t.Cleanup(func() { signInWaitOverride = previous })
}

func TestEnsureAuthenticatedOfflineWithExpiredToken(t *testing.T) {
	setupConfigDir(t)
	writeStoredToken(t, &Token{
		AccessToken: "dtg_abc",
		Site:        "https://dev.desktick.com",
		UserID:      "u1",
		ExpiresAt:   time.Now().Add(-time.Hour),
	})
	noBrowser(t)

	var out bytes.Buffer
	result, err := EnsureAuthenticated(&out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != GateOffline {
		t.Errorf("expected GateOffline, got %v", result)
	}
	if !strings.Contains(out.String(), "continuing offline") {
		t.Errorf("expected offline warning, got: %s", out.String())
	}
}

func TestEnsureAuthenticatedNoTokenAndNoSignInFails(t *testing.T) {
	setupConfigDir(t)
	noBrowser(t)

	var out bytes.Buffer
	_, err := EnsureAuthenticated(&out, nil)
	if err == nil {
		t.Fatal("expected an error when there is no token and nobody signs in")
	}
}

func TestEnsureAuthenticatedIgnoresATokenFromTheOldProduct(t *testing.T) {
	setupConfigDir(t)
	// What the product before DeskTick wrote: a token, and nothing else.
	writeStoredToken(t, &Token{AccessToken: "dtg_old", ExpiresAt: time.Now().Add(time.Hour)})
	noBrowser(t)

	var out bytes.Buffer
	result, err := EnsureAuthenticated(&out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result == GateAuthenticated {
		t.Error("the old product's token must not count as signed in")
	}
}

func TestEnsureAuthenticatedSignsIn(t *testing.T) {
	configDir := setupConfigDir(t)
	server := newSignInServer(t)
	t.Setenv(SiteEnvVar, server.URL)

	var out bytes.Buffer
	result, err := EnsureAuthenticated(&out, func(target string) error {
		server.browser(t, func(callback string, state string) []string {
			return []string{callback + "?code=the-code&state=" + url.QueryEscape(state)}
		})(target)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != GateAuthenticated {
		t.Errorf("expected GateAuthenticated, got %v", result)
	}
	if !strings.Contains(out.String(), server.URL+"/desktop-auth?") {
		t.Errorf("the address should be printed for a browser that did not open, got: %s", out.String())
	}

	// Token persisted with 0600.
	tokenFile := filepath.Join(configDir, "deskgit", "token.json")
	info, err := os.Stat(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("expected 0600 perms, got %v", info.Mode().Perm())
	}
	stored, _ := LoadToken()
	if !stored.Valid() || stored.UserID != "u1" {
		t.Errorf("stored token = %+v", stored)
	}
}

func TestSetConfiguredBaseURL(t *testing.T) {
	t.Cleanup(func() { SetConfiguredBaseURL("") })
	os.Unsetenv(SiteEnvVar)

	SetConfiguredBaseURL("https://custom.example.com/")
	if got := ResolveBaseURL(); got != "https://custom.example.com" {
		t.Errorf("config URL should win over default, got %s", got)
	}

	// Config left at the default (treated as "unset") → built-in default wins.
	SetConfiguredBaseURL(DefaultSiteURL)
	if got := ResolveBaseURL(); got != DefaultSiteURL {
		t.Errorf("default should win when config is unset, got %s", got)
	}

	// A config copied from the old README sample points at a retired host; it
	// must fall through to the current default, not pin the dead host.
	SetConfiguredBaseURL("https://api-leads.loudowls.com")
	if got := ResolveBaseURL(); got != DefaultSiteURL {
		t.Errorf("retired config host should fall through to default, got %s", got)
	}

	// A retired env var is ignored too.
	SetConfiguredBaseURL("")
	t.Setenv(SiteEnvVar, "https://api-leads.loudowls.com")
	if got := ResolveBaseURL(); got != DefaultSiteURL {
		t.Errorf("retired env host should fall through to default, got %s", got)
	}

	t.Setenv(SiteEnvVar, "http://env.example.com")
	SetConfiguredBaseURL("https://custom.example.com")
	if got := ResolveBaseURL(); got != "http://env.example.com" {
		t.Errorf("env should win over config, got %s", got)
	}
}
