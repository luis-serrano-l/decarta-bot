# Test Bot Locally - Quick Guide

Use your computer as the server for testing! This is perfect for development.

## Simple 3-Step Process

### Step 1: Start Your Bot

```bash
cd /home/luis-serrano-l/Documents/Go/demenu_bot
air  # or: go run main.go
```

The bot will run on `http://localhost:8080`

**Note:** For local testing, you can use **polling mode** (default) - no tunnel needed! The bot will check Telegram for messages automatically.

### Step 2: Test JSON Reading (Polling Mode - No Tunnel Needed!)

If you're using polling mode (default), you can test right away:

1. **Send a JSON message to your bot on Telegram**
   - Use `/template` to get the format
   - Copy and send a `menu.json` message

2. **Check if it was saved:**
   ```bash
   ls -la storage/carta_*.json
   ```

3. **Test the endpoint locally:**
   ```bash
   curl http://localhost:8080/api/user/AAAAAFtdDvuOVF1UEAgh4A==/menu.json
   ```

4. **Get your menu URL:**
   - Send `/myurl` to your bot
   - It will show your menu URL

### Step 3: Test with Hugo (Need Tunnel for GitHub Pages)

If you want to test with your Hugo site (especially if it's on GitHub Pages), you need a tunnel:

#### Install ngrok (easiest):
```bash
sudo snap install ngrok
# Or download from: https://ngrok.com/download
```

#### Start tunnel in a NEW terminal:
```bash
ngrok http 8080
```

Copy the HTTPS URL (e.g., `https://abc123.ngrok.io`)

#### Set Telegram webhook (for webhook mode):
```bash
# First, update .env to use webhook mode:
# USE_POLLING=false

# Then set webhook:
curl -X POST "https://api.telegram.org/bot<YOUR_BOT_TOKEN>/setWebhook?url=https://abc123.ngrok.io/webhook"
```

#### Update Hugo config:
```toml
# In demenu/hugo.toml or hugo.production.toml
[params]
  botApiUrl = "https://abc123.ngrok.io"
```

## Quick Test (Polling Mode - Simplest!)

**For just testing JSON reading, you don't need a tunnel!**

1. **Start bot:**
   ```bash
   cd /home/luis-serrano-l/Documents/Go/demenu_bot
   air
   ```

2. **Send JSON to bot on Telegram** (bot will receive it via polling)

3. **Check it was saved:**
   ```bash
   cat storage/carta_*.json
   ```

4. **Test API endpoint:**
   ```bash
   curl http://localhost:8080/api/user/AAAAAFtdDvuOVF1UEAgh4A==/menu.json
   ```

That's it! No tunnel needed for basic testing.

## When Do You Need a Tunnel?

- ✅ **No tunnel needed:** Testing JSON reading, local Hugo site
- ✅ **Tunnel needed:** Testing with GitHub Pages, webhook mode, public access

## Troubleshooting

**Bot not receiving messages:**
- Check bot is running: `curl http://localhost:8080/health`
- Make sure `TELEGRAM_BOT_TOKEN` is set in `.env`
- Check bot logs for errors

**JSON not saving:**
- Check you're an allowed user (set `ALLOWED_USER_IDS` in `.env`)
- Check JSON format matches template (`/template` command)
- Check bot logs for validation errors

**Endpoint not working:**
- Make sure bot is running
- Check the hash is correct (use `/myurl` to get it)
- Check file exists: `ls storage/carta_*.json`

## Next Steps

Once local testing works:
- Deploy to free server (see `FREE_SERVER_SETUP.md`)
- Or keep using tunnel for development



