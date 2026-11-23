package tracking

import (
	"testing"
	"time"
)

var chicago = must(time.LoadLocation("America/Chicago"))

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// now is 2024-06-20 20:00 local.
var now = time.Date(2024, 6, 20, 20, 0, 0, 0, chicago)

func daysAgo(n int) int64 { return now.AddDate(0, 0, -n).Unix() }

var next = &EpisodeRef{ID: 9, Season: 1, Number: 4, Name: "Ep 4"}

func TestCompute(t *testing.T) {
	tests := []struct {
		name      string
		row       ProgressRow
		stale     bool
		days      int
		caughtUp  bool
		upcoming  string
		canFinish bool
	}{
		{name: "inactive with episodes remaining (15d)", stale: true, days: 15,
			row: ProgressRow{MediaType: "tv", Status: "watching", LastWatchedAt: daysAgo(15), Next: next}},
		{name: "within threshold (14d)", stale: false, days: 14,
			row: ProgressRow{MediaType: "tv", Status: "watching", LastWatchedAt: daysAgo(14), Next: next}},
		{name: "badge days (21d)", stale: true, days: 21,
			row: ProgressRow{MediaType: "tv", Status: "watching", LastWatchedAt: daysAgo(21), Next: next}},
		{name: "caught up and waiting (30d)", stale: false, days: 30, caughtUp: true,
			row: ProgressRow{MediaType: "tv", Status: "watching", LastWatchedAt: daysAgo(30)}},
		{name: "started but never progressed (20d)", stale: true, days: 20,
			row: ProgressRow{MediaType: "tv", Status: "watching", StatusChangedAt: daysAgo(20), Next: next}},
		{name: "new episode airs today after 40d", stale: true, days: 40,
			row: ProgressRow{MediaType: "tv", Status: "watching", LastWatchedAt: daysAgo(40), Next: next}},
		{name: "watch time wins over later status change", stale: true, days: 30,
			row: ProgressRow{MediaType: "tv", Status: "watching", LastWatchedAt: daysAgo(30), StatusChangedAt: daysAgo(1), Next: next}},
		{name: "backlog never stale", stale: false, days: 60,
			row: ProgressRow{MediaType: "tv", Status: "backlog", StatusChangedAt: daysAgo(60), Next: next}},
		{name: "dropped never stale", stale: false, days: 60,
			row: ProgressRow{MediaType: "tv", Status: "dropped", LastWatchedAt: daysAgo(60), Next: next}},
		{name: "movie never stale", stale: false, days: 60, caughtUp: true,
			row: ProgressRow{MediaType: "movie", Status: "watching", StatusChangedAt: daysAgo(60)}},
		{name: "caught up with upcoming episode", caughtUp: true, upcoming: "2024-06-26", days: 2,
			row: ProgressRow{MediaType: "tv", Status: "watching", LastWatchedAt: daysAgo(2), NextAirDate: "2024-06-26"}},
		{name: "ended and finished offers completion", caughtUp: true, canFinish: true,
			row: ProgressRow{MediaType: "tv", Status: "watching", TMDBStatus: "Ended", AiredTotal: 8, WatchedTotal: 8, LastWatchedAt: daysAgo(0)}},
		{name: "returning series finished does not offer completion", caughtUp: true,
			row: ProgressRow{MediaType: "tv", Status: "watching", TMDBStatus: "Returning Series", AiredTotal: 8, WatchedTotal: 8, LastWatchedAt: daysAgo(0)}},
		{name: "ended with remaining episode does not offer completion", days: 0,
			row: ProgressRow{MediaType: "tv", Status: "watching", TMDBStatus: "Ended", AiredTotal: 8, WatchedTotal: 7, LastWatchedAt: daysAgo(0), Next: next}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := Compute(tc.row, now)
			if p.Stale != tc.stale {
				t.Errorf("stale=%v want %v", p.Stale, tc.stale)
			}
			if p.DaysIdle != tc.days {
				t.Errorf("days=%d want %d", p.DaysIdle, tc.days)
			}
			if (p.Next == nil) != tc.caughtUp {
				t.Errorf("caughtUp=%v want %v", p.Next == nil, tc.caughtUp)
			}
			gotUp := ""
			if p.Upcoming != nil {
				gotUp = p.Upcoming.Format(time.DateOnly)
			}
			if gotUp != tc.upcoming {
				t.Errorf("upcoming=%q want %q", gotUp, tc.upcoming)
			}
			if p.CanComplete != tc.canFinish {
				t.Errorf("canComplete=%v want %v", p.CanComplete, tc.canFinish)
			}
		})
	}
}

func TestDayPasses(t *testing.T) {
	lastWatch := time.Date(2024, 6, 1, 22, 0, 0, 0, chicago).Unix()
	row := ProgressRow{MediaType: "tv", Status: "watching", LastWatchedAt: lastWatch, Next: next}
	day14 := time.Date(2024, 6, 15, 23, 59, 0, 0, chicago)
	day15 := time.Date(2024, 6, 16, 0, 1, 0, 0, chicago)
	if Compute(row, day14).Stale {
		t.Error("stale on day 14")
	}
	if !Compute(row, day15).Stale {
		t.Error("not stale on day 15")
	}
}

func TestCalendarDaysNotHours(t *testing.T) {
	// Watched late evening; checked early morning 15 calendar days later (< 15*24h).
	lastWatch := time.Date(2024, 6, 1, 23, 30, 0, 0, chicago).Unix()
	row := ProgressRow{MediaType: "tv", Status: "watching", LastWatchedAt: lastWatch, Next: next}
	p := Compute(row, time.Date(2024, 6, 16, 8, 0, 0, 0, chicago))
	if p.DaysIdle != 15 || !p.Stale {
		t.Fatalf("days=%d stale=%v", p.DaysIdle, p.Stale)
	}
}

func TestDaysUseServerTZ(t *testing.T) {
	// 2024-06-20 03:00 UTC is still 2024-06-19 in Chicago.
	last := time.Date(2024, 6, 20, 3, 0, 0, 0, time.UTC).Unix()
	p := Compute(ProgressRow{MediaType: "tv", Status: "watching", LastWatchedAt: last}, now)
	if p.DaysIdle != 1 {
		t.Fatalf("days=%d want 1", p.DaysIdle)
	}
}
