package desktimers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestAuthClient(baseURL string) (*AuthClient, *[]time.Duration) {
	sleeps := &[]time.Duration{}
	auth := NewAuthClient(baseURL)
	auth.DeviceName = "test-machine"
	auth.sleep = func(ctx context.Context, d time.Duration) error {
		*sleeps = append(*sleeps, d)
		return ctx.Err()
	}
	return auth, sleeps
}

func TestRequestDeviceCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/device/code" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"device_code":               "dev123",
			"user_code":                 "ABCD-EFGH",
			"verification_uri":          "https://app.desktimers.com/device",
			"verification_uri_complete": "https://app.desktimers.com/device?code=ABCD-EFGH",
			"expires_in":                900,
			"interval":                  5,
		})
	}))
	defer server.Close()

	auth, _ := newTestAuthClient(server.URL)
	code, err := auth.RequestDeviceCode(context.Background())
	if err != nil {
		t.Fatalf("RequestDeviceCode: %v", err)
	}
	if code.DeviceCode != "dev123" || code.UserCode != "ABCD-EFGH" || code.Interval != 5 || code.ExpiresIn != 900 {
		t.Fatalf("unexpected device code: %+v", code)
	}
}

func TestPollForTokenPendingThenSuccess(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/device/token" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		body := map[string]string{}
		json.NewDecoder(r.Body).Decode(&body)
		if body["device_code"] != "dev123" || body["device_name"] != "test-machine" {
			t.Errorf("unexpected body: %v", body)
		}
		switch calls.Add(1) {
		case 1, 2:
			w.WriteHeader(http.StatusPreconditionRequired)
			json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
		default:
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "dtg_secret",
				"token_type":   "Bearer",
				"expires_in":   3600,
				"plan":         "paid",
			})
		}
	}))
	defer server.Close()

	auth, sleeps := newTestAuthClient(server.URL)
	token, err := auth.PollForToken(context.Background(), "dev123", 5, 900)
	if err != nil {
		t.Fatalf("PollForToken: %v", err)
	}
	if token.AccessToken != "dtg_secret" || token.Plan != "paid" || token.TokenType != "Bearer" {
		t.Fatalf("unexpected token: %+v", token)
	}
	if token.ExpiresAt.Before(time.Now().Add(59 * time.Minute)) {
		t.Errorf("expiry not derived from expires_in: %v", token.ExpiresAt)
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 polls, got %d", calls.Load())
	}
	for _, d := range *sleeps {
		if d != 5*time.Second {
			t.Errorf("expected 5s sleeps, got %v", *sleeps)
			break
		}
	}
}

func TestPollForTokenSlowDown(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]string{"error": "slow_down"})
		default:
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "dtg_secret", "token_type": "Bearer", "expires_in": 3600, "plan": "free",
			})
		}
	}))
	defer server.Close()

	auth, sleeps := newTestAuthClient(server.URL)
	if _, err := auth.PollForToken(context.Background(), "dev123", 5, 900); err != nil {
		t.Fatalf("PollForToken: %v", err)
	}
	want := []time.Duration{5 * time.Second, 10 * time.Second}
	if len(*sleeps) != len(want) || (*sleeps)[0] != want[0] || (*sleeps)[1] != want[1] {
		t.Fatalf("slow_down did not bump the interval: sleeps = %v", *sleeps)
	}
}

func TestPollForTokenExpired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "expired_token"})
	}))
	defer server.Close()

	auth, _ := newTestAuthClient(server.URL)
	_, err := auth.PollForToken(context.Background(), "dev123", 5, 900)
	authErr := &AuthError{}
	if !errors.As(err, &authErr) || authErr.Code != "expired_token" {
		t.Fatalf("expected AuthError(expired_token), got %v", err)
	}
}

func TestPollForTokenAccessDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "access_denied"})
	}))
	defer server.Close()

	auth, _ := newTestAuthClient(server.URL)
	_, err := auth.PollForToken(context.Background(), "dev123", 5, 900)
	authErr := &AuthError{}
	if !errors.As(err, &authErr) || authErr.Code != "access_denied" {
		t.Fatalf("expected AuthError(access_denied), got %v", err)
	}
}
