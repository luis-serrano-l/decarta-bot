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
- [ ] Update Hugo app to fetch JSON from bot API instead of local files
- [ ] Set up webhook/trigger mechanism to rebuild Hugo site when JSON is updated
- [ ] Deploy bot to production environment

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
- `ALLOWED_USER_IDS`: Comma-separated list of Telegram user IDs allowed to send files (optional in dev mode)
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

3. **Find your Telegram User ID**
   - Start a chat with your bot on Telegram
   - Send the command `/myid` to the bot
   - The bot will reply with your user ID
   - Add it to `.env`: `ALLOWED_USER_IDS=your_user_id_here`

4. **Install dependencies**
   ```bash
   go mod tidy
   ```

5. **Run the bot locally**
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
- `GET /api/meals.json` - Serves meals.json for Hugo app
- `GET /api/menu.json` - Serves menu.json for Hugo app
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

