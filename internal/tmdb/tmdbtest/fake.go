// Package tmdbtest provides an in-process fake of the TMDB endpoints WatchWhat uses.
package tmdbtest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chriscasey/watchwhat/internal/tmdb"
)

type Episode struct {
	ID      int64
	Season  int
	Number  int
	Name    string
	AirDate string
	Runtime int
}

type Show struct {
	ID         int64
	Name       string
	Status     string
	PosterPath string
	FirstAir   string
	Episodes   []Episode
	// ExtraSeasons lists season numbers that have no episodes.
	ExtraSeasons []int
}

type Movie struct {
	ID          int64
	Title       string
	Status      string
	ReleaseDate string
	PosterPath  string
	Runtime     int
}

// Fake serves /search/multi, /movie/{id}, /tv/{id} (with append_to_response=season/N).
type Fake struct {
	*httptest.Server

	mu     sync.Mutex
	Shows  map[int64]*Show
	Movies map[int64]*Movie
	// SearchJSON, if set, is returned verbatim from /search/multi.
	SearchJSON string
	// Down makes every request return 503.
	Down bool
	// RateLimitNext makes the next N requests return 429.
	RateLimitNext int
	Requests      []string
}

func New(t testing.TB) *Fake {
	f := &Fake{Shows: map[int64]*Show{}, Movies: map[int64]*Movie{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

// Client returns a tmdb.Client pointed at the fake, with no retry delay.
func (f *Fake) Client() *tmdb.Client {
	c := tmdb.New("test-token", "en-US")
	c.BaseURL = f.URL
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

func (f *Fake) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Requests)
}

func (f *Fake) Set(fn func(f *Fake)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Requests = append(f.Requests, r.URL.RequestURI())
	if r.Header.Get("Authorization") != "Bearer test-token" {
		http.Error(w, `{"status_message":"Invalid API key"}`, http.StatusUnauthorized)
		return
	}
	if f.Down {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	if f.RateLimitNext > 0 {
		f.RateLimitNext--
		w.Header().Set("Retry-After", "1")
		http.Error(w, `{"status_message":"rate limited"}`, http.StatusTooManyRequests)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case len(parts) == 2 && parts[0] == "search" && parts[1] == "multi":
		w.Write([]byte(f.SearchJSON))
	case len(parts) == 2 && parts[0] == "movie":
		id, _ := strconv.ParseInt(parts[1], 10, 64)
		m, ok := f.Movies[id]
		if !ok {
			notFound(w)
			return
		}
		writeJSON(w, map[string]any{"id": m.ID, "title": m.Title, "status": m.Status, "release_date": m.ReleaseDate,
			"poster_path": nullable(m.PosterPath), "runtime": m.Runtime, "overview": "A movie."})
	case len(parts) == 2 && parts[0] == "tv":
		id, _ := strconv.ParseInt(parts[1], 10, 64)
		s, ok := f.Shows[id]
		if !ok {
			notFound(w)
			return
		}
		writeJSON(w, s.response(r.URL.Query().Get("append_to_response")))
	default:
		notFound(w)
	}
}

func (s *Show) seasons() []int {
	seen := map[int]bool{}
	var out []int
	for _, e := range s.Episodes {
		if !seen[e.Season] {
			seen[e.Season] = true
			out = append(out, e.Season)
		}
	}
	for _, n := range s.ExtraSeasons {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

func (s *Show) response(appendTo string) map[string]any {
	var seasons []map[string]any
	for _, n := range s.seasons() {
		seasons = append(seasons, map[string]any{"season_number": n})
	}
	out := map[string]any{"id": s.ID, "name": s.Name, "status": s.Status, "first_air_date": s.FirstAir,
		"poster_path": nullable(s.PosterPath), "overview": "A show.", "episode_run_time": []int{45}, "seasons": seasons}
	if appendTo == "" {
		return out
	}
	keys := strings.Split(appendTo, ",")
	if len(keys) > 20 {
		panic("append_to_response over TMDB limit: " + strconv.Itoa(len(keys)))
	}
	for _, k := range keys {
		var n int
		if _, err := fmt.Sscanf(k, "season/%d", &n); err != nil {
			continue
		}
		eps := []map[string]any{}
		found := false
		for _, sn := range s.seasons() {
			found = found || sn == n
		}
		if !found {
			continue
		}
		for _, e := range s.Episodes {
			if e.Season == n {
				var rt any
				if e.Runtime > 0 {
					rt = e.Runtime
				}
				eps = append(eps, map[string]any{"id": e.ID, "season_number": e.Season, "episode_number": e.Number,
					"name": e.Name, "air_date": nullable(e.AirDate), "runtime": rt})
			}
		}
		out[k] = map[string]any{"season_number": n, "episodes": eps}
	}
	return out
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func notFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(`{"status_message":"The resource you requested could not be found."}`))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
