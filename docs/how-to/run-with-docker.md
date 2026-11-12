# How to run the bot with Docker

This guide assumes you have a bot token
([how](create-botfather-token.md)), a GitHub token
([how](create-github-token.md)) and a site repository with GitHub Pages
enabled. If you have none of these yet, follow the
[tutorial](../tutorial/first-menu.md) instead.

## Start

```bash
cp .env.example .env     # then fill in the required values
docker compose up --build --wait
```

`--wait` returns once the container's health check passes. The bot only
makes outbound HTTPS connections (to `api.telegram.org` and
`api.github.com`), so no port is published and no reverse proxy, domain or
certificate is needed.

## Check that it works

```bash
docker compose ps          # STATUS should say "healthy"
docker compose logs bot    # look for "telegram: polling for updates"
```

Then send `/status` to the bot in Telegram.

## Change the configuration

Edit `.env`, then recreate the container:

```bash
docker compose up -d --force-recreate --wait
```

A plain `docker compose restart` does not re-read `.env`.

## Grant someone access

1. They send `/request_access` to the bot (or `/myid`, and tell you the
   number).
2. Add the ID to `ALLOWED_USER_IDS` in `.env`, comma-separated.
3. Recreate the container as above.

## Update

```bash
git pull
docker compose up --build -d --wait
```

Read [CHANGELOG.md](../../CHANGELOG.md) first: entries marked as breaking
tell you what to change in `.env`.

## Stop

```bash
docker compose down        # keeps the data volume
```

Published menus stay online: they live in the site repository, not in the
bot.

## Back up

There is little to back up. The site repository holds every published
menu. The `demenu-data` volume holds a copy of each user's last published
menu, which the bot uses to answer `/link` and `/status`. The one thing
you must keep safe is `HASH_SECRET`: with a different secret every user
gets a different menu address.

To copy the volume out:

```bash
docker run --rm -v demenu_bot_demenu-data:/data:ro -v "$PWD":/out busybox \
  tar czf /out/demenu-data.tgz -C /data .
```

## Run without Docker

With Go 1.26 or newer:

```bash
go run .
```

It reads the same `.env` file.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| `configuration error:` followed by a list, and the container exits | A required variable is missing or malformed. The message names each variable; fix `.env` and start again. |
| Logs repeat `telegram: cannot authenticate` | `TELEGRAM_BOT_TOKEN` is wrong or revoked, or the machine cannot reach `api.telegram.org`. The bot keeps retrying; fix the token and recreate the container. |
| The bot does not answer at all | Another process is polling with the same token (only one may). Stop the other copy. |
| The bot answers "You are not authorised to publish menus" | Your ID is not in `ALLOWED_USER_IDS` or `ADMIN_USER_IDS`, or the container was not recreated after editing `.env`. |
| "Your menu is valid but could not be published right now" | The GitHub commit failed. `docker compose logs bot` shows the HTTP status: 401 means a bad or expired `GITHUB_TOKEN`; 403 or 404 means the token lacks **Contents: Read and write** on `GITHUB_REPO`, or the repository or `GITHUB_BRANCH` is wrong. |
| Published, but the page shows 404 | The site takes about a minute to rebuild. If it never appears, check the **Actions** tab of the site repository and that Pages uses **GitHub Actions** as its source. |
| "This button has expired or was already used" | Previews wait 15 minutes and do not survive a restart of the bot. Send the menu again. |
