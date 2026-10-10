package desktimers

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTokenRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// nothing stored yet
	token, err := LoadToken()
	if err != nil || token != nil {
		t.Fatalf("expected (nil, nil) for missing token, got (%+v, %v)", token, err)
	}

	saved := &Token{
		AccessToken: "dtg_secret",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second),
		Site:        "https://dev.desktick.com",
		UserID:      "u1",
		UserName:    "Ada",
	}
	if err := SaveToken(saved); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	path := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "deskgit", "token.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("token file missing: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file perms = %o, want 600", perm)
	}

	loaded, err := LoadToken()
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if loaded.AccessToken != saved.AccessToken || loaded.UserID != saved.UserID || loaded.UserName != saved.UserName ||
		loaded.Site != saved.Site || !loaded.ExpiresAt.Equal(saved.ExpiresAt) {
		t.Fatalf("loaded token mismatch: %+v", loaded)
	}

	if err := DeleteToken(); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}
	if token, err := LoadToken(); err != nil || token != nil {
		t.Fatalf("expected token deleted, got (%+v, %v)", token, err)
	}
}

func TestTokenValid(t *testing.T) {
	ours := func(change func(*Token)) *Token {
		token := &Token{AccessToken: "x", Site: "https://dev.desktick.com", UserID: "u1"}
		change(token)
		return token
	}
	if (*Token)(nil).Valid() {
		t.Error("nil token should be invalid")
	}
	if (&Token{}).Valid() {
		t.Error("empty token should be invalid")
	}
	if !ours(func(*Token) {}).Valid() {
		t.Error("token without expiry should be valid")
	}
	// A token file from the product before DeskTick has a token and nothing
	// that says whose it is or where from: it must not be offered here.
	if (&Token{AccessToken: "dtg_old"}).Valid() {
		t.Error("a token with no site and no user is the old product's: invalid")
	}
	if ours(func(t *Token) { t.Site = "" }).Valid() {
		t.Error("a token with no site should be invalid")
	}
	if ours(func(t *Token) { t.UserID = "" }).Valid() {
		t.Error("a token with no user should be invalid")
	}
	if !ours(func(t *Token) { t.ExpiresAt = time.Now().Add(time.Hour) }).Valid() {
		t.Error("unexpired token should be valid")
	}
	if ours(func(t *Token) { t.ExpiresAt = time.Now().Add(-time.Hour) }).Valid() {
		t.Error("expired token should be invalid")
	}
	if ours(func(t *Token) { t.ExpiresAt = time.Now().Add(30 * time.Second) }).Valid() {
		t.Error("token expiring within the 60s skew should be invalid")
	}
}

func TestResolveBaseURL(t *testing.T) {
	t.Setenv(SiteEnvVar, "")
	os.Unsetenv(SiteEnvVar)

	if got := ResolveBaseURL(); got != DefaultSiteURL {
		t.Errorf("default base URL = %q", got)
	}
	t.Setenv(SiteEnvVar, "http://localhost:3000/")
	if got := ResolveBaseURL(); got != "http://localhost:3000" {
		t.Errorf("env base URL = %q", got)
	}
}
