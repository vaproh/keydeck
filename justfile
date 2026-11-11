set shell := ["bash", "-cu"]

default:
    @just --list

# Run Go tests
test:
    go test ./...

# Format Go and frontend sources
fmt:
    gofmt -w cmd internal
    cd web && pnpm exec prettier --write .

# Vet and type-check
lint:
    go vet ./...
    cd web && pnpm exec svelte-kit sync && pnpm exec svelte-check --tsconfig ./tsconfig.json

# Build the frontend locally.
frontend:
    cd web && pnpm build

# Build the production image
build:
    docker build -t license-desk:local .

# Start the VPS stack
up:
    docker compose up -d

# Stop the stack
down:
    docker compose down

# Follow container logs
logs:
    docker compose logs -f desk caddy

# Regenerate sqlc code after changing migrations or queries
sqlc:
    /tmp/opencode/bin/sqlc generate

# Production audit of frontend runtime dependencies
audit:
    cd web && pnpm audit --prod --audit-level=high

# Apply pending database migrations by starting the app once
migrate:
    docker compose run --rm desk /desk migrate

# Generate an Argon2 admin password hash
passwd:
    go run ./cmd/desk passwd

# Generate product signing keys (usage: just keygen reorchestra)
keygen product:
    go run ./cmd/desk keygen {{product}}
