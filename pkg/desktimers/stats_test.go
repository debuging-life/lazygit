package desktimers

import (
	"testing"
	"time"
)

func TestStreakOf(t *testing.T) {
	now := time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC) // a Friday
	enough, short := StreakMinSeconds, StreakMinSeconds-1

	if got := StreakOf(map[string]int{}, now); got != 0 {
		t.Errorf("no days = %d, want 0", got)
	}
	// Today has not started: yesterday's run still stands.
	days := map[string]int{"2026-10-08": enough, "2026-10-07": enough, "2026-10-05": enough}
	if got := StreakOf(days, now); got != 2 {
		t.Errorf("streak ending yesterday = %d, want 2", got)
	}
	days["2026-10-09"] = enough
	if got := StreakOf(days, now); got != 3 {
		t.Errorf("streak with today = %d, want 3", got)
	}
	// A minute short is not a day.
	days["2026-10-08"] = short
	if got := StreakOf(days, now); got != 1 {
		t.Errorf("streak across a short day = %d, want 1", got)
	}
}

func TestWeekOf(t *testing.T) {
	days := map[string]int{
		"2026-10-04": 1000, // Sunday: last week
		"2026-10-05": 100,  // Monday
		"2026-10-09": 20,   // Friday
		"2026-10-11": 3,    // Sunday: still this week
		"2026-10-12": 5000, // Monday: next week
	}
	for _, now := range []time.Time{
		time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 11, 23, 59, 0, 0, time.UTC),
	} {
		if got := WeekOf(days, now); got != 123 {
			t.Errorf("week of %s = %d, want 123 (Monday to Sunday)", now.Format("Mon 2 Jan"), got)
		}
	}
}

func TestGetStats(t *testing.T) {
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	fake := newFakeDeskTick(t)
	fake.workspaces = []map[string]any{{"id": "ws1", "name": "Acme", "slug": "acme"}, {"id": "ws2", "name": "Zeta", "slug": "zeta"}}
	fake.summaries["ws1"] = []map[string]any{{"day": "2026-10-09", "seconds": 3600}, {"day": "2026-10-08", "seconds": 1800}}
	fake.summaries["ws2"] = []map[string]any{{"day": "2026-10-09", "seconds": 600}, {"day": "2026-10-01", "seconds": 9000}}
	fake.current["ws2"] = map[string]any{"taskCode": "ZED-4", "startedAt": "2026-10-09T14:30:00.000Z"}

	stats, err := fake.client().GetStats(now)
	if err != nil {
		t.Fatal(err)
	}
	// Both workspaces, added up.
	if stats.TodaySeconds != 4200 {
		t.Errorf("today = %d, want 4200", stats.TodaySeconds)
	}
	if stats.WeekSeconds != 6000 {
		t.Errorf("week = %d, want 6000 (last week's 9000 is not in it)", stats.WeekSeconds)
	}
	if stats.Streak != 2 {
		t.Errorf("streak = %d, want 2", stats.Streak)
	}
	if !stats.Running || stats.TrackingCode != "ZED-4" || !stats.TrackingSince.Equal(time.Date(2026, 10, 9, 14, 30, 0, 0, time.UTC)) {
		t.Errorf("tracking = %+v", stats)
	}
	// 62 days, which is what the API allows.
	asked := fake.asked("GET /api/workspaces/ws1/time/summary")
	if len(asked) != 1 || asked[0] != "GET /api/workspaces/ws1/time/summary?from=2026-08-09&to=2026-10-09" {
		t.Errorf("summary asked as %v", asked)
	}
}

func TestGetStatsWithNothingRunning(t *testing.T) {
	fake := newFakeDeskTick(t)
	fake.oneProject()
	stats, err := fake.client().GetStats(time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Running || stats.TrackingCode != "" || stats.TodaySeconds != 0 {
		t.Errorf("stats = %+v", stats)
	}
}
