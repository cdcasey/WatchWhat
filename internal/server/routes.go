package server

import "net/http"

// Handler returns the application's root handler.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	h := func(pattern string, fn appHandler) { mux.Handle(pattern, a.h(fn)) }

	h("GET /{$}", a.home)
	for _, s := range []string{"watching", "backlog", "completed", "dropped"} {
		h("GET /"+s, a.list(s))
	}

	h("GET /search", a.searchPage)
	h("GET /search/results", a.searchResults)
	h("POST /media", a.addMedia)

	h("GET /media/{id}", a.detail)
	h("DELETE /media/{id}", a.removeMedia)
	h("POST /media/{id}/status", a.setStatus)
	h("POST /media/{id}/refresh", a.refresh)
	h("POST /media/{id}/watched", a.movieWatched)
	h("PUT /episodes/{id}/watched", a.episodeWatched(true))
	h("DELETE /episodes/{id}/watched", a.episodeWatched(false))
	h("PUT /media/{id}/seasons/{n}/watched", a.seasonWatched)

	h("GET /schedule", a.schedulePage)
	h("POST /schedule/{day}", a.scheduleAdd)
	h("DELETE /schedule/{day}/{media_id}", a.scheduleRemove)
	h("POST /schedule/{day}/{media_id}/move", a.scheduleMove)

	mux.Handle("GET /static/", a.Assets.Handler())
	mux.Handle("GET /favicon.ico", a.Assets.Alias("favicon.ico"))
	mux.HandleFunc("GET /healthz", a.healthz)

	root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern == "" {
			// Unmatched: the mux answers 404 or 405; render 404s as HTML.
			mux.ServeHTTP(&notFoundWriter{ResponseWriter: w, app: a, r: r}, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
	return a.logging(a.recoverer(vary(root)))
}
