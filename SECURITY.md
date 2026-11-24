# Security

Do not open a public issue for a vulnerability.

Email the maintainer or use GitHub's private vulnerability reporting. Include reproduction steps and the affected version (current stable: 1.2.2).

## Boundaries

- The admin surface must stay behind HAProxy/TierHive TLS, the application login, Turnstile, Argon2id, TOTP, and a network ACL where available.
- License and OTA listeners must never mount admin routes.
- Signing keys and `DOWNLOAD_SECRET` are server-only secrets.
- The server clock is authoritative; never trust client time for key validity.
- Report leaked keys immediately; revoke the key and its activations.
- Signing seeds live in the `signing_keys` volume; back it up and restrict access.
- OTA update-check/download are currently public; treat artifacts as public downloads until issue #23 lands.

No desktop client is uncrackable. This project raises the cost of bypass and makes revocation, expiry and sharing control server-authoritative; it does not claim DRM-grade impossibility.
