# License platform (R5)

Two deployables sharing one Postgres database and one artifact volume:

| Service | Purpose | Exposure |
| --- | --- | --- |
| `lsapp.api_public` | activation, heartbeat, update check, download token | `license.example.com` |
| `lsapp.api_admin` | products, keys, licences, audit | `admin.example.com`, private only |

Updating the admin container never restarts the public API, and client OTA never
touches either server.

## Keys

Generate once, store offline, never in the repo:

```bash
uv run python - <<'PY'
import base64
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
for name in ("LICENSE_SIGNING_PRIVATE_KEY", "UPDATE_SIGNING_PRIVATE_KEY"):
    key = Ed25519PrivateKey.generate()
    print(name, base64.urlsafe_b64encode(key.private_bytes_raw()).decode().rstrip("="))
PY
```

Register the public halves with `POST /v1/admin/products/{id}/keys`; the client
pins their SPKI hashes.

## Admin login

Argon2 password hash plus TOTP:

```bash
uv run python -c "from argon2 import PasswordHasher; print(PasswordHasher().hash('…'))"
uv run python -c "import pyotp; print(pyotp.random_base32())"
```

Set `LICENSE_ADMIN_PASSWORD_HASH` and `LICENSE_ADMIN_TOTP_SECRET`, then
`POST /v1/admin/login` for an 8-hour bearer token.

## Licences

Per key: `plan_name`, `limits_json`, `features_json`, `expires_at`,
`max_activations`. The raw key is returned **once**; only its SHA-256 and last
four characters are stored. Actions: `revoke`, `reset` activations, `extend`.

No pricing, tiers, invoices, billing or customer portal exist by design.

## Deploy

```bash
export POSTGRES_PASSWORD=… LICENSE_SIGNING_PRIVATE_KEY=… UPDATE_SIGNING_PRIVATE_KEY=…
export LICENSE_DOWNLOAD_SECRET=… LICENSE_ADMIN_PASSWORD_HASH=… LICENSE_ADMIN_TOTP_SECRET=…
docker compose up -d --build
```

Point `license.example.com` and `admin.example.com` at the host, keep admin
behind VPN/allowlist, and back up Postgres plus the signing keys.

## Tests

```bash
uv run pytest license-server/tests -q
```
