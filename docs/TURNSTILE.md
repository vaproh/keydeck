# Cloudflare Turnstile

Keydeck uses Cloudflare Turnstile on the admin login form only. License, heartbeat, OTA, download, and authenticated admin routes do not use it.

## Create the widget

1. Open the Cloudflare dashboard → Turnstile → Add widget.
2. Add the admin hostname, for example `admin.example.com`.
3. Widget mode: **Managed**.
4. Appearance: **Always** (the login page renders the widget visibly).
5. Copy the **Site Key** and **Secret Key**.

The widget is rendered explicitly by the Svelte login page. The public Site Key is fetched at runtime from `GET /api/auth/config`; it is not baked into the frontend build. The Secret Key stays in the Go process.

## Environment

```env
TURNSTILE_SITE_KEY=0x...
TURNSTILE_SECRET_KEY=0x...
TURNSTILE_EXPECTED_HOSTNAME=admin.example.com
```

`TURNSTILE_EXPECTED_HOSTNAME` must exactly match the hostname Cloudflare returns from Siteverify.

If any value is missing, Keydeck refuses to start. Login is never silently unprotected.

## Validation

The Go backend posts `secret`, `response`, and `remoteip` to:

```text
https://challenges.cloudflare.com/turnstile/v0/siteverify
```

It fails closed on a missing/invalid token, duplicate token, hostname mismatch, action mismatch, non-2xx response, timeout, or malformed body.

The widget sets the action `admin_login`; the server requires that same action.

Tokens are single-use and expire after five minutes. Keydeck also keeps a short in-process replay cache; Cloudflare's eventual-consistency behavior means a single instance cannot rely on that cache alone.

## Development and tests

Unit tests mock Siteverify with `httptest`; they never call Cloudflare.

For local UI work, Cloudflare publishes test keys. Use them only in an isolated environment and never in production:

- Site Key: `1x00000000000000000000AA` (visible, always passes)
- Site Key: `1x00000000000000000000BB` (invisible, always passes)
- Site Key: `3x00000000000000000000FF` (forces interaction)
- Secret Key: `1x0000000000000000000000000000000AA` (always passes)
- Secret Key: `2x0000000000000000000000000000000AA` (always fails)
- Secret Key: `3x0000000000000000000000000000000AA` (already spent)

Test Siteverify responses return `hostname: "example.com"` and **omit `action`**. Keydeck requires both hostname and `action=admin_login`, so test keys validate the widget but **cannot complete a real login**. Unit tests mock Siteverify. To test the full login flow, use the real widget keys on a hostname registered in the Turnstile widget, or a local stub that returns the expected hostname and action.

This is intentional: accepting a missing action would let a token solved on another form unlock the admin login.

## Deployment

The VPS needs no frontend toolchain. Put the three values in `.env`; Docker Compose passes them to the Go container. The compiled frontend only fetches the public Site Key at runtime.

The existing Cloudflare WAF interstitial on `/admin/login` is independent. Do not change its scope to compensate for Turnstile.

## Failure behavior

- Missing config → process refuses to start.
- Siteverify unavailable/timeout → login returns `human verification failed`.
- Invalid/expired/duplicate token → login fails before password or TOTP.
- Valid Turnstile but wrong password/TOTP → login fails.
- No session is created unless every check passes.
