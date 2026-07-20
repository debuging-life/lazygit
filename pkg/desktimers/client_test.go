package desktimers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetTasks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/git-client/tasks" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("status"); got != "active" {
			t.Errorf("status query = %q, want active", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer dtg_secret" {
			t.Errorf("auth header = %q", got)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": []map[string]string{
				{"code": "MOB-101", "title": "Fix login redirect", "project": "Mobile", "status": "in_progress"},
				{"code": "DES-9", "title": "New header", "project": "Design", "status": "todo"},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "dtg_secret")
	tasks, err := client.GetTasks("active")
	if err != nil {
		t.Fatalf("GetTasks: %v", err)
	}
	if len(tasks) != 2 || tasks[0].Code != "MOB-101" || tasks[0].Project != "Mobile" || tasks[1].Status != "todo" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}
}

func TestGetTasksUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(server.URL, "revoked")
	if _, err := client.GetTasks(""); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestNewClientFromTokenWithoutToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := NewClientFromToken(); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized without a stored token, got %v", err)
	}
}
