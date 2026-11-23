// Package catalog adds, refreshes, and removes media using TMDB metadata.
package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/chriscasey/watchwhat/internal/store"
	"github.com/chriscasey/watchwhat/internal/tmdb"
)

// TMDB is the subset of the TMDB client the catalog uses.
type TMDB interface {
	Movie(ctx context.Context, id int64) (*tmdb.Movie, error)
	Show(ctx context.Context, id int64) (*tmdb.Show, error)
}

type Service struct {
	DB   *sql.DB
	Q    *store.Queries
	TMDB TMDB
	Now  func() time.Time
}

var ErrInvalid = errors.New("invalid input")

// Add stores a TMDB item with an initial status (backlog or watching). If the
// item is already in the catalog it is returned unchanged and TMDB is not called.
func (s *Service) Add(ctx context.Context, mediaType string, tmdbID int64, status string) (int64, error) {
	if mediaType != "tv" && mediaType != "movie" {
		return 0, fmt.Errorf("%w: media type %q", ErrInvalid, mediaType)
	}
	if status != "backlog" && status != "watching" {
		return 0, fmt.Errorf("%w: initial status %q", ErrInvalid, status)
	}
	existing, err := s.Q.GetMediaByTMDB(ctx, store.GetMediaByTMDBParams{MediaType: mediaType, TmdbID: tmdbID})
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	f, err := s.fetch(ctx, mediaType, tmdbID)
	if err != nil {
		return 0, err
	}
	now := s.Now().Unix()
	var id int64
	err = s.tx(ctx, func(q *store.Queries) error {
		if id, err = s.write(ctx, q, f, now); err != nil {
			return err
		}
		return q.InsertTracking(ctx, store.InsertTrackingParams{MediaID: id, Status: status, StatusChangedAt: now, AddedAt: now})
	})
	return id, err
}

// Refresh re-fetches TMDB metadata for a catalog item. Episodes are upserted
// and never deleted, so watch history survives upstream removals.
func (s *Service) Refresh(ctx context.Context, mediaID int64) error {
	m, err := s.Q.GetMedia(ctx, mediaID)
	if err != nil {
		return err
	}
	f, err := s.fetch(ctx, m.MediaType, m.TmdbID)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(q *store.Queries) error {
		_, err := s.write(ctx, q, f, s.Now().Unix())
		return err
	})
}

// Remove deletes an item; episodes, watches, tracking, and schedule rows cascade.
func (s *Service) Remove(ctx context.Context, mediaID int64) error {
	n, err := s.Q.DeleteMedia(ctx, mediaID)
	if err == nil && n == 0 {
		return sql.ErrNoRows
	}
	return err
}

type fetched struct {
	mediaType string
	movie     *tmdb.Movie
	show      *tmdb.Show
}

// fetch runs outside any transaction: no DB lock is held across network I/O.
func (s *Service) fetch(ctx context.Context, mediaType string, tmdbID int64) (fetched, error) {
	f := fetched{mediaType: mediaType}
	var err error
	if mediaType == "movie" {
		f.movie, err = s.TMDB.Movie(ctx, tmdbID)
	} else {
		f.show, err = s.TMDB.Show(ctx, tmdbID)
	}
	return f, err
}

func (s *Service) write(ctx context.Context, q *store.Queries, f fetched, now int64) (int64, error) {
	if f.movie != nil {
		m := f.movie
		return q.UpsertMedia(ctx, store.UpsertMediaParams{
			TmdbID: m.ID, MediaType: "movie", Title: m.Title, Overview: m.Overview,
			PosterPath: nstr(m.PosterPath), ReleaseDate: nstr(m.ReleaseDate), TmdbStatus: nstr(m.Status),
			RuntimeMinutes: nint(m.Runtime), RefreshedAt: now,
		})
	}
	sh := f.show
	id, err := q.UpsertMedia(ctx, store.UpsertMediaParams{
		TmdbID: sh.ID, MediaType: "tv", Title: sh.Name, Overview: sh.Overview,
		PosterPath: nstr(sh.PosterPath), ReleaseDate: nstr(sh.FirstAirDate), TmdbStatus: nstr(sh.Status),
		RuntimeMinutes: nint(sh.Runtime()), RefreshedAt: now,
	})
	if err != nil {
		return 0, err
	}
	for _, season := range sh.Seasons {
		for _, e := range season.Episodes {
			rt := 0
			if e.Runtime != nil {
				rt = *e.Runtime
			}
			if err := q.UpsertEpisode(ctx, store.UpsertEpisodeParams{
				MediaID: id, TmdbID: e.ID, SeasonNumber: int64(season.SeasonNumber), EpisodeNumber: int64(e.EpisodeNumber),
				Name: e.Name, AirDate: nstr(e.AirDate), RuntimeMinutes: nint(rt),
			}); err != nil {
				return 0, fmt.Errorf("episode %d: %w", e.ID, err)
			}
		}
	}
	return id, nil
}

func (s *Service) tx(ctx context.Context, fn func(q *store.Queries) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(s.Q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func nstr(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
func nint(n int) sql.NullInt64     { return sql.NullInt64{Int64: int64(n), Valid: n > 0} }
