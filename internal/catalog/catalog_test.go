package catalog_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/chriscasey/watchwhat/internal/catalog"
	"github.com/chriscasey/watchwhat/internal/db/dbtest"
	"github.com/chriscasey/watchwhat/internal/store"
	"github.com/chriscasey/watchwhat/internal/tmdb"
	"github.com/chriscasey/watchwhat/internal/tmdb/tmdbtest"
)

var t0 = time.Date(2024, 6, 20, 12, 0, 0, 0, time.UTC)

type env struct {
	db   *sql.DB
	q    *store.Queries
	fake *tmdbtest.Fake
	svc  *catalog.Service
	now  time.Time
}

func setup(t *testing.T) *env {
	d := dbtest.New(t)
	e := &env{db: d, q: store.New(d), fake: tmdbtest.New(t), now: t0}
	e.svc = &catalog.Service{DB: d, Q: e.q, TMDB: e.fake.Client(), Now: func() time.Time { return e.now }}
	return e
}

func showWithSpecials() *tmdbtest.Show {
	return &tmdbtest.Show{ID: 100, Name: "Shōgun", Status: "Returning Series", Episodes: []tmdbtest.Episode{
		{ID: 1, Season: 0, Number: 1, Name: "Making of", AirDate: "2024-01-01"},
		{ID: 2, Season: 1, Number: 1, Name: "Anjin", AirDate: "2024-02-27"},
		{ID: 3, Season: 1, Number: 2, Name: "Servants", AirDate: "2024-02-27"},
	}}
}

func (e *env) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAddShowStoresSeasonsAndSpecials(t *testing.T) {
	e := setup(t)
	e.fake.Shows[100] = showWithSpecials()
	id, err := e.svc.Add(context.Background(), "tv", 100, "backlog")
	if err != nil {
		t.Fatal(err)
	}
	tr, _ := e.q.GetTracking(context.Background(), id)
	if tr.Status != "backlog" || tr.AddedAt != t0.Unix() {
		t.Fatalf("tracking: %+v", tr)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM episodes WHERE media_id = ? AND season_number = 0`, id); n != 1 {
		t.Fatalf("specials stored: %d", n)
	}
	row, _ := e.q.GetItem(context.Background(), store.GetItemParams{Today: "2024-06-20", MediaID: id})
	if row.AiredTotal != 2 {
		t.Fatalf("progress total should exclude specials: %d", row.AiredTotal)
	}
}

func TestAddMovieAsWatching(t *testing.T) {
	e := setup(t)
	e.fake.Movies[7] = &tmdbtest.Movie{ID: 7, Title: "Heat", Status: "Released", Runtime: 170}
	id, err := e.svc.Add(context.Background(), "movie", 7, "watching")
	if err != nil {
		t.Fatal(err)
	}
	m, _ := e.q.GetMedia(context.Background(), id)
	tr, _ := e.q.GetTracking(context.Background(), id)
	if m.Title != "Heat" || m.MediaType != "movie" || tr.Status != "watching" {
		t.Fatalf("%+v %+v", m, tr)
	}
}

func TestAddIdempotent(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.fake.Shows[100] = showWithSpecials()
	id1, _ := e.svc.Add(ctx, "tv", 100, "watching")
	calls := e.fake.Calls()
	id2, err := e.svc.Add(ctx, "tv", 100, "backlog")
	if err != nil {
		t.Fatal(err)
	}
	tr, _ := e.q.GetTracking(ctx, id1)
	if id1 != id2 || tr.Status != "watching" || e.count(t, `SELECT COUNT(*) FROM media`) != 1 {
		t.Fatalf("ids %d/%d status %s", id1, id2, tr.Status)
	}
	if e.fake.Calls() != calls {
		t.Fatal("second add should not call TMDB")
	}
}

func TestAddRejectsBadInput(t *testing.T) {
	e := setup(t)
	if _, err := e.svc.Add(context.Background(), "tv", 1, "completed"); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := e.svc.Add(context.Background(), "person", 1, "backlog"); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestAddTMDBDownWritesNothing(t *testing.T) {
	e := setup(t)
	e.fake.Down = true
	if _, err := e.svc.Add(context.Background(), "tv", 100, "backlog"); !errors.Is(err, tmdb.ErrUnavailable) {
		t.Fatal(err)
	}
	if e.count(t, `SELECT COUNT(*) FROM media`) != 0 {
		t.Fatal("partial write on failure")
	}
}

func TestRefreshAddsAndRetains(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.fake.Shows[100] = showWithSpecials()
	id, _ := e.svc.Add(ctx, "tv", 100, "watching")
	var ep2 int64
	e.db.QueryRow(`SELECT id FROM episodes WHERE tmdb_id = 2`).Scan(&ep2)
	e.q.MarkEpisodeWatched(ctx, store.MarkEpisodeWatchedParams{EpisodeID: ep2, WatchedAt: 1})

	// Upstream: episode 2 removed, episode 3 renamed, new episode 4 announced.
	e.fake.Set(func(f *tmdbtest.Fake) {
		f.Shows[100].Episodes = []tmdbtest.Episode{
			{ID: 3, Season: 1, Number: 2, Name: "Servants of Two Masters", AirDate: "2024-02-27"},
			{ID: 4, Season: 1, Number: 3, Name: "Tomorrow", AirDate: "2024-03-05"},
		}
		f.Shows[100].Name = "Shōgun (2024)"
	})
	e.now = t0.Add(time.Hour)
	if err := e.svc.Refresh(ctx, id); err != nil {
		t.Fatal(err)
	}
	m, _ := e.q.GetMedia(ctx, id)
	if m.Title != "Shōgun (2024)" || m.RefreshedAt != e.now.Unix() {
		t.Fatalf("metadata not updated: %+v", m)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM episodes WHERE media_id = ?`, id); n != 4 {
		t.Fatalf("episodes=%d, want 4 (none deleted, one added)", n)
	}
	if e.count(t, `SELECT COUNT(*) FROM episode_watches WHERE episode_id = ?`, ep2) != 1 {
		t.Fatal("watch on removed-upstream episode lost")
	}
	var name string
	e.db.QueryRow(`SELECT name FROM episodes WHERE tmdb_id = 3`).Scan(&name)
	if name != "Servants of Two Masters" {
		t.Fatalf("title not updated: %q", name)
	}
}

func TestRemove(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.fake.Shows[100] = showWithSpecials()
	id, _ := e.svc.Add(ctx, "tv", 100, "watching")
	var ep int64
	e.db.QueryRow(`SELECT id FROM episodes WHERE tmdb_id = 2`).Scan(&ep)
	e.q.MarkEpisodeWatched(ctx, store.MarkEpisodeWatchedParams{EpisodeID: ep, WatchedAt: 1})
	e.q.AssignSchedule(ctx, store.AssignScheduleParams{Weekday: 2, MediaID: id})

	if err := e.svc.Remove(ctx, id); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range []string{"media", "tracking", "episodes", "episode_watches", "weekly_schedule"} {
		if n := e.count(t, "SELECT COUNT(*) FROM "+tbl); n != 0 {
			t.Errorf("%s has %d rows after remove", tbl, n)
		}
	}
	if err := e.svc.Remove(ctx, id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("second remove: %v", err)
	}
}

func TestRefresherSelection(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	add := func(id int64, status, tmdbStatus string) int64 {
		e.fake.Shows[id] = &tmdbtest.Show{ID: id, Name: "S", Status: tmdbStatus}
		mid, err := e.svc.Add(ctx, "tv", id, "backlog")
		if err != nil {
			t.Fatal(err)
		}
		e.q.SetStatus(ctx, store.SetStatusParams{Status: status, StatusChangedAt: 1, MediaID: mid})
		return mid
	}
	e.now = t0.Add(-25 * time.Hour) // these are all refreshed 25h before t0
	airingWatching := add(1, "watching", "Returning Series")
	airingBacklog := add(2, "backlog", "In Production")
	ended := add(3, "watching", "Ended")
	dropped := add(4, "dropped", "Returning Series")
	e.now = t0.Add(-1 * time.Hour)
	fresh := add(5, "watching", "Returning Series")
	e.fake.Movies[6] = &tmdbtest.Movie{ID: 6, Title: "M", Status: "Released"}
	e.svc.Add(ctx, "movie", 6, "watching")
	_ = fresh

	e.now = t0
	e.fake.Set(func(f *tmdbtest.Fake) { f.Requests = nil })
	r := &catalog.Refresher{Svc: e.svc, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if n := r.RunOnce(ctx); n != 2 {
		t.Fatalf("refreshed %d, want 2", n)
	}
	refreshed := func(id int64) bool { m, _ := e.q.GetMedia(ctx, id); return m.RefreshedAt == t0.Unix() }
	if !refreshed(airingWatching) || !refreshed(airingBacklog) || refreshed(ended) || refreshed(dropped) {
		t.Fatal("wrong selection")
	}
	// Second run: nothing is older than 24h any more.
	if n := r.RunOnce(ctx); n != 0 {
		t.Fatalf("second run refreshed %d", n)
	}
}

func TestRefresherFailureStaysEligible(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.fake.Shows[1] = &tmdbtest.Show{ID: 1, Name: "S", Status: "Returning Series"}
	e.now = t0.Add(-48 * time.Hour)
	id, _ := e.svc.Add(ctx, "tv", 1, "watching")
	e.now = t0
	e.fake.Set(func(f *tmdbtest.Fake) { f.Down = true })
	r := &catalog.Refresher{Svc: e.svc, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if r.RunOnce(ctx) != 0 {
		t.Fatal("should fail")
	}
	e.fake.Set(func(f *tmdbtest.Fake) { f.Down = false })
	if r.RunOnce(ctx) != 1 {
		t.Fatal("should retry next cycle")
	}
	m, _ := e.q.GetMedia(ctx, id)
	if m.RefreshedAt != t0.Unix() {
		t.Fatal("not refreshed")
	}
}

func TestRefresherRunStopsOnCancel(t *testing.T) {
	e := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	r := &catalog.Refresher{Svc: e.svc, InitialDelay: time.Millisecond, Interval: time.Millisecond,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop")
	}
}
