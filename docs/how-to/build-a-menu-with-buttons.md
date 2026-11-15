# How to build a menu with buttons

Use this when you want to write or change a menu from your phone, without
editing a file. You need to be a publisher (see
[Commands](../reference/commands.md)).

## Build a new menu

1. Send `/build`, or tap **Build my menu** under `/start`.
2. Name your first section: tap **Starters**, **Mains**, **Desserts** or
   **Drinks**, or type a name of your own.
3. Send the dishes of that section, one per line, the name and then the
   price:

   ```
   Gazpacho 6.50
   Croquetas 8
   ```

   Write them in the message box, as you would write to a person, and
   send. The bot answers with the section so far. Keep sending dishes for
   as long as you like.
4. To describe the dish you just added, tap **Describe last dish** and
   send a sentence.
5. Tap **Back to menu**, then **Add section** for the next section, and
   send its dishes.
6. When the menu is complete, tap **Back to menu** and **Done, preview**.
   Read the menu through and tap **Publish**.
7. Wait for the message `✅ Your menu is online!`, about a minute later.

If a line has no price, or the price is not a number, the bot tells you
which line and how to write it. Nothing from that message is added; send
it again, corrected.

## Change the menu that is online

1. Send `/build`. The bot shows the menu that is online.
2. To add a dish, tap **Add dish**, choose its section, and send it.
3. To change a price, tap **Edit dishes**, choose the section, tap the
   dish, tap **Change price** and send the new price. You are taken back
   to the list, ready for the next dish.
4. To take a dish off, tap it and then **Remove**.
5. To rename or remove a whole section, tap **Edit sections**.
6. Tap **Back to menu**, then **Done, preview**, then **Publish**.

## Stop and continue later

Tap **Cancel**, then **Keep for later**. Your draft waits for 7 days;
`/build` brings it back, also after the bot was restarted. **Discard
draft** throws it away. Neither changes what your customers see.

## Start again from what is online

Tap **Cancel**, then **Discard draft**, and send `/build`.
