CREATE TABLE IF NOT EXISTS admin_tokens (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    token_hint TEXT NOT NULL DEFAULT '',
    scopes_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    product_id BIGINT NOT NULL DEFAULT 0 REFERENCES products(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS admin_tokens_product_idx ON admin_tokens(product_id);
