package main

import (
	"demenu_bot/config"
	"fmt"
	"log"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Test if the bot token is valid
func main() {
	cfg := config.Load()

	if cfg.TelegramBotToken == "" {
		log.Fatal("ERROR: TELEGRAM_BOT_TOKEN not set in .env file")
	}

	fmt.Printf("Testing token: %s...\n", cfg.TelegramBotToken[:10])

	bot, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		fmt.Printf("❌ ERROR: %v\n", err)
		fmt.Println("\nPossible issues:")
		fmt.Println("1. Token is invalid or expired")
		fmt.Println("2. Bot was deleted")
		fmt.Println("3. Token has extra spaces or characters")
		fmt.Println("\nTo fix:")
		fmt.Println("1. Go to @BotFather on Telegram")
		fmt.Println("2. Send /token to see your current token")
		fmt.Println("3. Or create a new bot with /newbot")
		os.Exit(1)
	}

	fmt.Printf("✅ Token is valid!\n")
	fmt.Printf("✅ Bot username: @%s\n", bot.Self.UserName)
	fmt.Printf("✅ Bot ID: %d\n", bot.Self.ID)
	fmt.Printf("✅ Bot name: %s\n", bot.Self.FirstName)
}

