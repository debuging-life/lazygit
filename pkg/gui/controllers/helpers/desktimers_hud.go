package helpers

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gookit/color"
	"github.com/jesseduffield/lazygit/pkg/desktimers"
	"github.com/jesseduffield/lazygit/pkg/gui/context"
	"github.com/jesseduffield/lazygit/pkg/gui/style"
	"github.com/jesseduffield/lazygit/pkg/theme"
	"github.com/jesseduffield/lazygit/pkg/utils"
)

// ── the HUD look ────────────────────────────────────────────────────────────
// What deskgit draws that lazygit does not: the task on the information line
// as a plate, the picker's colours, and the Status panel's dashboard. The
// frames and heads are gocui's (Gui.Brackets); the palette is the theme's
// defaults (pkg/config). This file is the words and numbers.
//
// ONE ACCENT, and it is the theme's active border colour: somebody who
// changes that in their config changes all of it together.

// hudInk is what is written on a plate. A plate is a block of the accent,
// and the accent is a light colour, so its ink is near black whatever the
// terminal's own colours are.
const hudInk = "#141200"

// hudAccent is the accent as a text colour.
func (self *DesktimersHelper) hudAccent() style.TextStyle {
	return theme.GetTextStyle(self.c.UserConfig().Gui.Theme.ActiveBorderColor, false)
}

// hudPlate is the accent as a background, with ink on it.
func (self *DesktimersHelper) hudPlate() style.TextStyle {
	return theme.GetTextStyle(self.c.UserConfig().Gui.Theme.ActiveBorderColor, true).
		SetFg(style.NewRGBColor(color.HEX(hudInk, false)))
}

// hudDim is the line colour: frames, rules, and what is said quietly.
func (self *DesktimersHelper) hudDim() style.TextStyle {
	return theme.GetTextStyle(self.c.UserConfig().Gui.Theme.InactiveBorderColor, false)
}

// ── the person's time ───────────────────────────────────────────────────────

// statsEvery is how often the readouts are asked for again. A minute: the
// readouts are in minutes.
const statsEvery = time.Minute

// The goals a meter is drawn against: the desktop app's (40 hours a week,
// over five days).
const (
	weeklyGoalSeconds = 40 * 60 * 60
	dailyGoalSeconds  = weeklyGoalSeconds / 5
)

// hudStats is the last answer, and whether there has been one. It belongs to
// the program and not to a helper: helpers are made again when the repo is
// switched, and the person's time is the same in every repo.
var hudStats = struct {
	sync.Mutex
	stats *desktimers.Stats
	// failed is true when the last try did not get an answer.
	failed bool
	// started guards the one goroutine that asks.
	started bool
	// helper is the one to tell when an answer arrives.
	helper atomic.Pointer[DesktimersHelper]
}{}

// Stats is the person's time as last heard, or nil before the first answer.
func (self *DesktimersHelper) Stats() *desktimers.Stats {
	hudStats.Lock()
	defer hudStats.Unlock()
	return hudStats.stats
}

// StartStatsInBackground asks for the person's time now and then once a
// minute, for as long as the program runs. Failures are quiet: the readouts
// say they have no answer, and nothing interrupts a commit over it.
func (self *DesktimersHelper) StartStatsInBackground() {
	hudStats.helper.Store(self)
	hudStats.Lock()
	already := hudStats.started
	hudStats.started = true
	hudStats.Unlock()
	if already {
		return
	}
	go utils.Safe(func() {
		for {
			if helper := hudStats.helper.Load(); helper != nil {
				helper.refreshStats()
			}
			time.Sleep(statsEvery)
		}
	})
}

func (self *DesktimersHelper) refreshStats() {
	if !self.isAuthenticated() {
		return
	}
	client, err := desktimers.NewClientFromToken()
	var stats *desktimers.Stats
	if err == nil {
		stats, err = client.GetStats(time.Now())
	}
	hudStats.Lock()
	if err != nil {
		self.c.Log.Debugf("deskgit: no time readouts: %v", err)
		hudStats.failed = true
	} else {
		hudStats.stats, hudStats.failed = stats, false
	}
	hudStats.Unlock()

	// The information line is redrawn by any layout; the dashboard only when
	// it is what the main view is showing.
	self.c.OnUIThread(func() error {
		if current := self.c.Context().Current(); current != nil && current.GetKey() == context.STATUS_CONTEXT_KEY {
			current.HandleRenderToMain()
		}
		return nil
	})
}

// trackingNow is the task the timer is on and how long it has run, when it
// is running.
func (self *DesktimersHelper) trackingNow() (code string, elapsed string, ok bool) {
	stats := self.Stats()
	if stats == nil || !stats.Running {
		return "", "", false
	}
	return stats.TrackingCode, desktimers.Elapsed(stats.TrackingSince, time.Now()), true
}

// ── the information line ────────────────────────────────────────────────────

// StatusLine is the selected task as the bottom line shows it: its code on a
// plate, its title after, and the timer's clock when the timer is on that
// very task. A timer on another task is not this line's to show: this line
// is what a commit made now would be tagged with.
func (self *DesktimersHelper) StatusLine(maxWidth int) string {
	task := self.SelectedTask()
	if task == nil || task.Code == "" {
		return ""
	}
	line := self.hudPlate().SetBold().Sprint(" " + task.Code + " ")
	if room := maxWidth - len(task.Code) - 3; task.Title != "" && room > 4 {
		line += " " + utils.TruncateWithEllipsis(task.Title, room)
	}
	if code, elapsed, ok := self.trackingNow(); ok && strings.EqualFold(code, task.Code) {
		line += " " + self.hudAccent().Sprint("▶ "+elapsed)
	}
	return line
}

// ── the picker ──────────────────────────────────────────────────────────────

// styledTaskColumns colours a task's picker row: the one the timer is on in
// the accent, the ones under way in the terminal's own colour, and the ones
// still waiting quietly. The words are PickerColumns'.
func (self *DesktimersHelper) styledTaskColumns(columns []string, task desktimers.Task) []string {
	out := make([]string, len(columns))
	copy(out, columns)
	var paint style.TextStyle
	switch {
	case task.Tracking:
		paint = self.hudAccent().SetBold()
	case task.Status == desktimers.StatusInProgress:
		paint = style.New()
	default:
		paint = self.hudDim()
	}
	for i, column := range out {
		if column != "" {
			out[i] = paint.Sprint(column)
		}
	}
	// The code is what gets typed into a commit: never dimmed.
	if len(out) > 0 && !task.Tracking {
		out[0] = style.New().SetBold().Sprint(columns[0])
	}
	return out
}

// ── the dashboard ───────────────────────────────────────────────────────────

// tileWidth is a readout's panel; three of them and their gaps is the
// widest the dashboard draws anything.
const (
	tileWidth    = 22
	widestLayout = tileWidth*3 + 2
)

// pad fills `text` out to `width` cells. Colour codes take up no cells.
func pad(text string, width int) string {
	if gap := width - utils.StringWidth(utils.Decolorise(text)); gap > 0 {
		return text + strings.Repeat(" ", gap)
	}
	return text
}

// tile is a readout as a small panel, four lines tall: a head, the figure,
// a line under it, and the bottom edge.
func (self *DesktimersHelper) tile(label string, figure string, under string) []string {
	accent, dim := self.hudAccent(), self.hudDim()
	inner := tileWidth - 2
	head := " " + label + " "
	return []string{
		accent.Sprint("┏") + accent.Sprint(head) + dim.Sprint(strings.Repeat("─", inner-utils.StringWidth(head))+"┐"),
		accent.Sprint("┃") + style.New().SetBold().Sprint(pad(" "+figure, inner)) + dim.Sprint("│"),
		dim.Sprint("│") + pad(" "+under, inner) + accent.Sprint("┃"),
		dim.Sprint("└"+strings.Repeat("─", inner-1)) + accent.Sprint("━┛"),
	}
}

// sectionHead is "01  TITLE ─────────", `width` cells across.
func (self *DesktimersHelper) sectionHead(number int, title string, width int) string {
	head := fmt.Sprintf("%02d", number)
	rule := width - len(head) - 2 - utils.StringWidth(title) - 1
	return self.hudAccent().Sprint(head) + "  " + style.New().SetBold().Sprint(title) + " " + self.hudDim().Sprint(strings.Repeat("─", max(rule, 3)))
}

// meter is a share of a goal as a bar and a percentage.
func (self *DesktimersHelper) meter(seconds int, goal int) string {
	share := float64(seconds) / float64(goal)
	percent := int(share*100 + 0.5)
	return self.hudAccent().Sprint(desktimers.Meter(share, 10)) + self.hudDim().Sprintf(" %d%%", percent)
}

// Dashboard is what the Status panel shows in the main view: the person's
// time, what they are on, the keys that are deskgit's own, and where things
// are. Every figure is the server's; before it has answered they are dashes,
// and when it cannot be reached that is said.
//
// `width` is the cells there are to draw in. The tiles sit side by side when
// they fit and under each other when they do not: a wrapped tile is a row of
// broken boxes.
func (self *DesktimersHelper) Dashboard(version string, width int) string {
	// One cell of margin on the left (see the end), and one spare on the right.
	width = min(max(width-2, tileWidth), widestLayout)
	perRow := max(1, (width+1)/(tileWidth+1))
	accent, dim := self.hudAccent(), self.hudDim()
	bold := style.New().SetBold()
	lines := []string{
		"",
		self.hudPlate().SetBold().Sprint(" DESKGIT ") + " " + dim.Sprint(version) + "   " + dim.Sprint("DeskTick's git client"),
		"",
	}

	hudStats.Lock()
	stats, failed := hudStats.stats, hudStats.failed
	hudStats.Unlock()

	today, week, streak := "—", "—", "—"
	underToday, underWeek, underStreak := dim.Sprint("of 8h a day"), dim.Sprint("of 40h a week"), dim.Sprint("days in a row")
	if stats != nil {
		today, week = desktimers.Duration(stats.TodaySeconds), desktimers.Duration(stats.WeekSeconds)
		underToday, underWeek = self.meter(stats.TodaySeconds, dailyGoalSeconds), self.meter(stats.WeekSeconds, weeklyGoalSeconds)
		streak = fmt.Sprintf("%d", stats.Streak)
		if stats.Streak == 1 {
			underStreak = dim.Sprint("day in a row")
		}
	}
	tiles := [][]string{
		self.tile("TODAY", today, underToday),
		self.tile("THIS WEEK", week, underWeek),
		self.tile("STREAK", streak, underStreak),
	}
	for first := 0; first < len(tiles); first += perRow {
		group := tiles[first:min(first+perRow, len(tiles))]
		for row := range group[0] {
			cells := make([]string, len(group))
			for i, one := range group {
				cells[i] = one[row]
			}
			lines = append(lines, strings.Join(cells, " "))
		}
	}
	switch {
	case !self.isAuthenticated():
		lines = append(lines, dim.Sprint("Not signed in to DeskTick. Run `deskgit login`."))
	case stats == nil && failed:
		lines = append(lines, dim.Sprint("DeskTick could not be reached. These fill in when it can."))
	case stats == nil:
		lines = append(lines, dim.Sprint("Asking DeskTick…"))
	case failed:
		lines = append(lines, dim.Sprintf("DeskTick could not be reached. As of %s.", stats.AsOf.Format("15:04")))
	default:
		lines = append(lines, dim.Sprintf("Tracked time, from DeskTick. As of %s.", stats.AsOf.Format("15:04")))
	}

	lines = append(lines, "", self.sectionHead(1, "ON THE CLOCK", width))
	if code, elapsed, ok := self.trackingNow(); ok {
		lines = append(lines, "    "+accent.SetBold().Sprint("▶ "+code)+"  "+dim.Sprint("running for ")+bold.Sprint(elapsed))
	} else if stats == nil {
		// Not "no timer": nobody has said.
		lines = append(lines, "    "+dim.Sprint("—"))
	} else {
		lines = append(lines, "    "+dim.Sprint("No timer is running."))
	}

	lines = append(lines, "", self.sectionHead(2, "COMMITTING TO", width))
	if task := self.SelectedTask(); task != nil && task.Code != "" {
		lines = append(lines, "    "+self.hudPlate().SetBold().Sprint(" "+task.Code+" ")+" "+task.Title)
	} else {
		lines = append(lines, "    "+dim.Sprint("No task picked. A commit will ask for one."))
	}

	keys := self.c.UserConfig().Keybinding
	key := func(binding []string, what string) string {
		label := desktimers.FriendlyKeyLabel(binding)
		if label == "" {
			return ""
		}
		return "    " + accent.Sprint(pad(label, 8)) + what
	}
	lines = append(lines, "", self.sectionHead(3, "KEYS", width))
	for _, line := range []string{
		key(keys.Universal.DesktimersMenu, "the DeskTick menu, from anywhere"),
		key(keys.Universal.DesktimersTasks, "pick the task you are on"),
		key(keys.Files.CommitChanges, "commit: the task's code goes on the front"),
		key(keys.Universal.New, "new branch, named for the task"),
		key(keys.Universal.OptionMenu, "every key there is"),
	} {
		if line != "" {
			lines = append(lines, line)
		}
	}

	lines = append(lines, "", self.sectionHead(4, "ABOUT", width),
		"    "+dim.Sprint("DeskTick    ")+desktimers.ResolveBaseURL(),
		"    "+dim.Sprint("deskgit     ")+"https://github.com/debuging-life/lazygit",
		"    "+dim.Sprint("Built on    ")+"lazygit by Jesse Duffield and contributors (MIT)",
		"    "+dim.Sprint("            ")+"https://github.com/jesseduffield/lazygit",
		"",
	)
	// One cell in from the frame, all the way down.
	return " " + strings.Join(lines, "\n ")
}
