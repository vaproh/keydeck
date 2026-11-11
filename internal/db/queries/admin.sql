-- name: Stats :one
SELECT
    (SELECT count(*) FROM products) AS products,
    (SELECT count(*) FROM licenses) AS licenses,
    (SELECT count(*) FROM licenses WHERE status = 'active') AS active_licenses,
    (SELECT count(*) FROM activations WHERE revoked_at IS NULL) AS activations,
    (SELECT count(*) FROM licenses WHERE status = 'active' AND expires_at < now() + interval '7 days') AS expiring_7,
    (SELECT count(*) FROM licenses WHERE status = 'active' AND expires_at < now() + interval '30 days') AS expiring_30;

-- name: ExpiringLicenses :many
SELECT l.*, p.slug AS product_slug FROM licenses l JOIN products p ON p.id = l.product_id
WHERE l.status = 'active' AND l.expires_at < now() + interval '30 days'
ORDER BY l.expires_at LIMIT 20;

-- name: InsertAudit :exec
INSERT INTO audit_events (product_id, actor, action, target, metadata) VALUES ($1, $2, $3, $4, convert_from($5, 'UTF8')::jsonb);

-- name: ListAudit :many
SELECT * FROM audit_events WHERE ($1::bigint = 0 OR product_id = $1) ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: CreateSession :exec
INSERT INTO admin_sessions (id, actor, csrf_token, expires_at) VALUES ($1, $2, $3, $4);

-- name: GetSession :one
SELECT * FROM admin_sessions WHERE id = $1 AND expires_at > now();

-- name: DeleteSession :exec
DELETE FROM admin_sessions WHERE id = $1;

-- name: PruneSessions :exec
DELETE FROM admin_sessions WHERE expires_at < now();
