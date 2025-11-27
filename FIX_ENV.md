# Fix: Environment Variable Override Issue

## Problem
If you see `your_bot_t...` in the logs, it means an environment variable is overriding your .env file.

## Solution

**Option 1: Unset the variable (Quick Fix)**
```bash
unset TELEGRAM_BOT_TOKEN
go run main.go
```

**Option 2: Start a fresh terminal**
- Close your current terminal
- Open a new terminal
- Navigate to the project: `cd /home/luis-serrano-l/Documents/Go/demenu_bot`
- Run: `go run main.go`

**Option 3: Delete the setup script (Permanent Fix)**
The `setup_env.sh` script is not needed anymore since we use .env files:
```bash
rm setup_env.sh
```

## Why this happens
When you run `source setup_env.sh`, it sets environment variables in your shell.
Environment variables take precedence over .env file values.
The .env file is better because it's persistent and doesn't affect your shell.

## Recommended: Just use .env file
1. Make sure your .env file has the correct token
2. Don't run `source setup_env.sh`
3. Just run: `go run main.go`
