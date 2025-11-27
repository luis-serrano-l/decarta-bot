#!/bin/bash
# Helper script to check .env file

echo "Checking .env file..."
echo ""

if [ ! -f .env ]; then
    echo "❌ .env file not found!"
    echo "   Create a .env file with your bot token"
    exit 1
fi

echo "✅ .env file exists"
echo ""

# Check if token exists
if grep -q "TELEGRAM_BOT_TOKEN=" .env; then
    TOKEN=$(grep "TELEGRAM_BOT_TOKEN=" .env | cut -d'=' -f2 | tr -d ' ' | tr -d '"' | tr -d "'")
    
    if [ -z "$TOKEN" ] || [ "$TOKEN" = "your_bot_token_here" ]; then
        echo "❌ TELEGRAM_BOT_TOKEN is not set or still has placeholder value"
        echo "   Edit .env and replace 'your_bot_token_here' with your actual token"
        exit 1
    fi
    
    # Show first 10 chars
    PREVIEW="${TOKEN:0:10}..."
    echo "✅ TELEGRAM_BOT_TOKEN found: $PREVIEW"
    
    # Check format (should contain colon)
    if [[ "$TOKEN" == *":"* ]]; then
        echo "✅ Token format looks correct (contains colon)"
    else
        echo "⚠️  Warning: Token should contain a colon (format: numbers:letters)"
    fi
else
    echo "❌ TELEGRAM_BOT_TOKEN not found in .env"
    exit 1
fi

echo ""
echo "Checking USE_POLLING..."
if grep -q "USE_POLLING=" .env; then
    POLLING=$(grep "USE_POLLING=" .env | cut -d'=' -f2 | tr -d ' ' | tr -d '"' | tr -d "'")
    echo "✅ USE_POLLING=$POLLING"
else
    echo "⚠️  USE_POLLING not set (will default to true)"
fi

echo ""
echo "✅ .env file looks good!"
