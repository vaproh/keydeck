-- name: CreateLicenseEvent :one
INSERT INTO license_events (
    license_id, activation_id, event_type, installation_hash, machine_hash, app_version, ip,
    device_name, os, os_version, arch, user_agent
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *;

-- name: ListLicenseEvents :many
SELECT * FROM license_events
WHERE license_id = $1 AND event_type IN ('activation', 'deactivation')
ORDER BY created_at DESC LIMIT $2;

-- name: ListLicenseHeartbeats :many
SELECT * FROM license_events
WHERE license_id = $1 AND event_type = 'heartbeat'
ORDER BY created_at DESC LIMIT $2;

-- name: ListHeartbeats :many
SELECT e.*, l.holder_name, l.plan_name, l.key_last4, p.slug AS product_slug, p.name AS product_name
FROM license_events e
JOIN licenses l ON l.id = e.license_id
JOIN products p ON p.id = l.product_id
WHERE e.event_type = 'heartbeat'
  AND ($1::bigint = 0 OR p.id = $1)
  AND ($2::bigint = 0 OR e.license_id = $2)
ORDER BY e.created_at DESC LIMIT $3 OFFSET $4;

-- name: CountHeartbeats :one
SELECT count(*) FROM license_events e
JOIN licenses l ON l.id = e.license_id
WHERE e.event_type = 'heartbeat'
  AND ($1::bigint = 0 OR l.product_id = $1)
  AND ($2::bigint = 0 OR e.license_id = $2);

-- name: DeleteOldHeartbeats :execrows
DELETE FROM license_events
WHERE event_type = 'heartbeat' AND created_at < now() - interval '7 days';
