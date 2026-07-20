package desktimers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func revokeServer(t *testing.T, status int) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/git-client/token" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer dtg_abc" {
			t.Errorf("missing bearer token, got %q", r.Header.Get("Authorization"))
		}
		calls++
		w.WriteHeader(status)
		if status == http.StatusOK {
			w.Write([]byte(`{"success":true,"data":{"message":"Token revoked"}}`))
		}
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func TestRevokeToken(t *testing.T) {
	t.Run("200 succeeds", func(t *testing.T) {
		server, calls := revokeServer(t, http.StatusOK)
		if err := NewClient(server.URL, "dtg_abc").RevokeToken(); err != nil {
			t.Fatal(err)
		}
		if *calls != 1 {
			t.Errorf("expected 1 call, got %d", *calls)
		}
	})

	t.Run("401 counts as success", func(t *testing.T) {
		server, _ := revokeServer(t, http.StatusUnauthorized)
		if err := NewClient(server.URL, "dtg_abc").RevokeToken(); err != nil {
			t.Fatalf("401 should be treated as already-revoked, got %v", err)
		}
	})

	t.Run("500 is an error", func(t *testing.T) {
		server, _ := revokeServer(t, http.StatusInternalServerError)
		if err := NewClient(server.URL, "dtg_abc").RevokeToken(); err == nil {
			t.Fatal("expected an error for status 500")
		}
	})

	t.Run("network error is an error", func(t *testing.T) {
		if err := NewClient("http://127.0.0.1:1", "dtg_abc").RevokeToken(); err == nil {
			t.Fatal("expected a network error")
		}
	})
}

func tokenFileExists(t *testing.T) bool {
	t.Helper()
	token, err := LoadToken()
	if err != nil {
		t.Fatal(err)
	}
	return token != nil
}

func TestLogout(t *testing.T) {
	t.Run("no token → not logged in", func(t *testing.T) {
		setupConfigDir(t)
		outcome, err := Logout()
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Result != LogoutNotLoggedIn {
			t.Errorf("expected LogoutNotLoggedIn, got %v", outcome.Result)
		}
	})

	t.Run("revokes server-side and deletes locally", func(t *testing.T) {
		setupConfigDir(t)
		server, calls := revokeServer(t, http.StatusOK)
		writeStoredToken(t, &Token{
			AccessToken: "dtg_abc",
			ExpiresAt:   time.Now().Add(time.Hour),
			APIBaseURL:  server.URL,
		})
		os.Unsetenv("DESKTIMERS_API_URL")

		outcome, err := Logout()
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Result != LogoutRevoked {
			t.Errorf("expected LogoutRevoked, got %v (revokeErr: %v)", outcome.Result, outcome.RevokeErr)
		}
		if *calls != 1 {
			t.Errorf("expected 1 revoke call, got %d", *calls)
		}
		if tokenFileExists(t) {
			t.Error("token file should be deleted")
		}
	})

	t.Run("offline still deletes the local token", func(t *testing.T) {
		setupConfigDir(t)
		writeStoredToken(t, &Token{
			AccessToken: "dtg_abc",
			ExpiresAt:   time.Now().Add(time.Hour),
			APIBaseURL:  "http://127.0.0.1:1",
		})
		os.Unsetenv("DESKTIMERS_API_URL")

		outcome, err := Logout()
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Result != LogoutLocalOnly {
			t.Errorf("expected LogoutLocalOnly, got %v", outcome.Result)
		}
		if outcome.RevokeErr == nil {
			t.Error("expected RevokeErr to carry the network error")
		}
		if tokenFileExists(t) {
			t.Error("token file should be deleted even when the server is unreachable")
		}
	})

	t.Run("expired token still triggers revoke attempt", func(t *testing.T) {
		setupConfigDir(t)
		server, calls := revokeServer(t, http.StatusUnauthorized)
		writeStoredToken(t, &Token{
			AccessToken: "dtg_abc",
			ExpiresAt:   time.Now().Add(-time.Hour),
			APIBaseURL:  server.URL,
		})
		os.Unsetenv("DESKTIMERS_API_URL")

		outcome, err := Logout()
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Result != LogoutRevoked {
			t.Errorf("expected LogoutRevoked (401 = already dead), got %v", outcome.Result)
		}
		if *calls != 1 {
			t.Errorf("expected 1 revoke call, got %d", *calls)
		}
		if tokenFileExists(t) {
			t.Error("token file should be deleted")
		}
	})
}
