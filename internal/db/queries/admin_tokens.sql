-- name: CreateAdminToken :one
INSERT INTO admin_tokens (name, token_hash, token_hint, scopes_json, product_id, expires_at)
VALUES ($1, $2, $3, convert_from($4, 'UTF8')::jsonb, $5, $6)
RETURNING *;

-- name: ListAdminTokens :many
SELECT * FROM admin_tokens ORDER BY created_at DESC;

-- name: GetAdminTokenByHash :one
SELECT * FROM admin_tokens WHERE token_hash = $1;

-- name: TouchAdminToken :exec
UPDATE admin_tokens SET last_used_at = now() WHERE id = $1;

-- name: RevokeAdminToken :exec
UPDATE admin_tokens SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL;
