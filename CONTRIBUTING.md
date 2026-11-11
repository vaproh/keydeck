# Contributing

Keydeck accepts changes through pull requests only. `main` (stable) and `beta` (integration) are protected.

## Before you start

- Open an issue for large changes.
- Read [AGENTS.md](AGENTS.md) and [PRD.md](PRD.md).
- Never commit secrets, `.env`, signing keys, database dumps, or release artifacts.

## Branch and PR

```bash
git switch beta
git pull
git switch -c feat/short-name
# make the change
just fmt
just test
just lint
just audit
git push -u origin feat/short-name
gh pr create --base beta --fill
```

A PR should state:

- what changed and why
- how it was verified
- migrations or deployment impact
- security impact

Wait for the `test` check. Do not merge with failing checks. Release managers promote `beta` to `main`; contributors do not tag releases.

## Development

```bash
just test
just frontend
just lint
just audit
```

- Go: standard library style, `slog` for logs, no secrets in logs.
- SQL: add a numbered migration and sqlc query, then `just sqlc`.
- Svelte: runes mode, shadcn-svelte components, Lucide icons, restrained Motion.
- Keep API changes backward compatible for shipped clients.

## Commits

Conventional commits:

```text
feat: add release channels
fix: reject cross-product activation
docs: explain trial clock
chore: bump Go version
```

By contributing you agree that your contribution is licensed under the MIT license.
