-- Missing values come back as 0 / '' (episode ids start at 1).
-- Aggregates exclude specials (season 0). "Aired" means air_date <= CAST(@today AS TEXT),
-- where CAST(@today AS TEXT) is the server-TZ local date as YYYY-MM-DD.

-- name: GetItem :one
SELECT
  m.id, m.tmdb_id, m.media_type, m.title, m.overview, m.poster_path, m.release_date, m.tmdb_status, m.runtime_minutes, m.refreshed_at,
  t.status, t.status_changed_at, t.added_at, t.watched_at,
  CAST((SELECT COUNT(*) FROM episodes e
        WHERE e.media_id = m.id AND e.season_number > 0 AND e.air_date IS NOT NULL AND e.air_date <= CAST(@today AS TEXT)) AS INTEGER) AS aired_total,
  CAST((SELECT COUNT(*) FROM episodes e JOIN episode_watches w ON w.episode_id = e.id
        WHERE e.media_id = m.id AND e.season_number > 0 AND e.air_date IS NOT NULL AND e.air_date <= CAST(@today AS TEXT)) AS INTEGER) AS watched_total,
  CAST(COALESCE((SELECT MAX(w.watched_at) FROM episodes e JOIN episode_watches w ON w.episode_id = e.id
        WHERE e.media_id = m.id AND e.season_number > 0), 0) AS INTEGER) AS last_watched_at,
  CAST(COALESCE((SELECT e.id FROM episodes e LEFT JOIN episode_watches w ON w.episode_id = e.id
        WHERE e.media_id = m.id AND e.season_number > 0 AND e.air_date IS NOT NULL AND e.air_date <= CAST(@today AS TEXT) AND w.episode_id IS NULL
        ORDER BY e.season_number, e.episode_number LIMIT 1), 0) AS INTEGER) AS next_episode_id,
  CAST(COALESCE((SELECT MIN(e.air_date) FROM episodes e
        WHERE e.media_id = m.id AND e.season_number > 0 AND e.air_date > CAST(@today AS TEXT)), '') AS TEXT) AS next_air_date
FROM media m JOIN tracking t ON t.media_id = m.id
WHERE m.id = @media_id;

-- name: ListItemsByStatus :many
SELECT
  m.id, m.tmdb_id, m.media_type, m.title, m.overview, m.poster_path, m.release_date, m.tmdb_status, m.runtime_minutes, m.refreshed_at,
  t.status, t.status_changed_at, t.added_at, t.watched_at,
  CAST((SELECT COUNT(*) FROM episodes e
        WHERE e.media_id = m.id AND e.season_number > 0 AND e.air_date IS NOT NULL AND e.air_date <= CAST(@today AS TEXT)) AS INTEGER) AS aired_total,
  CAST((SELECT COUNT(*) FROM episodes e JOIN episode_watches w ON w.episode_id = e.id
        WHERE e.media_id = m.id AND e.season_number > 0 AND e.air_date IS NOT NULL AND e.air_date <= CAST(@today AS TEXT)) AS INTEGER) AS watched_total,
  CAST(COALESCE((SELECT MAX(w.watched_at) FROM episodes e JOIN episode_watches w ON w.episode_id = e.id
        WHERE e.media_id = m.id AND e.season_number > 0), 0) AS INTEGER) AS last_watched_at,
  CAST(COALESCE((SELECT e.id FROM episodes e LEFT JOIN episode_watches w ON w.episode_id = e.id
        WHERE e.media_id = m.id AND e.season_number > 0 AND e.air_date IS NOT NULL AND e.air_date <= CAST(@today AS TEXT) AND w.episode_id IS NULL
        ORDER BY e.season_number, e.episode_number LIMIT 1), 0) AS INTEGER) AS next_episode_id,
  CAST(COALESCE((SELECT MIN(e.air_date) FROM episodes e
        WHERE e.media_id = m.id AND e.season_number > 0 AND e.air_date > CAST(@today AS TEXT)), '') AS TEXT) AS next_air_date
FROM media m JOIN tracking t ON t.media_id = m.id
WHERE t.status = @status;
