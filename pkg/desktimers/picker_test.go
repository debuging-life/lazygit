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

func TestApplyPrefixTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		fallback string
		code     string
		want     string
	}{
		{"commit default", "{{code}}/", DefaultCommitPrefixTemplate, "LOUD-124", "LOUD-124/"},
		{"branch default", "feature/{{code}}-", DefaultBranchPrefixTemplate, "LOUD-124", "feature/LOUD-124-"},
		{"custom", "[{{code}}] ", DefaultCommitPrefixTemplate, "LOUD-124", "[LOUD-124] "},
		{"blank falls back", "", DefaultBranchPrefixTemplate, "LOUD-124", "feature/LOUD-124-"},
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
