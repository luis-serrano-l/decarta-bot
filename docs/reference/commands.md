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

The first eight are registered with Telegram (`setMyCommands`) each time the
bot starts, so they appear in the menu next to the message box.

| Command | Who | What it does |
|---|---|---|
| `/start` | everyone | Explains the bot in three lines, with the buttons **Build my menu**, **Get a template** and **How it works**. Tells non-publishers how to ask for access. |
| `/help` | everyone | Lists the commands available to your role. |
| `/build` | publishers | Opens the [menu builder](#building-a-menu-with-buttons): add and change sections and dishes with buttons. |
| `/template` | publishers | Sends `menu.json`, an example menu that is valid as it is. Edit it and send it back. |
| `/link` | publishers | Your menu's address, with an **Open my menu** button if a menu is published. Aliases: `/myurl`, `/menuurl`. |
| `/qr` | publishers | A QR code of your menu's address as an image, to print or show on a screen. Needs a published menu; before that it only tells you the address. |
| `/status` | everyone | Your access level; for publishers, whether a menu is published, its address, and any preview or delete waiting for confirmation. |
| `/delete` | publishers | Asks for confirmation, then unpublishes your menu. |
| `/myid` | everyone | Your Telegram user ID: the number an operator puts in `ALLOWED_USER_IDS` or `ADMIN_USER_IDS`. |
| `/request_access` | everyone | Sends your ID, username and name to the administrators. Once per hour. |

## Sending a menu

Publishers either [build the menu with buttons](#building-a-menu-with-buttons)
or send a whole menu in any of three ways:

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

## Building a menu with buttons

`/build` opens a draft of your menu in the chat. It starts from the draft
you were working on, or else from the menu you have online, or else empty.
Nothing changes online until you tap **Done, preview** and then
**Publish**.

The menu message lists every section with its dish count, and the dishes
of the section you are adding to.

| Button | Effect |
|---|---|
| **Add dish** | The bot waits for dishes for the current section. Send one per line, the name then the price: `Gazpacho 6.50`. Several lines add several dishes, and you can keep sending more. |
| **Describe last dish** | Asks for a description of the dish you just added. |
| **Add section** | Asks for the section's name; tap a suggestion (Starters, Mains, Desserts, Drinks) or type your own. The new section becomes the current one. |
| **Change section** | Choose which section dishes are added to. |
| **Edit / remove** | Choose a section, then a dish (eight per page): **Change price**, **Rename**, **Description**, **Remove**. For the section itself: **Rename section**, **Remove section** (asks first). |
| **Done, preview** | Checks the menu and shows the usual preview with **Publish** and **Cancel**, plus **Keep editing** to go back. |
| **Cancel** | **Keep for later** or **Discard draft**. Neither changes what is online. |

What you type:

- A price may use a dot or a comma and a currency sign: `6.50`, `6,50`,
  `6.50 €`. Three digits after a mark are thousands: `12.000` is twelve
  thousand.
- Punctuation between the name and the price is dropped, so
  `Flan: 4` and `Flan ..... 4` both work.
- If a line is wrong the bot names the line and the fix, adds nothing from
  that message, and keeps waiting for the answer.
- A whole menu (a `.json` file, pasted JSON or the list format) sent while
  the builder is waiting is treated as a menu and previewed, as always.
- Editing a message you already sent does not change the draft.
- Text sent when the builder is not waiting for an answer is handled as
  before the builder existed.

About the draft:

- There is one draft per user. It is saved after every change and
  survives a restart of the bot.
- It is kept for 7 days after the last change.
- Publishing a menu, by any route, and `/delete` remove it; the next
  `/build` starts from what is online.
- A button under an older builder message does nothing except show the
  menu as it is now.

## Buttons

The builder's own buttons are listed [above](#building-a-menu-with-buttons).

| Button | Appears under | Effect |
|---|---|---|
| **Build my menu** | `/start` | Same as `/build` |
| **Get a template** | `/start` | Same as `/template` |
| **How it works** | `/start` | The steps from template to published page, and the list format |
| **Publish** | A preview | Publishes the previewed menu. The message becomes "Menu published" with the link, a note that the page updates in about a minute, and the buttons **Open my menu** and **Get QR code**. |
| **Cancel** | A preview or a delete question | Discards it. Nothing changes. A draft in the builder is kept. |
| **Yes, delete it** | `/delete` | Removes the menu from the site. The page disappears in about a minute. |
| **Open my menu** | A published message, `/link`, `/status` | Opens the public page |
| **Get QR code** | A published message | Same as `/qr` |

Rules that apply to **Publish**, **Yes, delete it** and **Cancel**:

- A button works once, and only for the user it was shown to.
- It expires after 15 minutes, and when the bot restarts.
- Sending a new menu, or `/delete`, replaces whatever was waiting; the
  older buttons then answer "This button has expired or was already used".
- If publishing or deleting fails, the button stays valid so you can tap
  it again. Taps made while one is still in progress are ignored.

## Rate limit

Each user may publish `PUBLISH_RATE_LIMIT` times per `PUBLISH_RATE_WINDOW`
(5 per hour by default). Only a tap on **Publish** counts towards it:
previews, rejected menus, cancels and deletes do not, and neither does a
publish that fails.

Uploads have their own, looser limit: 20 menus checked per user per 10
minutes, valid or not. **Done, preview** in the builder counts as one;
adding and changing dishes does not. It is not configurable. Past it the bot says when
you can send another; a preview already on screen can still be published.

At the publish limit the bot says how long to wait ("You can publish again in 12
minutes") and keeps the preview until then, so the same **Publish** button
works once the wait is over.
