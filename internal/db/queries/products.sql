-- name: ListProducts :many
SELECT * FROM products ORDER BY name;

-- name: GetProduct :one
SELECT * FROM products WHERE id = $1;

-- name: GetProductBySlug :one
SELECT * FROM products WHERE slug = $1;

-- name: CreateProduct :one
INSERT INTO products (slug, name, status) VALUES ($1, $2, $3) RETURNING *;

-- name: UpdateProduct :one
UPDATE products SET name = $2, status = $3 WHERE id = $1 RETURNING *;

-- name: GetProductKey :one
SELECT * FROM product_keys WHERE product_id = $1;

-- name: UpsertProductKey :one
INSERT INTO product_keys (product_id, license_public_key, update_public_key, key_prefix)
VALUES ($1, $2, $3, $4)
ON CONFLICT (product_id) DO UPDATE SET license_public_key = EXCLUDED.license_public_key,
    update_public_key = EXCLUDED.update_public_key, key_prefix = EXCLUDED.key_prefix
RETURNING *;

-- name: ListLimits :many
SELECT * FROM product_limits WHERE product_id = $1 ORDER BY key;

-- name: AddLimit :one
INSERT INTO product_limits (product_id, key, label) VALUES ($1, $2, $3)
ON CONFLICT (product_id, key) DO UPDATE SET label = EXCLUDED.label RETURNING *;

-- name: DeleteLimit :exec
DELETE FROM product_limits WHERE id = $1 AND product_id = $2;

-- name: ListFeatures :many
SELECT * FROM product_features WHERE product_id = $1 ORDER BY key;

-- name: AddFeature :one
INSERT INTO product_features (product_id, key, label) VALUES ($1, $2, $3)
ON CONFLICT (product_id, key) DO UPDATE SET label = EXCLUDED.label RETURNING *;

-- name: DeleteFeature :exec
DELETE FROM product_features WHERE id = $1 AND product_id = $2;

-- name: ListPresets :many
SELECT * FROM plan_presets WHERE product_id = $1 ORDER BY name;

-- name: CreatePreset :one
INSERT INTO plan_presets (product_id, name, limits_json, features_json, validity_days, max_activations)
VALUES ($1, $2, convert_from($3, 'UTF8')::jsonb, convert_from($4, 'UTF8')::jsonb, $5, $6)
ON CONFLICT (product_id, name) DO UPDATE SET limits_json = EXCLUDED.limits_json,
    features_json = EXCLUDED.features_json, validity_days = EXCLUDED.validity_days,
    max_activations = EXCLUDED.max_activations
RETURNING *;

-- name: DeletePreset :exec
DELETE FROM plan_presets WHERE id = $1 AND product_id = $2;
