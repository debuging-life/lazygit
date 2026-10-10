package desktimers

import (
	"strings"
	"testing"
)

func TestGetProjects(t *testing.T) {
	fake := newFakeDeskTick(t)
	fake.workspaces = []map[string]any{{"id": "ws2", "name": "Zeta", "slug": "zeta"}, {"id": "ws1", "name": "Acme", "slug": "acme"}}
	fake.projects["ws1"] = []map[string]any{{"code": "MOB", "name": "Mobile"}, {"code": "CLK", "name": "Clock"}}
	fake.projects["ws2"] = []map[string]any{{"code": "CLK", "name": "Clock too"}}

	projects, err := fake.client().GetProjects()
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, p := range projects {
		got = append(got, p.Workspace+":"+p.Code+":"+p.Name)
	}
	// By workspace, then by name: the same order every time.
	if want := "Acme:CLK:Clock, Acme:MOB:Mobile, Zeta:CLK:Clock too"; strings.Join(got, ", ") != want {
		t.Errorf("projects = %v, want %s", got, want)
	}
	// The same code in two workspaces is two projects.
	if projects[0].ID == projects[2].ID {
		t.Errorf("two workspaces' CLK must not share an id: %q", projects[0].ID)
	}
}

func TestCreateTask(t *testing.T) {
	fake := newFakeDeskTick(t)
	fake.workspaces = []map[string]any{{"id": "ws1", "name": "Acme", "slug": "acme"}, {"id": "ws2", "name": "Zeta", "slug": "zeta"}}
	fake.projects["ws1"] = []map[string]any{{"code": "CLK", "name": "Clock"}}
	fake.projects["ws2"] = []map[string]any{{"code": "CLK", "name": "Clock too"}}
	client := fake.client()
	projects, err := client.GetProjects()
	if err != nil {
		t.Fatal(err)
	}

	task, err := client.CreateTask(projects[1].ID, "Fix the tick")
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/workspaces/ws2/projects/CLK/tasks"
	posted := fake.body(path)
	if posted == nil {
		t.Fatalf("the task should be created in Zeta, asked: %v", fake.asked("POST"))
	}
	if posted["title"] != "Fix the tick" {
		t.Errorf("title = %v", posted["title"])
	}
	// In the person's own name: it is the task they are about to commit to.
	if posted["assigneeId"] != "me" {
		t.Errorf("assigneeId = %v, want me", posted["assigneeId"])
	}
	if task.Code != "CLK-99" || task.Title != "Fix the tick" || task.Project != "Clock too" {
		t.Errorf("task = %+v", task)
	}
	if want := fake.server.URL + "/w/zeta/projects/CLK/tasks/CLK-99"; task.URL != want {
		t.Errorf("URL = %q, want %q", task.URL, want)
	}
}

func TestCreateTaskInAProjectThatWasNeverOffered(t *testing.T) {
	fake := newFakeDeskTick(t)
	fake.oneProject()
	if _, err := fake.client().CreateTask("ws9/NOPE", "x"); err == nil {
		t.Fatal("expected a refusal")
	}
	if asked := fake.asked("POST"); len(asked) != 0 {
		t.Errorf("nothing should be sent, got %v", asked)
	}
}

func TestCreateTaskRefusedInTheAPIsWords(t *testing.T) {
	fake := newFakeDeskTick(t)
	fake.oneProject()
	client := fake.client()
	projects, _ := client.GetProjects()
	fake.refuse["/api/workspaces/ws1/projects/CLK/tasks"] = "This project is archived."
	if _, err := client.CreateTask(projects[0].ID, "x y"); err == nil || !strings.Contains(err.Error(), "This project is archived.") {
		t.Fatalf("expected the API's reason, got %v", err)
	}
}
