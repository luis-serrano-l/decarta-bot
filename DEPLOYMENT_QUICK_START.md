# Quick Deployment Reference

## 🚀 Fastest Setup: Railway (5 minutes)

1. **Sign up:** https://railway.app (GitHub login)
2. **New Project** → Deploy from GitHub → Select `demenu_bot` repo
3. **Add Variables:**
   ```
   TELEGRAM_BOT_TOKEN=your_token
   ENVIRONMENT=production
   USE_POLLING=false
   STORAGE_PATH=/app/storage
   HUGO_BASE_URL=https://your-username.github.io/demenu
   ```
4. **Deploy** (automatic)
5. **Get URL** from Settings → Networking
6. **Set webhook:**
   ```bash
   curl -X POST "https://api.telegram.org/bot<TOKEN>/setWebhook?url=<YOUR_RAILWAY_URL>/webhook"
   ```
7. **Update Hugo:** Set `botApiUrl` in `hugo.production.toml`

## 📋 Required Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `TELEGRAM_BOT_TOKEN` | ✅ Yes | From @BotFather |
| `ENVIRONMENT` | No | `production` (default: `development`) |
| `USE_POLLING` | No | `false` for webhook mode (default: `true`) |
| `STORAGE_PATH` | No | `/app/storage` (default: `./storage`) |
| `HUGO_BASE_URL` | ✅ Yes | Your GitHub Pages URL |
| `HASH_SECRET` | No | Secret for hashing (recommended) |
| `ALLOWED_USER_IDS` | No | Comma-separated user IDs |
| `ADMIN_USER_IDS` | No | Comma-separated admin IDs |

## 🔍 Verify Deployment

```bash
# Health check
curl https://your-bot-url.com/health

# Test menu endpoint
curl https://your-bot-url.com/api/user/AAAAAFtdDvuOVF1UEAgh4A==/menu.json

# Check webhook
curl "https://api.telegram.org/bot<TOKEN>/getWebhookInfo"
```

## 📚 Full Guide

See [FREE_SERVER_SETUP.md](FREE_SERVER_SETUP.md) for detailed instructions for Railway, Render, and Fly.io.



