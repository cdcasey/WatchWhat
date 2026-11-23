package tmdb_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chriscasey/watchwhat/internal/tmdb"
	"github.com/chriscasey/watchwhat/internal/tmdb/tmdbtest"
)

func TestSearchDropsPeople(t *testing.T) {
	f := tmdbtest.New(t)
	b, err := os.ReadFile("testdata/search_multi.json")
	if err != nil {
		t.Fatal(err)
	}
	f.SearchJSON = string(b)
	res, err := f.Client().Search(context.Background(), "shogun")
	if err != nil {
		t.Fatal(err)
	}
	want := []tmdb.SearchResult{
		{TMDBID: 126308, Type: "tv", Title: "Shōgun", Year: "2024", PosterPath: "/shogun.jpg"},
		{TMDBID: 39345, Type: "movie", Title: "Shogun Assassin", Year: "1980"},
		{TMDBID: 555, Type: "movie", Title: "No Date"},
	}
	if len(res) != len(want) {
		t.Fatalf("got %+v", res)
	}
	for i := range want {
		if res[i] != want[i] {
			t.Errorf("[%d] got %+v want %+v", i, res[i], want[i])
		}
	}
	if !strings.Contains(f.Requests[0], "query=shogun") || !strings.Contains(f.Requests[0], "language=en-US") {
		t.Errorf("request: %s", f.Requests[0])
	}
}

func TestMovie(t *testing.T) {
	f := tmdbtest.New(t)
	f.Movies[7] = &tmdbtest.Movie{ID: 7, Title: "Heat", Status: "Released", ReleaseDate: "1995-12-15", Runtime: 170}
	m, err := f.Client().Movie(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "Heat" || m.Runtime != 170 || m.Status != "Released" {
		t.Fatalf("%+v", m)
	}
}

func TestShowChunksSeasons(t *testing.T) {
	f := tmdbtest.New(t)
	show := &tmdbtest.Show{ID: 1, Name: "Long Runner", Status: "Returning Series"}
	var id int64 = 1000
	for s := 0; s <= 25; s++ { // season 0 + 25 regular seasons = 26 seasons
		for e := 1; e <= 2; e++ {
			id++
			show.Episodes = append(show.Episodes, tmdbtest.Episode{ID: id, Season: s, Number: e, AirDate: "2000-01-01"})
		}
	}
	f.Shows[1] = show
	got, err := f.Client().Show(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Seasons) != 26 {
		t.Fatalf("seasons=%d", len(got.Seasons))
	}
	if got.Seasons[0].SeasonNumber != 0 || len(got.Seasons[0].Episodes) != 2 || got.Seasons[25].SeasonNumber != 25 {
		t.Fatalf("season mapping wrong: first=%d last=%d", got.Seasons[0].SeasonNumber, got.Seasons[25].SeasonNumber)
	}
	if f.Calls() != 3 { // base + 20 + 6
		t.Fatalf("requests=%d: %v", f.Calls(), f.Requests)
	}
	if got.Runtime() != 45 {
		t.Fatalf("runtime=%d", got.Runtime())
	}
}

func TestRetryOn429(t *testing.T) {
	f := tmdbtest.New(t)
	f.Movies[7] = &tmdbtest.Movie{ID: 7, Title: "Heat"}
	f.RateLimitNext = 1
	var slept time.Duration
	c := f.Client()
	c.Sleep = func(_ context.Context, d time.Duration) error { slept = d; return nil }
	if _, err := c.Movie(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if slept != time.Second || f.Calls() != 2 {
		t.Fatalf("slept=%v calls=%d", slept, f.Calls())
	}

	// Two 429s in a row: retried once, then unavailable.
	f.RateLimitNext = 2
	if _, err := c.Movie(context.Background(), 7); !errors.Is(err, tmdb.ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

func TestRetryAfterCapped(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"id":1,"title":"x"}`))
	}))
	defer srv.Close()
	c := tmdb.New("t", "")
	c.BaseURL = srv.URL
	var slept time.Duration
	c.Sleep = func(_ context.Context, d time.Duration) error { slept = d; return nil }
	if _, err := c.Movie(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if slept != 5*time.Second {
		t.Fatalf("slept=%v, want 5s cap", slept)
	}
}

func TestErrors(t *testing.T) {
	f := tmdbtest.New(t)
	c := f.Client()
	var se *tmdb.StatusError
	if _, err := c.Movie(context.Background(), 404); !errors.As(err, &se) || se.Code != 404 {
		t.Fatalf("want StatusError 404, got %v", err)
	}
	f.Down = true
	if _, err := c.Search(context.Background(), "x"); !errors.Is(err, tmdb.ErrUnavailable) {
		t.Fatalf("5xx: want ErrUnavailable, got %v", err)
	}
	f.Close()
	if _, err := c.Search(context.Background(), "x"); !errors.Is(err, tmdb.ErrUnavailable) {
		t.Fatalf("network: want ErrUnavailable, got %v", err)
	}
}

func TestPosterURL(t *testing.T) {
	if got := tmdb.PosterURL("/a.jpg", "w185"); got != "https://image.tmdb.org/t/p/w185/a.jpg" {
		t.Fatal(got)
	}
	if tmdb.PosterURL("", "w185") != "" {
		t.Fatal("empty path should give empty URL")
	}
}
