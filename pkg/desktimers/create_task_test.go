package desktimers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetProjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/git-client/projects" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer dtg_abc" {
			t.Errorf("missing bearer, got %q", r.Header.Get("Authorization"))
		}
		w.Write([]byte(`{"success":true,"data":[
			{"id":"p-1","name":"Mobile App","code":"MOB","workspace":"LoudOwls"},
			{"id":"p-2","name":"Leads","code":"LOUD","workspace":"DeskTimers"}
		]}`))
	}))
	defer server.Close()

	projects, err := NewClient(server.URL, "dtg_abc").GetProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 || projects[0].Code != "MOB" || projects[1].Workspace != "DeskTimers" {
		t.Errorf("unexpected projects: %+v", projects)
	}
}

func TestCreateTask(t *testing.T) {
	t.Run("creates and returns the task", func(t *testing.T) {
		var received map[string]string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/api/git-client/tasks" {
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatal(err)
			}
			w.Write([]byte(`{"success":true,"data":{"code":"LOUD-201","title":"Fix images","project":"Leads","status":"todo","url":"https://leads.desktimers.com/t/LOUD-201","tracking":false}}`))
		}))
		defer server.Close()

		task, err := NewClient(server.URL, "dtg_abc").CreateTask("p-2", "Fix images")
		if err != nil {
			t.Fatal(err)
		}
		if received["projectId"] != "p-2" || received["title"] != "Fix images" {
			t.Errorf("unexpected payload: %v", received)
		}
		if task.Code != "LOUD-201" || task.URL == "" || task.Project != "Leads" {
			t.Errorf("unexpected task: %+v", task)
		}
	})

	t.Run("401 → ErrUnauthorized", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		_, err := NewClient(server.URL, "dtg_x").CreateTask("p-1", "x")
		if err != ErrUnauthorized {
			t.Errorf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("error envelope message surfaces", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"success":false,"status":403,"error":"INSUFFICIENT_PERMISSIONS","message":"You cannot create tasks in this project"}`))
		}))
		defer server.Close()

		_, err := NewClient(server.URL, "dtg_x").CreateTask("p-1", "x")
		if err == nil || err.Error() != "creating task: You cannot create tasks in this project" {
			t.Errorf("expected the envelope message, got %v", err)
		}
	})
}
