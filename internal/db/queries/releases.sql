-- name: ListReleases :many
SELECT * FROM releases WHERE ($1::bigint = 0 OR product_id = $1) ORDER BY created_at DESC LIMIT 100;

-- name: GetRelease :one
SELECT * FROM releases WHERE id = $1;

-- name: FindUpdate :one
SELECT * FROM releases
WHERE product_id = $1 AND channel = $2 AND platform = $3 AND status = 'published'
ORDER BY created_at DESC LIMIT 1;

-- name: CreateRelease :one
INSERT INTO releases (
    product_id, version, channel, platform, min_version, mandatory, security,
    rollout_percent, notes, artifact_name, artifact_path, sha256, size_bytes,
    manifest, manifest_signature, status, published_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, now())
ON CONFLICT (product_id, version, channel, platform) DO UPDATE SET
    min_version = EXCLUDED.min_version, mandatory = EXCLUDED.mandatory,
    security = EXCLUDED.security, rollout_percent = EXCLUDED.rollout_percent,
    notes = EXCLUDED.notes, artifact_name = EXCLUDED.artifact_name,
    artifact_path = EXCLUDED.artifact_path, sha256 = EXCLUDED.sha256,
    size_bytes = EXCLUDED.size_bytes, manifest = EXCLUDED.manifest,
    manifest_signature = EXCLUDED.manifest_signature, status = EXCLUDED.status,
    published_at = now()
RETURNING *;
