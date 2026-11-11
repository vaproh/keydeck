# Contributing

Open an issue before a large change. Keep changes small and reviewable.

```bash
just fmt
just test
just lint
```

- Use conventional commits.
- Never commit secrets, keys, `.env`, database dumps or generated release artifacts.
- New SQL goes in `internal/db/migrations` and `internal/db/queries`; regenerate with `just sqlc`.
- Frontend code uses Svelte 5 runes and shadcn-svelte components.
- Run `just audit` before shipping frontend changes.

By contributing you agree that your contribution is licensed under the MIT license.
