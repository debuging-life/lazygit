package desktimers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// signInServer is the API's half of the sign-in: it knows which code goes
// with which challenge, and hands a session to whoever brings the code AND a
// verifier that hashes to that challenge.
type signInServer struct {
	*httptest.Server
	mu        sync.Mutex
	challenge string
	redeemed  int
}

func newSignInServer(t *testing.T) *signInServer {
	t.Helper()
	s := &signInServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/desktop/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body := map[string]string{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.redeemed++
		w.Header().Set("Content-Type", "application/json")
		if body["code"] != "the-code" || Challenge(body["verifier"]) != s.challenge {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"success":false,"error":"UNAUTHORIZED","message":"That sign-in link has expired or was already used."}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"token":"session-token","user":{"id":"u1","name":"Ada","email":"ada@example.com","image":null}}}`))
	}))
	t.Cleanup(s.Close)
	return s
}

// browser plays the person: reads the address deskgit opened, "signs in",
// and is sent back to the loopback port with whatever `answer` says.
func (s *signInServer) browser(t *testing.T, answer func(callback string, state string) []string) func(target string) {
	return func(target string) {
		parsed, err := url.Parse(target)
		if err != nil {
			t.Errorf("unparseable sign-in address %q", target)
			return
		}
		query := parsed.Query()
		if parsed.Path != "/desktop-auth" {
			t.Errorf("sign-in should open /desktop-auth, got %s", parsed.Path)
		}
		s.mu.Lock()
		s.challenge = query.Get("challenge")
		s.mu.Unlock()
		callback := "http://127.0.0.1:" + query.Get("port") + "/callback"
		go func() {
			for _, address := range answer(callback, query.Get("state")) {
				resp, err := http.Get(address)
				if err == nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			}
		}()
	}
}

func TestSignIn(t *testing.T) {
	server := newSignInServer(t)
	auth := NewAuthClient(server.URL + "/")
	opened := ""

	token, err := auth.SignIn(context.Background(), func(target string) {
		opened = target
		server.browser(t, func(callback string, state string) []string {
			return []string{callback + "?code=the-code&state=" + url.QueryEscape(state)}
		})(target)
	})
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "session-token" || token.UserID != "u1" || token.UserName != "Ada" || token.Site != server.URL {
		t.Errorf("token = %+v", token)
	}
	if !token.Valid() {
		t.Error("a fresh token should be valid")
	}

	// What the browser is given: the challenge, never the verifier.
	query, _ := url.ParseQuery(strings.SplitN(opened, "?", 2)[1])
	if len(query.Get("challenge")) != 43 {
		t.Errorf("the challenge should be a base64url SHA-256 (43 characters), got %q", query.Get("challenge"))
	}
	if query.Get("state") == "" || query.Get("port") == "" {
		t.Errorf("state and port must be in the address: %s", opened)
	}
	if strings.Contains(opened, "verifier") {
		t.Errorf("the verifier must never be in the address: %s", opened)
	}
}

func TestSignInIgnoresACallbackItDidNotAskFor(t *testing.T) {
	server := newSignInServer(t)
	auth := NewAuthClient(server.URL)

	token, err := auth.SignIn(context.Background(), server.browser(t, func(callback string, state string) []string {
		return []string{
			// Another program on the machine, guessing: a code of its own,
			// and a state that is not ours. Then no state at all. Then the
			// real one. Were the first two acted on, the sign-in would end
			// there, refused, before the real one arrived.
			callback + "?code=somebody-elses-code&state=not-ours",
			callback + "?code=somebody-elses-code",
			callback + "?code=the-code&state=" + url.QueryEscape(state),
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "session-token" {
		t.Errorf("token = %+v", token)
	}
	if server.redeemed != 1 {
		t.Errorf("only the callback with our state is redeemed, got %d redemptions", server.redeemed)
	}
}

func TestSignInRefusedInTheAPIsWords(t *testing.T) {
	server := newSignInServer(t)
	auth := NewAuthClient(server.URL)

	_, err := auth.SignIn(context.Background(), server.browser(t, func(callback string, state string) []string {
		return []string{callback + "?code=a-spent-code&state=" + url.QueryEscape(state)}
	}))
	if err == nil || !strings.Contains(err.Error(), "expired or was already used") {
		t.Fatalf("expected the API's reason, got %v", err)
	}
}

func TestSignInGivesUp(t *testing.T) {
	server := newSignInServer(t)
	auth := NewAuthClient(server.URL)
	auth.Wait = 50 * time.Millisecond

	_, err := auth.SignIn(context.Background(), func(string) {})
	if !errors.Is(err, ErrSignInTimedOut) {
		t.Fatalf("expected ErrSignInTimedOut, got %v", err)
	}
}

func TestCallbackPageEscapesWhatItIsTold(t *testing.T) {
	page := callbackPage("Sign-in did not finish", `<script>alert(1)</script>`)
	if strings.Contains(page, "<script>") {
		t.Errorf("the API's words must be text, never markup: %s", page)
	}
}
