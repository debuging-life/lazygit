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
		Plan:        "paid",
		APIBaseURL:  "https://staging.desktimers.com",
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
	if loaded.AccessToken != saved.AccessToken || loaded.Plan != saved.Plan ||
		loaded.APIBaseURL != saved.APIBaseURL || !loaded.ExpiresAt.Equal(saved.ExpiresAt) {
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
	if (*Token)(nil).Valid() {
		t.Error("nil token should be invalid")
	}
	if (&Token{}).Valid() {
		t.Error("empty token should be invalid")
	}
	if !(&Token{AccessToken: "x"}).Valid() {
		t.Error("token without expiry should be valid")
	}
	if !(&Token{AccessToken: "x", ExpiresAt: time.Now().Add(time.Hour)}).Valid() {
		t.Error("unexpired token should be valid")
	}
	if (&Token{AccessToken: "x", ExpiresAt: time.Now().Add(-time.Hour)}).Valid() {
		t.Error("expired token should be invalid")
	}
	if (&Token{AccessToken: "x", ExpiresAt: time.Now().Add(30 * time.Second)}).Valid() {
		t.Error("token expiring within the 60s skew should be invalid")
	}
}

func TestResolveBaseURL(t *testing.T) {
	t.Setenv("DESKTIMERS_API_URL", "")
	os.Unsetenv("DESKTIMERS_API_URL")

	if got := ResolveBaseURL(nil); got != DefaultAPIBaseURL {
		t.Errorf("default base URL = %q", got)
	}
	if got := ResolveBaseURL(&Token{APIBaseURL: "https://staging.desktimers.com/"}); got != "https://staging.desktimers.com" {
		t.Errorf("token base URL = %q", got)
	}
	t.Setenv("DESKTIMERS_API_URL", "http://localhost:3000/")
	if got := ResolveBaseURL(&Token{APIBaseURL: "https://staging.desktimers.com"}); got != "http://localhost:3000" {
		t.Errorf("env base URL = %q", got)
	}
}
