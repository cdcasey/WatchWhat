// Package server wires HTTP routes, rendering, and error handling.
package server

import (
	"database/sql"
	"log/slog"
	"time"

	"github.com/chriscasey/watchwhat/internal/catalog"
	"github.com/chriscasey/watchwhat/internal/config"
	"github.com/chriscasey/watchwhat/internal/schedule"
	"github.com/chriscasey/watchwhat/internal/store"
	"github.com/chriscasey/watchwhat/internal/tmdb"
	"github.com/chriscasey/watchwhat/internal/tracking"
	"github.com/chriscasey/watchwhat/web"
)

type App struct {
	DB      *sql.DB
	Q       *store.Queries
	TMDB    *tmdb.Client
	Catalog *catalog.Service
	Track   *tracking.Service
	Sched   *schedule.Service
	Cfg     config.Config
	Log     *slog.Logger
	Now     func() time.Time // in Cfg.Location; overridden in tests
	Assets  web.Manifest
}

// New wires services. now must return times in cfg.Location.
func New(cfg config.Config, db *sql.DB, tc *tmdb.Client, log *slog.Logger, now func() time.Time, assets web.Manifest) *App {
	q := store.New(db)
	return &App{
		DB: db, Q: q, TMDB: tc, Cfg: cfg, Log: log, Now: now, Assets: assets,
		Catalog: &catalog.Service{DB: db, Q: q, TMDB: tc, Now: now},
		Track:   &tracking.Service{DB: db, Q: q, Now: now},
		Sched:   &schedule.Service{DB: db, Q: q},
	}
}
