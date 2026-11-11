<p align="center">
  <img src="assets/keydeck.svg" width="84" alt="Keydeck" />
</p>

<h1 align="center">Keydeck</h1>

<p align="center"><strong>License and release control for desktop products.</strong></p>

<p align="center">
  <a href="https://github.com/vaproh/keydeck/actions/workflows/build.yml"><img alt="build" src="https://github.com/vaproh/keydeck/actions/workflows/build.yml/badge.svg" /></a>
  <a href="https://github.com/vaproh/keydeck/releases"><img alt="release" src="https://img.shields.io/github/v/release/vaproh/keydeck?color=d97745" /></a>
  <a href="https://github.com/vaproh/keydeck/blob/main/LICENSE"><img alt="MIT" src="https://img.shields.io/badge/license-MIT-123a2f" /></a>
  <a href="https://github.com/vaproh/keydeck/pkgs/container/keydeck"><img alt="GHCR" src="https://img.shields.io/badge/GHCR-keydeck-2496ED?logo=docker&logoColor=white" /></a>
  <a href="https://github.com/vaproh/keydeck/pulls"><img alt="PRs welcome" src="https://img.shields.io/badge/PRs-welcome-123a2f" /></a>
</p>
<p align="center">
  <img alt="Go 1.27" src="https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white" />
  <img alt="SvelteKit 3" src="https://img.shields.io/badge/SvelteKit-3-FF3E00?logo=svelte&logoColor=white" />
  <img alt="Svelte 5" src="https://img.shields.io/badge/Svelte-5-FF3E00?logo=svelte&logoColor=white" />
  <img alt="Postgres 18" src="https://img.shields.io/badge/Postgres-18-4169E1?logo=postgresql&logoColor=white" />
  <img alt="Docker" src="https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white" />
  <img alt="Self-hosted" src="https://img.shields.io/badge/self--hosted-yes-123a2f" />
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
- **Per-key holder names** so every key shows who or what it belongs to.
- **Multi-product** by design. One deployment can serve every tool you ship.
- **Trial protection**: starts on first activation, one activation, no grace.
- **Lifetime keys** with no expiry but full revocation and heartbeat checks.
- **Signed OTA manifests** with SHA-256, stable/beta channels, and short-lived download URLs.
- **Scoped agent tokens** for project agents: product-limited, scope-limited, revocable, and shown once.
- **Private admin** with Argon2id, TOTP, Cloudflare Turnstile, CSRF, rate limiting, IP allowlisting, seven-day sessions, and light/dark mode. See [docs/TURNSTILE.md](docs/TURNSTILE.md).
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
go run ./cmd/keydeck passwd               # put the hash in ADMIN_PASSWORD_HASH
openssl rand -base64 48                  # BOOTSTRAP_ADMIN_TOKEN, optional
```

Fill in `ADMIN_TOTP_SECRET` from your authenticator, set a long random `DOWNLOAD_SECRET`, set the three domains, and add the Cloudflare Turnstile site key, secret key, and admin hostname. See [docs/TURNSTILE.md](docs/TURNSTILE.md).

A fresh database is **empty**: no products, keys, plans, releases, or sessions. Nothing is seeded. Use the bootstrap token once to create the first scoped agent token, then remove it from `.env` and restart. See [docs/AGENT_INTEGRATION.md](docs/AGENT_INTEGRATION.md).

Creating a product through the admin API generates its license and update signing keys on the server and stores them under `SIGNING_KEYS_DIR` (`/data/keys` in Docker). Only the public keys are registered. Set the env overrides only if you want to manage the seeds yourself.

Back up the `signing_keys` Docker volume; losing it makes existing leases and OTA manifests unverifiable.

```bash
just test       # Go tests, including Postgres integration when TEST_DATABASE_URL is set
just lint       # go vet + svelte-check
just frontend   # local frontend build
just build      # production image
just up         # start the VPS stack
```

## Host health ports

Keydeck publishes its three listeners for local health checks and HAProxy:

```text
127.0.0.1:8080  license API
127.0.0.1:8081  OTA API
127.0.0.1:8082  admin SPA/API
```

`KEYDECK_BIND_IP` controls the bind address. Keep it on `127.0.0.1` when HAProxy runs on the same host. Use a private interface or `0.0.0.0` behind a firewall if HAProxy is remote; never expose `8082` publicly without an IP allowlist.

## Domains

Point three DNS records at the VPS:

- `license.example.com` → `:8080`
- `ota.example.com` → `:8081`
- `admin.example.com` → `:8082`

Caddy terminates TLS for all three. The admin host is restricted by `ADMIN_ALLOW` (IP or CIDR). The license and OTA hosts are public.

## Release flow

`main` is stable; `beta` is the integration branch. Feature PRs target `beta`, and release managers promote `beta` to `main`. See [docs/RELEASING.md](docs/RELEASING.md).

Every protected `v*` tag builds and publishes:

1. Go tests, race detector, and coverage gate.
2. Frontend production build.
3. Linux binary with the embedded frontend.
4. Docker image at `ghcr.io/<owner>/keydeck`.
5. GitHub release with automatically generated release notes.
6. Self-contained VPS bundle containing `docker-compose.yml`, `Caddyfile`, `.env.example`, the compiled `keydeck` binary, the compiled frontend, and `Dockerfile`.

Stable tags publish `latest`; beta tags publish only the pinned pre-release image, so a beta host does not drift onto stable.

On the VPS, download the bundle once, fill `.env`, and run:

```bash
docker compose up -d
```

The stack includes Watchtower. When a new `latest` image is published, it pulls and restarts the app container. Postgres and Caddy keep running.

## License behavior

- **Trial**: starts on first activation, one-time, one activation, zero grace.
- **Timed**: starts on issue or first activation, configurable days, admin extendable.
- **Lifetime**: no expiry, still revocable and heartbeat-checked.
- Per-key `plan_name`, `holder_name`, `limits`, `features`, `max_activations`, expiry, and notes.
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

See [CONTRIBUTING.md](CONTRIBUTING.md), [AGENTS.md](AGENTS.md), and the [agent integration guide](docs/AGENT_INTEGRATION.md). Changes go through a pull request; `main` is protected by required checks.

## Security

See [SECURITY.md](SECURITY.md). Report vulnerabilities privately.

## License

MIT. See [LICENSE](LICENSE).
