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
	AdminUserIDs     []int64 // Telegram user IDs with admin privileges (can use /myid and /status)
	UsePolling       bool    // Use polling instead of webhook (for local development)
	HashSecret       string  // Secret key for hashing user IDs (optional, has default)
	HugoBaseURL      string  // Base URL of the Hugo site (for generating menu links)
	DatabaseURL      string  // PostgreSQL database URL (for postgres storage type)
	StorageType      string  // Storage type: "file" or "postgres" (default: "file")
	WebhookURL       string  // Webhook URL for Telegram (auto-set in production)
}

func Load() *Config {
	// Try to load .env file
	envPath := ".env"
	if err := godotenv.Load(envPath); err != nil {
		log.Printf("Warning: Could not load .env file from %s: %v", envPath, err)
		log.Println("Trying to use environment variables instead...")
	} else {
		log.Printf("Loaded .env file from %s", envPath)
	}

	token := getEnv("TELEGRAM_BOT_TOKEN", "")
	if token == "" {
		log.Println("WARNING: TELEGRAM_BOT_TOKEN is empty!")
		log.Println("   Make sure your .env file contains: TELEGRAM_BOT_TOKEN=your_token_here")
	} else {
		// Show first few chars for debugging (without exposing full token)
		preview := token
		if len(preview) > 10 {
			preview = preview[:10] + "..."
		}
		log.Printf("Found TELEGRAM_BOT_TOKEN: %s", preview)
	}

	storageType := getEnv("STORAGE_TYPE", "file")
	if storageType != "file" && storageType != "postgres" {
		log.Printf("WARNING: Invalid STORAGE_TYPE '%s', defaulting to 'file'", storageType)
		storageType = "file"
	}

	// Determine webhook URL - check WEBHOOK_URL first, then try Railway's RAILWAY_PUBLIC_DOMAIN
	webhookURL := getEnv("WEBHOOK_URL", "")
	if webhookURL == "" {
		// Railway provides RAILWAY_PUBLIC_DOMAIN environment variable
		railwayDomain := getEnv("RAILWAY_PUBLIC_DOMAIN", "")
		if railwayDomain != "" {
			webhookURL = "https://" + railwayDomain + "/webhook"
		}
	}

	cfg := &Config{
		TelegramBotToken: token,
		Environment:      getEnv("ENVIRONMENT", "development"),
		StoragePath:      getEnv("STORAGE_PATH", "./storage"),
		AllowedUserIDs:   parseUserIDs(getEnv("ALLOWED_USER_IDS", "")),
		AdminUserIDs:     parseUserIDs(getEnv("ADMIN_USER_IDS", "")),
		UsePolling:       getEnv("USE_POLLING", "true") == "true", // Default to polling for local dev
		HashSecret:       getEnv("HASH_SECRET", ""),
		HugoBaseURL:      getEnv("HUGO_BASE_URL", ""), // Hugo site base URL (required for menu links)
		DatabaseURL:      getEnv("DATABASE_URL", ""),  // PostgreSQL connection URL (Railway provides this)
		StorageType:      storageType,                 // Storage type: "file" or "postgres"
		WebhookURL:       webhookURL,                  // Webhook URL (auto-detected from Railway or set manually)
	}

	// Initialize hash secret in utils if provided
	if cfg.HashSecret != "" {
		// We'll need to import utils here, but to avoid circular dependency,
		// we'll set it in main.go after config is loaded
	}

	return cfg
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
