-- name: InsertChangeEvent :exec
INSERT INTO change_events (
  entity_type,
  entity_id,
  user_id,
  op_type,
  version,
  payload,
  created_at
) VALUES (
  $1, $2, $3, $4, $5, $6, NOW()
);

-- name: ListChangeEvents :many
SELECT
  id,
  entity_type,
  entity_id,
  user_id,
  op_type,
  version,
  payload,
  created_at
FROM change_events
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: GetChangeEventByID :one
SELECT
  id,
  entity_type,
  entity_id,
  user_id,
  op_type,
  version,
  payload,
  created_at
FROM change_events
WHERE id = $1;

