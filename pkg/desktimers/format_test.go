package desktimers

import "testing"

func TestStatusLineSegment(t *testing.T) {
	tests := []struct {
		name     string
		state    *State
		maxWidth int
		want     string
	}{
		{
			name:     "nil state",
			state:    nil,
			maxWidth: 60,
			want:     "",
		},
		{
			name:     "empty code",
			state:    &State{},
			maxWidth: 60,
			want:     "",
		},
		{
			name:     "code and title",
			state:    &State{Code: "MOB-101", Title: "Fix login redirect"},
			maxWidth: 60,
			want:     "⏱ MOB-101 Fix login redirect",
		},
		{
			name:     "code only",
			state:    &State{Code: "MOB-101"},
			maxWidth: 60,
			want:     "⏱ MOB-101",
		},
		{
			name:     "truncated",
			state:    &State{Code: "MOB-101", Title: "A very long task title that will not fit"},
			maxWidth: 20,
			want:     "⏱ MOB-101 A very lo…",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StatusLineSegment(tt.state, tt.maxWidth)
			if got != tt.want {
				t.Errorf("StatusLineSegment() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMenuColumns(t *testing.T) {
	got := MenuColumns(Task{Code: "MOB-101", Title: "Fix login", Project: "Mobile App"})
	want := []string{"MOB-101", "Fix login", "(Mobile App)"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("MenuColumns()[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	noProject := MenuColumns(Task{Code: "MOB-101", Title: "Fix login"})
	if noProject[2] != "" {
		t.Errorf("expected empty project column, got %q", noProject[2])
	}
}

func TestPathKey(t *testing.T) {
	a := PathKey("/repo/one")
	b := PathKey("/repo/two")
	if a == b {
		t.Error("distinct paths must produce distinct keys")
	}
	if a != PathKey("/repo/one") {
		t.Error("PathKey must be deterministic")
	}
	if len(a) != 16 {
		t.Errorf("expected 16-char key, got %d chars", len(a))
	}
}
