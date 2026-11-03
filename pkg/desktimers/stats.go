package desktimers

import (
	"net/http"
	"net/url"
	"time"
)

// ── the readouts ────────────────────────────────────────────────────────────
// What the Status panel says about the person's time: today, this week, and
// how many days in a row. All of it is the server's count (`/time/summary`,
// a workspace at a time, added up), read with the same rules the desktop app
// uses, so the two never disagree about a number they both show.
//
// There is NO LEVEL here, on purpose. The desktop app's level is built from
// every day it has ever seen on that machine; a request reaches back 62
// days. A level worked out from 62 days would be a different number from the
// one the desktop app shows, under the same name.

// daysBack is how far a summary reaches. The API allows 62 days a request.
const daysBack = 61

// StreakMinSeconds is how much tracked time makes a day count for a streak:
// the desktop app's rule.
const StreakMinSeconds = 15 * 60

// Stats is the person's tracked time, as of when it was asked for.
type Stats struct {
	// TodaySeconds and WeekSeconds include a running timer up to AsOf.
	TodaySeconds int
	WeekSeconds  int
	// Streak is days in a row with tracked time, ending today or yesterday.
	Streak int
	// Running is true when a timer is going; then TrackingCode is what on
	// and TrackingSince when it started.
	Running       bool
	TrackingCode  string
	TrackingSince time.Time
	AsOf          time.Time
}

func dayKey(t time.Time) string { return t.Format("2006-01-02") }

// StreakOf counts days in a row with at least StreakMinSeconds, ending today
// — or yesterday, if today has not got there yet: a streak is not broken by
// it being 9am.
func StreakOf(days map[string]int, now time.Time) int {
	counts := func(d time.Time) bool { return days[dayKey(d)] >= StreakMinSeconds }
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if !counts(day) {
		day = day.AddDate(0, 0, -1)
	}
	n := 0
	for counts(day) {
		n++
		day = day.AddDate(0, 0, -1)
	}
	return n
}

// WeekOf adds up Monday to Sunday of the week `now` is in, as the timesheet
// counts one.
func WeekOf(days map[string]int, now time.Time) int {
	sinceMonday := (int(now.Weekday()) + 6) % 7
	monday := time.Date(now.Year(), now.Month(), now.Day()-sinceMonday, 0, 0, 0, 0, now.Location())
	total := 0
	for i := range 7 {
		total += days[dayKey(monday.AddDate(0, 0, i))]
	}
	return total
}

// GetStats asks every workspace the person is in for their days and their
// running timer.
func (c *Client) GetStats(now time.Time) (*Stats, error) {
	workspaces, err := c.workspaces()
	if err != nil {
		return nil, err
	}
	type answer struct {
		days  map[string]int
		code  string
		since time.Time
	}
	answers := make([]answer, len(workspaces))
	query := url.Values{"from": {dayKey(now.AddDate(0, 0, -daysBack))}, "to": {dayKey(now)}}
	if err := each(len(workspaces), func(i int) error {
		base := "/workspaces/" + url.PathEscape(workspaces[i].ID) + "/time"
		summary := struct {
			Days []struct {
				Day     string `json:"day"`
				Seconds int    `json:"seconds"`
			} `json:"days"`
		}{}
		if err := c.call(http.MethodGet, base+"/summary?"+query.Encode(), nil, &summary); err != nil {
			return err
		}
		answers[i].days = map[string]int{}
		for _, d := range summary.Days {
			answers[i].days[d.Day] += d.Seconds
		}
		// No running timer is not a reason to have no totals.
		answers[i].code, answers[i].since, _ = c.tracking(workspaces[i].ID)
		return nil
	}); err != nil {
		return nil, err
	}

	days := map[string]int{}
	stats := &Stats{AsOf: now}
	for _, a := range answers {
		for day, seconds := range a.days {
			days[day] += seconds
		}
		if a.code != "" {
			stats.Running, stats.TrackingCode, stats.TrackingSince = true, a.code, a.since
		}
	}
	stats.TodaySeconds = days[dayKey(now)]
	stats.WeekSeconds = WeekOf(days, now)
	stats.Streak = StreakOf(days, now)
	return stats, nil
}
