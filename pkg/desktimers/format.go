package desktimers

import (
	"fmt"
	"strings"
	"time"

	"github.com/jesseduffield/lazygit/pkg/utils"
)

// StatusLineSegment is the selected task as the information line says it,
// e.g. "MOB-101 Fix login redirect", cut to maxWidth. It says which task
// commits are going on, and nothing about a timer: whether one is running is
// the server's to say (Stats), not something a picked task implies.
func StatusLineSegment(state *State, maxWidth int) string {
	if state == nil || state.Code == "" {
		return ""
	}
	segment := state.Code
	if state.Title != "" {
		segment += " " + state.Title
	}
	return utils.TruncateWithEllipsis(segment, maxWidth)
}

// StatusLineHint is the "no task selected" nudge for the information line,
// e.g. "no task (alt+t)", cut to maxWidth. An empty keyLabel (binding
// disabled) yields no hint.
func StatusLineHint(keyLabel string, maxWidth int) string {
	if keyLabel == "" {
		return ""
	}
	return utils.TruncateWithEllipsis("no task ("+keyLabel+")", maxWidth)
}

// Duration says a number of seconds the way a readout does: "3h 12m", "42m",
// and "0m" for none. Seconds are left out: a readout that changes every
// second is one nobody reads.
func Duration(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	hours, minutes := seconds/3600, (seconds%3600)/60
	if hours > 0 {
		return fmt.Sprintf("%dh %02dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// Elapsed is how long a timer started at `since` has run at `now`, as a
// clock: "0:42", "3:07". Never negative: two machines' clocks disagree.
func Elapsed(since time.Time, now time.Time) string {
	seconds := int(now.Sub(since).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%d:%02d", seconds/3600, (seconds%3600)/60)
}

// Meter draws a share (0 to 1) as a bar `width` cells wide: "████░░░░".
func Meter(share float64, width int) string {
	if share < 0 {
		share = 0
	}
	if share > 1 {
		share = 1
	}
	filled := int(share*float64(width) + 0.5)
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// FriendlyKeyLabel turns the first key of a binding into a human-friendly
// label: "<alt+t>" → "alt+t". Returns "" for an unbound/disabled binding.
func FriendlyKeyLabel(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return strings.Trim(keys[0], "<>")
}
