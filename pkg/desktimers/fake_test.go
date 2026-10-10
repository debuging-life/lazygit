package desktimers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeDeskTick stands in for the DeskTick API: the routes deskgit uses, with
// the shapes the real ones answer in. A test fills in what exists and reads
// back what was asked.
type fakeDeskTick struct {
	t      *testing.T
	server *httptest.Server

	mu sync.Mutex
	// token is the bearer the fake accepts; anything else is a 401.
	token      string
	workspaces []map[string]any
	// projects, boards, repositories and current are keyed by workspace id,
	// or "<workspace id>/<project code>".
	projects     map[string][]map[string]any
	boards       map[string][]map[string]any
	repositories map[string][]string
	current      map[string]map[string]any
	summaries    map[string][]map[string]any
	// refuse answers a path with this status and message.
	refuse map[string]string
	// requests is every "METHOD path?query" that arrived, and bodies what
	// was posted to each path.
	requests []string
	bodies   map[string]map[string]any
}

func newFakeDeskTick(t *testing.T) *fakeDeskTick {
	t.Helper()
	f := &fakeDeskTick{
		t: t, token: "session-token",
		projects: map[string][]map[string]any{}, boards: map[string][]map[string]any{},
		repositories: map[string][]string{}, current: map[string]map[string]any{},
		summaries: map[string][]map[string]any{}, refuse: map[string]string{}, bodies: map[string]map[string]any{},
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeDeskTick) client() *Client {
	c := NewClient(f.server.URL, f.token)
	c.UserID = "me"
	return c
}

func (f *fakeDeskTick) asked(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []string{}
	for _, r := range f.requests {
		if strings.HasPrefix(r, prefix) {
			out = append(out, r)
		}
	}
	return out
}

func (f *fakeDeskTick) body(path string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[path]
}

func (f *fakeDeskTick) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	line := r.Method + " " + r.URL.Path
	if r.URL.RawQuery != "" {
		line += "?" + r.URL.RawQuery
	}
	f.requests = append(f.requests, line)

	reply := func(status int, data any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": status < 300, "data": data})
	}
	say := func(status int, message string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "X", "message": message})
	}

	if r.Header.Get("Authorization") != "Bearer "+f.token {
		say(http.StatusUnauthorized, "Authentication required.")
		return
	}
	if message, ok := f.refuse[r.URL.Path]; ok {
		say(http.StatusForbidden, message)
		return
	}
	if r.Body != nil && r.Method != http.MethodGet {
		posted := map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&posted)
		f.bodies[r.URL.Path] = posted
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
	switch {
	case r.URL.Path == "/api/workspaces":
		reply(200, f.workspaces)
	case r.URL.Path == "/api/auth/sign-out":
		reply(200, map[string]any{"success": true})
	case len(parts) == 3 && parts[2] == "projects":
		// The whole list on page 1, and nothing after it.
		items := f.projects[parts[1]]
		if r.URL.Query().Get("page") != "1" {
			items = nil
		}
		reply(200, map[string]any{"items": items, "total": len(f.projects[parts[1]])})
	case len(parts) == 5 && parts[4] == "board":
		reply(200, map[string]any{"columns": f.boards[parts[1]+"/"+parts[3]]})
	case len(parts) == 5 && parts[4] == "repositories":
		linked := []map[string]any{}
		for _, name := range f.repositories[parts[1]+"/"+parts[3]] {
			linked = append(linked, map[string]any{"fullName": name})
		}
		reply(200, map[string]any{"connected": true, "linked": linked})
	case len(parts) == 5 && parts[4] == "tasks" && r.Method == http.MethodPost:
		reply(200, map[string]any{"code": parts[3] + "-99"})
	case len(parts) == 7 && parts[6] == "move":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	case len(parts) == 5 && parts[2] == "time" && parts[4] == "current":
		reply(200, f.current[parts[1]])
	case len(parts) == 4 && parts[2] == "time" && parts[3] == "summary":
		reply(200, map[string]any{"days": f.summaries[parts[1]]})
	default:
		say(http.StatusNotFound, "Not found.")
	}
}

// column is a board column as the API sends one.
func column(id string, name string, position int, done bool, cards ...[2]string) map[string]any {
	tasks := []map[string]any{}
	for _, card := range cards {
		tasks = append(tasks, map[string]any{"code": card[0], "title": card[1]})
	}
	return map[string]any{"id": id, "name": name, "position": position, "isDone": done, "tasks": tasks}
}

// oneProject is the usual set-up: one workspace, one project, three columns.
func (f *fakeDeskTick) oneProject() {
	f.workspaces = []map[string]any{{"id": "ws1", "name": "Acme", "slug": "acme"}}
	f.projects["ws1"] = []map[string]any{{"code": "CLK", "name": "Clock"}}
	f.boards["ws1/CLK"] = []map[string]any{
		column("c-todo", "Backlog", 0, false, [2]string{"CLK-1", "Waiting"}),
		column("c-doing", "Building", 1, false, [2]string{"CLK-2", "Under way"}),
		column("c-done", "Shipped", 2, true, [2]string{"CLK-3", "Finished"}),
	}
}
