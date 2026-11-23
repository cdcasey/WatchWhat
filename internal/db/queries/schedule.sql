-- name: AssignSchedule :execrows
INSERT INTO weekly_schedule (weekday, media_id, position)
VALUES (?1, ?2, (SELECT COALESCE(MAX(position), 0) + 1 FROM weekly_schedule WHERE weekday = ?1))
ON CONFLICT (weekday, media_id) DO NOTHING;

-- name: RemoveSchedule :exec
DELETE FROM weekly_schedule WHERE weekday = ? AND media_id = ?;

-- name: GetScheduleEntry :one
SELECT * FROM weekly_schedule WHERE weekday = ? AND media_id = ?;

-- name: PrevScheduleEntry :one
SELECT * FROM weekly_schedule WHERE weekday = ? AND position < ? ORDER BY position DESC LIMIT 1;

-- name: NextScheduleEntry :one
SELECT * FROM weekly_schedule WHERE weekday = ? AND position > ? ORDER BY position ASC LIMIT 1;

-- name: SetSchedulePosition :exec
UPDATE weekly_schedule SET position = ? WHERE weekday = ? AND media_id = ?;

-- name: ListSchedule :many
SELECT s.weekday, s.media_id, s.position, m.title, m.poster_path, t.status
FROM weekly_schedule s
JOIN media m ON m.id = s.media_id
JOIN tracking t ON t.media_id = s.media_id
ORDER BY s.weekday, s.position;

-- name: ListScheduleDay :many
SELECT s.weekday, s.media_id, s.position, m.title, m.poster_path, t.status
FROM weekly_schedule s
JOIN media m ON m.id = s.media_id
JOIN tracking t ON t.media_id = s.media_id
WHERE s.weekday = ?
ORDER BY s.position;

-- name: ListSchedulable :many
SELECT m.id, m.title
FROM media m JOIN tracking t ON t.media_id = m.id
WHERE m.media_type = 'tv' AND t.status IN ('backlog', 'watching')
ORDER BY m.title COLLATE NOCASE;
