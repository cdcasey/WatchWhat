package components

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/chriscasey/watchwhat/internal/tracking"
)

func itoa[T ~int | ~int64](n T) string { return strconv.FormatInt(int64(n), 10) }

func EpLabel(e *tracking.EpisodeRef) string {
	return fmt.Sprintf("S%02dE%02d", e.Season, e.Number)
}

func StatusLabel(s string) string {
	switch s {
	case tracking.StatusBacklog:
		return "Backlog"
	case tracking.StatusWatching:
		return "Watching"
	case tracking.StatusCompleted:
		return "Completed"
	case tracking.StatusDropped:
		return "Dropped"
	}
	return s
}

func TypeLabel(t string) string {
	if t == "movie" {
		return "Movie"
	}
	return "TV"
}

func ShortDate(t time.Time) string { return t.Format("Mon, Jan 2") }

// AirDate formats a YYYY-MM-DD air date; empty means TBA.
func AirDate(s string) string {
	if s == "" {
		return "TBA"
	}
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return s
	}
	return t.Format("Jan 2, 2006")
}

func vals(kv ...string) string {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func mediaURL(id int64) string { return "/media/" + itoa(id) }

func pct(watched, aired int) string {
	if aired == 0 {
		return "0%"
	}
	return itoa(min(100, watched*100/aired)) + "%"
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
