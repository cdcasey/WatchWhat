package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) *sql.DB {
	t.Helper()
	d, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestOpenPragmas(t *testing.T) {
	d := openTemp(t)
	var fk int
	var mode string
	if err := d.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if fk != 1 || mode != "wal" {
		t.Fatalf("foreign_keys=%d journal_mode=%s", fk, mode)
	}
	if got := d.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections=%d", got)
	}
}

func TestMigrateUpDownUp(t *testing.T) {
	ctx := context.Background()
	d := openTemp(t)
	for i, step := range []func(context.Context, *sql.DB) error{Migrate, Reset, Migrate} {
		if err := step(ctx, d); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
}

func TestCascadeDelete(t *testing.T) {
	ctx := context.Background()
	d := openTemp(t)
	if err := Migrate(ctx, d); err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO media (id, tmdb_id, media_type, title, refreshed_at) VALUES (1, 100, 'tv', 'Show', 0)`)
	exec(`INSERT INTO tracking (media_id, status, status_changed_at, added_at) VALUES (1, 'watching', 0, 0)`)
	exec(`INSERT INTO episodes (id, media_id, tmdb_id, season_number, episode_number) VALUES (10, 1, 1000, 1, 1)`)
	exec(`INSERT INTO episode_watches (episode_id, watched_at) VALUES (10, 0)`)
	exec(`INSERT INTO weekly_schedule (weekday, media_id, position) VALUES (2, 1, 1)`)
	exec(`DELETE FROM media WHERE id = 1`)

	for _, tbl := range []string{"tracking", "episodes", "episode_watches", "weekly_schedule"} {
		var n int
		if err := d.QueryRow("SELECT COUNT(*) FROM " + tbl).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s: %d rows remain after cascade", tbl, n)
		}
	}
}
