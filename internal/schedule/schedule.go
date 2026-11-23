// Package schedule manages the household's weekday assignments and today's lineup.
package schedule

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/chriscasey/watchwhat/internal/store"
)

var ErrNotSchedulable = errors.New("only backlog or watching TV shows can be scheduled")

// WeekOrder is the display order, Monday first.
var WeekOrder = []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday}

type Service struct {
	DB *sql.DB
	Q  *store.Queries
}

type Entry struct {
	Weekday    time.Weekday
	MediaID    int64
	Title      string
	PosterPath string
	Status     string
	// Inactive entries (completed/dropped) are kept but hidden from the lineup.
	Inactive bool
}

type Day struct {
	Weekday time.Weekday
	Entries []Entry
}

func active(status string) bool { return status == "backlog" || status == "watching" }

// Assign adds a show to a weekday. Re-assigning is a no-op.
func (s *Service) Assign(ctx context.Context, wd time.Weekday, mediaID int64) error {
	m, err := s.Q.GetMedia(ctx, mediaID)
	if err != nil {
		return err
	}
	tr, err := s.Q.GetTracking(ctx, mediaID)
	if err != nil {
		return err
	}
	if m.MediaType != "tv" || !active(tr.Status) {
		return ErrNotSchedulable
	}
	_, err = s.Q.AssignSchedule(ctx, store.AssignScheduleParams{Weekday: int64(wd), MediaID: mediaID})
	return err
}

func (s *Service) Remove(ctx context.Context, wd time.Weekday, mediaID int64) error {
	return s.Q.RemoveSchedule(ctx, store.RemoveScheduleParams{Weekday: int64(wd), MediaID: mediaID})
}

// Move swaps a show's position with its neighbour. Moving past either end is a no-op.
func (s *Service) Move(ctx context.Context, wd time.Weekday, mediaID int64, up bool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.Q.WithTx(tx)
	cur, err := q.GetScheduleEntry(ctx, store.GetScheduleEntryParams{Weekday: int64(wd), MediaID: mediaID})
	if err != nil {
		return err
	}
	var other store.WeeklySchedule
	if up {
		other, err = q.PrevScheduleEntry(ctx, store.PrevScheduleEntryParams{Weekday: int64(wd), Position: cur.Position})
	} else {
		other, err = q.NextScheduleEntry(ctx, store.NextScheduleEntryParams{Weekday: int64(wd), Position: cur.Position})
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := q.SetSchedulePosition(ctx, store.SetSchedulePositionParams{Position: other.Position, Weekday: int64(wd), MediaID: cur.MediaID}); err != nil {
		return err
	}
	if err := q.SetSchedulePosition(ctx, store.SetSchedulePositionParams{Position: cur.Position, Weekday: int64(wd), MediaID: other.MediaID}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) Day(ctx context.Context, wd time.Weekday) (Day, error) {
	rows, err := s.Q.ListScheduleDay(ctx, int64(wd))
	if err != nil {
		return Day{}, err
	}
	d := Day{Weekday: wd}
	for _, r := range rows {
		d.Entries = append(d.Entries, entry(r.Weekday, r.MediaID, r.Title, r.PosterPath, r.Status))
	}
	return d, nil
}

// Week returns all seven days, Monday first.
func (s *Service) Week(ctx context.Context) ([]Day, error) {
	rows, err := s.Q.ListSchedule(ctx)
	if err != nil {
		return nil, err
	}
	byDay := map[time.Weekday][]Entry{}
	for _, r := range rows {
		byDay[time.Weekday(r.Weekday)] = append(byDay[time.Weekday(r.Weekday)], entry(r.Weekday, r.MediaID, r.Title, r.PosterPath, r.Status))
	}
	out := make([]Day, 0, 7)
	for _, wd := range WeekOrder {
		out = append(out, Day{Weekday: wd, Entries: byDay[wd]})
	}
	return out, nil
}

// Lineup returns the active shows assigned to wd, in schedule order.
func (s *Service) Lineup(ctx context.Context, wd time.Weekday) ([]int64, error) {
	d, err := s.Day(ctx, wd)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, e := range d.Entries {
		if !e.Inactive {
			ids = append(ids, e.MediaID)
		}
	}
	return ids, nil
}

type Option struct {
	MediaID int64
	Title   string
}

// Schedulable lists the shows that may be assigned to a day.
func (s *Service) Schedulable(ctx context.Context) ([]Option, error) {
	rows, err := s.Q.ListSchedulable(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Option, len(rows))
	for i, r := range rows {
		out[i] = Option{MediaID: r.ID, Title: r.Title}
	}
	return out, nil
}

func entry(wd, mediaID int64, title string, poster sql.NullString, status string) Entry {
	return Entry{Weekday: time.Weekday(wd), MediaID: mediaID, Title: title, PosterPath: poster.String, Status: status, Inactive: !active(status)}
}

// ParseWeekday parses a 0–6 (Sunday = 0) path segment.
func ParseWeekday(s string) (time.Weekday, error) {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil || n < 0 || n > 6 || fmt.Sprint(n) != s {
		return 0, fmt.Errorf("invalid weekday %q", s)
	}
	return time.Weekday(n), nil
}
