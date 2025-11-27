package main

import (
	"demenu_bot/config"
	"log"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// This utility clears any existing webhook so polling can work
func main() {
	cfg := config.Load()

	if cfg.TelegramBotToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN not set in .env file")
	}

	bot, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Clear webhook
	_, err = bot.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: true})
	if err != nil {
		log.Fatalf("Failed to clear webhook: %v", err)
	}

	log.Println("✅ Webhook cleared successfully!")
	log.Println("Now you can use polling mode. Run: go run main.go")
}

