package desktimers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestEnsureAuthenticatedOfflineWithExpiredToken(t *testing.T) {
	setupConfigDir(t)
	writeStoredToken(t, &Token{
		AccessToken: "dtg_abc",
		ExpiresAt:   time.Now().Add(-time.Hour),
	})
	// Unreachable API → device flow fails → offline grace.
	t.Setenv("DESKTIMERS_API_URL", "http://127.0.0.1:1")

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

func TestEnsureAuthenticatedNoTokenUnreachableAPIFails(t *testing.T) {
	setupConfigDir(t)
	t.Setenv("DESKTIMERS_API_URL", "http://127.0.0.1:1")

	var out bytes.Buffer
	_, err := EnsureAuthenticated(&out, nil)
	if err == nil {
		t.Fatal("expected an error when no token exists and the API is down")
	}
}

func TestEnsureAuthenticatedRunsDeviceFlow(t *testing.T) {
	configDir := setupConfigDir(t)

	authorized := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/device/code":
			json.NewEncoder(w).Encode(map[string]any{
				"device_code":      "dev123",
				"user_code":        "ABCD-EFGH",
				"verification_uri": "http://example.com/device",
				"expires_in":       900,
				"interval":         0,
			})
		case "/api/device/token":
			if !authorized {
				authorized = true
				w.WriteHeader(http.StatusPreconditionRequired)
				json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "dtg_new",
				"token_type":   "Bearer",
				"expires_in":   3600,
				"plan":         "free",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("DESKTIMERS_API_URL", server.URL)

	browserOpened := ""
	var out bytes.Buffer
	result, err := EnsureAuthenticated(&out, func(url string) error {
		browserOpened = url
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != GateAuthenticated {
		t.Errorf("expected GateAuthenticated, got %v", result)
	}
	if !strings.Contains(out.String(), "ABCD-EFGH") {
		t.Errorf("expected the user code to be printed, got: %s", out.String())
	}
	if browserOpened == "" {
		t.Error("expected the browser opener to be invoked")
	}

	// Token persisted with 0600.
	tokenFile := filepath.Join(configDir, "deskgit", "token.json")
	info, err := os.Stat(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected 0600 perms, got %v", info.Mode().Perm())
	}
}

func TestSetConfiguredBaseURL(t *testing.T) {
	t.Cleanup(func() { SetConfiguredBaseURL("") })
	os.Unsetenv("DESKTIMERS_API_URL")

	SetConfiguredBaseURL("https://staging.desktimers.com/")
	if got := ResolveBaseURL(nil); got != "https://staging.desktimers.com" {
		t.Errorf("config URL should win over default, got %s", got)
	}

	// The default value acts as "unset" so a token's URL can still apply.
	SetConfiguredBaseURL(DefaultAPIBaseURL)
	token := &Token{APIBaseURL: "https://selfhosted.example.com"}
	if got := ResolveBaseURL(token); got != "https://selfhosted.example.com" {
		t.Errorf("token URL should apply when config is default, got %s", got)
	}

	t.Setenv("DESKTIMERS_API_URL", "http://env.example.com")
	SetConfiguredBaseURL("https://staging.desktimers.com")
	if got := ResolveBaseURL(nil); got != "http://env.example.com" {
		t.Errorf("env should win over config, got %s", got)
	}
}
