<p align="center">
  <img src="assets/keydeck.svg" width="84" alt="Keydeck" />
</p>

<h1 align="center">Keydeck</h1>

<p align="center"><strong>License and release control for desktop products.</strong></p>

<p align="center">
  <a href="https://github.com/vaproh/keydeck/actions/workflows/build.yml"><img alt="build" src="https://github.com/vaproh/keydeck/actions/workflows/build.yml/badge.svg" /></a>
  <a href="https://github.com/vaproh/keydeck/blob/main/LICENSE"><img alt="MIT" src="https://img.shields.io/badge/license-MIT-123a2f" /></a>
  <a href="https://github.com/vaproh/keydeck/releases"><img alt="release" src="https://img.shields.io/github/v/release/vaproh/keydeck?color=d97745" /></a>
</p>

Keydeck is a self-hosted license server, OTA update service, and private admin panel for desktop software. One Go binary serves three isolated surfaces and embeds the compiled SvelteKit frontend.

| Surface | Default port | Purpose |
|---|---:|---|
| License API | `8080` | activation, heartbeat, deactivation |
| OTA | `8081` | update check, signed manifest, artifact download |
| Admin | `8082` | private SvelteKit control panel and admin API |

Postgres is the source of truth for products, keys, activations, releases, sessions, and audit events.

## Why Keydeck

- **Trial, timed, and lifetime keys** with server-authoritative clocks.
- **Per-key limits and features** managed in the admin panel. No pricing or billing.
- **Multi-product** by design. One deployment can serve every tool you ship.
- **Trial protection**: starts on first activation, one activation, no grace.
- **Lifetime keys** with no expiry but full revocation and heartbeat checks.
- **Signed OTA manifests** with SHA-256, stable/beta channels, and short-lived download URLs.
- **Private admin** with Argon2id, TOTP, captcha, CSRF, rate limiting, and IP allowlisting.
- **Structured JSON logs** with request IDs. Keys, tokens, cookies, and bodies are never logged.
- **Docker-only deployment** with Caddy TLS and Watchtower-based continuous deployment.

## Stack

- Go 1.27, chi, `net/http`
- Postgres 18, pgx, sqlc
- SvelteKit 3, Svelte 5, TypeScript
- Tailwind CSS, shadcn-svelte, Lucide, Motion
- Docker Compose on the VPS; no systemd

## Quick start

```bash
cp .env.example .env
go run ./cmd/keydeck keygen reorchestra   # put the output in .env
go run ./cmd/keydeck passwd               # put the hash in ADMIN_PASSWORD_HASH
```

Fill in `ADMIN_TOTP_SECRET` from your authenticator, set a long random `DOWNLOAD_SECRET`, and set the three domains.

```bash
just test       # Go tests, including Postgres integration when TEST_DATABASE_URL is set
just lint       # go vet + svelte-check
just frontend   # local frontend build
just build      # production image
just up         # start the VPS stack
```

## Domains

Point three DNS records at the VPS:

- `license.example.com` → `:8080`
- `ota.example.com` → `:8081`
- `admin.example.com` → `:8082`

Caddy terminates TLS for all three. The admin host is restricted by `ADMIN_ALLOW` (IP or CIDR). The license and OTA hosts are public.

## Release flow

Every protected `v*` tag builds and publishes:

1. Go tests and `go vet`.
2. Frontend production build.
3. Linux binary with the embedded frontend.
4. Docker image at `ghcr.io/<owner>/keydeck`.
5. GitHub release with automatically generated release notes.
6. Self-contained VPS bundle containing `docker-compose.yml`, `Caddyfile`, `.env.example`, the compiled `keydeck` binary, the compiled frontend, and `Dockerfile`.

On the VPS, download the bundle once, fill `.env`, and run:

```bash
docker compose up -d
```

The stack includes Watchtower. When a new `latest` image is published, it pulls and restarts the app container. Postgres and Caddy keep running.

## License behavior

- **Trial**: starts on first activation, one-time, one activation, zero grace.
- **Timed**: starts on issue or first activation, configurable days, admin extendable.
- **Lifetime**: no expiry, still revocable and heartbeat-checked.
- Per-key `plan_name`, `limits`, `features`, `max_activations`, expiry, and notes.
- The server is the key clock. Clients use `server_now`, `expires_at`, and `remaining_seconds`; the local clock is never authoritative.
- Signed Ed25519 leases bind the product, installation, plan, limits, and expiry.

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

Never commit `.env`, signing keys, the download secret, password hashes, TOTP secrets, or database dumps.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md). Changes go through a pull request; `main` is protected by required checks.

## Security

See [SECURITY.md](SECURITY.md). Report vulnerabilities privately.

## License

MIT. See [LICENSE](LICENSE).
