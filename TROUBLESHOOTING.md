# Troubleshooting Guide

## Bot Not Replying

### 1. Check if bot is running
When you run `go run main.go`, you should see:
```
Authorized on account YourBotName
Bot is running in polling mode (local development)
Starting polling goroutine...
Bot is polling for updates...
Starting server on port 8080
```

### 2. Check your .env file
Make sure your `.env` file has:
```
TELEGRAM_BOT_TOKEN=your_actual_token_here
USE_POLLING=true
ENVIRONMENT=development
```

### 3. Verify bot token
- Make sure there are no extra spaces in the token
- Token should start with numbers and contain a colon (e.g., `123456789:ABCdef...`)

### 4. Check if you started a conversation
- In Telegram, you need to START a conversation with your bot first
- Send `/start` to your bot
- The bot should reply

### 5. Check logs
When you send a message, you should see logs like:
```
Received update: ...
Processing command: start from user 123456789
```

### 6. Common issues
- **Bot service is nil**: Check if token is loaded correctly
- **No updates received**: Make sure USE_POLLING=true
- **Bot not authorized**: Check token is correct
