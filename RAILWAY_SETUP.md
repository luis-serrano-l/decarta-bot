# Railway Deployment Setup Guide

Follow these steps to deploy your demenu_bot to Railway.

## Step 1: Commit and Push Your Changes

```bash
cd /home/luis-serrano-l/Documents/Go/demenu_bot
git add middleware/ DEPLOYMENT.md handlers/bot.go main.go
git commit -m "Add IP logging middleware and public endpoint support"
git push origin server-fix
```

## Step 2: Sign Up / Login to Railway

1. Go to https://railway.app
2. Click "Start a New Project" or "Login"
3. Sign up with GitHub (recommended - easiest way to connect your repo)

## Step 3: Create New Project

1. Click **"New Project"**
2. Select **"Deploy from GitHub repo"**
3. Authorize Railway to access your GitHub if prompted
4. Select your repository: `luis-serrano-l/demenu_bot`
5. Select the branch: `server-fix` (or `main` if you merge first)

## Step 4: Configure Environment Variables

Once the project is created:

1. Click on your project
2. Go to the **"Variables"** tab
3. Click **"New Variable"** and add each of these:

### Required Variables:

```
TELEGRAM_BOT_TOKEN=your_telegram_bot_token_here
ENVIRONMENT=production
USE_POLLING=false
PORT=8080
STORAGE_PATH=/app/storage
HUGO_BASE_URL=https://luis-serrano-l.github.io/demenu
```

### Recommended Variables:

```
HASH_SECRET=your-secret-key-here
ADMIN_USER_IDS=your_telegram_user_id
```

**To get your Telegram User ID:**
- Send `/myid` to your bot (if you're an admin)
- Or use: https://t.me/userinfobot

**To generate a HASH_SECRET:**
```bash
openssl rand -hex 32
```

## Step 5: Wait for Deployment

Railway will automatically:
1. Detect the Dockerfile
2. Build the Docker image
3. Deploy the application

Watch the deployment logs in the Railway dashboard. It should complete in 2-3 minutes.

## Step 6: Get Your Public URL

1. Go to **Settings** → **Networking**
2. Click **"Generate Domain"** (if not already generated)
3. Copy the public URL (e.g., `https://demenu-bot-production.up.railway.app`)

## Step 7: Set Telegram Webhook

Replace `<YOUR_BOT_TOKEN>` and `<YOUR_RAILWAY_URL>`:

```bash
curl -X POST "https://api.telegram.org/bot<YOUR_BOT_TOKEN>/setWebhook?url=<YOUR_RAILWAY_URL>/webhook"
```

Example:
```bash
curl -X POST "https://api.telegram.org/bot123456:ABC-DEF/setWebhook?url=https://demenu-bot-production.up.railway.app/webhook"
```

Verify webhook is set:
```bash
curl "https://api.telegram.org/bot<YOUR_BOT_TOKEN>/getWebhookInfo"
```

## Step 8: Test Your Endpoint

Test the health endpoint:
```bash
curl https://your-railway-url.up.railway.app/health
```

Should return: `{"status":"ok"}`

Test the menu endpoint (replace with your hash):
```bash
curl https://your-railway-url.up.railway.app/api/user/AAAAAFtdDvuOVF1UEAgh4A==/menu.json
```

## Step 9: Update Hugo Configuration

Update `demenu/hugo.production.toml`:

```toml
[params]
  botApiUrl = "https://your-railway-url.up.railway.app"
```

Then rebuild and deploy your Hugo site:
```bash
cd /home/luis-serrano-l/Documents/Go/demenu
hugo --config hugo.production.toml
# Commit and push to GitHub Pages
```

## Troubleshooting

### Deployment fails
- Check the deployment logs in Railway dashboard
- Verify all required environment variables are set
- Make sure Dockerfile is in the root directory

### Webhook not working
- Verify `USE_POLLING=false` in environment variables
- Check that the webhook URL is correct
- Verify your bot token is correct

### Endpoint returns 404
- Make sure you've uploaded menu data via Telegram (`/template` command)
- Verify the hash is correct (use `/myurl` in Telegram bot)
- Check Railway logs for errors

### CORS errors
- CORS is already configured in the code
- Make sure you're using HTTPS (Railway provides this automatically)
- Verify `botApiUrl` in Hugo config matches your Railway URL exactly

## Railway Free Tier

- $5/month credit (usually enough for small bots)
- 500 hours of usage
- Auto-sleeps after inactivity (wakes on request)
- Perfect for development and small projects

## Next Steps

Once deployed, your endpoint will be publicly accessible at:
`https://your-railway-url.up.railway.app/api/user/:hash/menu.json`

All requests will be logged with IP addresses for monitoring.

