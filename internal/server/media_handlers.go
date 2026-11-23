package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/chriscasey/watchwhat/internal/store"
	"github.com/chriscasey/watchwhat/internal/tracking"
	"github.com/chriscasey/watchwhat/internal/views/components"
	"github.com/chriscasey/watchwhat/internal/views/pages"
)

func (a *App) searchPage(w http.ResponseWriter, r *http.Request) error {
	return a.render(w, r, "Search", "search", pages.Search())
}

// searchResults never writes to the catalog. An empty query makes no TMDB call.
func (a *App) searchResults(w http.ResponseWriter, r *http.Request) error {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		return nil
	}
	results, err := a.TMDB.Search(r.Context(), q)
	if err != nil {
		return err
	}
	keys, err := a.Q.ListCatalogKeys(r.Context())
	if err != nil {
		return err
	}
	type key struct {
		typ string
		id  int64
	}
	inCatalog := make(map[key]store.ListCatalogKeysRow, len(keys))
	for _, k := range keys {
		inCatalog[key{k.MediaType, k.TmdbID}] = k
	}
	rows := make([]components.SearchRowVM, len(results))
	for i, res := range results {
		rows[i] = components.SearchRowVM{TMDBID: res.TMDBID, Type: res.Type, Title: res.Title, Year: res.Year, PosterPath: res.PosterPath}
		if k, ok := inCatalog[key{res.Type, res.TMDBID}]; ok {
			rows[i].MediaID, rows[i].Status = k.ID, k.Status
		}
	}
	return a.fragment(w, r, components.SearchResults(rows, q))
}

func (a *App) addMedia(w http.ResponseWriter, r *http.Request) error {
	tmdbID, err := strconv.ParseInt(r.FormValue("tmdb_id"), 10, 64)
	if err != nil || tmdbID <= 0 {
		return badRequest("Invalid TMDB id.")
	}
	status := r.FormValue("status")
	if status == "" {
		status = tracking.StatusBacklog
	}
	id, err := a.Catalog.Add(r.Context(), r.FormValue("media_type"), tmdbID, status)
	if err != nil {
		return err
	}
	m, err := a.Q.GetMedia(r.Context(), id)
	if err != nil {
		return err
	}
	tr, err := a.Q.GetTracking(r.Context(), id)
	if err != nil {
		return err
	}
	row := components.SearchRowVM{TMDBID: m.TmdbID, Type: m.MediaType, Title: m.Title, PosterPath: m.PosterPath.String, MediaID: m.ID, Status: tr.Status}
	if len(m.ReleaseDate.String) >= 4 {
		row.Year = m.ReleaseDate.String[:4]
	}
	return a.fragment(w, r, components.SearchRow(row))
}

func (a *App) loadDetail(r *http.Request, id int64) (pages.Detail, error) {
	it, err := a.Track.Item(r.Context(), id)
	if err != nil {
		return pages.Detail{}, err
	}
	d := pages.Detail{Item: it}
	if it.IsTV() {
		if d.Seasons, err = a.Track.Seasons(r.Context(), id); err != nil {
			return pages.Detail{}, err
		}
	}
	return d, nil
}

func (a *App) detail(w http.ResponseWriter, r *http.Request) error {
	id, err := pathInt(r, "id")
	if err != nil {
		return err
	}
	d, err := a.loadDetail(r, id)
	if err != nil {
		return err
	}
	return a.render(w, r, d.Item.Title, d.Item.Status, pages.DetailContent(d))
}

func (a *App) detailContent(w http.ResponseWriter, r *http.Request, id int64) error {
	d, err := a.loadDetail(r, id)
	if err != nil {
		return err
	}
	return a.fragment(w, r, pages.DetailContent(d))
}

func (a *App) removeMedia(w http.ResponseWriter, r *http.Request) error {
	id, err := pathInt(r, "id")
	if err != nil {
		return err
	}
	tr, err := a.Q.GetTracking(r.Context(), id)
	if err != nil {
		return err
	}
	if err := a.Catalog.Remove(r.Context(), id); err != nil {
		return err
	}
	w.Header().Set("HX-Redirect", "/"+tr.Status)
	w.WriteHeader(http.StatusOK)
	return nil
}

func (a *App) refresh(w http.ResponseWriter, r *http.Request) error {
	id, err := pathInt(r, "id")
	if err != nil {
		return err
	}
	if err := a.Catalog.Refresh(r.Context(), id); err != nil {
		return err
	}
	return a.detailContent(w, r, id)
}

// belongs reports whether a card with status may remain in the list identified by ctx.
func belongs(ctx, status string) bool {
	switch ctx {
	case tracking.StatusWatching, tracking.StatusBacklog, tracking.StatusCompleted, tracking.StatusDropped:
		return status == ctx
	case "lineup":
		return status == tracking.StatusWatching || status == tracking.StatusBacklog
	case "continue":
		return status == tracking.StatusWatching
	}
	return true
}

// card answers a card action: the updated card, or an empty body when the item
// no longer belongs in the list the card was shown in.
func (a *App) card(w http.ResponseWriter, r *http.Request, id int64, undo int64) error {
	ctx := r.FormValue("ctx")
	if ctx == "detail" {
		return a.detailContent(w, r, id)
	}
	it, err := a.Track.Item(r.Context(), id)
	if err != nil {
		return err
	}
	if !belongs(ctx, it.Status) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		return nil
	}
	return a.fragment(w, r, components.MediaCard(it, ctx, undo))
}

func (a *App) setStatus(w http.ResponseWriter, r *http.Request) error {
	id, err := pathInt(r, "id")
	if err != nil {
		return err
	}
	if err := a.Track.SetStatus(r.Context(), id, r.FormValue("status")); err != nil {
		return err
	}
	return a.card(w, r, id, 0)
}

func (a *App) movieWatched(w http.ResponseWriter, r *http.Request) error {
	id, err := pathInt(r, "id")
	if err != nil {
		return err
	}
	if err := a.Track.MarkMovieWatched(r.Context(), id); err != nil {
		return err
	}
	return a.card(w, r, id, 0)
}

func (a *App) episodeWatched(mark bool) appHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		epID, err := pathInt(r, "id")
		if err != nil {
			return err
		}
		var mediaID int64
		if mark {
			mediaID, err = a.Track.MarkEpisode(r.Context(), epID)
		} else {
			mediaID, err = a.Track.UnmarkEpisode(r.Context(), epID)
		}
		if err != nil {
			return err
		}
		if r.FormValue("ctx") != "detail" {
			undo := int64(0)
			if mark {
				undo = epID
			}
			return a.card(w, r, mediaID, undo)
		}
		ep, err := a.Track.Episode(r.Context(), epID)
		if err != nil {
			return err
		}
		it, err := a.Track.Item(r.Context(), mediaID)
		if err != nil {
			return err
		}
		return a.fragment(w, r, templ.Join(
			components.EpisodeRow(ep),
			components.ProgressCounter(mediaID, it.Progress.Watched, it.Progress.Aired, true, true),
		))
	}
}

func (a *App) seasonWatched(w http.ResponseWriter, r *http.Request) error {
	id, err := pathInt(r, "id")
	if err != nil {
		return err
	}
	n, err := strconv.ParseInt(r.PathValue("n"), 10, 64)
	if err != nil || n < 0 {
		return errNotFound
	}
	if err := a.Track.MarkSeason(r.Context(), id, n); err != nil {
		return err
	}
	se, err := a.Track.Season(r.Context(), id, n)
	if err != nil {
		return err
	}
	it, err := a.Track.Item(r.Context(), id)
	if err != nil {
		return err
	}
	return a.fragment(w, r, templ.Join(
		components.SeasonBlock(se, true),
		components.ProgressCounter(id, it.Progress.Watched, it.Progress.Aired, true, true),
	))
}
