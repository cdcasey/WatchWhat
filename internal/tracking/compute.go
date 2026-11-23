// Package tracking owns status, watch state, and the derived progress rules
// (next episode, caught up, stale). Compute is pure; Service does I/O.
package tracking

import "time"

// StaleAfterDays: a watching show is stale when idle for more than this many calendar days.
const StaleAfterDays = 14

const (
	StatusBacklog   = "backlog"
	StatusWatching  = "watching"
	StatusCompleted = "completed"
	StatusDropped   = "dropped"
)

var Statuses = []string{StatusWatching, StatusBacklog, StatusCompleted, StatusDropped}

func ValidStatus(s string) bool {
	switch s {
	case StatusBacklog, StatusWatching, StatusCompleted, StatusDropped:
		return true
	}
	return false
}

type EpisodeRef struct {
	ID     int64
	Season int64
	Number int64
	Name   string
}

// ProgressRow is the per-item aggregate the store returns. Zero values mean "none".
type ProgressRow struct {
	MediaType       string
	Status          string
	TMDBStatus      string
	StatusChangedAt int64 // unix seconds
	WatchedAt       int64 // movies: when marked watched
	AiredTotal      int64 // aired regular-season episodes
	WatchedTotal    int64 // watched among those
	LastWatchedAt   int64 // latest regular-season watch
	Next            *EpisodeRef
	NextAirDate     string // YYYY-MM-DD of the next unaired regular episode
}

type Progress struct {
	Next         *EpisodeRef // nil => caught up (tv)
	Upcoming     *time.Time  // next known air date, set only when caught up
	Watched      int
	Aired        int
	LastActivity time.Time
	DaysIdle     int // calendar days in now's location
	Stale        bool
	// CanComplete: an ended show being watched with nothing left to watch.
	CanComplete bool
}

// Compute derives progress as of now. now's location defines calendar days.
func Compute(r ProgressRow, now time.Time) Progress {
	loc := now.Location()
	p := Progress{Watched: int(r.WatchedTotal), Aired: int(r.AiredTotal)}

	switch {
	case r.LastWatchedAt > 0:
		p.LastActivity = time.Unix(r.LastWatchedAt, 0).In(loc)
	case r.MediaType == "movie" && r.WatchedAt > 0:
		p.LastActivity = time.Unix(r.WatchedAt, 0).In(loc)
	default:
		p.LastActivity = time.Unix(r.StatusChangedAt, 0).In(loc)
	}
	p.DaysIdle = daysBetween(p.LastActivity, now)

	if r.MediaType != "tv" {
		return p
	}
	p.Next = r.Next
	if p.Next == nil && r.NextAirDate != "" {
		if d, err := time.ParseInLocation(time.DateOnly, r.NextAirDate, loc); err == nil {
			p.Upcoming = &d
		}
	}
	p.Stale = r.Status == StatusWatching && p.Next != nil && p.DaysIdle > StaleAfterDays
	p.CanComplete = r.Status == StatusWatching && p.Next == nil && p.Upcoming == nil &&
		p.Aired > 0 && (r.TMDBStatus == "Ended" || r.TMDBStatus == "Canceled")
	return p
}

// daysBetween counts calendar-date boundaries from a to b (both in b's location).
func daysBetween(a, b time.Time) int {
	a = a.In(b.Location())
	da := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	db := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(db.Sub(da).Hours() / 24)
}
