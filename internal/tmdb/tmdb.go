// Package tmdb is a minimal TMDB API v3 client.
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.themoviedb.org/3"

// ErrUnavailable wraps network failures and 5xx responses.
var ErrUnavailable = errors.New("tmdb unavailable")

// StatusError is a non-2xx response that isn't a 5xx.
type StatusError struct {
	Code int
	Msg  string
}

func (e *StatusError) Error() string { return fmt.Sprintf("tmdb: HTTP %d: %s", e.Code, e.Msg) }

type Client struct {
	BaseURL  string
	Token    string
	Language string
	HTTP     *http.Client
	// Sleep waits between a 429 and its retry; overridable in tests.
	Sleep func(ctx context.Context, d time.Duration) error
}

func New(token, language string) *Client {
	return &Client{
		BaseURL:  DefaultBaseURL,
		Token:    token,
		Language: language,
		HTTP:     &http.Client{Timeout: 10 * time.Second},
		Sleep:    sleepCtx,
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

const maxRetryAfter = 5 * time.Second

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	if q == nil {
		q = url.Values{}
	}
	if c.Language != "" {
		q.Set("language", c.Language)
	}
	u := strings.TrimRight(c.BaseURL, "/") + path + "?" + q.Encode()

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Accept", "application/json")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("%w: reading body: %v", ErrUnavailable, err)
		}

		switch {
		case resp.StatusCode == http.StatusTooManyRequests && attempt == 0:
			if err := c.Sleep(ctx, retryAfter(resp.Header.Get("Retry-After"))); err != nil {
				return err
			}
			continue
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			return fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
		case resp.StatusCode < 200 || resp.StatusCode > 299:
			var e struct {
				StatusMessage string `json:"status_message"`
			}
			_ = json.Unmarshal(body, &e)
			return &StatusError{Code: resp.StatusCode, Msg: e.StatusMessage}
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("tmdb: decoding %s: %w", path, err)
		}
		return nil
	}
}

func retryAfter(h string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || secs < 0 {
		return time.Second
	}
	return min(time.Duration(secs)*time.Second, maxRetryAfter)
}

type SearchResult struct {
	TMDBID     int64
	Type       string // "tv" or "movie"
	Title      string
	Year       string
	PosterPath string
}

// Search queries /search/multi and drops person results.
func (c *Client) Search(ctx context.Context, query string) ([]SearchResult, error) {
	var resp struct {
		Results []struct {
			ID           int64  `json:"id"`
			MediaType    string `json:"media_type"`
			Title        string `json:"title"`
			Name         string `json:"name"`
			ReleaseDate  string `json:"release_date"`
			FirstAirDate string `json:"first_air_date"`
			PosterPath   string `json:"poster_path"`
		} `json:"results"`
	}
	q := url.Values{"query": {query}, "include_adult": {"false"}, "page": {"1"}}
	if err := c.get(ctx, "/search/multi", q, &resp); err != nil {
		return nil, err
	}
	out := make([]SearchResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		switch r.MediaType {
		case "movie":
			out = append(out, SearchResult{r.ID, "movie", r.Title, year(r.ReleaseDate), r.PosterPath})
		case "tv":
			out = append(out, SearchResult{r.ID, "tv", r.Name, year(r.FirstAirDate), r.PosterPath})
		}
	}
	return out, nil
}

func year(date string) string {
	if len(date) >= 4 {
		return date[:4]
	}
	return ""
}

type Movie struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Overview    string `json:"overview"`
	PosterPath  string `json:"poster_path"`
	ReleaseDate string `json:"release_date"`
	Status      string `json:"status"`
	Runtime     int    `json:"runtime"`
}

func (c *Client) Movie(ctx context.Context, id int64) (*Movie, error) {
	var m Movie
	if err := c.get(ctx, fmt.Sprintf("/movie/%d", id), nil, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

type Episode struct {
	ID            int64  `json:"id"`
	SeasonNumber  int    `json:"season_number"`
	EpisodeNumber int    `json:"episode_number"`
	Name          string `json:"name"`
	AirDate       string `json:"air_date"`
	Runtime       *int   `json:"runtime"`
}

type Season struct {
	SeasonNumber int       `json:"season_number"`
	Episodes     []Episode `json:"episodes"`
}

type Show struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	Overview       string   `json:"overview"`
	PosterPath     string   `json:"poster_path"`
	FirstAirDate   string   `json:"first_air_date"`
	Status         string   `json:"status"`
	EpisodeRunTime []int    `json:"episode_run_time"`
	Seasons        []Season `json:"-"`
}

// Runtime is the show's typical episode runtime, or 0 if unknown.
func (s *Show) Runtime() int {
	if len(s.EpisodeRunTime) > 0 {
		return s.EpisodeRunTime[0]
	}
	return 0
}

// appendLimit is TMDB's maximum number of append_to_response items.
const appendLimit = 20

// Show fetches /tv/{id} and then every listed season (including season 0)
// via append_to_response, 20 seasons per request.
func (c *Client) Show(ctx context.Context, id int64) (*Show, error) {
	var base struct {
		Show
		Seasons []struct {
			SeasonNumber int `json:"season_number"`
		} `json:"seasons"`
	}
	path := fmt.Sprintf("/tv/%d", id)
	if err := c.get(ctx, path, nil, &base); err != nil {
		return nil, err
	}
	show := base.Show
	nums := make([]int, 0, len(base.Seasons))
	for _, s := range base.Seasons {
		nums = append(nums, s.SeasonNumber)
	}
	for start := 0; start < len(nums); start += appendLimit {
		chunk := nums[start:min(start+appendLimit, len(nums))]
		keys := make([]string, len(chunk))
		for i, n := range chunk {
			keys[i] = fmt.Sprintf("season/%d", n)
		}
		var resp map[string]json.RawMessage
		if err := c.get(ctx, path, url.Values{"append_to_response": {strings.Join(keys, ",")}}, &resp); err != nil {
			return nil, err
		}
		for i, k := range keys {
			raw, ok := resp[k]
			if !ok {
				continue // listed but not returned; skip rather than fail the whole show
			}
			var s Season
			if err := json.Unmarshal(raw, &s); err != nil {
				return nil, fmt.Errorf("tmdb: decoding %s: %w", k, err)
			}
			s.SeasonNumber = chunk[i]
			show.Seasons = append(show.Seasons, s)
		}
	}
	return &show, nil
}

// PosterURL builds a TMDB image CDN URL. size is w92, w185, or w342.
func PosterURL(path, size string) string {
	if path == "" {
		return ""
	}
	return "https://image.tmdb.org/t/p/" + size + path
}
