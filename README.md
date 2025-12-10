# Demenu Telegram Bot

A Telegram bot built with Go and Gin framework that allows users to send JSON files (meals.json and menu.json) via personal chat. These files will be processed and stored for use by the Hugo app at `/Go/demenu` that is published on GitHub Pages.

> 📝 See [CHANGELOG.md](CHANGELOG.md) for detailed information about how the bot works and recent changes.

## Project Plan

### Phase 1: Basic Bot Setup ✅
- [x] Initialize Go module with Gin framework
- [x] Create project structure
- [x] Set up Telegram bot integration
- [x] Implement webhook handler
- [x] Add JSON file validation
- [x] Implement file storage

### Phase 2: Telegram Bot Features ✅
- [x] Register bot with Telegram Bot API
- [x] Implement webhook endpoint for receiving updates
- [x] Handle document messages (JSON files)
- [x] Validate JSON structure (meals.json and menu.json schemas)
- [x] Store validated JSON files
- [x] Send confirmation messages to users
- [x] Add user authorization (only allowed users can send files)
- [x] Error handling and user feedback
- [x] Add command handlers (/start, /help, /status)

### Phase 3: Integration with Hugo App
- [x] Create API endpoint to serve JSON files to Hugo app
- [x] Update Hugo app to fetch JSON from bot API instead of local files
- [x] Set up hash-based routing for user-specific menus
- [x] Deploy bot to production environment (see [FREE_SERVER_SETUP.md](FREE_SERVER_SETUP.md))

### Phase 4: Additional Features (Future)
- [ ] Implement JSON preview before saving
- [ ] Add versioning/history of JSON files
- [ ] Add rollback functionality
- [ ] Implement file diff/comparison
- [ ] Add notifications for successful updates

## Project Structure

```
demenu_bot/
├── main.go              # Application entry point
├── config/
│   └── config.go        # Configuration management
├── handlers/
│   └── bot.go           # Telegram bot handlers
├── services/
│   └── bot.go           # Telegram bot service
├── storage/
│   └── storage.go       # JSON file storage and validation
├── go.mod               # Go module dependencies
├── go.sum               # Go module checksums
└── README.md            # This file
```

## How It Works

**Development:** Uses polling mode by default (polls Telegram API every 60s). No webhook needed for local development.

**Endpoints:**
- `POST /webhook` - Receives Telegram updates (production only)
- `GET /api/meals.json` - Serves meals data for Hugo app
- `GET /api/menu.json` - Serves menu data for Hugo app
- `GET /health` - Health check

**Storage:** File-based storage in `./storage/` directory. JSON files are validated against schema before saving (required fields: name, price, description). Only authorized users can upload files.

## Configuration

The bot uses a `.env` file for configuration. Create one from `.env.example`:

- `TELEGRAM_BOT_TOKEN`: Your Telegram bot token (obtained from @BotFather) - **Required**
- `ENVIRONMENT`: Environment mode (development/production) - Default: development
- `STORAGE_PATH`: Path where JSON files will be stored - Default: ./storage
- `ALLOWED_USER_IDS`: Comma-separated list of Telegram user IDs allowed to send files (required - if empty, all users are denied)
- `ADMIN_USER_IDS`: Comma-separated list of Telegram user IDs with admin privileges (can use `/myid` and `/status` commands)
- `USE_POLLING`: Use polling instead of webhook (true/false) - Default: true (for local development)
- `PORT`: Server port - Default: 8080

**Note:** In polling mode (default), the bot actively checks for new messages. This is perfect for local development - no need to expose your computer to the internet or set up webhooks!

## JSON File Formats

The bot accepts two types of JSON files:

### meals.json
```json
{
  "meals": [
    {
      "name": "Meal Name",
      "price": 24.99,
      "description": "Meal description",
      "dietary": "Optional dietary info",
      "image": "/images/meal.jpg"
    }
  ]
}
```

### menu.json
```json
{
  "categories": [
    {
      "id": "category-id",
      "name": "Category Name",
      "description": "Category description",
      "items": [
        {
          "name": "Item Name",
          "description": "Item description",
          "price": 12.99
        }
      ]
    }
  ]
}
```

## Getting Started

1. **Create a Telegram Bot**
   - Message @BotFather on Telegram
   - Use `/newbot` command
   - Follow instructions to get your bot token

2. **Set up your `.env` file**
   - Copy `.env.example` to `.env` (or edit the existing `.env`)
   - Add your bot token: `TELEGRAM_BOT_TOKEN=your_actual_token_here`
   - The bot runs in **polling mode** by default (perfect for local development)

3. **Find your Telegram User ID** (Admin only)
   - Start a chat with your bot on Telegram
   - Send the command `/myid` to the bot (requires admin access)
   - The bot will reply with your user ID
   - Add it to `.env`: 
     - `ALLOWED_USER_IDS=your_user_id_here` (to allow file uploads)
     - `ADMIN_USER_IDS=your_user_id_here` (to grant admin privileges)

4. **Install dependencies**
   ```bash
   go mod tidy
   ```

5. **Install Air (for hot reload during development - optional but recommended)**
   ```bash
   # Using go install (recommended)
   go install github.com/air-verse/air@latest
   
   # Add to PATH (add to ~/.zshrc or ~/.bashrc)
   echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.zshrc
   source ~/.zshrc  # or restart your terminal
   
   # Or using curl (Linux/Mac)
   curl -sSfL https://raw.githubusercontent.com/air-verse/air/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
   
   # Or using Homebrew (Mac)
   brew install air
   ```
   
   The project includes a `.air.toml` configuration file. Once Air is installed, you can use it instead of `go run`.

6. **Run the bot locally**
   
   **Option A: Using Air (hot reload - recommended for development)**
   ```bash
   air
   ```
   Air will automatically rebuild and restart the bot when you make changes to `.go` files.
   
   **Option B: Using go run (standard)**
   ```bash
   go run main.go
   ```
   
   **How Polling Works:**
   - The bot actively requests updates from Telegram's API every 60 seconds
   - **No endpoint/webhook needed** - your computer doesn't need to be accessible from the internet
   - The bot automatically clears any existing webhook when starting in polling mode
   - You'll see logs like:
     ```
     ✅ Authorized on account YourBotName
     ✅ Webhook cleared - ready for polling
     Bot is running in polling mode (local development)
     Bot is polling for updates...
     ```
   
   The bot will:
   - Start polling for messages (no webhook needed!)
   - Listen on port 8080 for API requests
   - Process JSON files sent via Telegram

6. **For production deployment** (webhook mode)
   - Set `USE_POLLING=false` in your `.env` or environment
   - Set up webhook URL:
   ```bash
   curl -X POST "https://api.telegram.org/bot<YOUR_BOT_TOKEN>/setWebhook" \
     -d "url=https://your-domain.com/webhook"
   ```

## Deployment

### Free Server Options

Deploy your bot to a free hosting service:

**Quick Start:** See [DEPLOYMENT_QUICK_START.md](DEPLOYMENT_QUICK_START.md) for a 5-minute setup guide.

**Full Guide:** See [FREE_SERVER_SETUP.md](FREE_SERVER_SETUP.md) for detailed instructions for deployment platforms.

### Deployment Files

The repository includes:
- `Dockerfile` - Container configuration

### Required Environment Variables for Production

```bash
TELEGRAM_BOT_TOKEN=your_telegram_bot_token
ENVIRONMENT=production
USE_POLLING=false  # Use webhook mode in production
STORAGE_PATH=/app/storage
HUGO_BASE_URL=https://your-username.github.io/demenu
HASH_SECRET=your_secret_key_here  # Optional but recommended
```

### After Deployment

1. Get your bot's public URL from the hosting platform
2. Set the Telegram webhook:
   ```bash
   curl -X POST "https://api.telegram.org/bot<TOKEN>/setWebhook?url=<YOUR_BOT_URL>/webhook"
   ```
3. Update Hugo config with your bot URL:
   ```toml
   # In demenu/hugo.production.toml
   [params]
     botApiUrl = "https://your-bot-url.com"
   ```

## Development

### Local Testing
For local development, you can use ngrok or similar tools to expose your local server:
```bash
ngrok http 8080
```
Then set the webhook URL to your ngrok URL.

### Testing
```bash
go test ./...
```

## Dependencies

- `github.com/gin-gonic/gin`: Web framework for handling HTTP requests
- `github.com/go-telegram-bot-api/telegram-bot-api/v5`: Telegram Bot API client

## API Endpoints

- `POST /webhook` - Telegram webhook endpoint for receiving bot updates
- `GET /api/meals.json` - Serves meals.json for Hugo app (default menu)
- `GET /api/menu.json` - Serves menu.json for Hugo app (default menu)
- `GET /api/user/:hash/menu.json` - Serves user-specific menu.json by hash
- `GET /api/user/:hash/meals.json` - Serves user-specific meals.json by hash
- `GET /health` - Health check endpoint

## Next Steps

1. ✅ Add Telegram Bot API client library
2. ✅ Implement webhook handler to process Telegram updates
3. ✅ Add JSON schema validation
4. ✅ Implement file storage with proper error handling
5. ✅ Add user authorization
6. ✅ Create API endpoint for Hugo app integration
7. Update Hugo app to fetch JSON from bot API instead of local files
8. Set up webhook/trigger mechanism to rebuild Hugo site when JSON is updated
9. Deploy bot to production environment

