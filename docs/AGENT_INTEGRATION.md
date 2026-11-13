# Agent integration guide

How a coding agent should add Keydeck licensing to a product, and how to add the limits and feature flags that product needs.

Read this before touching a client app or changing the server. If you are changing Keydeck itself, also read [AGENTS.md](../AGENTS.md) and [PRD.md](../PRD.md).

## The contract

Keydeck is the authority. The client never decides validity.

- The server issues a signed lease.
- The lease includes the product, plan, limits, features, installation hash, and server time.
- The client verifies the lease signature, then enforces the limits and features locally.
- The client must not trust the local wall clock for validity.
- The server does not enforce product-specific limits. The client does.
- A limit is a product rule, not a security boundary against a patched client.

## Agent access tokens

Do not give a coding agent the admin password or TOTP seed. Create a scoped token in **Agent access** and put it in the client project's `.env`:

```env
KEYDECK_ADMIN_URL=https://admin.example.com
KEYDECK_PRODUCT_SLUG=reorchestra
KEYDECK_ADMIN_TOKEN=kd_admin_...
```

If the database is empty, start once with `BOOTSTRAP_ADMIN_TOKEN` set to a strong random value. It has every scope and can create the first scoped token; remove it from `.env` and restart immediately after.

The agent sends the scoped token as a bearer:

```bash
curl -fsS "$KEYDECK_ADMIN_URL/api/admin/products/$PRODUCT_ID/limits" \
  -H "Authorization: Bearer $KEYDECK_ADMIN_TOKEN" \
  -H "content-type: application/json" \
  -d '{"key":"max_projects","label":"Maximum projects"}'
```

Rules:

- Keep `.env` out of git.
- Never ship the token in a desktop client.
- Use one token per project or agent.
- Grant only the scopes required: `products:read`, `products:write`, `licenses:read`, `licenses:write`, `releases:write`, `audit:read`.
- Scope the token to the product whenever possible.
- Bearer tokens do not use CSRF; do not send the session cookie.
- Revoke a token from **Agent access** if it leaks.

## 1. Create the product

In the admin panel:

1. Create a product with a stable lowercase slug, e.g. `reorchestra`.
2. Register its key prefix, e.g. `REO`.
3. Omit the public keys and Keydeck generates the license and update signing keys server-side; or register existing public keys.
4. Add the product limits.
5. Add the product features.
6. Create a plan preset for each common shape. The admin UI turns product limits into number inputs and features into checkboxes; the API still accepts `limits` and `features` JSON.
7. Issue a test key.

The admin API equivalents:

```http
POST /api/admin/products
{
  "slug": "reorchestra",
  "name": "ReOrchestra",
  "key_prefix": "REO"
}
```

```http
POST /api/admin/products/{id}/limits
{"key":"max_accounts","label":"Maximum accounts"}

POST /api/admin/products/{id}/features
{"key":"warmup","label":"Account warming"}
```

Every admin example in this guide uses a scoped bearer token. Agents never use the human session cookie.

## 2. Add a new limit

Limits are generic JSON. No migration is required.

Example:

```json
{"max_accounts": 50}
```

Steps:

1. Add the limit key in the product's admin page.
2. Add it to the product's plan presets if the preset should include it.
3. Add it to the client's typed license model.
4. Enforce it at the boundary where the limited resource is created or changed.
5. Add a test for:
   - missing limit
   - zero
   - exactly at the limit
   - one above the limit
   - an increased limit after re-activation/heartbeat
6. Document the limit in the client README.

Rules:

- Use stable snake_case keys.
- Never rename a shipped key. Add a new key and migrate the client if needed.
- The server returns limits as-is; it does not know what `max_accounts` means.
- The client must fail closed when a required limit is missing and the feature is not explicitly unlimited.
- Do not use the limit as the only defense against a malicious local user. It is a product rule.

Example enforcement:

```text
on account import:
  limits = lease.limits
  if "max_accounts" not in limits: reject with "license missing max_accounts"
  if current_accounts + new_accounts > limits.max_accounts: reject
```

## 3. Add a new feature

Features are a string array:

```json
["warmup", "campaigns"]
```

Steps:

1. Add the feature key in the product's admin page.
2. Include it in presets and issued keys.
3. Add a typed feature set in the client.
4. Gate the route, action, or UI control.
5. Add tests proving the control is unavailable when the feature is absent.
6. Document the feature key.

Rules:

- A feature key is a stable lowercase identifier.
- The server only returns the list. The client enforces it.
- The UI should show why a control is disabled, not silently hide it.
- Never add a feature whose absence can corrupt data; validate before the user starts work.

## 4. Issue a key

Trial:

```json
{
  "plan_name": "Trial",
  "key_type": "trial",
  "trial_minutes": 15,
  "max_activations": 1,
  "limits": {"max_accounts": 3},
  "features": ["warmup"]
}
```

Timed:

```json
{
  "plan_name": "Monthly",
  "key_type": "timed",
  "validity_days": 30,
  "starts_on_activation": true,
  "max_activations": 2,
  "limits": {"max_accounts": 50},
  "features": ["warmup", "campaigns"]
}
```

Lifetime:

```json
{
  "plan_name": "Lifetime",
  "key_type": "lifetime",
  "max_activations": 3,
  "limits": {"max_accounts": 200},
  "features": ["warmup", "campaigns"]
}
```

The raw key is returned once. Store only the hash server-side; show the raw key once to the operator.

## 5. Client activation

License base URL: `https://<license-domain>/v1`.

```http
POST /v1/activate
Content-Type: application/json

{
  "key": "REO-...",
  "installation_hash": "<64 hex chars>",
  "machine_hash": "<optional>",
  "app_version": "1.0.0",
  "product": "reorchestra"
}
```

Response:

```json
{
  "lease": "<base64-json>.<base64-signature>",
  "plan_name": "Lifetime",
  "key_type": "lifetime",
  "lifetime": true,
  "limits": {"max_accounts": 200},
  "features": ["warmup", "campaigns"],
  "server_now": 1760000000,
  "expires_at": null,
  "remaining_seconds": null,
  "grace_seconds": 86400
}
```

Heartbeat:

```http
POST /v1/heartbeat
```

Same body as activation. Heartbeat requires an existing activation.

Deactivate:

```http
POST /v1/deactivate
```

Same body. It revokes that installation and frees a slot.

## Device telemetry

Activation and heartbeat requests may include device data:

```json
{
  "key": "REO-...",
  "installation_hash": "<64 hex>",
  "machine_hash": "<optional>",
  "app_version": "1.4.0",
  "product": "reorchestra",
  "device_name": "DESKTOP-7F2A",
  "os": "Windows",
  "os_version": "11",
  "arch": "amd64",
  "user_agent": "ReOrchestra/1.4.0"
}
```

Every activate, heartbeat and deactivate is written to an event log. The activation ID links heartbeats to the activation record. If no activation matches, the heartbeat is stored with `activation_id: null` and shown as `unknown`.

Heartbeat events are retained for **7 days**; activation and deactivation events are retained. The server prunes old heartbeats every six hours.

## 6. Verify the lease

The lease is:

```text
base64(raw JSON claims) + "." + base64(Ed25519 signature)
```

Verify the signature with the product's license public key over the **raw decoded JSON bytes**. Do not re-marshal the JSON before verifying.

Required claims:

```json
{
  "product": "reorchestra",
  "license_id": 42,
  "plan_name": "Lifetime",
  "key_type": "lifetime",
  "lifetime": true,
  "limits": {"max_accounts": 200},
  "features": ["warmup", "campaigns"],
  "installation_hash": "…",
  "server_now": 1760000000,
  "issued_at": 1760000000,
  "lease_expires_at": 1760086400,
  "expires_at": null,
  "remaining_seconds": null,
  "grace_until": null
}
```

Reject the lease if:

- the signature fails
- `product` does not match the embedded product slug
- `installation_hash` does not match the local installation
- `server_now` is too far from the last server time
- the license is expired and the grace window has ended

## 7. Server clock

- Use `server_now` and `remaining_seconds` to render countdowns.
- Use monotonic time only to animate the countdown.
- Never extend access because the local clock was moved backward.
- On startup, call activation/heartbeat before unlocking the UI.
- Trial keys have `grace_seconds: 0`.
- Paid keys may have grace; never exceed `grace_until`.

## 8. Retries and errors

Retry network errors, timeouts, `429`, and `5xx` with bounded exponential backoff and jitter.

Do not retry:

- `401` invalid or revoked
- `403` expired, wrong product, or disabled product
- `409` activation limit, already used, or no activation

On `403`/`401`, lock the client immediately. On network failure, keep a valid cached lease only until `grace_until`.

## 9. Gate the whole app

The activation screen is the only surface available without a valid lease.

- Block the dashboard, task/queue actions, account import, settings changes, and automation.
- Re-check a middleware gate on every sensitive request, not only at startup.
- Stop background work when the lease expires or is revoked.
- Keep the app unable to perform the licensed function when the key is missing.

## 10. Publish a release and wire OTA

Use a token with `products:read` and `releases:write`, scoped to the product.

```env
KEYDECK_ADMIN_URL=https://admin.example.com
KEYDECK_OTA_URL=https://ota.example.com
KEYDECK_PRODUCT_SLUG=reorchestra
KEYDECK_ADMIN_TOKEN=kd_admin_...
```

Build the client ZIP, then upload it:

```bash
curl -fsS "$KEYDECK_ADMIN_URL/api/admin/releases" \
  -H "Authorization: Bearer $KEYDECK_ADMIN_TOKEN" \
  -F "product_id=$PRODUCT_ID" \
  -F "version=1.4.0" \
  -F "channel=stable" \
  -F "platform=windows-amd64" \
  -F "min_version=1.0.0" \
  -F "mandatory=false" \
  -F "security=false" \
  -F "notes=First stable build" \
  -F "artifact=@dist/reorchestra-1.4.0-windows-amd64.zip"
```

Keydeck:

1. Stores the artifact.
2. Computes SHA-256.
3. Signs the update manifest with the product's update signing key.
4. Publishes the release for the selected channel.

The shipped client checks:

```http
GET $KEYDECK_OTA_URL/v1/update-check?product=reorchestra&channel=stable&platform=windows-amd64&current=1.3.0
```

It must:

- verify the manifest signature with the embedded update public key
- compare the returned version using semver
- download the short-lived URL
- verify SHA-256
- reject unsafe ZIP paths
- stage the update, self-test, and roll back on failure
- preserve user data

Never put `KEYDECK_ADMIN_TOKEN` in the shipped client. OTA check/download endpoints are public today; the admin token is only for publishing. Lease-bound OTA is tracked in issue #23.

## 11. OTA client

Update base URL: `https://<ota-domain>/v1`.

```http
GET /v1/update-check?product=reorchestra&channel=stable&platform=windows-amd64&current=1.0.0
```

The response includes the release metadata, signed manifest, and download URL. Verify the manifest with the product's update public key, download, verify SHA-256, and only then install. This endpoint is public today; do not assume a license is required until issue #23 ships.

## 12. Agent checklist

Use this as the definition of done when adding a product, limit, or feature:

- [ ] Product exists with the correct slug and key prefix.
- [ ] Product signing keys were generated server-side or registered explicitly.
- [ ] Every new limit is defined in the admin and the client.
- [ ] Every new feature is defined in the admin and the client.
- [ ] A test key exists with the intended limits/features.
- [ ] Client verifies the lease signature and installation hash.
- [ ] Client enforces missing/zero/at-limit/above-limit cases.
- [ ] Client gates the UI and the backend action for the feature.
- [ ] Trial has zero grace; paid grace uses `grace_until`.
- [ ] Local clock cannot extend access.
- [ ] Revoked/expired keys stop background work.
- [ ] Retries do not retry `401`, `403`, or `409`.
- [ ] No key, token, cookie, or signing secret is logged.
- [ ] Client tests cover success, expired, revoked, wrong product, and no activation.
- [ ] README and client docs list the limits and features.
- [ ] A PR is open; `main` is never pushed directly.

## 13. Copy-paste prompt for a client agent

```text
You are adding Keydeck licensing to <product>.

Read the Keydeck docs:
- AGENTS.md
- PRD.md
- docs/AGENT_INTEGRATION.md

Product slug: <slug>
License API: https://<license-domain>/v1
OTA API: https://<ota-domain>/v1
Admin API: https://<admin-domain>/api
Admin token: from the project .env (never commit it)
License public key: <base64>
Update public key: <base64>

Required limits:
- <key>: <meaning>

Required features:
- <key>: <meaning>

Do:
1. Add typed license claims for the limits/features.
2. Verify the Ed25519 lease before trusting it.
3. Gate the entire app behind activation.
4. Enforce every limit where the resource is created.
5. Gate every feature route and UI control.
6. Use server_now/remaining_seconds, never the local clock.
7. Add tests for missing limit, zero, at limit, above limit, disabled feature,
   expired, revoked, wrong product, and no activation.
8. Do not log keys, tokens, cookies, or machine hashes.
9. Publish the first release with the scoped admin token, then verify OTA
   update-check, manifest signature, SHA-256, and rollback.
10. Open a PR with the evidence.

Do not add pricing, billing, tiers, or a customer portal.
Do not add a new key type without a server migration.
Do not claim the client is uncrackable.
```

## 14. Changing Keydeck itself

If the task changes the server:

- Keep `/v1/activate`, `/v1/heartbeat`, `/v1/deactivate`, and OTA backward compatible.
- Add a numbered migration for schema changes.
- Update sqlc queries and regenerate.
- Add a Postgres integration test for activation/expiry/revocation.
- Update the admin UI for new first-class fields.
- Update `PRD.md`, `AGENTS.md`, and this guide.
- Open a PR against `beta`; never push `main` or `beta` directly.

If the task only adds a product-specific limit or feature, do not change the server schema. Add it through the admin data and enforce it in the client.
