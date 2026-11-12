# Reference: commands

The bot works in private chats only. Messages in groups and channels, and
messages from other bots, are ignored.

There are three roles:

| Role | Who | Set by |
|---|---|---|
| Administrator | May publish; receives access requests | `ADMIN_USER_IDS` |
| Publisher | May publish | `ALLOWED_USER_IDS` |
| Everyone else | May ask for access | |

## Commands

The first six are registered with Telegram (`setMyCommands`) each time the
bot starts, so they appear in the menu next to the message box.

| Command | Who | What it does |
|---|---|---|
| `/start` | everyone | Explains the bot in three lines, with the buttons **Get a template** and **How it works**. Tells non-publishers how to ask for access. |
| `/help` | everyone | Lists the commands available to your role. |
| `/template` | publishers | Sends `menu.json`, an example menu that is valid as it is. Edit it and send it back. |
| `/link` | publishers | Your menu's address, with an **Open my menu** button if a menu is published. Aliases: `/myurl`, `/menuurl`. |
| `/status` | everyone | Your access level; for publishers, whether a menu is published, its address, and any preview or delete waiting for confirmation. |
| `/delete` | publishers | Asks for confirmation, then unpublishes your menu. |
| `/myid` | everyone | Your Telegram user ID: the number an operator puts in `ALLOWED_USER_IDS` or `ADMIN_USER_IDS`. |
| `/request_access` | everyone | Sends your ID, username and name to the administrators. Once per hour. |

## Sending a menu

Publishers send a menu in any of three ways:

- a `.json` file,
- JSON pasted into a message,
- the [list format](menu-schema.md#the-list-format) typed into a message.

Editing a message you already sent submits it again. While the bot
downloads and checks the menu the chat shows "typing…".

| The menu is | The bot replies with |
|---|---|
| valid | A summary (each category with its item count, totals, price range) and the buttons **Publish** and **Cancel** |
| invalid | Up to five problems, each with the field path, what is wrong and how to fix it, and the total if there are more |
| larger than `MAX_UPLOAD_BYTES` | The size limit |
| not a menu at all (ordinary chat) | A hint to use `/template` |

## Buttons

| Button | Appears under | Effect |
|---|---|---|
| **Get a template** | `/start` | Same as `/template` |
| **How it works** | `/start` | The steps from template to published page, and the list format |
| **Publish** | A preview | Publishes the previewed menu. The message becomes "Menu published" with the link, a note that the page updates in about a minute, and **Open my menu**. |
| **Cancel** | A preview or a delete question | Discards it. Nothing changes. |
| **Yes, delete it** | `/delete` | Removes the menu from the site. The page disappears in about a minute. |
| **Open my menu** | A published message, `/link`, `/status` | Opens the public page |

Rules that apply to **Publish**, **Yes, delete it** and **Cancel**:

- A button works once, and only for the user it was shown to.
- It expires after 15 minutes, and when the bot restarts.
- Sending a new menu, or `/delete`, replaces whatever was waiting; the
  older buttons then answer "This button has expired or was already used".
- If publishing or deleting fails, the button stays valid so you can tap
  it again.

## Rate limit

Each user may publish `PUBLISH_RATE_LIMIT` times per `PUBLISH_RATE_WINDOW`
(5 per hour by default). Only a tap on **Publish** counts: previews,
rejected menus, cancels and deletes are free.

At the limit the bot says how long to wait ("You can publish again in 12
minutes") and keeps the preview until then, so the same **Publish** button
works once the wait is over.
