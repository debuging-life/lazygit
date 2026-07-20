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

func TestStatusLineHint(t *testing.T) {
	tests := []struct {
		name     string
		keyLabel string
		maxWidth int
		want     string
	}{
		{name: "normal", keyLabel: "alt+t", maxWidth: 60, want: "⏱ no task (alt+t)"},
		{name: "disabled binding", keyLabel: "", maxWidth: 60, want: ""},
		{name: "truncated", keyLabel: "alt+t", maxWidth: 12, want: "⏱ no task (…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StatusLineHint(tt.keyLabel, tt.maxWidth); got != tt.want {
				t.Errorf("StatusLineHint(%q, %d) = %q, want %q", tt.keyLabel, tt.maxWidth, got, tt.want)
			}
		})
	}
}

func TestFriendlyKeyLabel(t *testing.T) {
	tests := []struct {
		keys []string
		want string
	}{
		{keys: []string{"<alt+t>"}, want: "alt+t"},
		{keys: []string{"t"}, want: "t"},
		{keys: []string{"<c-t>", "<alt+t>"}, want: "c-t"},
		{keys: nil, want: ""},
		{keys: []string{}, want: ""},
	}

	for _, tt := range tests {
		if got := FriendlyKeyLabel(tt.keys); got != tt.want {
			t.Errorf("FriendlyKeyLabel(%v) = %q, want %q", tt.keys, got, tt.want)
		}
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
