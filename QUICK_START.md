# Quick Start Guide

## How Polling Works (No Endpoint Needed!)

**Polling mode** means your bot actively asks Telegram "Do you have any new messages?" every 60 seconds. 
- ✅ No webhook/endpoint needed
- ✅ Works behind firewalls
- ✅ Works on your local computer
- ✅ No need to expose your computer to the internet

## Step-by-Step Setup

### 1. Set up your .env file
```bash
TELEGRAM_BOT_TOKEN=your_token_here
USE_POLLING=true
ENVIRONMENT=development
```

### 2. Run the bot
```bash
cd /home/luis-serrano-l/Documents/Go/demenu_bot
go run main.go
```

### 3. What you should see in the console (logs):
```
✅ Authorized on account YourBotName
Clearing any existing webhook for polling mode...
✅ Webhook cleared - ready for polling
Bot is running in polling mode (local development)
Starting polling goroutine...
Bot is polling for updates...
Starting server on port 8080
```

### 4. Test the bot in Telegram
- Open Telegram
- Find your bot (search by username)
- Click "Start" or send `/start`
- **Watch the console** - you should see:
  ```
  Received update: ...
  Processing command: start from user 123456789
  Message sent successfully to chat 123456789
  ```

### 5. If the bot doesn't reply:
- Check the console logs - errors will be shown there
- Make sure you see "✅ Authorized on account..."
- Make sure you see "Bot is polling for updates..."
- Make sure you clicked "Start" in Telegram first
- Try sending `/start` again

## Viewing Logs

The logs appear in your terminal where you ran `go run main.go`. 
- All messages are printed to the console
- Errors will show in red (if your terminal supports it)
- Debug messages show what the bot is doing

## Troubleshooting

**Bot not replying?**
1. Check console for errors
2. Make sure token is correct (should see "✅ Authorized")
3. Make sure USE_POLLING=true in .env
4. Make sure you started the conversation in Telegram

**No logs appearing?**
- The bot might not be running
- Check if there are any error messages
- Make sure your .env file is in the same directory as main.go
