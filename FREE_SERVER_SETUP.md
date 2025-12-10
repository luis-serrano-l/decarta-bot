# Free Server Setup Guide

This guide will help you deploy your `demenu_bot` to a free hosting service. Choose one of the options below.

## Prerequisites

1. **GitHub account** (for connecting your repository)
2. **Telegram Bot Token** (from [@BotFather](https://t.me/botfather))
3. **Your bot repository** pushed to GitHub

## Option 1: Railway (Recommended - Easiest)

Railway offers a generous free tier with $5/month credit.

### Steps:

1. **Sign up at Railway:**
   - Go to https://railway.app
   - Sign up with GitHub

2. **Create a new project:**
   - Click "New Project"
   - Select "Deploy from GitHub repo"
   - Choose your `demenu_bot` repository

3. **Configure environment variables:**
   - Go to your project → Variables tab
   - Add these variables:
     ```
     TELEGRAM_BOT_TOKEN=your_telegram_bot_token
     ENVIRONMENT=production
     USE_POLLING=false
     STORAGE_PATH=/app/storage
     HUGO_BASE_URL=https://your-username.github.io/demenu
     HASH_SECRET=your_secret_key_here (optional, but recommended)
     ALLOWED_USER_IDS=123456789,987654321 (optional, comma-separated)
     ADMIN_USER_IDS=123456789 (optional, comma-separated)
     ```

4. **Deploy:**
   - Railway will automatically detect the Dockerfile and deploy
   - Wait for deployment to complete

5. **Get your bot URL:**
   - Go to Settings → Networking
   - Railway will provide a public URL like: `https://demenu-bot-production.up.railway.app`
   - Copy this URL

6. **Set up Telegram webhook:**
   ```bash
   curl -X POST "https://api.telegram.org/bot<YOUR_BOT_TOKEN>/setWebhook?url=https://demenu-bot-production.up.railway.app/webhook"
   ```

7. **Update Hugo config:**
   ```toml
   # In demenu/hugo.production.toml
   [params]
     botApiUrl = "https://demenu-bot-production.up.railway.app"
   ```

**Railway Free Tier:**
- $5/month credit (usually enough for small bots)
- 500 hours of usage
- Auto-sleeps after inactivity (wakes on request)

---

## Option 2: Render (Free Tier Available)

Render offers a free tier with some limitations.

### Steps:

1. **Sign up at Render:**
   - Go to https://render.com
   - Sign up with GitHub

2. **Create a new Web Service:**
   - Click "New +" → "Web Service"
   - Connect your GitHub repository
   - Select your `demenu_bot` repository

3. **Configure the service:**
   - **Name:** `demenu-bot`
   - **Environment:** `Docker`
   - **Region:** Choose closest to you
   - **Branch:** `main` (or your default branch)
   - **Root Directory:** Leave empty (or `demenu_bot` if repo is in subdirectory)
   - **Dockerfile Path:** `Dockerfile` (or `demenu_bot/Dockerfile` if in subdirectory)

4. **Add environment variables:**
   - Scroll to "Environment Variables" section
   - Add the same variables as Railway (see above)

5. **Deploy:**
   - Click "Create Web Service"
   - Wait for deployment (first deploy takes ~5-10 minutes)

6. **Get your bot URL:**
   - Render provides a URL like: `https://demenu-bot.onrender.com`
   - Copy this URL

7. **Set up Telegram webhook:**
   ```bash
   curl -X POST "https://api.telegram.org/bot<YOUR_BOT_TOKEN>/setWebhook?url=https://demenu-bot.onrender.com/webhook"
   ```

8. **Update Hugo config:**
   ```toml
   # In demenu/hugo.production.toml
   [params]
     botApiUrl = "https://demenu-bot.onrender.com"
   ```

**Render Free Tier:**
- Free tier available
- Spins down after 15 minutes of inactivity (takes ~30 seconds to wake up)
- 750 hours/month free

---

## Option 3: Fly.io (Free Tier Available)

Fly.io offers a generous free tier with 3 shared-cpu VMs.

### Steps:

1. **Install Fly CLI:**
   ```bash
   curl -L https://fly.io/install.sh | sh
   # Or on macOS: brew install flyctl
   ```

2. **Sign up:**
   ```bash
   fly auth signup
   # Or: fly auth login
   ```

3. **Initialize your app:**
   ```bash
   cd /home/luis-serrano-l/Documents/Go/demenu_bot
   fly launch
   ```
   - When prompted, don't deploy yet (we'll set secrets first)

4. **Set secrets (environment variables):**
   ```bash
   fly secrets set TELEGRAM_BOT_TOKEN=your_telegram_bot_token
   fly secrets set ENVIRONMENT=production
   fly secrets set USE_POLLING=false
   fly secrets set STORAGE_PATH=/app/storage
   fly secrets set HUGO_BASE_URL=https://your-username.github.io/demenu
   fly secrets set HASH_SECRET=your_secret_key_here
   # Optional:
   fly secrets set ALLOWED_USER_IDS=123456789,987654321
   fly secrets set ADMIN_USER_IDS=123456789
   ```

5. **Deploy:**
   ```bash
   fly deploy
   ```

6. **Get your bot URL:**
   - Your app will be available at: `https://demenu-bot.fly.dev`
   - Or check with: `fly status`

7. **Set up Telegram webhook:**
   ```bash
   curl -X POST "https://api.telegram.org/bot<YOUR_BOT_TOKEN>/setWebhook?url=https://demenu-bot.fly.dev/webhook"
   ```

8. **Update Hugo config:**
   ```toml
   # In demenu/hugo.production.toml
   [params]
     botApiUrl = "https://demenu-bot.fly.dev"
   ```

**Fly.io Free Tier:**
- 3 shared-cpu VMs with 256MB RAM each
- 160GB outbound data transfer
- Apps sleep after inactivity but wake quickly

---

## Post-Deployment Steps

### 1. Verify Bot is Running

Test the health endpoint:
```bash
curl https://your-bot-url.com/health
```

Should return: `{"status":"ok"}`

### 2. Test Menu Endpoint

Test with your hash:
```bash
curl https://your-bot-url.com/api/user/AAAAAFtdDvuOVF1UEAgh4A==/menu.json
```

### 3. Set Telegram Webhook

Replace `<YOUR_BOT_TOKEN>` and `<YOUR_BOT_URL>`:
```bash
curl -X POST "https://api.telegram.org/bot<YOUR_BOT_TOKEN>/setWebhook?url=<YOUR_BOT_URL>/webhook"
```

Verify webhook is set:
```bash
curl "https://api.telegram.org/bot<YOUR_BOT_TOKEN>/getWebhookInfo"
```

### 4. Update Hugo Configuration

Update `demenu/hugo.production.toml`:
```toml
[params]
  botApiUrl = "https://your-bot-url.com"
```

Then rebuild and deploy your Hugo site:
```bash
cd /home/luis-serrano-l/Documents/Go/demenu
hugo --config hugo.production.toml
# Commit and push to GitHub
```

### 5. Test End-to-End

1. Send a JSON message to your bot on Telegram
2. Get your menu URL using `/myurl` command
3. Visit the URL in your browser
4. Verify the menu loads correctly

---

## Troubleshooting

### Bot not responding to messages:
- Check webhook is set correctly: `curl "https://api.telegram.org/bot<TOKEN>/getWebhookInfo"`
- Check bot logs in your hosting platform
- Verify `TELEGRAM_BOT_TOKEN` is set correctly

### Menu not loading on Hugo site:
- Check browser console (F12) for errors
- Verify `botApiUrl` in Hugo config matches your bot URL
- Test the endpoint directly: `curl https://your-bot-url.com/api/user/<hash>/menu.json`
- Check CORS headers (should be enabled in `main.go`)

### Storage issues:
- Free tiers may have ephemeral storage (data lost on restart)
- Consider using a database or persistent volume for production
- For now, files are stored in `/app/storage` which may be ephemeral

### App sleeping (Render):
- First request after sleep takes ~30 seconds
- Consider upgrading to paid tier for always-on
- Or use Railway/Fly.io which wake faster

---

## Comparison of Free Tiers

| Feature | Railway | Render | Fly.io |
|---------|---------|--------|--------|
| **Free Credit** | $5/month | Free tier | Free tier |
| **Sleep Behavior** | Auto-sleep | 15min timeout | Auto-sleep |
| **Wake Time** | ~5 seconds | ~30 seconds | ~5 seconds |
| **Storage** | Ephemeral | Ephemeral | Ephemeral |
| **Ease of Setup** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ |
| **Best For** | Quick setup | Simple needs | More control |

---

## Recommendation

**For beginners:** Use **Railway** - it's the easiest to set up and has good free tier.

**For stability:** Use **Fly.io** - faster wake times and more reliable.

**For simplicity:** Use **Render** - straightforward setup, but slower wake times.

---

## Next Steps

After deployment:
1. ✅ Test your bot with Telegram
2. ✅ Verify menu endpoint works
3. ✅ Update Hugo config with bot URL
4. ✅ Test end-to-end flow
5. ✅ Monitor logs for any issues

For persistent storage (recommended for production), consider:
- Adding a database (PostgreSQL, MongoDB)
- Using cloud storage (AWS S3, Google Cloud Storage)
- Using Railway/Render persistent volumes (paid feature)



