package config

import (
	"errors"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestDefaults(t *testing.T) {
	c, err := Load(env(map[string]string{"WATCHWHAT_TMDB_TOKEN": "tok"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8080" || c.DBPath != "/data/watchwhat.db" || c.TMDBLanguage != "en-US" || c.Location.String() != "UTC" {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestOverrides(t *testing.T) {
	c, err := Load(env(map[string]string{
		"WATCHWHAT_TMDB_TOKEN": "tok", "WATCHWHAT_ADDR": ":9000", "WATCHWHAT_DB_PATH": "/tmp/x.db",
		"WATCHWHAT_TMDB_LANGUAGE": "de-DE", "TZ": "America/Chicago",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":9000" || c.DBPath != "/tmp/x.db" || c.TMDBLanguage != "de-DE" || c.Location.String() != "America/Chicago" {
		t.Fatalf("overrides not applied: %+v", c)
	}
}

func TestMissingToken(t *testing.T) {
	if _, err := Load(env(nil)); !errors.Is(err, ErrMissingToken) {
		t.Fatalf("want ErrMissingToken, got %v", err)
	}
}

func TestBadTZ(t *testing.T) {
	if _, err := Load(env(map[string]string{"WATCHWHAT_TMDB_TOKEN": "t", "TZ": "Nowhere/Land"})); err == nil {
		t.Fatal("want error for invalid TZ")
	}
}
