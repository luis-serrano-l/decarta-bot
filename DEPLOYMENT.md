# Deployment Guide

This guide covers deploying the demenu_bot server publicly so the Hugo site can access menu JSON data via the `/api/user/:hash/menu.json` endpoint.

## Security Model

The endpoint is secured through:
- **Hash-based access**: Users can only access JSON files associated with their unique hash
- **HMAC signature validation**: Hash tampering is prevented via cryptographic signatures
- **File path validation**: Directory traversal attacks are prevented
- **IP logging**: All requests are logged with client IP for monitoring and abuse detection

## Quick Deploy Options

### Option 1: Railway (Recommended for Quick Setup)

1. **Sign up:** https://railway.app (GitHub login)
2. **New Project** → Deploy from GitHub → Select `demenu_bot` repo
3. **Add PostgreSQL Database:**
   - In Railway dashboard, click **"+ New"** → **"Database"** → **"Add PostgreSQL"**
   - Railway will automatically create a PostgreSQL service and provide `DATABASE_URL` environment variable
4. **Add Environment Variables:**
   - Go to your bot service → **Variables** tab
   - Add these variables:
   ```
   TELEGRAM_BOT_TOKEN=your_token
   ENVIRONMENT=production
   USE_POLLING=false
   PORT=8080
   STORAGE_TYPE=postgres
   DATABASE_URL=<automatically set by Railway when you add PostgreSQL>
   HUGO_BASE_URL=https://your-username.github.io/demenu
   HASH_SECRET=your-secret-key-here
   ADMIN_USER_IDS=your_telegram_user_id
   ```
   - **Note:** `DATABASE_URL` is automatically set by Railway when you add PostgreSQL service - you don't need to set it manually
5. **Run Database Migration:**
   - Railway will automatically run migrations on startup (table creation is handled in code)
   - Or manually run: Connect to Railway PostgreSQL and execute `storage/migrations/001_create_user_menus.sql`
6. **Deploy** (automatic)
7. **Get URL** from Settings → Networking (e.g., `https://demenu-bot-production.up.railway.app`)
8. **Set Telegram webhook:**
   ```bash
   curl -X POST "https://api.telegram.org/bot<TOKEN>/setWebhook?url=<YOUR_RAILWAY_URL>/webhook"
   ```
9. **Update Hugo config:** Set `botApiUrl` in `hugo.production.toml` to your Railway URL

### Option 2: Fly.io

1. **Install flyctl:** https://fly.io/docs/getting-started/installing-flyctl/
2. **Login:** `fly auth login`
3. **Launch app:** `fly launch` (follow prompts)
4. **Set secrets:**
   ```bash
   fly secrets set TELEGRAM_BOT_TOKEN=your_token
   fly secrets set ENVIRONMENT=production
   fly secrets set USE_POLLING=false
   fly secrets set HUGO_BASE_URL=https://your-username.github.io/demenu
   fly secrets set HASH_SECRET=your-secret-key-here
   fly secrets set ADMIN_USER_IDS=your_telegram_user_id
   ```
5. **Deploy:** `fly deploy`
6. **Get URL:** `fly info` (e.g., `https://demenu-bot.fly.dev`)
7. **Set webhook and update Hugo config** (same as Railway)

### Option 3: Render

1. **Sign up:** https://render.com
2. **New Web Service** → Connect GitHub repo
3. **Configure:**
   - Build Command: `go build -o bin/demenu_bot`
   - Start Command: `./bin/demenu_bot`
   - Environment: `Docker` or `Go`
4. **Add Environment Variables** (same as Railway)
5. **Deploy** (automatic)
6. **Get URL** from dashboard (e.g., `https://demenu-bot.onrender.com`)
7. **Set webhook and update Hugo config**

## VPS Deployment (DigitalOcean, AWS EC2, etc.)

### Prerequisites
- Ubuntu/Debian VPS
- Domain name (optional but recommended)
- SSH access

### Step 1: Install Dependencies

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install Go
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Install nginx (for reverse proxy with SSL)
sudo apt install nginx certbot python3-certbot-nginx -y
```

### Step 2: Deploy the Bot

```bash
# Clone repository
cd /opt
sudo git clone https://github.com/yourusername/demenu_bot.git
cd demenu_bot

# Build
go build -o bin/demenu_bot

# Create systemd service
sudo nano /etc/systemd/system/demenu-bot.service
```

**Service file content:**
```ini
[Unit]
Description=Demenu Bot API Server
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/demenu_bot
Environment="TELEGRAM_BOT_TOKEN=your_token"
Environment="ENVIRONMENT=production"
Environment="USE_POLLING=false"
Environment="PORT=8080"
Environment="STORAGE_PATH=/opt/demenu_bot/storage"
Environment="HUGO_BASE_URL=https://your-username.github.io/demenu"
Environment="HASH_SECRET=your-secret-key-here"
Environment="ADMIN_USER_IDS=your_telegram_user_id"
ExecStart=/opt/demenu_bot/bin/demenu_bot
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

```bash
# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable demenu-bot
sudo systemctl start demenu-bot
sudo systemctl status demenu-bot
```

### Step 3: Configure Nginx Reverse Proxy

```bash
# Create nginx config
sudo nano /etc/nginx/sites-available/demenu-bot
```

**Nginx config:**
```nginx
server {
    listen 80;
    server_name your-domain.com;  # or your VPS IP

    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
    }
}
```

```bash
# Enable site
sudo ln -s /etc/nginx/sites-available/demenu-bot /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

### Step 4: Set Up SSL (Let's Encrypt)

```bash
# Get SSL certificate
sudo certbot --nginx -d your-domain.com

# Auto-renewal is set up automatically
```

### Step 5: Configure Firewall

```bash
# Allow HTTP/HTTPS
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow 22/tcp  # SSH
sudo ufw enable
```

## Environment Variables

| Variable | Required | Description | Example |
|----------|----------|-------------|---------|
| `TELEGRAM_BOT_TOKEN` | ✅ Yes | Bot token from @BotFather | `123456:ABC-DEF...` |
| `ENVIRONMENT` | No | `production` or `development` | `production` |
| `USE_POLLING` | No | `true` for local dev, `false` for webhook | `false` |
| `PORT` | No | Server port (default: `8080`) | `8080` |
| `STORAGE_TYPE` | No | Storage backend: `file` or `postgres` (default: `file`) | `postgres` |
| `STORAGE_PATH` | Conditional | Path to storage directory (only used when `STORAGE_TYPE=file`) | `/app/storage` |
| `DATABASE_URL` | Conditional | PostgreSQL connection URL (required when `STORAGE_TYPE=postgres`, auto-set by Railway) | `postgresql://user:pass@host/db` |
| `HUGO_BASE_URL` | ✅ Yes | Your GitHub Pages URL | `https://user.github.io/demenu` |
| `HASH_SECRET` | Recommended | Secret for hash signing (use strong random string) | `your-secret-key-here` |
| `ADMIN_USER_IDS` | Recommended | Comma-separated Telegram user IDs | `123456789,987654321` |
| `ALLOWED_USER_IDS` | No | Comma-separated allowed user IDs | `123456789` |

## IP Logging

All API requests are automatically logged with:
- Client IP address (handles X-Forwarded-For and X-Real-IP headers)
- Request timestamp
- HTTP method and path
- Hash value (if present)
- HTTP status code
- Response size
- User agent
- Request duration

Logs are output in JSON format to stdout/stderr for easy parsing and monitoring.

Example log entry:
```json
{"timestamp":"2025-12-12T18:00:00Z","ip":"192.168.1.100","method":"GET","path":"/api/user/AAAAAFtdDvuOVF1UEAgh4A==/menu.json","hash":"AAAAAFtdDvuOVF1UEAgh4A==","status_code":200,"response_size":1234,"user_agent":"Mozilla/5.0...","duration_ms":"5ms"}
```

## Security Best Practices

1. **Use HTTPS**: Always use SSL/TLS in production (Let's Encrypt is free)
2. **Set HASH_SECRET**: Use a strong, random secret key for hash signing
3. **Restrict Admin Access**: Only add trusted Telegram user IDs to `ADMIN_USER_IDS`
4. **Monitor Logs**: Regularly check access logs for suspicious activity
5. **Keep Updated**: Regularly update Go and system packages
6. **Firewall**: Only expose necessary ports (80, 443, 22)
7. **Backup Database**: If using PostgreSQL, Railway provides automatic backups. For file storage, regularly backup the `storage/` directory

## Verifying Deployment

```bash
# Health check
curl https://your-bot-url.com/health
# Expected: {"status":"ok"}

# Test menu endpoint (replace with your hash)
curl https://your-bot-url.com/api/user/AAAAAFtdDvuOVF1UEAgh4A==/menu.json
# Expected: JSON menu data or 404 if not found

# Check webhook status
curl "https://api.telegram.org/bot<TOKEN>/getWebhookInfo"
```

## Troubleshooting

### Bot not receiving messages
- Check webhook is set: `curl "https://api.telegram.org/bot<TOKEN>/getWebhookInfo"`
- Verify `USE_POLLING=false` in production
- Check bot service logs: `sudo journalctl -u demenu-bot -f`

### Endpoint returns 404
- Verify user has uploaded menu via Telegram (`/template` command)
- Check hash is correct (use `/myurl` in Telegram bot)
- If using file storage: Verify storage directory exists and has correct permissions
- If using PostgreSQL: Check database connection and verify table exists

### Database connection errors
- Verify `DATABASE_URL` is set correctly (Railway sets this automatically)
- Check PostgreSQL service is running in Railway
- Verify database table was created (check logs for "PostgreSQL storage initialized")
- For local development: Ensure PostgreSQL is running and accessible

### CORS errors in browser
- Verify CORS headers are set (already configured in code)
- Check if request is going to correct endpoint
- Verify `botApiUrl` in Hugo config matches deployed URL

### IP logging shows wrong IP
- If behind reverse proxy, ensure X-Forwarded-For or X-Real-IP headers are set
- Nginx config should include: `proxy_set_header X-Real-IP $remote_addr;`

## Updating Hugo Configuration

After deploying, update `demenu/hugo.production.toml`:

```toml
[params]
  botApiUrl = "https://your-deployed-bot-url.com"
```

Then rebuild and deploy Hugo:
```bash
cd demenu
hugo --config hugo.production.toml
# Deploy public/ directory to GitHub Pages
```

## Local Development with PostgreSQL

### Option 1: Docker PostgreSQL (Recommended)

```bash
# Start PostgreSQL in Docker
docker run -d \
  --name demenu-postgres \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=demenu \
  -p 5432:5432 \
  postgres:15

# Set environment variable
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/demenu?sslmode=disable"
export STORAGE_TYPE=postgres
```

### Option 2: Local PostgreSQL Installation

```bash
# Install PostgreSQL (Ubuntu/Debian)
sudo apt install postgresql postgresql-contrib

# Create database
sudo -u postgres createdb demenu
sudo -u postgres psql -c "ALTER USER postgres PASSWORD 'postgres';"

# Set environment variable
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/demenu?sslmode=disable"
export STORAGE_TYPE=postgres
```

### Option 3: Use File Storage (Fallback)

If PostgreSQL is not available, the bot will use file storage by default:
```bash
export STORAGE_TYPE=file
export STORAGE_PATH=./storage
```

## Monitoring and Maintenance

- **Logs**: Check application logs regularly for errors
- **Storage**: 
  - File storage: Monitor disk usage of `storage/` directory
  - PostgreSQL: Railway provides automatic backups and monitoring
- **Updates**: Keep Go version and dependencies updated
- **Backups**: 
  - File storage: Regularly backup `storage/` directory and `.env` file
  - PostgreSQL: Railway provides automatic backups, or use `pg_dump` for manual backups

