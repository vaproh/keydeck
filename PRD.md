# Keydeck — Product Requirements

## 1. Problem

Independent desktop software makers need a simple way to:

- give a user a time-limited trial,
- sell monthly or lifetime access,
- revoke a leaked or refunded key,
- enforce per-customer limits,
- ship updates without rebuilding a custom updater.

Existing license services are SaaS-first, expensive, or tied to one product. Keydeck is self-hosted, open source, multi-product, and small enough to run on one VPS.

## 2. Users

**Primary user:** the operator. A developer who ships one or more desktop tools and wants a private control plane.

**Secondary user:** the end customer. They receive a key, enter it in the desktop app, and expect activation to be quick and reliable.

## 3. Goals

- A desktop client is unusable without a valid server-issued key.
- The server is the sole authority for validity, activation, expiry, limits, and revocation.
- One deployment serves multiple products.
- Trial, timed, and lifetime keys are first-class.
- The operator can change limits and expiry per key from an admin panel.
- OTA delivery is product-scoped, signed, and verifiable.
- Deployment is one Docker Compose stack on a VPS.
- The repository is open source and releases are reproducible from tags.

## 4. Non-goals

- Payment processing, invoices, subscriptions, or a customer portal.
- Per-seat SaaS billing or pricing tiers.
- A public marketplace.
- DRM-grade impossibility against a determined local administrator.
- Mobile or web client SDKs.
- Email, Telegram, or webhook notifications.
- Multi-tenant organizations or role-based admin teams.

## 5. Key model

Every key belongs to one product and carries:

| Field | Meaning |
|---|---|
| `plan_name` | Free-text label shown to the client |
| `key_type` | `trial`, `timed`, or `lifetime` |
| `trial_minutes` | Trial duration from first activation |
| `validity_days` | Timed duration |
| `starts_on_activation` | Start the clock on first activation, not issue |
| `one_time_activation` | Bind to one installation |
| `grace_seconds` | Offline grace; forced to `0` for trials |
| `limits_json` | Product-defined numeric limits, e.g. `{"max_accounts": 50}` |
| `features_json` | Enabled features, e.g. `["warmup","campaigns"]` |
| `expires_at` | Absolute expiry for issue-started timed keys |
| `max_activations` | Maximum live installations |
| `status` | `active` or `revoked` |
| `notes` | Operator-only context |

### Trial

- Starts on first activation.
- One-time, one activation.
- Zero grace.
- Expired or used trials cannot be reactivated.
- The same installation may refresh its lease until expiry.

### Timed

- Configurable days.
- Starts on issue or first activation.
- Admin can extend.
- Multiple activations up to the limit.

### Lifetime

- No expiry.
- Heartbeat continues.
- Revocable.
- Admin cannot extend; there is nothing to extend.

## 6. Server-authoritative time

Activation and heartbeat responses include:

```json
{
  "server_now": 1760000000,
  "expires_at": 1760000900,
  "remaining_seconds": 900,
  "grace_seconds": 0
}
```

The client may render a countdown from `remaining_seconds` using monotonic time, but must not grant access based on the local wall clock. Changing the Windows clock must not extend a key.

The VPS must run NTP. A wrong server clock makes every key wrong.

## 7. Activation lifecycle

```text
key entered
  -> POST /v1/activate
  -> product active?
  -> key active?
  -> effective expiry in the future?
  -> activation limit?
  -> one-time key already used by another installation?
  -> create/refresh activation
  -> start trial clock or timed clock on first activation
  -> sign Ed25519 lease
  -> return lease + server clock + plan + limits + features
```

Heartbeat requires an existing activation. A key alone is not enough.

Deactivation revokes one installation and frees a slot. It is idempotent.

## 8. OTA requirements

- Every release belongs to one product.
- Channels: `stable` and `beta`.
- Manifest is signed with the product's update key.
- Client verifies signature and SHA-256.
- Download URLs are short-lived and HMAC-signed.
- `min_version`, `mandatory`, and `security` are explicit.
- Rollout percentage is stored for future use.
- The update service never updates the license server or admin panel.

## 9. Admin requirements

- Private hostname, TLS, IP allowlist.
- Argon2id password, TOTP, Cloudflare Turnstile.
- HttpOnly, SameSite cookie; CSRF token on mutations.
- Login rate limit.
- Product switcher.
- Product limits, features, and plan presets.
- Issue, inspect, extend, reset, and revoke keys.
- Release upload with SHA-256 and notes.
- Audit log for privileged actions.
- No secrets in HTML or logs.

## 10. Deployment requirements

- One Go binary with embedded frontend.
- Three listeners: license, OTA, admin.
- Postgres 18.
- Caddy for TLS and host routing.
- Docker Compose only; no systemd.
- Watchtower pulls `latest` after a release.
- VPS release bundle includes compiled binary and frontend; no build toolchain required.
- `.env` holds secrets and domains.

## 11. Release requirements

- Tag `v*` triggers CI.
- Tests and frontend build must pass.
- Build Linux amd64 binary.
- Build and push GHCR image.
- Generate release notes automatically.
- Attach a versioned VPS bundle.
- Publish the GitHub release.
- Never publish from a non-`main` commit.

## 12. Success criteria

- A fresh VPS can deploy from the bundle and `.env` in under 15 minutes.
- A key cannot activate for the wrong product.
- A disabled product cannot activate or update.
- A trial cannot be reused by another installation.
- A revoked key fails on the next heartbeat.
- A clock change on the client does not extend validity.
- The admin panel is unreachable outside `ADMIN_ALLOW`.
- CI passes on every PR and tag.
- Release notes and assets are generated without manual editing.

## 13. Risks

- A local administrator can patch a desktop client. Server checks make sharing, expiry, and revocation hard to bypass, not impossible.
- Lost signing keys require reissuing product keys and clients.
- Fully automatic releases require protected tags and a clean CI pipeline.
- Watchtower restarts can interrupt in-flight requests; use health checks and short leases.
