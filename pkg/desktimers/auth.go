package desktimers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ── signing in through the browser ──────────────────────────────────────────
// deskgit signs in the way the DeskTick desktop app does, against the same
// two routes, so there is one sign-in to keep right and not two:
//
//   1. we make a random VERIFIER and STATE, and listen on a loopback port
//   2. the browser opens  <site>/desktop-auth?challenge=sha256(verifier)
//      &state=…&port=…  and the person signs in there, however they like
//      (password, Google, a passkey, a two-step code), and confirms
//   3. the page sends the browser to  http://127.0.0.1:<port>/callback
//      ?code=…&state=…
//   4. we trade the code AND the verifier for a session (POST
//      /api/desktop/token)
//
// That is OAuth's authorization-code flow with PKCE and a loopback redirect
// (RFC 8252). The code alone is useless to anything that sees it go by: it
// is single use, dies in two minutes, and needs the verifier, which never
// leaves this process. `state` is checked so a callback we did not ask for
// is ignored.
//
// It replaced a device-code flow ("type BDBG-GSGX into the page") that the
// server before DeskTick had and this one does not.

// signInWait is how long we wait for the browser to come back.
const signInWait = 5 * time.Minute

// signInWaitOverride shortens the wait in tests. Zero means signInWait.
var signInWaitOverride time.Duration

// ErrSignInTimedOut is returned when nobody confirmed in the browser.
var ErrSignInTimedOut = errors.New("desktick: nobody confirmed the sign-in in the browser in time")

type AuthClient struct {
	// Site is the DeskTick web app's address; the API is under /api.
	Site       string
	HTTPClient *http.Client
	// Wait bounds the whole sign-in. Zero means signInWait.
	Wait time.Duration
}

func NewAuthClient(site string) *AuthClient {
	return &AuthClient{
		Site:       strings.TrimSuffix(site, "/"),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func randomURLSafe() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("making a random value: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// Challenge is what the browser is given in place of the verifier: its
// SHA-256, base64url, which is 43 characters.
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

type signInReply struct {
	Success bool `json:"success"`
	Data    struct {
		Token string `json:"token"`
		User  struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"user"`
	} `json:"data"`
	Message string `json:"message"`
}

// redeem trades the browser's one-time code, with the verifier, for a
// session.
func (a *AuthClient) redeem(ctx context.Context, code string, verifier string) (*Token, error) {
	payload, err := json.Marshal(map[string]string{"code": code, "verifier": verifier})
	if err != nil {
		return nil, fmt.Errorf("encoding the sign-in request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.Site+"/api/desktop/token", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("building the sign-in request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("finishing the sign-in: %w", err)
	}
	defer resp.Body.Close()

	reply := signInReply{}
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&reply)
	if resp.StatusCode != http.StatusOK {
		if decodeErr == nil && reply.Message != "" {
			return nil, errors.New(reply.Message)
		}
		return nil, fmt.Errorf("finishing the sign-in: unexpected status %d", resp.StatusCode)
	}
	if decodeErr != nil || reply.Data.Token == "" || reply.Data.User.ID == "" {
		return nil, errors.New("the sign-in did not complete")
	}
	return &Token{
		AccessToken: reply.Data.Token,
		TokenType:   "Bearer",
		Site:        a.Site,
		UserID:      reply.Data.User.ID,
		UserName:    reply.Data.User.Name,
	}, nil
}

// callbackPage is what the browser tab shows once we have answered. The
// message is escaped: part of it can be the API's words.
func callbackPage(title string, message string) string {
	return `<!doctype html><meta charset="utf-8"><title>deskgit</title>` +
		`<body style="background:#0b0b0c;color:#e9e9ea;font:16px/1.5 ui-monospace,Menlo,monospace;display:grid;place-items:center;height:100vh;margin:0">` +
		`<div style="border:1px solid #2a2a2c;padding:28px 32px;max-width:28rem">` +
		`<div style="color:#ffd92e;letter-spacing:.14em;font-size:12px;text-transform:uppercase">deskgit</div>` +
		`<h1 style="font-size:20px;margin:10px 0 6px">` + html.EscapeString(title) + `</h1>` +
		`<p style="margin:0;color:#a1a1a6">` + html.EscapeString(message) + `</p></div></body>`
}

// SignIn runs the whole sign-in and returns the token WITHOUT saving it.
// `announce` is given the address to open, once we are listening for the
// answer.
func (a *AuthClient) SignIn(ctx context.Context, announce func(target string)) (*Token, error) {
	verifier, err := randomURLSafe()
	if err != nil {
		return nil, err
	}
	state, err := randomURLSafe()
	if err != nil {
		return nil, err
	}

	// Loopback only, and a port the OS picks.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listening for the browser: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	wait := a.Wait
	if wait <= 0 {
		wait = signInWaitOverride
	}
	if wait <= 0 {
		wait = signInWait
	}
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	type outcome struct {
		token *Token
		err   error
	}
	// Buffered, and written once: a second callback (a reload of the tab)
	// must not block its handler for ever.
	done := make(chan outcome, 1)
	answered := false

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		query := r.URL.Query()
		code := query.Get("code")
		// A callback that is not the one we asked for (another program
		// guessing the port) is told nothing and changes nothing.
		if code == "" || subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(state)) != 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, callbackPage("That is not a sign-in we asked for", "Go back to the terminal and start again."))
			return
		}
		if answered {
			_, _ = io.WriteString(w, callbackPage("Already done", "You can close this tab."))
			return
		}
		answered = true
		token, err := a.redeem(ctx, code, verifier)
		if err != nil {
			_, _ = io.WriteString(w, callbackPage("Sign-in did not finish", err.Error()))
		} else {
			_, _ = io.WriteString(w, callbackPage("You're signed in", "Go back to the terminal. You can close this tab."))
		}
		done <- outcome{token, err}
	})
	// One request at a time, so `answered` needs no lock.
	server := &http.Server{Handler: serially(mux), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer func() {
		shutdownCtx, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = server.Shutdown(shutdownCtx)
	}()

	target := fmt.Sprintf("%s/desktop-auth?challenge=%s&state=%s&port=%d",
		a.Site, url.QueryEscape(Challenge(verifier)), url.QueryEscape(state), port)
	if announce != nil {
		announce(target)
	}

	select {
	case result := <-done:
		return result.token, result.err
	case <-ctx.Done():
		return nil, ErrSignInTimedOut
	}
}

// serially lets one request through at a time.
func serially(next http.Handler) http.Handler {
	gate := make(chan struct{}, 1)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gate <- struct{}{}
		defer func() { <-gate }()
		next.ServeHTTP(w, r)
	})
}

// RunSignIn signs in from a terminal: says where to go, opens the browser,
// waits, and saves the token.
func RunSignIn(w io.Writer, openBrowser func(url string) error) (*Token, error) {
	auth := NewAuthClient(ResolveBaseURL())
	token, err := auth.SignIn(context.Background(), func(target string) {
		fmt.Fprintln(w, "Opening DeskTick in your browser to sign in.")
		fmt.Fprintf(w, "If it does not open, go to:\n  %s\n", target)
		if openBrowser != nil {
			_ = openBrowser(target)
		}
		fmt.Fprintln(w, "Waiting for you to confirm there...")
	})
	if err != nil {
		return nil, err
	}
	if err := SaveToken(token); err != nil {
		return nil, err
	}
	return token, nil
}
