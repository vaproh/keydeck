-- name: ListLicenses :many
SELECT l.*, p.slug AS product_slug,
    (SELECT count(*) FROM activations a WHERE a.license_id = l.id AND a.revoked_at IS NULL) AS activation_count
FROM licenses l JOIN products p ON p.id = l.product_id
WHERE ($1::bigint = 0 OR l.product_id = $1)
ORDER BY l.created_at DESC LIMIT $2 OFFSET $3;

-- name: CountLicenses :one
SELECT count(*) FROM licenses WHERE ($1::bigint = 0 OR product_id = $1);

-- name: GetLicense :one
SELECT * FROM licenses WHERE id = $1;

-- name: GetLicenseByHash :one
SELECT * FROM licenses WHERE key_hash = $1;

-- name: CreateLicense :one
INSERT INTO licenses (
    product_id, key_hash, key_last4, plan_name, holder_name, key_type, trial_minutes, validity_days,
    starts_on_activation, one_time_activation, grace_seconds, lease_ttl_seconds, limits_json, features_json,
    expires_at, max_activations, notes
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, convert_from($13, 'UTF8')::jsonb, convert_from($14, 'UTF8')::jsonb, $15, $16, $17)
RETURNING *;

-- name: UpdateLicense :one
UPDATE licenses SET plan_name = $2, holder_name = $3,
    limits_json = convert_from($4, 'UTF8')::jsonb,
    features_json = convert_from($5, 'UTF8')::jsonb,
    max_activations = $6, notes = $7, grace_seconds = $8, lease_ttl_seconds = $9, expires_at = $10
WHERE id = $1 RETURNING *;

-- name: RevokeLicense :exec
UPDATE licenses SET status = 'revoked' WHERE id = $1;

-- name: ActivateLicense :one
UPDATE licenses SET first_activated_at = COALESCE(first_activated_at, $2),
    activation_expires_at = COALESCE(activation_expires_at, $3),
    used_at = COALESCE(used_at, $2)
WHERE id = $1 RETURNING *;

-- name: ResetLicense :one
UPDATE licenses SET first_activated_at = NULL, activation_expires_at = NULL, used_at = NULL,
    status = 'active' WHERE id = $1 RETURNING *;

-- name: ExtendLicense :one
UPDATE licenses SET expires_at = $2, activation_expires_at = $3 WHERE id = $1 RETURNING *;

-- name: ListActivations :many
SELECT * FROM activations WHERE license_id = $1 ORDER BY created_at DESC;

-- name: GetActivation :one
SELECT * FROM activations WHERE license_id = $1 AND installation_hash = $2;

-- name: CountActiveActivations :one
SELECT count(*) FROM activations WHERE license_id = $1 AND revoked_at IS NULL;

-- name: CreateActivation :one
INSERT INTO activations (license_id, installation_hash, machine_hash, app_version, ip)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: TouchActivation :exec
UPDATE activations SET last_seen_at = now(), app_version = $3
WHERE license_id = $1 AND installation_hash = $2;

-- name: RevokeActivation :exec
UPDATE activations SET revoked_at = now() WHERE license_id = $1 AND installation_hash = $2 AND revoked_at IS NULL;
