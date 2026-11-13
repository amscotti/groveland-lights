-- Probe counter queries. The probe table is created by
-- migrations/0001_init.sql and always holds exactly one row (id = 1).

-- name: ProbeCount :one
SELECT n
FROM probe
WHERE id = 1;

-- name: ProbeInc :one
UPDATE probe
SET n = n + 1
WHERE id = 1
RETURNING n;
