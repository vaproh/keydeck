# syntax=docker/dockerfile:1.7

FROM node:24-bookworm-slim AS web
WORKDIR /src/web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM golang:1.27.3-bookworm AS gobuild
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/build ./web/build
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/keydeck ./cmd/keydeck

FROM alpine:3.22 AS dirs
RUN mkdir -p /data/artifacts /data/keys && chown -R 65532:65532 /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=dirs /data /data
COPY --from=gobuild /out/keydeck /keydeck
EXPOSE 8080 8081 8082
USER nonroot:nonroot
ENTRYPOINT ["/keydeck"]
