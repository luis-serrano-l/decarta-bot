package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramBotToken string
	Environment      string
	StoragePath      string
	AllowedUserIDs   []int64 // Telegram user IDs allowed to send JSON files
	UsePolling       bool    // Use polling instead of webhook (for local development)
}

func Load() *Config {
	// Try to load .env file
	envPath := ".env"
	if err := godotenv.Load(envPath); err != nil {
		log.Printf("Warning: Could not load .env file from %s: %v", envPath, err)
		log.Println("Trying to use environment variables instead...")
	} else {
		log.Printf("✅ Loaded .env file from %s", envPath)
	}

	token := getEnv("TELEGRAM_BOT_TOKEN", "")
	if token == "" {
		log.Println("⚠️  WARNING: TELEGRAM_BOT_TOKEN is empty!")
		log.Println("   Make sure your .env file contains: TELEGRAM_BOT_TOKEN=your_token_here")
	} else {
		// Show first few chars for debugging (without exposing full token)
		preview := token
		if len(preview) > 10 {
			preview = preview[:10] + "..."
		}
		log.Printf("✅ Found TELEGRAM_BOT_TOKEN: %s", preview)
	}

	return &Config{
		TelegramBotToken: token,
		Environment:      getEnv("ENVIRONMENT", "development"),
		StoragePath:      getEnv("STORAGE_PATH", "./storage"),
		AllowedUserIDs:   parseUserIDs(getEnv("ALLOWED_USER_IDS", "")),
		UsePolling:       getEnv("USE_POLLING", "true") == "true", // Default to polling for local dev
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseUserIDs(userIDsStr string) []int64 {
	if userIDsStr == "" {
		return []int64{}
	}

	parts := strings.Split(userIDsStr, ",")
	userIDs := make([]int64, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if id, err := strconv.ParseInt(part, 10, 64); err == nil {
			userIDs = append(userIDs, id)
		}
	}

	return userIDs
}
