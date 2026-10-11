package desktimers

import (
	"errors"
	"strings"
	"testing"
)

func TestGetTasks(t *testing.T) {
	fake := newFakeDeskTick(t)
	fake.oneProject()
	fake.current["ws1"] = map[string]any{"taskCode": "CLK-2", "startedAt": "2026-10-10T09:00:00.000Z"}

	tasks, err := fake.client().GetTasks("active")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected the two open tasks and not the finished one, got %+v", tasks)
	}
	byCode := map[string]Task{}
	for _, task := range tasks {
		byCode[task.Code] = task
	}
	if _, finished := byCode["CLK-3"]; finished {
		t.Error("a task in a finished column must not be offered")
	}
	// Status is where the column IS, not what it is called.
	if got := byCode["CLK-1"].Status; got != StatusTodo {
		t.Errorf("first open column should read as todo, got %q", got)
	}
	if got := byCode["CLK-2"].Status; got != StatusInProgress {
		t.Errorf("a later open column should read as in progress, got %q", got)
	}
	if !byCode["CLK-2"].Tracking || byCode["CLK-1"].Tracking {
		t.Errorf("only the task the timer is on is tracking: %+v", tasks)
	}
	if got, want := byCode["CLK-2"].URL, fake.server.URL+"/w/acme/projects/CLK/tasks/CLK-2"; got != want {
		t.Errorf("task URL = %q, want %q", got, want)
	}
	if byCode["CLK-1"].Project != "Clock" {
		t.Errorf("project name = %q", byCode["CLK-1"].Project)
	}

	// MINE, not everything I may see: the projects I am on, and my cards.
	if asked := fake.asked("GET /api/workspaces/ws1/projects?"); len(asked) == 0 || !strings.Contains(asked[0], "memberId=me") {
		t.Errorf("projects must be asked for by member, got %v", asked)
	}
	if asked := fake.asked("GET /api/workspaces/ws1/projects/CLK/board"); len(asked) != 1 || !strings.Contains(asked[0], "assigneeId=me") {
		t.Errorf("the board must be asked for by assignee, got %v", asked)
	}
}

func TestGetTasksWithoutATimerOrItsRoute(t *testing.T) {
	fake := newFakeDeskTick(t)
	fake.oneProject()
	// The timer route refusing costs the marker, not the list.
	fake.refuse["/api/workspaces/ws1/time/timer/current"] = "no"

	tasks, err := fake.client().GetTasks("active")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	for _, task := range tasks {
		if task.Tracking {
			t.Errorf("%s cannot be tracking with no timer", task.Code)
		}
	}
}

func TestGetTasksUnauthorized(t *testing.T) {
	fake := newFakeDeskTick(t)
	fake.oneProject()
	client := fake.client()
	client.AccessToken = "someone-else"

	if _, err := client.GetTasks("active"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestStatusOfBeforeTheServerSaysWhatIsDone(t *testing.T) {
	// A server that does not send isDone leaves every column open: nothing is
	// hidden, and nothing is called done.
	columns := []boardColumn{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	if got := []string{statusOf(columns, 0), statusOf(columns, 1), statusOf(columns, 2)}; got[0] != StatusTodo || got[1] != StatusInProgress || got[2] != StatusInProgress {
		t.Errorf("statuses = %v", got)
	}
	// A done column first does not make the next one "in progress".
	columns = []boardColumn{{ID: "a", IsDone: true}, {ID: "b"}, {ID: "c"}}
	if got := statusOf(columns, 1); got != StatusTodo {
		t.Errorf("first OPEN column should be todo, got %q", got)
	}
}

func TestMoveGoesToTheWorkspaceTheTaskIsIn(t *testing.T) {
	// The same project code in two workspaces: a code alone does not say
	// which. Listing tasks is what teaches it.
	fake := newFakeDeskTick(t)
	fake.workspaces = []map[string]any{{"id": "ws1", "name": "Acme", "slug": "acme"}, {"id": "ws2", "name": "Zeta", "slug": "zeta"}}
	fake.projects["ws1"] = []map[string]any{{"code": "CLK", "name": "Clock"}}
	fake.projects["ws2"] = []map[string]any{{"code": "CLK", "name": "Clock too"}}
	fake.boards["ws1/CLK"] = []map[string]any{column("a", "To do", 0, false, [2]string{"CLK-1", "In Acme"})}
	fake.boards["ws2/CLK"] = []map[string]any{column("b", "To do", 0, false, [2]string{"CLK-7", "In Zeta"}), column("z-done", "Done", 1, true)}
	client := fake.client()
	if _, err := client.GetTasks("active"); err != nil {
		t.Fatal(err)
	}

	columns, err := client.GetTaskColumns("CLK-7")
	if err != nil {
		t.Fatal(err)
	}
	if len(columns) != 2 || columns[1].Slug != "z-done" || columns[1].Role != "done" {
		t.Fatalf("expected Zeta's columns, got %+v", columns)
	}
	if err := client.MoveTaskToColumn("clk-7", "z-done"); err != nil {
		t.Fatal(err)
	}
	path := "/api/workspaces/ws2/projects/CLK/tasks/CLK-7/move"
	if got := fake.body(path); got == nil || got["columnId"] != "z-done" {
		t.Errorf("the move should reach %s with the column id, got %v (asked: %v)", path, got, fake.asked("POST"))
	}
}

func TestMoveRefusedInTheAPIsWords(t *testing.T) {
	fake := newFakeDeskTick(t)
	fake.oneProject()
	fake.refuse["/api/workspaces/ws1/projects/CLK/tasks/CLK-1/move"] = "Building takes two tasks a person."
	err := fake.client().MoveTaskToColumn("CLK-1", "c-doing")
	if err == nil || !strings.Contains(err.Error(), "Building takes two tasks a person.") {
		t.Fatalf("expected the API's reason, got %v", err)
	}
}

func TestATaskInAProjectIAmNotOn(t *testing.T) {
	fake := newFakeDeskTick(t)
	fake.oneProject()
	if _, err := fake.client().GetTaskColumns("NOPE-4"); err == nil || !strings.Contains(err.Error(), "NOPE-4") {
		t.Fatalf("expected a refusal naming the task, got %v", err)
	}
}

func TestProjectCodeOf(t *testing.T) {
	for code, want := range map[string]string{"CLK-12": "CLK", "clk-12": "CLK", "A1-3": "A1", "CLK": "", "-5": ""} {
		if got := ProjectCodeOf(code); got != want {
			t.Errorf("ProjectCodeOf(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestNewClientFromTokenWithoutToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if _, err := NewClientFromToken(); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized without a token, got %v", err)
	}
}
