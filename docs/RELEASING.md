# Releasing Keydeck

Keydeck uses two long-lived branches and protected tags.

## Branches

| Branch | Purpose |
|---|---|
| `main` | Stable. Every commit here is releasable. |
| `beta` | Integration branch for the next release. |

Feature PRs target `beta`. A release manager promotes `beta` to `main` after the beta has been exercised.

Both branches require the `test` check and a pull request. Do not push directly to either branch.

## Tags

| Tag | Branch | Channel | Image |
|---|---|---|---|
| `v1.4.0` | `main` | stable | `1.4.0`, `1.4`, `latest` |
| `v1.4.0-beta.1` | `beta` | beta | `1.4.0-beta.1` |

The release workflow verifies that the tagged commit is on the matching branch:

- stable tag must be on `main`
- pre-release tag must be on `beta`

It then runs tests, builds the frontend, pushes the multi-arch image, creates the GitHub release with generated notes, and attaches the self-contained VPS bundle.

## Agent workflow

Coding agents must:

1. Branch from `beta`.
2. Open a PR targeting `beta`.
3. Wait for `test` to pass.
4. Never create a release tag.

Only a release manager creates tags, and only after the PR is merged.

## VPS deployment

The stable bundle points Compose at the `latest` image. Watchtower pulls it on a new stable release.

The beta bundle pins the exact beta image, so a beta host is not moved onto stable by Watchtower. To return a beta host to stable, replace the image tag with `latest`.

## Rollback

Use the previous image tag in `docker-compose.yml` and restart the app container. Database migrations are forward-only; keep a database backup before a release that adds columns.
