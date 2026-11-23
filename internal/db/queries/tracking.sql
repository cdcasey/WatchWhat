-- name: InsertTracking :exec
INSERT INTO tracking (media_id, status, status_changed_at, added_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (media_id) DO NOTHING;

-- name: GetTracking :one
SELECT * FROM tracking WHERE media_id = ?;

-- name: SetStatus :execrows
UPDATE tracking SET status = ?1, status_changed_at = ?2
WHERE media_id = ?3 AND status <> ?1;

-- name: SetMovieWatched :exec
UPDATE tracking SET status = 'completed', status_changed_at = ?1, watched_at = ?1
WHERE media_id = ?2;

-- name: MarkEpisodeWatched :execrows
INSERT INTO episode_watches (episode_id, watched_at) VALUES (?, ?)
ON CONFLICT (episode_id) DO NOTHING;

-- name: UnmarkEpisodeWatched :execrows
DELETE FROM episode_watches WHERE episode_id = ?;

-- name: MarkSeasonWatched :execrows
INSERT INTO episode_watches (episode_id, watched_at)
SELECT e.id, @watched_at FROM episodes e
WHERE e.media_id = @media_id AND e.season_number = @season_number
  AND e.air_date IS NOT NULL AND e.air_date <= CAST(@today AS TEXT)
ON CONFLICT (episode_id) DO NOTHING;

-- name: GetEpisode :one
SELECT * FROM episodes WHERE id = ?;

-- name: ListEpisodes :many
SELECT e.id, e.season_number, e.episode_number, e.name, e.air_date, e.runtime_minutes, w.watched_at
FROM episodes e LEFT JOIN episode_watches w ON w.episode_id = e.id
WHERE e.media_id = ?
ORDER BY e.season_number = 0, e.season_number, e.episode_number;

-- name: CountWatches :one
SELECT COUNT(*) FROM episode_watches w JOIN episodes e ON e.id = w.episode_id WHERE e.media_id = ?;

-- name: CountEpisodeWatch :one
SELECT COUNT(*) FROM episode_watches WHERE episode_id = ?;
