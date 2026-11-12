# Changelog

## [Unreleased]

Hardening for the open source release. This is a breaking release.

### Changed

- Menus are published to the site repository as `static/menus/<id>.json`
  through the GitHub Contents API (`GITHUB_TOKEN`, `GITHUB_REPO`,
  `GITHUB_BRANCH`). The bot no longer serves menus over HTTP.
- Menu ids are base62 of a truncated HMAC-SHA256(`HASH_SECRET`, user ID),
  17 characters long. **All existing menu URLs change.**
- `HASH_SECRET` is mandatory and must be at least 32 characters; the
  hard-coded default is gone and is rejected if supplied.
- Long polling is the only mode.
- Users in `ALLOWED_USER_IDS` can publish (previously administrators only).
- Menus can be sent as a `.json` file as well as a message.
- Item `description` is optional; items may carry an `image`.
- Logs are structured, never contain tokens, and identify users only by
  their menu id.
- The container image is distroless, non-root, read-only and pinned by
  digest, with a `HEALTHCHECK`.

### Added

- Upload cap (`MAX_UPLOAD_BYTES`, 256 KB) and publish rate limit
  (`PUBLISH_RATE_LIMIT` per `PUBLISH_RATE_WINDOW`, 5 per hour).
- Strict validation: unknown JSON fields rejected, string lengths and item
  counts capped, image URLs restricted to https or site-relative paths.
- `compose.yaml`, a complete `.env.example`, CI, Dependabot, `SECURITY.md`.

### Removed

- `/api/debug`, every `/api/*` menu endpoint, `/webhook` and the CORS
  middleware. Only `GET /health` remains.
- Webhook mode (`USE_POLLING`, `WEBHOOK_URL`, `ENVIRONMENT`).
- The Postgres backend (`STORAGE_TYPE`, `DATABASE_URL`): the site
  repository is now the source of truth.
- `cmd/check_db`, `cmd/clear_webhook`, `cmd/get_hash`, `cmd/test_token`.
- The gin and lib/pq dependencies.

## [v0.1.0] - 2025-11-27

### Key Features

**Development:** Polling mode (default) - bot polls Telegram API every 60s. Webhook mode available for production.

**Endpoints:**
- `POST /webhook` - Telegram updates (production)
- `GET /api/meals.json` - Serves meals data
- `GET /api/menu.json` - Serves menu data
- `GET /health` - Health check

**Storage:** File-based in `./storage/`. Validates JSON schema (required fields: name, price, description) before saving.

**Security:** User authorization via `ALLOWED_USER_IDS`. Only `meals.json` and `menu.json` files accepted.

**Status:** ✅ Functional for local development. 🚧 Hugo integration and production deployment pending.

