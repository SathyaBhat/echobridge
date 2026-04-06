# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is EchoBridge

EchoBridge is a self-hosted cross-posting application that publishes to multiple social platforms simultaneously. Currently supports Mastodon; Twitter, Bluesky, Telegram, and Discord are planned.

## Commands

```bash
make build          # Compile Go binary to backend/echobridge
make run            # Build and run locally on :8080
make clean          # Remove binary and backend/data/

make docker-build   # Build Docker image via docker compose
make docker-run     # Run via docker compose (uses Traefik + Dokploy)
make docker-clean   # docker compose down -v

cd backend && go test ./...   # Run tests
cd backend && go vet ./...    # Vet Go code
```

## Architecture

```
frontend/ (Vanilla JS + Pico CSS)
    ↓ REST API
backend/ (Go + chi router)
    ↓
SQLite (backend/data/echobridge.db)
    ↓
Social Platforms (Mastodon, ...)
```

**Backend structure:**
- `cmd/server/main.go` — entry point, initializes DB, starts HTTP server, serves frontend static files
- `internal/api/router.go` — chi router with CORS, logging, recovery, and PNA middleware
- `internal/api/handlers.go` — HTTP handlers for accounts, media, posts, OAuth callbacks
- `internal/db/` — SQLite layer (accounts, mastodon_apps, media tables)
- `internal/models/models.go` — shared data structures
- `internal/providers/` — `provider.go` defines the `Provider` interface; `mastodon.go` implements it

**Adding a new platform:** implement the `Provider` interface (`Name()`, `Post()`, `UploadMedia()`) in `internal/providers/`, then register it in the handlers.

## Key Design Points

**OAuth flow (Mastodon):** The app dynamically registers itself per-instance. Instance credentials are cached in `mastodon_apps` DB table. The OAuth callback URL must match `ECHOBRIDGE_BASE_URL`.

**Cross-posting flow:** Media files are uploaded locally first, then for each selected account the media is re-uploaded to the platform's API, and the post is created with the platform's media IDs.

**Access tokens** use `json:"-"` and are never returned in API responses.

**PNA header:** The `Access-Control-Allow-Private-Network: true` header is unconditionally set via Traefik middleware to support Chrome's Private Network Access policy for local deployments.

**Path prefix support:** `ECHOBRIDGE_PATH_PREFIX` enables subdirectory deployment (e.g., `/echobridge`). The frontend gets `config.js` served dynamically from the backend which sets `window.apiBase`.

## Environment Variables

| Variable | Default | Notes |
|---|---|---|
| `ECHOBRIDGE_PORT` | `8080` | Listen port |
| `ECHOBRIDGE_DB_PATH` | `./data/echobridge.db` | SQLite file path |
| `ECHOBRIDGE_UPLOAD_DIR` | `./data/uploads` | Local media storage |
| `ECHOBRIDGE_BASE_URL` | `http://localhost:8080` | Must be set correctly for OAuth callbacks |
| `ECHOBRIDGE_FRONTEND_DIR` | `../frontend` | Relative to binary location |
| `ECHOBRIDGE_PATH_PREFIX` | `` | Subdirectory prefix (e.g. `/echobridge`) |

## Docker / Deployment

The `docker-compose.yml` is configured for Dokploy with Traefik. Key labels set up:
- TLS via Let's Encrypt (`dokploy-network` external network required)
- Traefik middlewares: PNA header injection, CORS, strip prefix
- PNA middleware must be ordered **before** CORS middleware

For reference documentation, see `SPEC.md` (API specs, data models) and `TASKS.md` (phased roadmap).
