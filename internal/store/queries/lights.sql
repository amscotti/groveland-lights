-- Light inventory queries. The lights table is created by
-- migrations/0002_lights.sql; soft-deleted rows (deleted_at IS NULL filters)
-- are invisible to every read and every edit, which is the A-208 store half.
-- created_at/updated_at/deleted_at are RFC3339 TEXT stamped by the typed
-- wrappers in internal/store/lights.go.

-- name: CreateLight :one
INSERT INTO lights (
  asset_no, kind, label, lat, lon, accuracy_m, photo_id,
  gml_pole_no, created_by, created_at, updated_at
) VALUES (
  sqlc.narg('asset_no'), sqlc.arg('kind'), sqlc.arg('label'), sqlc.arg('lat'),
  sqlc.arg('lon'), sqlc.narg('accuracy_m'), sqlc.narg('photo_id'),
  sqlc.narg('gml_pole_no'), sqlc.narg('created_by'),
  sqlc.arg('created_at'), sqlc.arg('updated_at')
)
RETURNING id, asset_no, kind, label, lat, lon, accuracy_m, photo_id,
  status, status_source, status_at, status_confirmed_at, gml_pole_no,
  created_by, created_at, updated_at, deleted_at;

-- name: GetLight :one
SELECT id, asset_no, kind, label, lat, lon, accuracy_m, photo_id,
  status, status_source, status_at, status_confirmed_at, gml_pole_no,
  created_by, created_at, updated_at, deleted_at
FROM lights
WHERE id = ?1 AND deleted_at IS NULL;

-- name: GetLightByAssetNo :one
SELECT id, asset_no, kind, label, lat, lon, accuracy_m, photo_id,
  status, status_source, status_at, status_confirmed_at, gml_pole_no,
  created_by, created_at, updated_at, deleted_at
FROM lights
WHERE asset_no = ?1 AND deleted_at IS NULL;

-- name: ListRecentLights :many
SELECT id, asset_no, kind, label, lat, lon, accuracy_m, photo_id,
  status, status_source, status_at, status_confirmed_at, gml_pole_no,
  created_by, created_at, updated_at, deleted_at
FROM lights
WHERE deleted_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT ?1 OFFSET ?2;

-- name: UpdateLight :one
UPDATE lights
SET label = ?1, asset_no = ?2, kind = ?3, updated_at = ?4
WHERE id = ?5 AND deleted_at IS NULL
RETURNING id, asset_no, kind, label, lat, lon, accuracy_m, photo_id,
  status, status_source, status_at, status_confirmed_at, gml_pole_no,
  created_by, created_at, updated_at, deleted_at;

-- name: SoftDeleteLight :one
UPDATE lights
SET deleted_at = ?1
WHERE id = ?2 AND deleted_at IS NULL
RETURNING id;

-- name: NearbyLights :many
SELECT id, asset_no, kind, label, lat, lon, accuracy_m, photo_id,
  status, status_source, status_at, status_confirmed_at, gml_pole_no,
  created_by, created_at, updated_at, deleted_at
FROM lights
WHERE deleted_at IS NULL
  AND lat >= sqlc.arg('min_lat') AND lat <= sqlc.arg('max_lat')
  AND lon >= sqlc.arg('min_lon') AND lon <= sqlc.arg('max_lon')
ORDER BY created_at DESC, id DESC;
