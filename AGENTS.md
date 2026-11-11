# AGENTS.md — Keydeck

Keydeck is an open-source, self-hosted license and OTA control plane for desktop products. It is a Go service with an embedded SvelteKit admin panel and Postgres as the source of truth.

## Non-negotiables

- **Never push directly to `main` or `beta`.** Create a feature branch, commit there, push it, and open a pull request against `beta`. Wait for required checks.
- **Never commit secrets.** No `.env`, signing keys, `DOWNLOAD_SECRET`, password hashes, TOTP secrets, database dumps, or customer keys.
- **The server is the key clock.** Do not introduce client-clock validity checks, trusting the local clock, or client-controlled expiry.
- **Trial keys have zero grace.** Do not add offline trial access.
- **Keep the three surfaces isolated.** License and OTA listeners must never mount admin routes.
- **Do not log secrets.** Log identifiers (`product_id`, `license_id`, request id) and outcome names, never keys, tokens, cookies, authorization headers, or request bodies.
- **Do not silently change the API.** `/v1/activate`, `/v1/heartbeat`, `/v1/deactivate`, and OTA routes have shipped clients.

## Workflow

1. Read the relevant issue or PRD.
2. Branch from `beta` and open the PR against `beta`. Use `feat/...`, `fix/...`, or `chore/...`.
3. Make the smallest complete change, including tests and docs.
4. Run:

```bash
just fmt
just test
just lint
just audit
```

5. Commit with conventional commits.
6. Push the branch and open a PR with:
   - what changed
   - why
   - how it was verified
   - migrations or deployment impact
   - security impact
7. Wait for `test` to pass. Do not merge your own PR without checking the diff.
8. Delete the branch after merge.

`main` is stable. Only release managers promote `beta` to `main` and create tags. See [docs/RELEASING.md](docs/RELEASING.md).

Do not use `--no-verify`, force-push `main`, or rewrite shared history.

## Client agents

If the task is to integrate Keydeck into another product, add a limit, or add a feature gate, follow [docs/AGENT_INTEGRATION.md](docs/AGENT_INTEGRATION.md) before changing code. Agents use a scoped bearer token from the project `.env`; never the admin password, TOTP seed, or session cookie.

The short version:

- A limit or feature is data, not a new table.
- Add the definition in the product's admin page.
- Add it to plan presets and issued keys.
- Enforce it in the client at the resource boundary.
- Add tests for missing, zero, at-limit, above-limit, and disabled-feature states.
- The server returns limits/features; it does not enforce product-specific ones.
- Publish artifacts through the admin API with a `releases:write` token; the OTA client verifies the signed manifest and SHA-256.
- Never add pricing, billing, or tiers.

## Repository map

```text
cmd/keydeck/              CLI: serve, migrate, passwd, keygen, healthcheck
internal/api/             public license/OTA handlers + private admin API
internal/auth/            Argon2id, TOTP, Turnstile, sessions, CSRF
internal/config/          environment and per-product signing keys
internal/db/              pgx pool, embedded migrations
internal/db/queries/      sqlc source queries
internal/store/           generated sqlc code
internal/licensing/       activation, heartbeat, leases, limits
internal/ota/             update check, manifests, download tokens
internal/server/          listeners, middleware, structured logging
web/                      SvelteKit 3 admin panel, embedded at build time
```

The Go binary embeds `web/build` through `web/embed.go`. The frontend is built in Docker/CI before `go build`.

## Local development

Prerequisites: Go 1.27, Node 24, pnpm 12, Postgres 18, Docker.

```bash
cp .env.example .env
go run ./cmd/keydeck keygen reorchestra
go run ./cmd/keydeck passwd
just test
just frontend
just build
```

Frontend:

```bash
cd web
pnpm install --frozen-lockfile
pnpm dev
pnpm exec svelte-check --tsconfig ./tsconfig.json
pnpm build
```

The admin panel is served under `/admin`. In development, Vite proxies `/api` to the admin listener.

## Tests

- `just test` runs Go unit tests and Postgres integration tests when `TEST_DATABASE_URL` is set.
- CI starts Postgres 18 and sets `TEST_DATABASE_URL`.
- Add a unit test for pure logic and a Postgres test for query/service behavior.
- Frontend checks: `svelte-check`, production build, `pnpm audit --prod`.

## Database changes

1. Add a numbered migration in `internal/db/migrations/`.
2. Add or update queries in `internal/db/queries/`.
3. Run `just sqlc`.
4. Commit generated `internal/store` changes.
5. Test both a fresh database and the upgrade path.

Migrations are additive and forward-only. Never edit an applied migration.

## Releases

- `main` is stable; `beta` is the integration branch.
- Releases are created only from protected `v*` tags: stable on `main`, pre-release on `beta`.
- The workflow runs tests, builds the frontend, builds `keydeck`, pushes the GHCR image, generates release notes automatically, and attaches a VPS bundle.
- The VPS bundle must contain the compiled binary and frontend; the VPS must not need a Go or Node toolchain.
- Watchtower pulls the new `latest` image for stable releases. Beta bundles pin the beta image.
- Never move or reuse a published tag.
- Agents never create release tags.

## UI direction

The admin is a private control room, not a marketing site. Use the existing stone/pine/terracotta palette, shadcn-svelte components, Lucide icons, and restrained Motion transitions. Keep dense operational tables readable. Respect reduced motion.

## Security boundaries

- Admin is private: TLS + `ADMIN_ALLOW`.
- License/OTA listeners are public but never expose admin routes.
- Per-product license and update signing keys are separate.
- Download tokens are short-lived and HMAC-signed.
- Revocation wins over cached leases.
- Desktop clients can be patched by a determined local admin; do not claim DRM-grade impossibility.

## When in doubt

Open a draft PR early and state assumptions. Do not merge uncertainty into `main`.
