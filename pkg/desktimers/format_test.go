package desktimers

import (
	"testing"
	"time"
)

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
			want:     "MOB-101 Fix login redirect",
		},
		{
			name:     "code only",
			state:    &State{Code: "MOB-101"},
			maxWidth: 60,
			want:     "MOB-101",
		},
		{
			name:     "truncated",
			state:    &State{Code: "MOB-101", Title: "A very long task title that will not fit"},
			maxWidth: 20,
			want:     "MOB-101 A very long…",
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
		{name: "normal", keyLabel: "alt+t", maxWidth: 60, want: "no task (alt+t)"},
		{name: "disabled binding", keyLabel: "", maxWidth: 60, want: ""},
		{name: "truncated", keyLabel: "alt+t", maxWidth: 12, want: "no task (al…"},
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

func TestDuration(t *testing.T) {
	for seconds, want := range map[int]string{-5: "0m", 0: "0m", 59: "0m", 60: "1m", 3599: "59m", 3600: "1h 00m", 11520: "3h 12m", 360000: "100h 00m"} {
		if got := Duration(seconds); got != want {
			t.Errorf("Duration(%d) = %q, want %q", seconds, got, want)
		}
	}
}

func TestElapsed(t *testing.T) {
	start := time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC)
	for after, want := range map[time.Duration]string{0: "0:00", 59 * time.Second: "0:00", 42 * time.Minute: "0:42", 3*time.Hour + 7*time.Minute: "3:07"} {
		if got := Elapsed(start, start.Add(after)); got != want {
			t.Errorf("Elapsed(+%s) = %q, want %q", after, got, want)
		}
	}
	// The server's clock ahead of this machine's: not a negative time.
	if got := Elapsed(start, start.Add(-time.Minute)); got != "0:00" {
		t.Errorf("Elapsed before the start = %q, want 0:00", got)
	}
}

func TestMeter(t *testing.T) {
	for share, want := range map[float64]string{-1: "░░░░", 0: "░░░░", 0.5: "██░░", 1: "████", 7: "████"} {
		if got := Meter(share, 4); got != want {
			t.Errorf("Meter(%v) = %q, want %q", share, got, want)
		}
	}
}
