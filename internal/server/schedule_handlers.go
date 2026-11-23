package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/chriscasey/watchwhat/internal/schedule"
	"github.com/chriscasey/watchwhat/internal/views/components"
	"github.com/chriscasey/watchwhat/internal/views/pages"
)

func (a *App) schedulePage(w http.ResponseWriter, r *http.Request) error {
	days, err := a.Sched.Week(r.Context())
	if err != nil {
		return err
	}
	opts, err := a.Sched.Schedulable(r.Context())
	if err != nil {
		return err
	}
	return a.render(w, r, "Schedule", "schedule", pages.Schedule(days, opts, a.Now().Weekday()))
}

// day answers a planner action with only the affected day's column.
func (a *App) day(w http.ResponseWriter, r *http.Request, wd time.Weekday) error {
	d, err := a.Sched.Day(r.Context(), wd)
	if err != nil {
		return err
	}
	opts, err := a.Sched.Schedulable(r.Context())
	if err != nil {
		return err
	}
	return a.fragment(w, r, components.DayColumn(d, opts, wd == a.Now().Weekday()))
}

func pathDay(r *http.Request) (time.Weekday, error) {
	wd, err := schedule.ParseWeekday(r.PathValue("day"))
	if err != nil {
		return 0, errNotFound
	}
	return wd, nil
}

func (a *App) scheduleAdd(w http.ResponseWriter, r *http.Request) error {
	wd, err := pathDay(r)
	if err != nil {
		return err
	}
	id, err := strconv.ParseInt(r.FormValue("media_id"), 10, 64)
	if err != nil || id <= 0 {
		return badRequest("Pick a show to add.")
	}
	if err := a.Sched.Assign(r.Context(), wd, id); err != nil {
		return err
	}
	return a.day(w, r, wd)
}

func (a *App) scheduleRemove(w http.ResponseWriter, r *http.Request) error {
	wd, err := pathDay(r)
	if err != nil {
		return err
	}
	id, err := pathInt(r, "media_id")
	if err != nil {
		return err
	}
	if err := a.Sched.Remove(r.Context(), wd, id); err != nil {
		return err
	}
	return a.day(w, r, wd)
}

func (a *App) scheduleMove(w http.ResponseWriter, r *http.Request) error {
	wd, err := pathDay(r)
	if err != nil {
		return err
	}
	id, err := pathInt(r, "media_id")
	if err != nil {
		return err
	}
	dir := r.FormValue("dir")
	if dir != "up" && dir != "down" {
		return badRequest("Invalid direction.")
	}
	if err := a.Sched.Move(r.Context(), wd, id, dir == "up"); err != nil {
		return err
	}
	return a.day(w, r, wd)
}
