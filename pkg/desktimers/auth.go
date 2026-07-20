package desktimers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// DeviceCode is the server's response to a device authorization request
// (OAuth RFC 8628 wire format).
type DeviceCode struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// AuthError is a terminal device-flow failure reported by the server.
type AuthError struct {
	Code string
}

func (e *AuthError) Error() string {
	switch e.Code {
	case "expired_token":
		return "desktimers: the device code expired before it was approved"
	case "access_denied":
		return "desktimers: the authorization request was denied"
	default:
		return fmt.Sprintf("desktimers: authorization failed (%s)", e.Code)
	}
}

// AuthClient performs the device authorization flow.
type AuthClient struct {
	BaseURL    string
	DeviceName string
	HTTPClient *http.Client

	// sleep is injectable for tests; defaults to a context-aware sleep.
	sleep func(ctx context.Context, d time.Duration) error
}

// NewAuthClient builds an auth client for the given base URL. The device name
// defaults to the machine hostname.
func NewAuthClient(baseURL string) *AuthClient {
	hostname, _ := os.Hostname()
	return &AuthClient{
		BaseURL:    strings.TrimSuffix(baseURL, "/"),
		DeviceName: hostname,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		sleep:      sleepCtx,
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (a *AuthClient) postJSON(ctx context.Context, path string, body any) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return a.HTTPClient.Do(req)
}

// RequestDeviceCode starts the device flow.
func (a *AuthClient) RequestDeviceCode(ctx context.Context) (*DeviceCode, error) {
	resp, err := a.postJSON(ctx, "/api/device/code", struct{}{})
	if err != nil {
		return nil, fmt.Errorf("requesting device code: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("requesting device code: unexpected status %d", resp.StatusCode)
	}
	code := &DeviceCode{}
	if err := json.NewDecoder(resp.Body).Decode(code); err != nil {
		return nil, fmt.Errorf("parsing device code response: %w", err)
	}
	return code, nil
}

type deviceTokenRequest struct {
	DeviceCode string `json:"device_code"`
	DeviceName string `json:"device_name"`
}

type deviceTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Plan        string `json:"plan"`
	Error       string `json:"error"`
}

type pollResult int

const (
	pollSuccess pollResult = iota
	pollPending
	pollSlowDown
)

// PollForToken polls the token endpoint until the user approves the device,
// the code expires, or the context is cancelled. interval and expiresIn are
// the values from the device code response, in seconds.
func (a *AuthClient) PollForToken(ctx context.Context, deviceCode string, interval int, expiresIn int) (*Token, error) {
	if expiresIn > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(expiresIn)*time.Second)
		defer cancel()
	}
	for {
		if err := a.sleep(ctx, time.Duration(interval)*time.Second); err != nil {
			return nil, fmt.Errorf("device authorization timed out: %w", err)
		}
		token, result, err := a.requestToken(ctx, deviceCode)
		if err != nil {
			return nil, err
		}
		switch result {
		case pollSuccess:
			return token, nil
		case pollSlowDown:
			interval += 5
		case pollPending:
			// keep polling at the same interval
		}
	}
}

// requestToken performs one poll of the token endpoint.
func (a *AuthClient) requestToken(ctx context.Context, deviceCode string) (*Token, pollResult, error) {
	resp, err := a.postJSON(ctx, "/api/device/token", deviceTokenRequest{
		DeviceCode: deviceCode,
		DeviceName: a.DeviceName,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("polling for token: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("reading token response: %w", err)
	}
	parsed := deviceTokenResponse{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, 0, fmt.Errorf("parsing token response: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		tokenType := parsed.TokenType
		if tokenType == "" {
			tokenType = "Bearer"
		}
		token := &Token{
			AccessToken: parsed.AccessToken,
			TokenType:   tokenType,
			Plan:        parsed.Plan,
		}
		if parsed.ExpiresIn > 0 {
			token.ExpiresAt = time.Now().Add(time.Duration(parsed.ExpiresIn) * time.Second)
		}
		return token, pollSuccess, nil
	case http.StatusPreconditionRequired: // 428: authorization_pending
		return nil, pollPending, nil
	case http.StatusTooManyRequests: // 429: slow_down
		return nil, pollSlowDown, nil
	default:
		if parsed.Error != "" {
			return nil, 0, &AuthError{Code: parsed.Error}
		}
		return nil, 0, fmt.Errorf("polling for token: unexpected status %d", resp.StatusCode)
	}
}

// RunDeviceFlow performs the whole login: request a code, show it to the
// user, open the browser, poll until approved, and persist the token.
// openBrowser may be nil; browser-open failures are ignored (the printed URL
// is the fallback).
func RunDeviceFlow(w io.Writer, openBrowser func(url string) error) (*Token, error) {
	// Reuse the stored token's base URL (if any) so re-auth against a
	// custom deployment keeps working.
	stored, _ := LoadToken()
	baseURL := ResolveBaseURL(stored)
	auth := NewAuthClient(baseURL)

	ctx := context.Background()
	code, err := auth.RequestDeviceCode(ctx)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(w, "First, copy your one-time code: %s\n", code.UserCode)
	fmt.Fprintf(w, "Then approve this device at: %s\n", code.VerificationURI)
	if openBrowser != nil {
		target := code.VerificationURIComplete
		if target == "" {
			target = code.VerificationURI
		}
		// Best effort only; the printed URL is the fallback.
		_ = openBrowser(target)
	}
	fmt.Fprintln(w, "Waiting for approval...")

	token, err := auth.PollForToken(ctx, code.DeviceCode, code.Interval, code.ExpiresIn)
	if err != nil {
		return nil, err
	}
	token.APIBaseURL = baseURL
	if err := SaveToken(token); err != nil {
		return nil, err
	}
	return token, nil
}
