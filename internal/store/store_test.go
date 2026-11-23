package store_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/chriscasey/watchwhat/internal/db/dbtest"
	"github.com/chriscasey/watchwhat/internal/store"
)

func str(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

func TestUpsertMediaIdempotent(t *testing.T) {
	ctx := context.Background()
	q := store.New(dbtest.New(t))
	p := store.UpsertMediaParams{TmdbID: 42, MediaType: "tv", Title: "Old", TmdbStatus: str("Returning Series"), RefreshedAt: 1}
	id1, err := q.UpsertMedia(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	p.Title, p.RefreshedAt = "New", 2
	id2, err := q.UpsertMedia(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("upsert changed id: %d -> %d", id1, id2)
	}
	m, err := q.GetMedia(ctx, id1)
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "New" || m.RefreshedAt != 2 {
		t.Fatalf("upsert did not update: %+v", m)
	}
	// Same TMDB id in the other namespace is a different row.
	p.MediaType = "movie"
	id3, err := q.UpsertMedia(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if id3 == id1 {
		t.Fatal("tv and movie ids must be separate namespaces")
	}
}

func TestUpsertEpisodeAndWatchesIdempotent(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	q := store.New(d)
	mid, err := q.UpsertMedia(ctx, store.UpsertMediaParams{TmdbID: 1, MediaType: "tv", Title: "S", RefreshedAt: 0})
	if err != nil {
		t.Fatal(err)
	}
	ep := store.UpsertEpisodeParams{MediaID: mid, TmdbID: 500, SeasonNumber: 1, EpisodeNumber: 1, Name: "Pilot", AirDate: str("2024-01-01")}
	for _, name := range []string{"Pilot", "Pilot (renamed)"} {
		ep.Name = name
		if err := q.UpsertEpisode(ctx, ep); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := q.CountEpisodes(ctx, mid); n != 1 {
		t.Fatalf("episodes=%d, want 1", n)
	}
	var epID int64
	var name string
	if err := d.QueryRow(`SELECT id, name FROM episodes WHERE tmdb_id = 500`).Scan(&epID, &name); err != nil {
		t.Fatal(err)
	}
	if name != "Pilot (renamed)" {
		t.Fatalf("name=%q", name)
	}

	n1, err := q.MarkEpisodeWatched(ctx, store.MarkEpisodeWatchedParams{EpisodeID: epID, WatchedAt: 100})
	if err != nil {
		t.Fatal(err)
	}
	n2, err := q.MarkEpisodeWatched(ctx, store.MarkEpisodeWatchedParams{EpisodeID: epID, WatchedAt: 200})
	if err != nil {
		t.Fatal(err)
	}
	if n1 != 1 || n2 != 0 {
		t.Fatalf("rows affected: first=%d second=%d, want 1/0", n1, n2)
	}
	var at int64
	if err := d.QueryRow(`SELECT watched_at FROM episode_watches WHERE episode_id = ?`, epID).Scan(&at); err != nil {
		t.Fatal(err)
	}
	if at != 100 {
		t.Fatalf("second mark overwrote watched_at: %d", at)
	}
	// Re-upserting the episode must not disturb its watch.
	if err := q.UpsertEpisode(ctx, ep); err != nil {
		t.Fatal(err)
	}
	if n, _ := q.CountWatches(ctx, mid); n != 1 {
		t.Fatalf("watches=%d after re-upsert", n)
	}
}

func TestInsertTrackingDoesNotReset(t *testing.T) {
	ctx := context.Background()
	q := store.New(dbtest.New(t))
	mid, _ := q.UpsertMedia(ctx, store.UpsertMediaParams{TmdbID: 1, MediaType: "movie", Title: "M"})
	if err := q.InsertTracking(ctx, store.InsertTrackingParams{MediaID: mid, Status: "watching", StatusChangedAt: 1, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertTracking(ctx, store.InsertTrackingParams{MediaID: mid, Status: "backlog", StatusChangedAt: 2, AddedAt: 2}); err != nil {
		t.Fatal(err)
	}
	tr, _ := q.GetTracking(ctx, mid)
	if tr.Status != "watching" {
		t.Fatalf("status reset to %q", tr.Status)
	}
}

func TestProgressAggregates(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	q := store.New(d)
	mid, _ := q.UpsertMedia(ctx, store.UpsertMediaParams{TmdbID: 1, MediaType: "tv", Title: "S"})
	_ = q.InsertTracking(ctx, store.InsertTrackingParams{MediaID: mid, Status: "watching"})
	eps := []struct {
		tmdb     int64
		s, e     int64
		air      string
		watchedA int64
	}{
		{1, 0, 1, "2024-01-01", 0}, // special: excluded
		{2, 1, 1, "2024-01-01", 10},
		{3, 1, 2, "2024-01-08", 0}, // next
		{4, 1, 3, "2024-01-15", 30},
		{5, 1, 4, "2024-02-01", 0}, // upcoming
		{6, 1, 5, "", 0},           // TBA
	}
	for _, e := range eps {
		air := sql.NullString{}
		if e.air != "" {
			air = str(e.air)
		}
		_ = q.UpsertEpisode(ctx, store.UpsertEpisodeParams{MediaID: mid, TmdbID: e.tmdb, SeasonNumber: e.s, EpisodeNumber: e.e, AirDate: air})
		if e.watchedA > 0 {
			var id int64
			d.QueryRow(`SELECT id FROM episodes WHERE tmdb_id = ?`, e.tmdb).Scan(&id)
			_, _ = q.MarkEpisodeWatched(ctx, store.MarkEpisodeWatchedParams{EpisodeID: id, WatchedAt: e.watchedA})
		}
	}
	r, err := q.GetItem(ctx, store.GetItemParams{Today: "2024-01-20", MediaID: mid})
	if err != nil {
		t.Fatal(err)
	}
	var nextID int64
	d.QueryRow(`SELECT id FROM episodes WHERE tmdb_id = 3`).Scan(&nextID)
	if r.AiredTotal != 3 || r.WatchedTotal != 2 || r.LastWatchedAt != 30 || r.NextEpisodeID != nextID || r.NextAirDate != "2024-02-01" {
		t.Fatalf("aggregates: %+v", r)
	}
	rows, err := q.ListItemsByStatus(ctx, store.ListItemsByStatusParams{Today: "2024-01-20", Status: "watching"})
	if err != nil || len(rows) != 1 || store.GetItemRow(rows[0]) != r {
		t.Fatalf("list row mismatch: %v %+v", err, rows)
	}
}
