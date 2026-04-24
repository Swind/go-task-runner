-- name: InsertJob :exec
INSERT INTO jobs (id, type, args_data, status, result, priority, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpsertJob :exec
INSERT OR REPLACE INTO jobs (id, type, args_data, status, result, priority, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateJobStatus :exec
UPDATE jobs
SET status = ?, result = ?, updated_at = ?
WHERE id = ?;

-- name: GetJob :one
SELECT id, type, args_data, status, result, priority, created_at, updated_at
FROM jobs
WHERE id = ?;

-- name: ListAllJobs :many
SELECT id, type, args_data, status, result, priority, created_at, updated_at
FROM jobs
ORDER BY created_at ASC
LIMIT CASE WHEN ? = 0 THEN -1 ELSE ? END
OFFSET ?;

-- name: ListJobsByStatus :many
SELECT id, type, args_data, status, result, priority, created_at, updated_at
FROM jobs
WHERE status = ?
ORDER BY created_at ASC
LIMIT CASE WHEN ? = 0 THEN -1 ELSE ? END
OFFSET ?;

-- name: ListJobsByType :many
SELECT id, type, args_data, status, result, priority, created_at, updated_at
FROM jobs
WHERE type = ?
ORDER BY created_at ASC
LIMIT CASE WHEN ? = 0 THEN -1 ELSE ? END
OFFSET ?;

-- name: ListJobsByStatusAndType :many
SELECT id, type, args_data, status, result, priority, created_at, updated_at
FROM jobs
WHERE status = ? AND type = ?
ORDER BY created_at ASC
LIMIT CASE WHEN ? = 0 THEN -1 ELSE ? END
OFFSET ?;

-- name: GetPendingJobs :many
SELECT id, type, args_data, status, result, priority, created_at, updated_at
FROM jobs
WHERE status = 'PENDING'
ORDER BY created_at ASC;

-- name: DeleteJob :exec
DELETE FROM jobs WHERE id = ?;
