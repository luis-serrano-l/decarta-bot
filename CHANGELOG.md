# Changelog

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

