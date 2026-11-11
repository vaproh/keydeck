# License desk

Open-source license and OTA server for Windows desktop products. One Go binary serves three isolated surfaces:

| Surface | Default port | Purpose |
|---|---:|---|
| License API | `8080` | activation, heartbeat, deactivation |
| OTA | `8081` | update check, signed manifest, artifact download |
| Admin | `8082` | private SvelteKit control panel and admin API |

The binary embeds the compiled SvelteKit frontend. Postgres is the source of truth for keys, activations, releases, sessions and audit events.

## Stack

- Go 1.27, chi, net/http
- Postgres 18, pgx, sqlc
- SvelteKit 3, Svelte 5, TypeScript
- Tailwind CSS, shadcn-svelte, Lucide, Motion
- Docker Compose on the VPS; no systemd

## Local setup

```bash
cp .env.example .env
go run ./cmd/desk keygen reorchestra   # put the output in .env
go run ./cmd/desk passwd               # put the hash in ADMIN_PASSWORD_HASH
```

Fill in `ADMIN_TOTP_SECRET` from your authenticator and set a long random `DOWNLOAD_SECRET`.

```bash
just test
just build
just up
```

The frontend is built on GitHub Actions and embedded into the Go binary.

## Domains

Point three DNS records at the VPS:

- `license.example.com` → `:8080`
- `ota.example.com` → `:8081`
- `admin.example.com` → `:8082`

Caddy terminates TLS for all three. The admin host is restricted by `ADMIN_ALLOW` (IP or CIDR). The license and OTA hosts are public.

## Release flow

Every protected `v*` tag builds and publishes:

1. Go tests.
2. Frontend production build.
3. Linux binary with embedded frontend.
4. Multi-platform Docker image at `ghcr.io/<owner>/license-desk`.
5. GitHub release with automatically generated release notes.
6. Self-contained VPS bundle containing `docker-compose.yml`, `Caddyfile`, `.env.example`, the compiled `desk` binary and the compiled frontend.

On the VPS, download the latest bundle once, fill `.env`, and run:

```bash
docker compose up -d
```

The stack includes Watchtower. When a new `latest` image is published, it pulls and restarts the app container. Postgres and Caddy keep running.

## License behavior

- **Trial**: starts on first activation, one-time, one activation, zero grace.
- **Timed**: starts on issue or first activation, configurable days, admin extendable.
- **Lifetime**: no expiry, still revocable and heartbeat-checked.
- Per-key `plan_name`, `limits`, `features`, `max_activations`, expiry, notes.
- The server is the key clock. Clients use `server_now`, `expires_at` and `remaining_seconds`; the local Windows clock is never authoritative.
- Activation, heartbeat and deactivation are idempotent enough for retries. Signed Ed25519 leases bind the product, installation, plan, limits and expiry.

## OTA behavior

- Product-scoped signed manifests.
- SHA-256 verified by the client.
- Short-lived HMAC download URLs.
- Stable and beta channels.
- Artifacts are stored under `ARTIFACTS_DIR` and mounted as a Docker volume.

## Operations

```bash
just logs
just migrate
just down
```

Never commit `.env`, signing keys, the download secret, password hashes, TOTP secrets or database dumps.

## License

MIT. See [LICENSE](LICENSE).
