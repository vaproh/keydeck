# Keydeck admin

SvelteKit 3 + Svelte 5 + TypeScript control panel for Keydeck.

## Development

```bash
pnpm install --frozen-lockfile
pnpm dev
pnpm check
pnpm build
```

The app is served at `/admin` by the Go binary. Vite proxies `/api` to the admin listener during development.

## UI rules

- Use shadcn-svelte components, Lucide icons, Tailwind tokens, and restrained Motion.
- Product limits render as number inputs and features as checkboxes in plan presets.
- Do not require raw JSON for known product variables.
- Keep the stone/pine/terracotta palette and support light/dark mode, including auto system detection on login.
- Run `pnpm check` and `pnpm build` before committing.
