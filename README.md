<p align="center"><img src="docs/logo.svg" alt="Demenu logo" width="160"></p>

# demenu_bot

[![CI](https://github.com/luis-serrano-l/demenu_bot/actions/workflows/ci.yml/badge.svg)](https://github.com/luis-serrano-l/demenu_bot/actions/workflows/ci.yml)

A Telegram bot that lets authorised users publish a restaurant menu. The
bot validates what it receives and commits it as `static/menus/<id>.json`
to the companion static site,
[luis-serrano-l/demenu](https://github.com/luis-serrano-l/demenu), through
the GitHub Contents API. The site serves the menus; the bot serves nothing
but a health check.

```
Telegram user ──▶ bot (long polling) ──▶ validate ──▶ rate limit ──▶ GitHub Contents API
                                                                        │
                                             site repo: static/menus/<id>.json ──▶ GitHub Pages
```

## Quick start

You need a bot token from [@BotFather](https://t.me/BotFather) and a
fine-grained GitHub token limited to the site repository with
**Contents: Read and write**.

```bash
cp .env.example .env        # fill in the required values
docker compose up --build --wait
```

Or without Docker (Go 1.26 or newer):

```bash
go run .
```

The bot uses long polling, so it needs outbound HTTPS only: no public URL,
no open ports, no TLS certificate.

## Configuration

Every variable is documented in [.env.example](.env.example).

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `TELEGRAM_BOT_TOKEN` | yes | | Bot token from @BotFather |
| `HASH_SECRET` | yes | | Key for menu ids, at least 32 characters (`openssl rand -hex 32`) |
| `GITHUB_TOKEN` | yes | | Token with Contents read/write on the site repository |
| `GITHUB_REPO` | yes | | Site repository, `owner/name` |
| `HUGO_BASE_URL` | yes | | Public base URL of the site |
| `GITHUB_BRANCH` | no | `main` | Branch to commit to |
| `ALLOWED_USER_IDS` | no | none | Telegram user IDs that may publish |
| `ADMIN_USER_IDS` | no | none | May publish, use admin commands, receive access requests |
| `STORAGE_PATH` | no | `./data` | Local copy of published menus |
| `PORT` | no | `8080` | Port of `/health` |
| `MAX_UPLOAD_BYTES` | no | `262144` | Largest accepted menu (256 KB) |
| `PUBLISH_RATE_LIMIT` | no | `5` | Publishes per user per window |
| `PUBLISH_RATE_WINDOW` | no | `1h` | Rate-limit window |

The bot refuses to start if a required variable is missing, if
`HASH_SECRET` is shorter than 32 characters, or if any value is malformed.

## Using the bot

Send the bot a menu in a private chat, either as a message or as a `.json`
file. Editing a sent message publishes it again.

| Command | Who | What |
|---|---|---|
| `/start`, `/help` | everyone | Usage |
| `/request_access` | everyone | Ask the administrators for access (once per hour) |
| `/template` | publishers | An example menu to copy |
| `/myurl` | publishers | The public URL of your menu |
| `/status`, `/myid` | administrators | Bot status; your Telegram ID |

List format:

```
CATEGORY: Appetizers
DESCRIPTION: Start your meal

- ITEM: Bruschetta
  PRICE: 12.99
  DESCRIPTION: Classic tomato basil
```

JSON format (this is also what gets published):

```json
{
  "categories": [
    {
      "id": "appetizers",
      "name": "Appetizers",
      "description": "Start your meal",
      "items": [
        { "name": "Bruschetta", "price": 12.99, "description": "Classic tomato basil", "image": "/images/bruschetta.jpg" }
      ]
    }
  ]
}
```

Validation is strict: unknown fields are rejected, strings and counts are
capped (50 categories, 200 items each, names 100 characters, descriptions
500), prices must be between 0 and 1,000,000, and `image` must be an
`https://` URL or a site-relative path such as `/images/x.jpg`.

## Security

- **Menu ids** are base62 of the first 12 bytes of
  HMAC-SHA256(`HASH_SECRET`, user ID). They cannot be turned back into a
  Telegram ID without the secret. There is no default secret.
- **Input** is size-capped before parsing and re-encoded before publishing,
  so only validated, canonical JSON reaches the site.
- **Rate limit**: 5 publishes per user per hour by default. The counters
  are in memory and reset when the bot restarts.
- **Logs** never contain tokens (a redacting writer wraps all output) and
  identify users only by their menu id.
- **HTTP surface**: `GET /health` only.
- **Container**: distroless, non-root, read-only root filesystem, all
  capabilities dropped, base images pinned by digest.

See [SECURITY.md](SECURITY.md) to report a vulnerability.

## Development

```bash
go test -race ./...            # tests (no network needed)
gofmt -l . && go vet ./...     # format check and vet
golangci-lint run              # lint, v2 (config in .golangci.yml)
govulncheck ./...              # known vulnerabilities
```

The layout is described in [CLAUDE.md](CLAUDE.md). Commits follow
[Conventional Commits](https://www.conventionalcommits.org/).

## Licence

[MIT](LICENSE)
