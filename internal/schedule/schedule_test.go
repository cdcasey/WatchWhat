package schedule_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/chriscasey/watchwhat/internal/db/dbtest"
	"github.com/chriscasey/watchwhat/internal/schedule"
	"github.com/chriscasey/watchwhat/internal/store"
)

type env struct {
	q   *store.Queries
	svc *schedule.Service
	seq int64
}

func setup(t *testing.T) *env {
	d := dbtest.New(t)
	q := store.New(d)
	return &env{q: q, svc: &schedule.Service{DB: d, Q: q}}
}

func (e *env) add(t *testing.T, typ, title, status string) int64 {
	t.Helper()
	e.seq++
	id, err := e.q.UpsertMedia(context.Background(), store.UpsertMediaParams{TmdbID: e.seq, MediaType: typ, Title: title})
	if err != nil {
		t.Fatal(err)
	}
	e.q.InsertTracking(context.Background(), store.InsertTrackingParams{MediaID: id, Status: status})
	return id
}

func titles(d schedule.Day) (s []string) {
	for _, e := range d.Entries {
		s = append(s, e.Title)
	}
	return
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAssignIdempotentAndMultipleDays(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	sh := e.add(t, "tv", "Shōgun", "watching")
	for range 2 {
		if err := e.svc.Assign(ctx, time.Tuesday, sh); err != nil {
			t.Fatal(err)
		}
	}
	e.svc.Assign(ctx, time.Monday, sh)
	e.svc.Assign(ctx, time.Thursday, sh)
	tue, _ := e.svc.Day(ctx, time.Tuesday)
	if !eq(titles(tue), []string{"Shōgun"}) {
		t.Fatalf("tuesday: %v", titles(tue))
	}
	week, _ := e.svc.Week(ctx)
	if week[0].Weekday != time.Monday || week[6].Weekday != time.Sunday {
		t.Fatal("week must start Monday")
	}
	if len(week[0].Entries) != 1 || len(week[3].Entries) != 1 || len(week[2].Entries) != 0 {
		t.Fatal("multi-day assignment wrong")
	}
}

func TestAssignRejects(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	movie := e.add(t, "movie", "Heat", "watching")
	done := e.add(t, "tv", "Done", "completed")
	for _, id := range []int64{movie, done} {
		if err := e.svc.Assign(ctx, time.Monday, id); !errors.Is(err, schedule.ErrNotSchedulable) {
			t.Fatalf("%d: %v", id, err)
		}
	}
	if err := e.svc.Assign(ctx, time.Monday, 999); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
}

func TestRemoveAndReorder(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	a, b, c := e.add(t, "tv", "A", "watching"), e.add(t, "tv", "B", "watching"), e.add(t, "tv", "C", "backlog")
	for _, id := range []int64{a, b, c} {
		e.svc.Assign(ctx, time.Friday, id)
	}
	e.svc.Assign(ctx, time.Saturday, a)

	if err := e.svc.Move(ctx, time.Friday, b, true); err != nil {
		t.Fatal(err)
	}
	fri, _ := e.svc.Day(ctx, time.Friday)
	if !eq(titles(fri), []string{"B", "A", "C"}) {
		t.Fatalf("after move up: %v", titles(fri))
	}
	e.svc.Move(ctx, time.Friday, b, true)  // already first: no-op
	e.svc.Move(ctx, time.Friday, c, false) // already last: no-op
	e.svc.Move(ctx, time.Friday, a, false)
	fri, _ = e.svc.Day(ctx, time.Friday)
	if !eq(titles(fri), []string{"B", "C", "A"}) {
		t.Fatalf("after moves: %v", titles(fri))
	}

	e.svc.Remove(ctx, time.Friday, a)
	fri, _ = e.svc.Day(ctx, time.Friday)
	sat, _ := e.svc.Day(ctx, time.Saturday)
	if !eq(titles(fri), []string{"B", "C"}) || !eq(titles(sat), []string{"A"}) {
		t.Fatalf("remove: fri=%v sat=%v", titles(fri), titles(sat))
	}
	// New assignments go to the end even after removals/reorders.
	d := e.add(t, "tv", "D", "watching")
	e.svc.Assign(ctx, time.Friday, d)
	fri, _ = e.svc.Day(ctx, time.Friday)
	if !eq(titles(fri), []string{"B", "C", "D"}) {
		t.Fatalf("append: %v", titles(fri))
	}
}

func TestLineupExcludesInactiveButKeepsAssignment(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	a, b := e.add(t, "tv", "A", "watching"), e.add(t, "tv", "B", "watching")
	e.svc.Assign(ctx, time.Wednesday, a)
	e.svc.Assign(ctx, time.Wednesday, b)
	e.q.SetStatus(ctx, store.SetStatusParams{Status: "dropped", MediaID: a})

	ids, _ := e.svc.Lineup(ctx, time.Wednesday)
	if len(ids) != 1 || ids[0] != b {
		t.Fatalf("lineup: %v", ids)
	}
	wed, _ := e.svc.Day(ctx, time.Wednesday)
	if len(wed.Entries) != 2 || !wed.Entries[0].Inactive || wed.Entries[1].Inactive {
		t.Fatalf("schedule should keep dropped show as inactive: %+v", wed.Entries)
	}
	e.q.SetStatus(ctx, store.SetStatusParams{Status: "watching", MediaID: a})
	if ids, _ := e.svc.Lineup(ctx, time.Wednesday); len(ids) != 2 || ids[0] != a {
		t.Fatalf("returned to watching should reappear in order: %v", ids)
	}
}

func TestParseWeekday(t *testing.T) {
	for _, ok := range []string{"0", "6"} {
		if _, err := schedule.ParseWeekday(ok); err != nil {
			t.Error(ok, err)
		}
	}
	for _, bad := range []string{"7", "-1", "x", "01", ""} {
		if _, err := schedule.ParseWeekday(bad); err == nil {
			t.Error("accepted", bad)
		}
	}
}
