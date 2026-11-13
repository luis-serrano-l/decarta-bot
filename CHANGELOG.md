# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `/qr` sends a QR code of the menu address as an image, to print or show
  on a screen. The "Menu published" message has a **Get QR code** button
  that does the same.

## [1.0.0] - 2026-11-12

This is a breaking release: menus move to the site repository, every
menu address changes, and the configuration is different. Start from a
fresh `.env.example`.

### Added

- Preview before publishing: a valid menu gets a summary (categories,
  item count, price range) with **Publish** and **Cancel** buttons, and is
  published only on confirmation. The confirmation carries an **Open my
  menu** button and says that the page updates in about a minute.
- `/delete` unpublishes a menu after a confirmation button.
- `/link` returns the menu address (`/myurl` and `/menuurl` remain as
  aliases).
- `/template` sends an example `menu.json` as a file.
- `/start` explains the bot in three lines with **Get a template** and
  **How it works** buttons.
- The command menu is registered with Telegram at startup.
- The bot shows "typing…" while it downloads, checks, publishes or
  deletes.
- Validation reports every problem with its exact field path and a fix,
  at most five per message.
- The list format accepts its keywords in any letter case.
- Upload rate limit: 20 menus checked per user per 10 minutes, separate
  from the publish limit.
- Documentation organised by Diátaxis under `docs/`: a tutorial, how-to
  guides, reference (including a JSON Schema) and explanation (design and
  threat model). `CONTRIBUTING.md`.
- Upload cap (`MAX_UPLOAD_BYTES`, 256 KB) and publish rate limit
  (`PUBLISH_RATE_LIMIT` per `PUBLISH_RATE_WINDOW`, 5 per hour).
- Strict validation: unknown JSON fields rejected, string lengths and item
  counts capped, image URLs restricted to https or site-relative paths.
- `compose.yaml`, a complete `.env.example`, CI, Dependabot, `SECURITY.md`.

### Changed

- The project, repository, Go module, binary and Docker image are renamed
  from `demenu_bot` to `decarta-bot`, and the site repository from `demenu`
  to `decarta-site`: update `GITHUB_REPO` and `HUGO_BASE_URL`. With Docker
  Compose the data volume becomes `decarta-bot_decarta-data`; copy an
  existing volume across before upgrading.
- Sending a menu no longer publishes it immediately; see the preview
  above. The publish rate limit is counted at confirmation, so previews
  and rejected menus do not use it up, and the reply says when to retry.
- `/status` and `/myid` are available to everyone (previously
  administrators only). `/status` reports the caller's access and menu.
- Ordinary chat messages get a hint instead of a list of syntax errors.
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

### Removed

- `/api/debug`, every `/api/*` menu endpoint, `/webhook` and the CORS
  middleware. Only `GET /health` remains.
- Webhook mode (`USE_POLLING`, `WEBHOOK_URL`, `ENVIRONMENT`).
- The Postgres backend (`STORAGE_TYPE`, `DATABASE_URL`): the site
  repository is now the source of truth.
- `cmd/check_db`, `cmd/clear_webhook`, `cmd/get_hash`, `cmd/test_token`.
- The gin and lib/pq dependencies.
- Hosting-specific deployment and tunnelling guides.

### Security

- `HASH_SECRET` is mandatory, with no default; menu ids cannot be turned
  back into Telegram user IDs without it.
- Tokens are redacted from all log output, and users are identified in
  logs only by menu id.
- Confirmation buttons are single-use, bound to their user and expire
  after 15 minutes.

## [0.1.0] - 2025-11-27

### Added

- Telegram bot with polling mode for development and webhook mode for
  production.
- HTTP endpoints: `POST /webhook`, `GET /api/meals.json`,
  `GET /api/menu.json`, `GET /health`.
- File-based storage in `./storage/`, with validation of the required
  fields (name, price, description) before saving.
- Authorisation through `ALLOWED_USER_IDS`; only `meals.json` and
  `menu.json` files accepted.

[Unreleased]: https://github.com/luis-serrano-l/decarta-bot/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/luis-serrano-l/decarta-bot/releases/tag/v1.0.0
