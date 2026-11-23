-- +goose Up
CREATE TABLE media (
  id              INTEGER PRIMARY KEY,
  tmdb_id         INTEGER NOT NULL,
  media_type      TEXT    NOT NULL CHECK (media_type IN ('tv','movie')),
  title           TEXT    NOT NULL,
  overview        TEXT    NOT NULL DEFAULT '',
  poster_path     TEXT,
  release_date    TEXT,
  tmdb_status     TEXT,
  runtime_minutes INTEGER,
  refreshed_at    INTEGER NOT NULL,
  UNIQUE (media_type, tmdb_id)
);

CREATE TABLE tracking (
  media_id          INTEGER PRIMARY KEY REFERENCES media(id) ON DELETE CASCADE,
  status            TEXT    NOT NULL CHECK (status IN ('backlog','watching','completed','dropped')),
  status_changed_at INTEGER NOT NULL,
  added_at          INTEGER NOT NULL,
  watched_at        INTEGER
);
CREATE INDEX tracking_status ON tracking(status);

CREATE TABLE episodes (
  id              INTEGER PRIMARY KEY,
  media_id        INTEGER NOT NULL REFERENCES media(id) ON DELETE CASCADE,
  tmdb_id         INTEGER NOT NULL UNIQUE,
  season_number   INTEGER NOT NULL,
  episode_number  INTEGER NOT NULL,
  name            TEXT    NOT NULL DEFAULT '',
  air_date        TEXT,
  runtime_minutes INTEGER
);
CREATE INDEX episodes_order ON episodes(media_id, season_number, episode_number);

CREATE TABLE episode_watches (
  episode_id INTEGER PRIMARY KEY REFERENCES episodes(id) ON DELETE CASCADE,
  watched_at INTEGER NOT NULL
);

CREATE TABLE weekly_schedule (
  weekday  INTEGER NOT NULL CHECK (weekday BETWEEN 0 AND 6),
  media_id INTEGER NOT NULL REFERENCES media(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  PRIMARY KEY (weekday, media_id)
);
CREATE INDEX weekly_schedule_media ON weekly_schedule(media_id);

-- +goose Down
DROP TABLE weekly_schedule;
DROP TABLE episode_watches;
DROP TABLE episodes;
DROP TABLE tracking;
DROP TABLE media;
