-- name: ListLifts :many
SELECT id, lift_type, created_at, coaching_cue, coaching_diagnosis
FROM lifts
ORDER BY created_at DESC;

-- name: GetLift :one
SELECT id, lift_type, created_at, coaching_cue, coaching_diagnosis
FROM lifts
WHERE id = ?;

-- name: CreateLift :one
INSERT INTO lifts (lift_type, created_at)
VALUES (?, ?)
RETURNING id, lift_type, created_at, coaching_cue, coaching_diagnosis;

-- name: DeleteLift :exec
DELETE FROM lifts WHERE id = ?;
