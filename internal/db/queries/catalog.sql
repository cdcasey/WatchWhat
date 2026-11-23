-- name: UpsertMedia :one
INSERT INTO media (tmdb_id, media_type, title, overview, poster_path, release_date, tmdb_status, runtime_minutes, refreshed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (media_type, tmdb_id) DO UPDATE SET
  title = excluded.title,
  overview = excluded.overview,
  poster_path = excluded.poster_path,
  release_date = excluded.release_date,
  tmdb_status = excluded.tmdb_status,
  runtime_minutes = excluded.runtime_minutes,
  refreshed_at = excluded.refreshed_at
RETURNING id;

-- name: UpsertEpisode :exec
INSERT INTO episodes (media_id, tmdb_id, season_number, episode_number, name, air_date, runtime_minutes)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (tmdb_id) DO UPDATE SET
  media_id = excluded.media_id,
  season_number = excluded.season_number,
  episode_number = excluded.episode_number,
  name = excluded.name,
  air_date = excluded.air_date,
  runtime_minutes = excluded.runtime_minutes;

-- name: GetMedia :one
SELECT * FROM media WHERE id = ?;

-- name: GetMediaByTMDB :one
SELECT * FROM media WHERE media_type = ? AND tmdb_id = ?;

-- name: DeleteMedia :execrows
DELETE FROM media WHERE id = ?;

-- name: ListCatalogKeys :many
SELECT m.id, m.media_type, m.tmdb_id, t.status
FROM media m JOIN tracking t ON t.media_id = m.id;

-- name: ListRefreshCandidates :many
SELECT m.id, m.title
FROM media m JOIN tracking t ON t.media_id = m.id
WHERE m.media_type = 'tv'
  AND m.tmdb_status IN ('Returning Series', 'In Production', 'Planned', 'Pilot')
  AND t.status IN ('backlog', 'watching')
  AND m.refreshed_at < ?
ORDER BY m.refreshed_at;

-- name: CountEpisodes :one
SELECT COUNT(*) FROM episodes WHERE media_id = ?;
