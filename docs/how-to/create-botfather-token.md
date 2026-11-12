# How to create the BotFather token

`TELEGRAM_BOT_TOKEN` identifies your bot to Telegram. You get it from
[@BotFather](https://t.me/BotFather), Telegram's own bot for managing
bots.

## Create the bot

1. Open a chat with [@BotFather](https://t.me/BotFather). Check the blue
   verification mark: impostors exist.
2. Send `/newbot`.
3. Send a display name. It can contain spaces and can be changed later.
4. Send a username. It must be unique, 5 to 32 characters, and end in
   `bot` (for example `myrestaurant_menu_bot`). It cannot be changed later.
5. BotFather replies with the token, in the form
   `1234567890:AAExampleExampleExampleExampleExamp`.

Put it in `.env`:

```ini
TELEGRAM_BOT_TOKEN=1234567890:AAExampleExampleExampleExampleExamp
```

## Optional polish

All in the BotFather chat, after choosing your bot:

- `/setdescription`: the text shown before a user taps **Start**.
- `/setuserpic`: the bot's picture.
- `/setjoingroups` → **Disable**: the bot only works in private chats, so
  there is no reason to let it be added to groups.

You do not need `/setcommands`: the bot registers its own command menu
every time it starts.

## Keep it safe

Anyone holding the token can read the bot's messages and send messages as
the bot.

- Never commit `.env` (it is in `.gitignore`) and never paste the token
  into an issue or a chat.
- The bot never logs the token: every log line passes through a redactor.

## Rotate it

If the token may have leaked:

1. In BotFather, send `/revoke` and choose the bot. The old token stops
   working immediately and you get a new one.
2. Replace `TELEGRAM_BOT_TOKEN` in `.env`.
3. `docker compose up -d --force-recreate --wait`

Menu addresses do not change: they depend on `HASH_SECRET`, not on the bot
token.
