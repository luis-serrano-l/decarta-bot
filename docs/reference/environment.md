# Reference: environment variables

The bot is configured entirely through environment variables. `go run .`
also reads a `.env` file in the working directory; `compose.yaml` passes
`.env` to the container, and the snap reads
`/var/snap/decarta-bot/common/.env`. [.env.example](../../.env.example) is a template
with every variable.

The bot validates everything at startup and exits with status 2, listing
every problem by variable name (never by value), if anything is missing or
malformed. Changes take effect only after a restart.

## Required

| Variable | Format | Purpose |
|---|---|---|
| `TELEGRAM_BOT_TOKEN` | Token from @BotFather | Authenticates the bot with Telegram. See [how to create it](../how-to/create-botfather-token.md). |
| `HASH_SECRET` | At least 32 characters; published example values are rejected | Key of the HMAC that turns a Telegram user ID into a public menu id. Generate with `openssl rand -hex 32`. **Changing it changes every menu address.** |
| `GITHUB_TOKEN` | Fine-grained personal access token | Commits menus to the site repository. Needs **Contents: Read and write** on that repository only. See [how to create it](../how-to/create-github-token.md). |
| `GITHUB_REPO` | `owner/name` | The site repository that receives `static/menus/<id>.json`. |
| `HUGO_BASE_URL` | Absolute `https://` URL, no credentials, query or fragment; `http://` is accepted for `localhost` only | Public base URL of the site. Menu links are `HUGO_BASE_URL/menu/<id>`. A trailing slash is ignored. |

## Optional

| Variable | Default | Format | Purpose |
|---|---|---|---|
| `GITHUB_BRANCH` | `main` | Branch name | Branch of `GITHUB_REPO` to commit to. |
| `ALLOWED_USER_IDS` | none | Comma-separated positive integers | Telegram user IDs that may publish. A malformed entry is an error, not skipped. |
| `ADMIN_USER_IDS` | none | Comma-separated positive integers | May publish, and receive `/request_access` notifications. If empty, the bot logs a warning and nobody receives access requests. |
| `STORAGE_PATH` | `./data` | Directory | Local copy of each user's last published menu. `compose.yaml` always sets it to `/data`, the named volume, and the snap to `/var/snap/decarta-bot/common/data`. |
| `ENV_FILE` | `.env` | File path | The file of variables to read at startup. A missing file is not an error. A variable already in the environment wins over the file. The snap sets this; you rarely need to. |
| `PORT` | `8080` | 1 to 65535 | Port of `GET /health`. Not published by `compose.yaml`. |
| `MAX_UPLOAD_BYTES` | `262144` (256 KB) | 1 to 20971520 | Largest accepted menu, as a message or a file. The ceiling is Telegram's own 20 MB download limit for bots. |
| `PUBLISH_RATE_LIMIT` | `5` | 1 to 1000 | Publishes allowed per user within `PUBLISH_RATE_WINDOW`. |
| `PUBLISH_RATE_WINDOW` | `1h` | Go duration (`30m`, `1h`, `24h`), 1s to 720h | Length of the sliding rate-limit window. |

With both `ALLOWED_USER_IDS` and `ADMIN_USER_IDS` empty, nobody can
publish. Anyone can still use `/start`, `/help`, `/status`, `/myid` and
`/request_access`.

## Fixed behaviour

These are not configurable:

| Behaviour | Value |
|---|---|
| Update delivery | Long polling; any webhook left on the bot is cleared at startup |
| Time a preview or delete confirmation waits for its button | 15 minutes |
| Menus checked and previewed (uploads) | 20 per user per 10 minutes |
| `/request_access` notifications | 1 per user per hour |
| Problems listed in one validation reply | 5 |
| Timeout of a publish or delete | 45 seconds |
| HTTP endpoints | `GET /health` only |

## The health endpoint

`GET /health` returns `200` with:

```json
{"status":"ok","telegram":"connected"}
```

`telegram` is `"connecting"` until Telegram has accepted the token. The
endpoint reports liveness, so it stays `200` while the bot retries a bad
token: restarting the container would not fix that. The image's
`HEALTHCHECK` calls it through `decarta-bot -healthcheck`.

## Command-line flags

| Flag | Effect |
|---|---|
| `--version` | Prints `decarta-bot <version>` and exits. Release builds carry their version; a plain `go build` prints `dev`. The same value is in the "starting" log line. |
| `-healthcheck` | Requests `/health` on the local `PORT` and exits 0 if it answers `200`. Used by the image's `HEALTHCHECK`. |
