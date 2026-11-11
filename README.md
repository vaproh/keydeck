# License desk

A private, multi-product license server and key management desk. Separate from
any client repository.

Two deployables share one Postgres database and one artifact volume:

| Service | Purpose | Exposure |
| --- | --- | --- |
| `lsapp.api_public` | activation, heartbeat, deactivate, update check, download | public |
| `lsapp.api_admin` | dashboard, products, limits, features, plan presets, keys, releases, audit | private |

Updating the admin container never restarts the public API. Client OTA never
updates either service.

## What the desk manages

- Products with their own signing keys, prefix and status.
- Product limits and features, used as the vocabulary for plans and keys.
- Plan presets: trial, timed, lifetime; validity, activations, grace, limits,
  features.
- Keys with per-key plan name, limits, features, expiry, activations, notes.
- Activation records, revocation, reset, extend, deactivate.
- Releases and signed OTA manifests, per product and channel.
- Audit history.

There is no pricing, billing, customer portal or tier monetization.

## Login

Password + TOTP + a single-use math captcha. Sessions are HttpOnly,
SameSite=strict, with CSRF on every form. Failed attempts are rate-limited by
address.

Generate the credentials once:

```bash
uv run python - <<'PY'
from argon2 import PasswordHasher
import pyotp
print("password hash:", PasswordHasher().hash("replace-me"))
print("totp secret:", pyotp.random_base32())
PY
```

Set `LICENSE_ADMIN_PASSWORD_HASH` and `LICENSE_ADMIN_TOTP_SECRET`.

## Local development

```bash
uv sync --all-groups
export LICENSE_DB_URL=sqlite:///./license.db
export PUBLIC_BASE_URL=http://127.0.0.1:8000
export LICENSE_DOWNLOAD_SECRET=dev-secret
export LICENSE_ADMIN_PASSWORD_HASH='...'
export LICENSE_ADMIN_TOTP_SECRET='...'
export ADMIN_COOKIE_SECURE=0
export LICENSE_SIGNING_KEY_REORCHESTRA='...'
export UPDATE_SIGNING_KEY_REORCHESTRA='...'
uv run uvicorn lsapp.api_admin:app --reload --port 8000
```

Public API runs separately:

```bash
uv run uvicorn lsapp.api_public:app --reload --port 8001
```

The admin desk is at `/admin`. The login page is `/admin/login`.

## Keys

Generate one pair per product and store the private halves offline:

```bash
uv run python - <<'PY'
import base64
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
for name in ("LICENSE_SIGNING_KEY_REORCHESTRA", "UPDATE_SIGNING_KEY_REORCHESTRA"):
    key = Ed25519PrivateKey.generate()
    print(name, base64.urlsafe_b64encode(key.private_bytes_raw()).decode().rstrip("="))
PY
```

`ProductKey` stores only public halves and env variable names. Never commit
private keys.

## Deploy

```bash
cp .env.example .env
# fill .env
docker compose up -d --build
```

Point `license.example.com` at the public API and `admin.example.com` at the
admin service. Keep admin behind VPN/IP allowlist. Replace the sample ranges in
`Caddyfile`. Back up Postgres, artifacts and signing keys.

## Tests

```bash
uv run pytest -q
uv run ruff check .
```

## API contract

Public:

- `POST /v1/activate`
- `POST /v1/heartbeat`
- `POST /v1/deactivate`
- `GET /v1/update-check`
- `GET /v1/download/{id}`

Admin: browser routes under `/admin`; CI release publish at
`POST /v1/admin/products/{slug}/releases` with `X-Service-Token`.

The server is the key clock. Responses include `server_now`,
`remaining_seconds`, `expires_at` and `grace_seconds`.
