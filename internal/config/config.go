// Package config parses WatchWhat's environment configuration.
package config

import (
	"errors"
	"fmt"
	"time"
	_ "time/tzdata" // distroless has no zoneinfo; TZ must resolve from the binary.
)

type Config struct {
	Addr         string
	DBPath       string
	TMDBToken    string
	TMDBLanguage string
	// Location defines "today" for schedules, air dates, and stale calculation.
	Location *time.Location
}

var ErrMissingToken = errors.New("WATCHWHAT_TMDB_TOKEN is required")

// Load reads configuration via getenv (os.Getenv in production).
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		Addr:         or(getenv("WATCHWHAT_ADDR"), ":8080"),
		DBPath:       or(getenv("WATCHWHAT_DB_PATH"), "/data/watchwhat.db"),
		TMDBToken:    getenv("WATCHWHAT_TMDB_TOKEN"),
		TMDBLanguage: or(getenv("WATCHWHAT_TMDB_LANGUAGE"), "en-US"),
	}
	if c.TMDBToken == "" {
		return Config{}, ErrMissingToken
	}
	loc, err := time.LoadLocation(or(getenv("TZ"), "UTC"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid TZ: %w", err)
	}
	c.Location = loc
	return c, nil
}

func or(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
