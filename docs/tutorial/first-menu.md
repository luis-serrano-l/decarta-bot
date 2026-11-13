# Tutorial: publish your first menu

In this tutorial you will run your own copy of the bot and use it to put a
restaurant menu on a public web page. It takes about fifteen minutes. At
the end you will have:

- your own menu site on GitHub Pages,
- your own Telegram bot, running in Docker on your machine,
- a published menu at `https://<you>.github.io/decarta-site/menu/<id>`.

You need [Docker](https://docs.docker.com/get-docker/) with Compose, `git`,
a GitHub account and a Telegram account. Replace `<you>` with your GitHub
username throughout.

## 1. Create the site

The bot does not serve web pages. It writes menu files into a second
repository, the site, and GitHub Pages serves them.

1. Open [luis-serrano-l/decarta-site](https://github.com/luis-serrano-l/decarta-site)
   and click **Fork**. Keep the name `decarta-site`.
2. In your fork, open **Settings → Pages**. Under **Build and deployment**,
   set **Source** to **GitHub Actions**.
3. Open the **Actions** tab. GitHub disables workflows on new forks, so
   click the button to enable them.
4. Select **Deploy Hugo site to Pages** and click **Run workflow**.

When the run turns green, open `https://<you>.github.io/decarta-site`. You should
see the demo site.

## 2. Create the Telegram bot

1. In Telegram, open a chat with [@BotFather](https://t.me/BotFather).
2. Send `/newbot`.
3. Send a display name, for example `My Restaurant Menus`.
4. Send a username ending in `bot`, for example `myrestaurant_menu_bot`.

BotFather replies with a token that looks like
`1234567890:AAE...`. Keep that message open; you need the token in step 4.
Treat it like a password: anyone who has it controls the bot.

## 3. Create the GitHub token

The bot needs permission to commit to your site, and to nothing else.

1. Open
   [github.com/settings/personal-access-tokens/new](https://github.com/settings/personal-access-tokens/new).
2. **Token name**: `decarta-bot`. **Expiration**: 90 days is a good start.
3. **Repository access**: **Only select repositories**, then pick
   `<you>/decarta-site`.
4. **Permissions → Repository permissions → Contents**: **Read and write**.
   GitHub adds **Metadata: Read-only** by itself. Leave everything else
   alone.
5. Click **Generate token** and copy it. It starts with `github_pat_`.

## 4. Configure the bot

```bash
git clone https://github.com/luis-serrano-l/decarta-bot.git
cd decarta-bot
cp .env.example .env
openssl rand -hex 32
```

The last command prints 64 random characters. Open `.env` in an editor and
fill in these five lines:

```ini
TELEGRAM_BOT_TOKEN=<the token from BotFather>
HASH_SECRET=<the 64 characters you just generated>
GITHUB_TOKEN=<the token from GitHub>
GITHUB_REPO=<you>/decarta-site
HUGO_BASE_URL=https://<you>.github.io/decarta-site
```

Leave the rest as it is.

## 5. Start the bot

```bash
docker compose up --build --wait
```

The first build takes a minute or two. The command returns when the
container reports healthy. Check that the bot reached Telegram:

```bash
docker compose logs bot
```

You should see a line containing `telegram: authorised` followed by
`telegram: polling for updates`.

## 6. Give yourself access

Nobody may publish yet, on purpose. Open your bot in Telegram (BotFather's
message has a link) and tap **Start**. The bot explains itself and tells
you that you cannot publish yet.

Send `/myid`. The bot replies with your Telegram user ID, a number. Put it
in `.env`:

```ini
ADMIN_USER_IDS=<your number>
```

Then restart the bot so that it reads the new value:

```bash
docker compose up -d --force-recreate --wait
```

Send `/status`. It now says `Access: administrator`.

## 7. Publish a menu

1. Send `/template`. The bot sends you a file, `menu.json`.
2. Send that file back to the bot. On a phone, forward the message to the
   bot's own chat; on a computer, save it and drag it into the chat.
3. The bot replies with what it read:

   ```
   📋 Your menu is valid. Here is what I read:

   • Starters (2 items)
   • Main dishes (2 items)
   • Desserts (1 item)

   Categories: 3
   Items: 5
   Prices: 5.00 to 18.90

   Publish it?
   ```

4. Tap **Publish**. The message changes to `✅ Menu published!` with an
   **Open my menu** button.
5. Wait about a minute, then tap **Open my menu**.

That is your menu, on the public web. In your fork, the **Actions** tab
shows the build the bot triggered, and `static/menus/` contains the file
it committed.

## 8. Make it yours

Open `menu.json` in any text editor, change a dish name and a price, and
send it again. The bot shows a new preview and warns that it replaces the
menu that is online. Tap **Publish**.

Now make a mistake on purpose: delete a `"price"` line and send the file.
Instead of a preview you get the exact place and the fix:

```
❌ Your menu has 1 problem:

1. categories[0].items[0].price
   missing price
   Fix: add a number, for example "price": 9.5
```

Nothing was published; the page still shows your last good menu.

## Where to go next

- Let someone else publish: they send `/request_access`, you receive their
  ID, and you add it to `ALLOWED_USER_IDS`. See
  [Environment variables](../reference/environment.md).
- Everything a menu may contain: [Menu JSON schema](../reference/menu-schema.md).
- Every command and button: [Commands](../reference/commands.md).
- Keep the bot running on a server: [Run with Docker](../how-to/run-with-docker.md).
- Why it works this way: [Design](../explanation/design.md).
