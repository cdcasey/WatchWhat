package server

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/chriscasey/watchwhat/internal/tracking"
	"github.com/chriscasey/watchwhat/internal/views/pages"
)

const continueWatchingLimit = 8

func (a *App) home(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	now := a.Now()
	ids, err := a.Sched.Lineup(ctx, now.Weekday())
	if err != nil {
		return err
	}
	lineup := make([]tracking.Item, 0, len(ids))
	for _, id := range ids {
		it, err := a.Track.Item(ctx, id)
		if err != nil {
			return err
		}
		lineup = append(lineup, it)
	}
	watching, err := a.Track.List(ctx, tracking.StatusWatching)
	if err != nil {
		return err
	}
	cont := tracking.Apply(watching, tracking.ListOpts{})
	cont = cont[:min(continueWatchingLimit, len(cont))]
	return a.render(w, r, "Home", "home", pages.Home(now.Weekday(), lineup, cont))
}

// parseListOpts reads type/stale/sort. Stale filter and sort exist only on Watching.
func parseListOpts(status string, r *http.Request) tracking.ListOpts {
	q := r.URL.Query()
	var o tracking.ListOpts
	if t := q.Get("type"); t == "tv" || t == "movie" {
		o.Type = t
	}
	if status == tracking.StatusWatching {
		o.StaleOnly = q.Get("stale") == "1"
		if q.Get("sort") == "stale" {
			o.Sort = "stale"
		}
	}
	return o
}

func (a *App) list(status string) appHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		items, err := a.Track.List(r.Context(), status)
		if err != nil {
			return err
		}
		o := parseListOpts(status, r)
		title := pages.ListTitle(status)
		return a.render(w, r, title, status, pages.List(status, tracking.Apply(items, o), o))
	}
}

func (a *App) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := a.DB.PingContext(ctx); err != nil {
		a.Log.Warn("healthz: database unreachable", "err", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("unavailable\n"))
		return
	}
	w.Write([]byte("ok\n"))
}

func pathInt(r *http.Request, name string) (int64, error) {
	n, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || n <= 0 {
		return 0, errNotFound
	}
	return n, nil
}
