# How to smoke-test a release

The automated tests fake Telegram and GitHub. This checklist is the manual
run against the real ones. Do it before tagging a release, and after moving
the bot to a new machine. It takes about ten minutes.

It publishes to a scratch branch, so the public site never changes.

## Prepare

1. Create the scratch branch in the site repository, from `main`:

   ```bash
   gh api repos/<owner>/decarta-site/git/refs \
     -f ref=refs/heads/smoke-test \
     -f sha="$(gh api repos/<owner>/decarta-site/git/ref/heads/main --jq .object.sha)"
   ```

   The bot commits to an existing branch; it does not create one.

2. In `.env`, set `GITHUB_BRANCH=smoke-test`. Your own Telegram ID must be
   in `ADMIN_USER_IDS`.

3. Start the bot:

   ```bash
   docker compose up --build -d --wait
   docker compose ps          # STATUS should say "healthy"
   docker compose logs bot    # look for "telegram: polling for updates"
   ```

## Checklist

Send each message to the bot in a private chat.

| # | Do this | Expect |
|---|---|---|
| 1 | Open the bot and look at the command menu next to the message box | Six commands, starting with `/start` |
| 2 | `/start` | Three lines of explanation with **Get a template** and **How it works** |
| 3 | `/status` | Your access level; no menu published |
| 4 | `/template` | A `menu.json` file |
| 5 | Send that `menu.json` back unchanged | A summary with **Publish** and **Cancel** |
| 6 | Tap **Cancel** | Discarded; no new commit on the scratch branch |
| 7 | Send `menu.json` again and tap **Publish** | "Menu published" with a link and **Open my menu** |
| 8 | Open the scratch branch on GitHub | A commit `menu: update <id>` adding `static/menus/<id>.json` |
| 9 | Tap **Publish** on the same message again | "This button has expired or was already used" |
| 10 | Send the text `hello` | A hint to use `/template` |
| 11 | Send `{"name": ""}` | A list of problems, each with a field path and a fix |
| 12 | `/link` | The same address as in step 7 |
| 13 | `/delete`, then **Yes, delete it** | Confirmation; a commit `menu: delete <id>` on the scratch branch |
| 14 | `/status` | No menu published |
| 15 | From a second Telegram account, if you have one: `/start`, then `/request_access` | It is told how to ask for access; you receive the request with its ID |

The link from step 7 answers 404 during this test: the site is built from
`main` only, and the menu is on the scratch branch. That is expected.

Then check the logs for anything that should not be there:

```bash
docker compose logs bot | grep -E 'level=(WARN|ERROR)'
docker compose logs bot | grep -c "$(grep '^TELEGRAM_BOT_TOKEN=' .env | cut -d= -f2)"
```

The first should print nothing you cannot explain. The second must print
`0`: the token never appears in the logs.

If a step fails, see the troubleshooting table in
[Run with Docker](run-with-docker.md#troubleshooting).

## Finish

1. Set `GITHUB_BRANCH=main` in `.env` and recreate the container:

   ```bash
   docker compose up -d --force-recreate --wait
   ```

2. Delete the scratch branch:

   ```bash
   gh api -X DELETE repos/<owner>/decarta-site/git/refs/heads/smoke-test
   ```

3. Publish one real menu and open its link after about a minute. This is
   the only step that proves the site rebuild.
