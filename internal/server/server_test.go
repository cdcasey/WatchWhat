package server_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chriscasey/watchwhat/internal/config"
	"github.com/chriscasey/watchwhat/internal/db/dbtest"
	"github.com/chriscasey/watchwhat/internal/server"
	"github.com/chriscasey/watchwhat/internal/tmdb/tmdbtest"
	"github.com/chriscasey/watchwhat/web"
)

var chicago, _ = time.LoadLocation("America/Chicago")

// tuesday is 2024-06-18 20:00 in America/Chicago.
var tuesday = time.Date(2024, 6, 18, 20, 0, 0, 0, chicago)

type harness struct {
	t    *testing.T
	app  *server.App
	h    http.Handler
	fake *tmdbtest.Fake
	now  time.Time
	logs *bytes.Buffer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	d := dbtest.New(t)
	fake := tmdbtest.New(t)
	assets, err := web.NewManifest(web.Static)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, fake: fake, now: tuesday, logs: &bytes.Buffer{}}
	log := slog.New(slog.NewTextHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Config{Location: chicago, TMDBToken: "test-token"}
	h.app = server.New(cfg, d, fake.Client(), log, func() time.Time { return h.now }, assets)
	h.h = h.app.Handler()
	return h
}

type req struct {
	method, path string
	form         url.Values
	htmx         bool
	headers      map[string]string
}

// do issues a request the way htmx 2 does: GET and DELETE carry parameters in
// the query string (htmx's default methodsThatUseUrlParams), others in a form body.
func (h *harness) do(r req) *httptest.ResponseRecorder {
	h.t.Helper()
	urlParams := r.method == http.MethodGet || r.method == http.MethodDelete
	path, body := r.path, ""
	if r.form != nil {
		if urlParams {
			path += "?" + r.form.Encode()
		} else {
			body = r.form.Encode()
		}
	}
	hr := httptest.NewRequest(r.method, path, strings.NewReader(body))
	if r.form != nil && !urlParams {
		hr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if r.htmx {
		hr.Header.Set("HX-Request", "true")
	}
	for k, v := range r.headers {
		hr.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, hr)
	return rec
}

func (h *harness) get(path string) *httptest.ResponseRecorder {
	return h.do(req{method: http.MethodGet, path: path})
}

func (h *harness) hx(method, path string, form url.Values) *httptest.ResponseRecorder {
	return h.do(req{method: method, path: path, form: form, htmx: true})
}

// addShow registers a show with the fake and adds it through the catalog.
func (h *harness) addShow(tmdbID int64, name, tmdbStatus, status string, eps ...tmdbtest.Episode) int64 {
	h.t.Helper()
	h.fake.Set(func(f *tmdbtest.Fake) {
		f.Shows[tmdbID] = &tmdbtest.Show{ID: tmdbID, Name: name, Status: tmdbStatus, Episodes: eps}
	})
	id, err := h.app.Catalog.Add(context.Background(), "tv", tmdbID, status)
	if err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *harness) addMovie(tmdbID int64, title, status string) int64 {
	h.t.Helper()
	h.fake.Set(func(f *tmdbtest.Fake) {
		f.Movies[tmdbID] = &tmdbtest.Movie{ID: tmdbID, Title: title, Status: "Released"}
	})
	id, err := h.app.Catalog.Add(context.Background(), "movie", tmdbID, status)
	if err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *harness) episodeID(tmdbEpisodeID int64) int64 {
	h.t.Helper()
	var id int64
	if err := h.app.DB.QueryRow(`SELECT id FROM episodes WHERE tmdb_id = ?`, tmdbEpisodeID).Scan(&id); err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *harness) count(q string, args ...any) int {
	h.t.Helper()
	var n int
	if err := h.app.DB.QueryRow(q, args...).Scan(&n); err != nil {
		h.t.Fatal(err)
	}
	return n
}

func eps(season int, n int, startID int64, airDate string) []tmdbtest.Episode {
	var out []tmdbtest.Episode
	for i := 1; i <= n; i++ {
		out = append(out, tmdbtest.Episode{ID: startID + int64(i), Season: season, Number: i, Name: "Episode " + string(rune('A'-1+i)), AirDate: airDate})
	}
	return out
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func mustContain(t *testing.T, body string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(body, s) {
			t.Errorf("body missing %q", s)
		}
	}
}

func mustNotContain(t *testing.T, body string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if strings.Contains(body, s) {
			t.Errorf("body unexpectedly contains %q", s)
		}
	}
}

func isDocument(body string) bool { return strings.Contains(body, "<html") }
