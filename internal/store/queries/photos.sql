-- Photo metadata queries. The photos table is created by
-- migrations/0002_lights.sql; the photo BLOB itself lives in PHOTO_DIR on
-- disk, the row only records the stored path and processed dimensions.
-- owner_id stays NULL until the auth slice (hand-written database/sql in the
-- session/user store files) begins attaching photos to user accounts.

-- name: CreatePhoto :one
INSERT INTO photos (path, width, height, bytes, sha256, created_at)
VALUES (?1, ?2, ?3, ?4, ?5, ?6)
RETURNING id, owner_id, path, width, height, bytes, sha256, created_at;

-- name: GetPhoto :one
SELECT id, owner_id, path, width, height, bytes, sha256, created_at
FROM photos
WHERE id = ?1;

-- name: ListPhotos :many
SELECT id, owner_id, path, width, height, bytes, sha256, created_at
FROM photos
ORDER BY id
LIMIT ?1;
