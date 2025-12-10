# Local Testing Guide

Test your bot locally by exposing it to the internet using a tunnel. This is perfect for development and testing!

## Quick Setup (5 minutes)

### Step 1: Start Your Bot Locally

```bash
cd /home/luis-serrano-l/Documents/Go/demenu_bot
air  # or: go run main.go
```

Your bot should be running on `http://localhost:8080`

### Step 2: Install a Tunnel Tool (Choose One)

**Option A: ngrok (Recommended)**
```bash
# Install ngrok
sudo snap install ngrok
# Or download from: https://ngrok.com/download
```

**Option B: cloudflared (Cloudflare Tunnel)**
```bash
# Download from: https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/installation/
```

**Option C: localtunnel (Simple)**
```bash
npm install -g localtunnel
```

### Step 3: Start the Tunnel

**Using ngrok:**
```bash
ngrok http 8080
```

**Using cloudflared:**
```bash
cloudflared tunnel --url http://localhost:8080
```

**Using localtunnel:**
```bash
lt --port 8080
```

You'll get a public URL like:
- ngrok: `https://abc123.ngrok.io`
- cloudflared: `https://random-words.trycloudflare.com`
- localtunnel: `https://random-name.loca.lt`

**Copy this URL!** You'll need it in the next steps.

### Step 4: Set Telegram Webhook

Replace `<YOUR_BOT_TOKEN>` and `<TUNNEL_URL>` with your values:

```bash
curl -X POST "https://api.telegram.org/bot<YOUR_BOT_TOKEN>/setWebhook?url=<TUNNEL_URL>/webhook"
```

Example:
```bash
curl -X POST "https://api.telegram.org/bot123456:ABC-DEF/setWebhook?url=https://abc123.ngrok.io/webhook"
```

Verify webhook is set:
```bash
curl "https://api.telegram.org/bot<YOUR_BOT_TOKEN>/getWebhookInfo"
```

### Step 5: Update Hugo Config for Testing

Update `demenu/hugo.toml` (for local testing):

```toml
[params]
  botApiUrl = "https://abc123.ngrok.io"  # Your tunnel URL
```

Or update `demenu/hugo.production.toml` if testing production build:

```toml
[params]
  botApiUrl = "https://abc123.ngrok.io"  # Your tunnel URL
```

### Step 6: Test!

1. **Test bot health:**
   ```bash
   curl https://abc123.ngrok.io/health
   ```
   Should return: `{"status":"ok"}`

2. **Send a JSON message to your bot on Telegram**
   - Use `/template` to get the format
   - Send a `menu.json` or `meals.json` message

3. **Get your menu URL:**
   - Send `/myurl` to your bot
   - It will give you a URL like: `https://your-hugo-site.com/menu/AAAAAFtdDvuOVF1UEAgh4A==`

4. **Test the menu endpoint:**
   ```bash
   curl https://abc123.ngrok.io/api/user/AAAAAFtdDvuOVF1UEAgh4A==/menu.json
   ```

5. **Visit the menu URL in your browser** (if Hugo is running locally)

## Using the Helper Script

You can also use the provided script:

```bash
cd /home/luis-serrano-l/Documents/Go/demenu_bot
./start_tunnel.sh 8080 ngrok
```

## Important Notes

⚠️ **Tunnel URLs change:**
- **ngrok (free):** URL changes every time you restart
- **cloudflared:** URL changes each time
- **localtunnel:** URL changes each time

⚠️ **Keep both running:**
- Keep your bot running (`air` or `go run main.go`)
- Keep the tunnel running in a separate terminal
- If either stops, the bot won't work

⚠️ **For production:** Use a permanent deployment (see `FREE_SERVER_SETUP.md`)

## Troubleshooting

### Bot not responding:
- Check bot is running: `curl http://localhost:8080/health`
- Check tunnel is running (should show active connections)
- Verify webhook: `curl "https://api.telegram.org/bot<TOKEN>/getWebhookInfo"`

### Menu not loading:
- Check tunnel URL is correct in Hugo config
- Test endpoint directly: `curl <TUNNEL_URL>/api/user/<hash>/menu.json`
- Check browser console (F12) for errors

### Tunnel connection issues:
- Make sure port 8080 is not blocked by firewall
- Try a different tunnel service
- Check if your ISP blocks certain ports

## Quick Test Checklist

- [ ] Bot running on localhost:8080
- [ ] Tunnel running and showing public URL
- [ ] Webhook set to tunnel URL
- [ ] Hugo config updated with tunnel URL
- [ ] Sent test JSON to bot via Telegram
- [ ] Tested `/myurl` command
- [ ] Tested menu endpoint with curl
- [ ] Menu loads in browser

## Next Steps

Once testing works locally, you can:
1. Deploy to a free server (see `FREE_SERVER_SETUP.md`)
2. Get a permanent URL
3. Update Hugo config with production URL



