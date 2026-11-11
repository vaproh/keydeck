CREATE TABLE IF NOT EXISTS products (
    id BIGSERIAL PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS product_keys (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL UNIQUE REFERENCES products(id) ON DELETE CASCADE,
    license_public_key TEXT NOT NULL,
    update_public_key TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS product_limits (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    UNIQUE (product_id, key)
);

CREATE TABLE IF NOT EXISTS product_features (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    UNIQUE (product_id, key)
);

CREATE TABLE IF NOT EXISTS plan_presets (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    limits_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    features_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    validity_days INT,
    max_activations INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (product_id, name)
);

CREATE TABLE IF NOT EXISTS licenses (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    key_hash TEXT NOT NULL UNIQUE,
    key_last4 TEXT NOT NULL DEFAULT '',
    plan_name TEXT NOT NULL DEFAULT 'standard',
    key_type TEXT NOT NULL DEFAULT 'timed' CHECK (key_type IN ('trial', 'timed', 'lifetime')),
    trial_minutes INT,
    validity_days INT,
    starts_on_activation BOOLEAN NOT NULL DEFAULT false,
    one_time_activation BOOLEAN NOT NULL DEFAULT false,
    grace_seconds INT NOT NULL DEFAULT 86400,
    first_activated_at TIMESTAMPTZ,
    activation_expires_at TIMESTAMPTZ,
    used_at TIMESTAMPTZ,
    limits_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    features_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    expires_at TIMESTAMPTZ NOT NULL,
    max_activations INT NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS licenses_product_idx ON licenses(product_id);
CREATE INDEX IF NOT EXISTS licenses_expiry_idx ON licenses(expires_at);

CREATE TABLE IF NOT EXISTS activations (
    id BIGSERIAL PRIMARY KEY,
    license_id BIGINT NOT NULL REFERENCES licenses(id) ON DELETE CASCADE,
    installation_hash TEXT NOT NULL,
    machine_hash TEXT NOT NULL DEFAULT '',
    app_version TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT '',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    UNIQUE (license_id, installation_hash)
);
CREATE INDEX IF NOT EXISTS activations_license_idx ON activations(license_id);

CREATE TABLE IF NOT EXISTS releases (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    version TEXT NOT NULL,
    channel TEXT NOT NULL DEFAULT 'stable',
    platform TEXT NOT NULL DEFAULT 'windows-amd64',
    min_version TEXT NOT NULL DEFAULT '',
    mandatory BOOLEAN NOT NULL DEFAULT false,
    security BOOLEAN NOT NULL DEFAULT false,
    rollout_percent INT NOT NULL DEFAULT 100,
    notes TEXT NOT NULL DEFAULT '',
    artifact_name TEXT NOT NULL,
    artifact_path TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    manifest TEXT NOT NULL DEFAULT '',
    manifest_signature TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'published' CHECK (status IN ('draft', 'published', 'unpublished')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    UNIQUE (product_id, version, channel, platform)
);
CREATE INDEX IF NOT EXISTS releases_lookup_idx ON releases(product_id, channel, platform, status);

CREATE TABLE IF NOT EXISTS admin_sessions (
    id TEXT PRIMARY KEY,
    actor TEXT NOT NULL,
    csrf_token TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS admin_sessions_expiry_idx ON admin_sessions(expires_at);

CREATE TABLE IF NOT EXISTS audit_events (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT REFERENCES products(id) ON DELETE SET NULL,
    actor TEXT NOT NULL,
    action TEXT NOT NULL,
    target TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS audit_created_idx ON audit_events(created_at DESC);
