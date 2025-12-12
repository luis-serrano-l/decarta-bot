# Railway Quick Start Checklist

## ✅ Pre-Deployment Checklist

- [x] Code is ready (IP logging, security headers, endpoint implemented)
- [x] Dockerfile exists
- [ ] Code committed and pushed to GitHub
- [ ] Railway account created
- [ ] Environment variables ready

## 🚀 Deployment Steps

### 1. Commit and Push Code
```bash
cd /home/luis-serrano-l/Documents/Go/demenu_bot
git commit -m "Add IP logging middleware and public endpoint support"
git push origin server-fix
```

### 2. Railway Setup
1. Go to https://railway.app
2. Sign up/Login with GitHub
3. Click "New Project" → "Deploy from GitHub repo"
4. Select: `luis-serrano-l/demenu_bot`
5. Select branch: `server-fix`

### 3. Environment Variables (in Railway Dashboard → Variables)

Copy these from your `.env` file:

**Required:**
- `TELEGRAM_BOT_TOKEN` = (from your .env)
- `ENVIRONMENT` = `production`
- `USE_POLLING` = `false`
- `PORT` = `8080`
- `STORAGE_PATH` = `/app/storage`
- `HUGO_BASE_URL` = (from your .env, e.g., `https://luis-serrano-l.github.io/demenu`)

**Recommended:**
- `HASH_SECRET` = (generate with: `openssl rand -hex 32`)
- `ADMIN_USER_IDS` = (from your .env)

### 4. Get Public URL
- Railway → Settings → Networking → Generate Domain
- Copy the URL (e.g., `https://demenu-bot-production.up.railway.app`)

### 5. Set Telegram Webhook
```bash
curl -X POST "https://api.telegram.org/bot<YOUR_BOT_TOKEN>/setWebhook?url=<YOUR_RAILWAY_URL>/webhook"
```

### 6. Test Endpoint
```bash
# Health check
curl https://your-railway-url.up.railway.app/health

# Menu endpoint (replace with your hash)
curl https://your-railway-url.up.railway.app/api/user/AAAAAFtdDvuOVF1UEAgh4A==/menu.json
```

### 7. Update Hugo Config
Edit `demenu/hugo.production.toml`:
```toml
[params]
  botApiUrl = "https://your-railway-url.up.railway.app"
```

Then rebuild Hugo:
```bash
cd /home/luis-serrano-l/Documents/Go/demenu
hugo --config hugo.production.toml
```

## 📝 Notes

- Railway will auto-deploy on every push to your branch
- Check deployment logs in Railway dashboard if issues occur
- Free tier: $5/month credit, auto-sleeps when inactive

