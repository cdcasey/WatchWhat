package tracking_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/chriscasey/watchwhat/internal/db/dbtest"
	"github.com/chriscasey/watchwhat/internal/store"
	"github.com/chriscasey/watchwhat/internal/tracking"
)

type env struct {
	db  *sql.DB
	q   *store.Queries
	svc *tracking.Service
	now time.Time
}

func setup(t *testing.T) *env {
	d := dbtest.New(t)
	e := &env{db: d, q: store.New(d), now: time.Date(2024, 6, 20, 20, 0, 0, 0, time.UTC)}
	e.svc = &tracking.Service{DB: d, Q: e.q, Now: func() time.Time { return e.now }}
	return e
}

var tmdbSeq int64

func nextTMDB() int64 { tmdbSeq++; return tmdbSeq }

type ep struct {
	s, n int64
	air  string
}

// show inserts a tv show with episodes; returns media id and episode ids keyed "s.n".
func (e *env) show(t *testing.T, tmdbStatus, status string, eps ...ep) (int64, map[[2]int64]int64) {
	t.Helper()
	ctx := context.Background()
	id, err := e.q.UpsertMedia(ctx, store.UpsertMediaParams{TmdbID: nextTMDB(), MediaType: "tv", Title: "Show",
		TmdbStatus: sql.NullString{String: tmdbStatus, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	e.q.InsertTracking(ctx, store.InsertTrackingParams{MediaID: id, Status: status, StatusChangedAt: e.now.Unix(), AddedAt: e.now.Unix()})
	ids := map[[2]int64]int64{}
	for i, x := range eps {
		air := sql.NullString{String: x.air, Valid: x.air != ""}
		if err := e.q.UpsertEpisode(ctx, store.UpsertEpisodeParams{MediaID: id, TmdbID: id*10000 + int64(i), SeasonNumber: x.s, EpisodeNumber: x.n, AirDate: air}); err != nil {
			t.Fatal(err)
		}
		var eid int64
		e.db.QueryRow(`SELECT id FROM episodes WHERE tmdb_id = ?`, id*10000+int64(i)).Scan(&eid)
		ids[[2]int64{x.s, x.n}] = eid
	}
	return id, ids
}

func (e *env) item(t *testing.T, id int64) tracking.Item {
	t.Helper()
	it, err := e.svc.Item(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func TestGapInProgress(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id, eps := e.show(t, "Ended", "watching", ep{1, 1, "2024-01-01"}, ep{1, 2, "2024-01-08"}, ep{1, 3, "2024-01-15"})
	e.svc.MarkEpisode(ctx, eps[[2]int64{1, 1}])
	e.svc.MarkEpisode(ctx, eps[[2]int64{1, 3}])
	p := e.item(t, id).Progress
	if p.Next == nil || p.Next.Season != 1 || p.Next.Number != 2 {
		t.Fatalf("next=%+v", p.Next)
	}
	if p.Watched != 2 || p.Aired != 3 {
		t.Fatalf("progress %d/%d", p.Watched, p.Aired)
	}
}

func TestCaughtUpWithUpcoming(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id, eps := e.show(t, "Returning Series", "watching", ep{1, 1, "2024-01-01"}, ep{2, 5, "2024-06-26"})
	e.svc.MarkEpisode(ctx, eps[[2]int64{1, 1}])
	p := e.item(t, id).Progress
	if p.Next != nil || p.Upcoming == nil || p.Upcoming.Format(time.DateOnly) != "2024-06-26" {
		t.Fatalf("%+v", p)
	}
}

func TestNewEpisodeAiringTodayMakesStale(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id, eps := e.show(t, "Returning Series", "watching", ep{1, 1, "2024-01-01"}, ep{1, 2, "2024-06-20"})
	now := e.now
	e.now = now.AddDate(0, 0, -40)
	e.svc.MarkEpisode(ctx, eps[[2]int64{1, 1}])
	e.now = now
	p := e.item(t, id).Progress
	if p.Next == nil || p.Next.Number != 2 || !p.Stale || p.DaysIdle != 40 {
		t.Fatalf("%+v next=%+v", p, p.Next)
	}
}

func TestSpecialsExcluded(t *testing.T) {
	e := setup(t)
	id, _ := e.show(t, "Ended", "watching", ep{0, 1, "2024-01-01"}, ep{1, 1, "2024-01-01"})
	p := e.item(t, id).Progress
	if p.Aired != 1 || p.Next == nil || p.Next.Season != 1 {
		t.Fatalf("%+v", p)
	}
}

func TestMarkTwiceAndUnmark(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id, eps := e.show(t, "Ended", "watching", ep{1, 1, "2024-01-01"}, ep{1, 2, "2024-01-02"})
	e1 := eps[[2]int64{1, 1}]
	for range 2 {
		if mid, err := e.svc.MarkEpisode(ctx, e1); err != nil || mid != id {
			t.Fatal(mid, err)
		}
	}
	if n, _ := e.q.CountWatches(ctx, id); n != 1 {
		t.Fatalf("watches=%d", n)
	}
	if e.item(t, id).Progress.Next.Number != 2 {
		t.Fatal("next should advance once")
	}
	e.svc.UnmarkEpisode(ctx, e1)
	if n, _ := e.q.CountWatches(ctx, id); n != 0 {
		t.Fatal("unmark did not clear")
	}
	if e.item(t, id).Progress.Next.Number != 1 {
		t.Fatal("next should return to E1 after undo")
	}
	if _, err := e.svc.MarkEpisode(ctx, 999999); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unknown episode: %v", err)
	}
}

func TestMarkSeasonAiredOnly(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id, eps := e.show(t, "Returning Series", "watching",
		ep{1, 1, "2024-01-01"}, ep{1, 2, "2024-06-20"}, ep{1, 3, "2024-07-01"}, ep{1, 4, ""}, ep{2, 1, "2024-01-01"})
	if err := e.svc.MarkSeason(ctx, id, 1); err != nil {
		t.Fatal(err)
	}
	se, err := e.svc.Season(ctx, id, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64]bool{}
	for _, x := range se.Eps {
		got[x.Number] = x.Watched
	}
	if !got[1] || !got[2] || got[3] || got[4] {
		t.Fatalf("watched: %v", got)
	}
	if se.Watched != 2 || se.Aired != 2 {
		t.Fatalf("season counts %d/%d", se.Watched, se.Aired)
	}
	if ep, _ := e.svc.Episode(ctx, eps[[2]int64{2, 1}]); ep.Watched {
		t.Fatal("other season touched")
	}
}

func TestSetStatus(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id, _ := e.show(t, "Ended", "backlog")
	start := e.now
	e.now = start.Add(time.Hour)
	if err := e.svc.SetStatus(ctx, id, "watching"); err != nil {
		t.Fatal(err)
	}
	tr, _ := e.q.GetTracking(ctx, id)
	if tr.Status != "watching" || tr.StatusChangedAt != e.now.Unix() {
		t.Fatalf("%+v", tr)
	}
	e.now = start.Add(2 * time.Hour)
	e.svc.SetStatus(ctx, id, "watching")
	tr, _ = e.q.GetTracking(ctx, id)
	if tr.StatusChangedAt != start.Add(time.Hour).Unix() {
		t.Fatal("same-status set reset status_changed_at")
	}
	if err := e.svc.SetStatus(ctx, id, "paused"); !errors.Is(err, tracking.ErrInvalid) {
		t.Fatal(err)
	}
	if err := e.svc.SetStatus(ctx, 9999, "watching"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
}

func TestMovieWatched(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id, _ := e.q.UpsertMedia(ctx, store.UpsertMediaParams{TmdbID: 7, MediaType: "movie", Title: "Heat"})
	e.q.InsertTracking(ctx, store.InsertTrackingParams{MediaID: id, Status: "watching"})
	if err := e.svc.MarkMovieWatched(ctx, id); err != nil {
		t.Fatal(err)
	}
	it := e.item(t, id)
	if it.Status != "completed" || it.WatchedAt == nil || !it.WatchedAt.Equal(e.now) {
		t.Fatalf("%+v", it)
	}
	tv, _ := e.show(t, "Ended", "watching")
	if err := e.svc.MarkMovieWatched(ctx, tv); !errors.Is(err, tracking.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestApply(t *testing.T) {
	now := time.Now()
	mk := func(title, typ string, days int, stale bool) tracking.Item {
		return tracking.Item{Title: title, Type: typ, Progress: tracking.Progress{DaysIdle: days, Stale: stale, LastActivity: now.AddDate(0, 0, -days)}}
	}
	items := []tracking.Item{mk("B", "tv", 1, false), mk("A", "tv", 0, false), mk("C", "tv", 30, true), mk("D", "movie", 20, false), mk("E", "tv", 16, true)}
	titles := func(xs []tracking.Item) (s string) {
		for _, x := range xs {
			s += x.Title
		}
		return
	}
	if got := titles(tracking.Apply(items, tracking.ListOpts{})); got != "ABEDC" {
		t.Errorf("recent: %s", got)
	}
	if got := titles(tracking.Apply(items, tracking.ListOpts{Sort: "stale"})); got != "CDEBA" {
		t.Errorf("most stale: %s", got)
	}
	if got := titles(tracking.Apply(items, tracking.ListOpts{StaleOnly: true})); got != "EC" {
		t.Errorf("stale only: %s", got)
	}
	if got := titles(tracking.Apply(items, tracking.ListOpts{Type: "movie"})); got != "D" {
		t.Errorf("movies: %s", got)
	}
}
