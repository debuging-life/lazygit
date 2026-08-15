package desktimers

import (
	"strings"
	"time"

	"github.com/jesseduffield/lazygit/pkg/utils"
)

// timerFrames is the set of clock-face emoji that cycle to animate the timer.
var timerFrames = []string{"🕐", "🕑", "🕒", "🕓", "🕔", "🕕", "🕖", "🕗", "🕘", "🕙", "🕚", "🕛"}

// TimerFrame returns a clock emoji that cycles once per second.
// Because StatusLineSegment is called on every layout pass, this produces a
// real-time spinning animation in the status bar.
func TimerFrame() string {
	return timerFrames[time.Now().Unix()%int64(len(timerFrames))]
}

// StatusLineSegment renders the selected task for the status/information
// line, e.g. "🕐 MOB-101 Fix login redirect", truncated to maxWidth.
// The clock emoji rotates every second to indicate a running timer.
func StatusLineSegment(state *State, maxWidth int) string {
	if state == nil || state.Code == "" {
		return ""
	}
	segment := TimerFrame() + " " + state.Code
	if state.Title != "" {
		segment += " " + state.Title
	}
	return utils.TruncateWithEllipsis(segment, maxWidth)
}

// StatusLineHint renders the "no task selected" nudge for the information
// line, e.g. "⏱ no task (alt+t)", truncated to maxWidth. An empty keyLabel
// (binding disabled) yields no hint.
func StatusLineHint(keyLabel string, maxWidth int) string {
	if keyLabel == "" {
		return ""
	}
	return utils.TruncateWithEllipsis("⏱ no task ("+keyLabel+")", maxWidth)
}

// FriendlyKeyLabel turns the first key of a binding into a human-friendly
// label: "<alt+t>" → "alt+t". Returns "" for an unbound/disabled binding.
func FriendlyKeyLabel(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return strings.Trim(keys[0], "<>")
}
