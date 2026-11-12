ALTER TABLE activations
    ADD COLUMN IF NOT EXISTS device_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS os TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS os_version TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS arch TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS user_agent TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS license_events (
    id BIGSERIAL PRIMARY KEY,
    license_id BIGINT NOT NULL REFERENCES licenses(id) ON DELETE CASCADE,
    activation_id BIGINT REFERENCES activations(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('activation', 'heartbeat', 'deactivation')),
    installation_hash TEXT NOT NULL DEFAULT '',
    machine_hash TEXT NOT NULL DEFAULT '',
    app_version TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT '',
    device_name TEXT NOT NULL DEFAULT '',
    os TEXT NOT NULL DEFAULT '',
    os_version TEXT NOT NULL DEFAULT '',
    arch TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS license_events_license_idx ON license_events(license_id, created_at DESC);
CREATE INDEX IF NOT EXISTS license_events_type_created_idx ON license_events(event_type, created_at DESC);
