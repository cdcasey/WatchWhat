package tracking

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/chriscasey/watchwhat/internal/store"
)

var ErrInvalid = errors.New("invalid input")

type Service struct {
	DB  *sql.DB
	Q   *store.Queries
	Now func() time.Time // in the server's configured location
}

// Item is a catalog item with its household state and derived progress.
type Item struct {
	ID         int64
	TMDBID     int64
	Type       string
	Title      string
	Overview   string
	PosterPath string
	Year       string
	TMDBStatus string
	Status     string
	Runtime    int
	WatchedAt  *time.Time // movies
	Progress   Progress
}

func (i Item) IsTV() bool { return i.Type == "tv" }

// Today is the server-local date used for "aired" comparisons.
func Today(now time.Time) string { return now.Format(time.DateOnly) }

func (s *Service) Item(ctx context.Context, mediaID int64) (Item, error) {
	now := s.Now()
	r, err := s.Q.GetItem(ctx, store.GetItemParams{Today: Today(now), MediaID: mediaID})
	if err != nil {
		return Item{}, err
	}
	return s.build(ctx, r, now)
}

func (s *Service) List(ctx context.Context, status string) ([]Item, error) {
	now := s.Now()
	rows, err := s.Q.ListItemsByStatus(ctx, store.ListItemsByStatusParams{Today: Today(now), Status: status})
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(rows))
	for _, r := range rows {
		it, err := s.build(ctx, store.GetItemRow(r), now)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, nil
}

func (s *Service) build(ctx context.Context, r store.GetItemRow, now time.Time) (Item, error) {
	pr := ProgressRow{
		MediaType: r.MediaType, Status: r.Status, TMDBStatus: r.TmdbStatus.String,
		StatusChangedAt: r.StatusChangedAt, WatchedAt: r.WatchedAt.Int64,
		AiredTotal: r.AiredTotal, WatchedTotal: r.WatchedTotal, LastWatchedAt: r.LastWatchedAt,
		NextAirDate: r.NextAirDate,
	}
	if r.NextEpisodeID != 0 {
		e, err := s.Q.GetEpisode(ctx, r.NextEpisodeID)
		if err != nil {
			return Item{}, fmt.Errorf("next episode %d: %w", r.NextEpisodeID, err)
		}
		pr.Next = &EpisodeRef{ID: e.ID, Season: e.SeasonNumber, Number: e.EpisodeNumber, Name: e.Name}
	}
	it := Item{
		ID: r.ID, TMDBID: r.TmdbID, Type: r.MediaType, Title: r.Title, Overview: r.Overview,
		PosterPath: r.PosterPath.String, TMDBStatus: r.TmdbStatus.String, Status: r.Status,
		Runtime: int(r.RuntimeMinutes.Int64), Progress: Compute(pr, now),
	}
	if len(r.ReleaseDate.String) >= 4 {
		it.Year = r.ReleaseDate.String[:4]
	}
	if r.WatchedAt.Valid {
		t := time.Unix(r.WatchedAt.Int64, 0).In(now.Location())
		it.WatchedAt = &t
	}
	return it, nil
}

// ListOpts are the list-view query options.
type ListOpts struct {
	Type      string // "", "tv", "movie"
	StaleOnly bool
	Sort      string // "recent" (default) or "stale"
}

// Apply filters and sorts items in memory (lists are at most dozens of rows).
func Apply(items []Item, o ListOpts) []Item {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if o.Type != "" && it.Type != o.Type {
			continue
		}
		if o.StaleOnly && !it.Progress.Stale {
			continue
		}
		out = append(out, it)
	}
	byTitle := func(a, b Item) int { return cmp.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title)) }
	switch o.Sort {
	case "stale":
		slices.SortStableFunc(out, func(a, b Item) int {
			return cmp.Or(cmp.Compare(b.Progress.DaysIdle, a.Progress.DaysIdle), byTitle(a, b))
		})
	default:
		slices.SortStableFunc(out, func(a, b Item) int {
			return cmp.Or(b.Progress.LastActivity.Compare(a.Progress.LastActivity), byTitle(a, b))
		})
	}
	return out
}

// SetStatus changes an item's status. Setting the current status is a no-op
// and does not reset status_changed_at.
func (s *Service) SetStatus(ctx context.Context, mediaID int64, status string) error {
	if !ValidStatus(status) {
		return fmt.Errorf("%w: status %q", ErrInvalid, status)
	}
	if _, err := s.Q.GetTracking(ctx, mediaID); err != nil {
		return err
	}
	_, err := s.Q.SetStatus(ctx, store.SetStatusParams{Status: status, StatusChangedAt: s.Now().Unix(), MediaID: mediaID})
	return err
}

// MarkEpisode marks an episode watched (idempotent) and returns its media id.
func (s *Service) MarkEpisode(ctx context.Context, episodeID int64) (int64, error) {
	e, err := s.Q.GetEpisode(ctx, episodeID)
	if err != nil {
		return 0, err
	}
	_, err = s.Q.MarkEpisodeWatched(ctx, store.MarkEpisodeWatchedParams{EpisodeID: episodeID, WatchedAt: s.Now().Unix()})
	return e.MediaID, err
}

// UnmarkEpisode clears an episode's watch and returns its media id.
func (s *Service) UnmarkEpisode(ctx context.Context, episodeID int64) (int64, error) {
	e, err := s.Q.GetEpisode(ctx, episodeID)
	if err != nil {
		return 0, err
	}
	_, err = s.Q.UnmarkEpisodeWatched(ctx, episodeID)
	return e.MediaID, err
}

// MarkSeason marks every aired episode of a season watched; unaired episodes are untouched.
func (s *Service) MarkSeason(ctx context.Context, mediaID, season int64) error {
	if _, err := s.Q.GetTracking(ctx, mediaID); err != nil {
		return err
	}
	now := s.Now()
	_, err := s.Q.MarkSeasonWatched(ctx, store.MarkSeasonWatchedParams{
		WatchedAt: now.Unix(), MediaID: mediaID, SeasonNumber: season, Today: Today(now),
	})
	return err
}

// MarkMovieWatched records the watch time and sets the movie to completed.
func (s *Service) MarkMovieWatched(ctx context.Context, mediaID int64) error {
	m, err := s.Q.GetMedia(ctx, mediaID)
	if err != nil {
		return err
	}
	if m.MediaType != "movie" {
		return fmt.Errorf("%w: %d is not a movie", ErrInvalid, mediaID)
	}
	return s.Q.SetMovieWatched(ctx, store.SetMovieWatchedParams{StatusChangedAt: s.Now().Unix(), MediaID: mediaID})
}

type EpisodeVM struct {
	ID      int64
	MediaID int64
	Season  int64
	Number  int64
	Name    string
	AirDate string
	Aired   bool
	Watched bool
}

type Season struct {
	MediaID int64
	Number  int64
	Eps     []EpisodeVM
	Aired   int
	Watched int
}

func (s Season) Label() string {
	if s.Number == 0 {
		return "Specials"
	}
	return fmt.Sprintf("Season %d", s.Number)
}

// Seasons lists episodes grouped by season, regular seasons first, specials last.
func (s *Service) Seasons(ctx context.Context, mediaID int64) ([]Season, error) {
	rows, err := s.Q.ListEpisodes(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	today := Today(s.Now())
	var out []Season
	for _, r := range rows {
		if len(out) == 0 || out[len(out)-1].Number != r.SeasonNumber {
			out = append(out, Season{MediaID: mediaID, Number: r.SeasonNumber})
		}
		cur := &out[len(out)-1]
		ep := EpisodeVM{
			ID: r.ID, MediaID: mediaID, Season: r.SeasonNumber, Number: r.EpisodeNumber, Name: r.Name,
			AirDate: r.AirDate.String, Aired: r.AirDate.Valid && r.AirDate.String <= today, Watched: r.WatchedAt.Valid,
		}
		if ep.Aired {
			cur.Aired++
		}
		if ep.Watched {
			cur.Watched++
		}
		cur.Eps = append(cur.Eps, ep)
	}
	return out, nil
}

// Season returns one season's block.
func (s *Service) Season(ctx context.Context, mediaID, number int64) (Season, error) {
	all, err := s.Seasons(ctx, mediaID)
	if err != nil {
		return Season{}, err
	}
	for _, se := range all {
		if se.Number == number {
			return se, nil
		}
	}
	return Season{}, sql.ErrNoRows
}

// Episode returns one episode row.
func (s *Service) Episode(ctx context.Context, episodeID int64) (EpisodeVM, error) {
	e, err := s.Q.GetEpisode(ctx, episodeID)
	if err != nil {
		return EpisodeVM{}, err
	}
	today := Today(s.Now())
	n, err := s.Q.CountEpisodeWatch(ctx, episodeID)
	if err != nil {
		return EpisodeVM{}, err
	}
	return EpisodeVM{
		ID: e.ID, MediaID: e.MediaID, Season: e.SeasonNumber, Number: e.EpisodeNumber, Name: e.Name,
		AirDate: e.AirDate.String, Aired: e.AirDate.Valid && e.AirDate.String <= today, Watched: n > 0,
	}, nil
}
