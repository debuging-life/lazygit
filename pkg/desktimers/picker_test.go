package desktimers

import (
	"reflect"
	"testing"
)

func TestOrderTasksForPicker(t *testing.T) {
	tasks := []Task{
		{Code: "LOUD-1", Status: "todo"},
		{Code: "LOUD-2", Status: StatusInProgress},
		{Code: "LOUD-3", Status: "todo"},
		{Code: "LOUD-4", Status: StatusInProgress},
	}

	t.Run("in-progress first, stable within groups", func(t *testing.T) {
		ordered, idx := OrderTasksForPicker(tasks, "")
		codes := []string{ordered[0].Code, ordered[1].Code, ordered[2].Code, ordered[3].Code}
		want := []string{"LOUD-2", "LOUD-4", "LOUD-1", "LOUD-3"}
		if !reflect.DeepEqual(codes, want) {
			t.Errorf("order = %v, want %v", codes, want)
		}
		if idx != 0 {
			t.Errorf("no current code should preselect index 0, got %d", idx)
		}
	})

	t.Run("current task preselected at its ordered position", func(t *testing.T) {
		ordered, idx := OrderTasksForPicker(tasks, "LOUD-3")
		if ordered[idx].Code != "LOUD-3" {
			t.Errorf("preselected item = %s, want LOUD-3 (idx %d)", ordered[idx].Code, idx)
		}
		if idx != 3 {
			t.Errorf("expected idx 3, got %d", idx)
		}
	})

	t.Run("unknown current code falls back to 0", func(t *testing.T) {
		_, idx := OrderTasksForPicker(tasks, "NOPE-9")
		if idx != 0 {
			t.Errorf("expected idx 0, got %d", idx)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		ordered, idx := OrderTasksForPicker(nil, "LOUD-1")
		if len(ordered) != 0 || idx != 0 {
			t.Errorf("expected empty/0, got %v/%d", ordered, idx)
		}
	})

	t.Run("tracking outranks in-progress and is preselected", func(t *testing.T) {
		withTracking := []Task{
			{Code: "LOUD-1", Status: "todo"},
			{Code: "LOUD-2", Status: StatusInProgress},
			{Code: "LOUD-3", Status: "todo", Tracking: true},
			{Code: "LOUD-4", Status: StatusInProgress},
		}
		ordered, idx := OrderTasksForPicker(withTracking, "")
		codes := []string{ordered[0].Code, ordered[1].Code, ordered[2].Code, ordered[3].Code}
		want := []string{"LOUD-3", "LOUD-2", "LOUD-4", "LOUD-1"}
		if !reflect.DeepEqual(codes, want) {
			t.Errorf("order = %v, want %v", codes, want)
		}
		if idx != 0 || !ordered[idx].Tracking {
			t.Errorf("tracking task should be preselected at 0, got idx %d", idx)
		}
	})

	t.Run("tracking preselection beats the state-file current task", func(t *testing.T) {
		withTracking := []Task{
			{Code: "LOUD-1", Status: "todo"},
			{Code: "LOUD-2", Status: StatusInProgress, Tracking: true},
			{Code: "LOUD-3", Status: "todo"},
		}
		ordered, idx := OrderTasksForPicker(withTracking, "LOUD-3")
		if ordered[idx].Code != "LOUD-2" {
			t.Errorf("preselected = %s, want tracking task LOUD-2", ordered[idx].Code)
		}
	})

	t.Run("no tracking task → current task still preselected", func(t *testing.T) {
		ordered, idx := OrderTasksForPicker(tasks, "LOUD-3")
		if ordered[idx].Code != "LOUD-3" {
			t.Errorf("preselected = %s, want LOUD-3", ordered[idx].Code)
		}
	})
}

func TestPickerColumnsTrackingMarker(t *testing.T) {
	tracking := PickerColumns(Task{Code: "LOUD-2", Title: "T", Status: StatusInProgress, Tracking: true})
	if tracking[3] != "⏱ tracking" {
		t.Errorf("tracking marker = %q, want \"⏱ tracking\"", tracking[3])
	}
	inProgress := PickerColumns(Task{Code: "LOUD-4", Title: "T", Status: StatusInProgress})
	if inProgress[3] != "● in progress" {
		t.Errorf("in-progress marker = %q", inProgress[3])
	}
}

func TestPickerColumns(t *testing.T) {
	inProgress := PickerColumns(Task{Code: "LOUD-124", Title: "Fix images", Project: "Leads", Status: StatusInProgress})
	want := []string{"LOUD-124", "Fix images", "(Leads)", "● in progress"}
	if !reflect.DeepEqual(inProgress, want) {
		t.Errorf("PickerColumns = %v, want %v", inProgress, want)
	}

	todo := PickerColumns(Task{Code: "LOUD-125", Title: "Other", Status: "todo"})
	if todo[3] != "" {
		t.Errorf("non-in-progress task should have empty marker, got %q", todo[3])
	}
}

func TestProjectsSpanWorkspaces(t *testing.T) {
	if ProjectsSpanWorkspaces(nil) {
		t.Error("no projects → false")
	}
	single := []Project{
		{Name: "Web", Workspace: "LoudOwls"},
		{Name: "Mobile", Workspace: "LoudOwls"},
	}
	if ProjectsSpanWorkspaces(single) {
		t.Error("same workspace everywhere → false")
	}
	multi := append(single, Project{Name: "Leads", Workspace: "DeskTimers"})
	if !ProjectsSpanWorkspaces(multi) {
		t.Error("two workspaces → true")
	}
}

func TestProjectMenuColumns(t *testing.T) {
	project := Project{Name: "Mobile App", Code: "MOB", Workspace: "LoudOwls"}

	got := ProjectMenuColumns(project, false)
	want := []string{"Mobile App", "(MOB)", ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("single-workspace columns = %v, want %v", got, want)
	}

	got = ProjectMenuColumns(project, true)
	want = []string{"Mobile App", "(MOB)", "LoudOwls"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("multi-workspace columns = %v, want %v", got, want)
	}
}

func TestApplyBranchPrefixTemplate(t *testing.T) {
	tests := []struct {
		name       string
		template   string
		branchType string
		code       string
		want       string
	}{
		{"default template", "", "bugfix", "LOUD-183", "bugfix/LOUD-183-"},
		{"explicit default", "{{type}}/{{code}}-", "feature", "LOUD-124", "feature/LOUD-124-"},
		{"custom with type", "{{type}}--{{code}}/", "hotfix", "LOUD-9", "hotfix--LOUD-9/"},
		{"custom without type", "wip/{{code}}-", "feature", "LOUD-124", "wip/LOUD-124-"},
		{"blank falls back to default", "  ", "docs", "LOUD-7", "docs/LOUD-7-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ApplyBranchPrefixTemplate(tt.template, tt.branchType, tt.code); got != tt.want {
				t.Errorf("ApplyBranchPrefixTemplate(%q, %q, %q) = %q, want %q", tt.template, tt.branchType, tt.code, got, tt.want)
			}
		})
	}
}

func TestBranchTemplateUsesType(t *testing.T) {
	tests := []struct {
		template string
		want     bool
	}{
		{"", true},    // blank → default template, which has {{type}}
		{"   ", true}, // whitespace → default
		{"{{type}}/{{code}}-", true},
		{"feature/{{code}}-", false}, // custom template without {{type}} → skip the type menu
		{"{{code}}/", false},
	}
	for _, tt := range tests {
		if got := BranchTemplateUsesType(tt.template); got != tt.want {
			t.Errorf("BranchTemplateUsesType(%q) = %v, want %v", tt.template, got, tt.want)
		}
	}
}

func TestFirstBranchType(t *testing.T) {
	if got := FirstBranchType(nil); got != "feature" {
		t.Errorf("empty list should fall back to the built-in default, got %q", got)
	}
	if got := FirstBranchType([]string{"chore", "feature"}); got != "chore" {
		t.Errorf("expected first configured type, got %q", got)
	}
}

func TestComposeWithTaskPrefix(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		typed  string
		want   string
	}{
		{"prepends to plain text", "LOUD-124/", "fix images", "LOUD-124/fix images"},
		{"branch prefix", "feature/LOUD-124-", "image-cache", "feature/LOUD-124-image-cache"},
		{"no double prefix: same code typed", "LOUD-124/", "LOUD-124/fix images", "LOUD-124/fix images"},
		{"different code typed wins (override)", "LOUD-124/", "LOUD-99/other thing", "LOUD-99/other thing"},
		{"code anywhere in typed text wins", "LOUD-124/", "revert LOUD-77 change", "revert LOUD-77 change"},
		{"empty typed stays empty (stays invalid)", "LOUD-124/", "", ""},
		{"whitespace-only typed unchanged", "LOUD-124/", "   ", "   "},
		{"no prefix (no-task valve) unchanged", "", "fix images", "fix images"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ComposeWithTaskPrefix(tt.prefix, tt.typed); got != tt.want {
				t.Errorf("ComposeWithTaskPrefix(%q, %q) = %q, want %q", tt.prefix, tt.typed, got, tt.want)
			}
		})
	}
}

func TestTitleWithTaskPrefix(t *testing.T) {
	if got := TitleWithTaskPrefix("Commit summary", "LOUD-124/"); got != "Commit summary — LOUD-124/" {
		t.Errorf("got %q", got)
	}
	if got := TitleWithTaskPrefix("Commit summary", ""); got != "Commit summary" {
		t.Errorf("no prefix should leave the title unchanged, got %q", got)
	}
}

func TestApplyPrefixTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		fallback string
		code     string
		want     string
	}{
		{"commit default", "{{code}}/", DefaultCommitPrefixTemplate, "LOUD-124", "LOUD-124/"},
		{"custom", "[{{code}}] ", DefaultCommitPrefixTemplate, "LOUD-124", "[LOUD-124] "},
		{"whitespace falls back", "  ", DefaultCommitPrefixTemplate, "LOUD-124", "LOUD-124/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ApplyPrefixTemplate(tt.template, tt.fallback, tt.code); got != tt.want {
				t.Errorf("ApplyPrefixTemplate(%q, %q, %q) = %q, want %q", tt.template, tt.fallback, tt.code, got, tt.want)
			}
		})
	}
}
