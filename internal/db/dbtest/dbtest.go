// Package dbtest provides a migrated temp-file SQLite database for tests.
package dbtest

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/chriscasey/watchwhat/internal/db"
)

// New returns a migrated database in t.TempDir() (a file, so WAL and the
// DSN pragmas are exercised). It is closed on test cleanup.
func New(t testing.TB) *sql.DB {
	t.Helper()
	ctx := context.Background()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := db.Migrate(ctx, d); err != nil {
		t.Fatal(err)
	}
	return d
}
