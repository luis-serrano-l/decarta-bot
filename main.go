package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"demenu_bot/handlers"
	"demenu_bot/config"
	"demenu_bot/services"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Validate bot token
	if cfg.TelegramBotToken == "" {
		log.Fatal("ERROR: TELEGRAM_BOT_TOKEN not set in .env file. Please add your bot token.")
	}

	// Initialize bot service
	botService, err := services.NewBotService(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	if botService == nil {
		log.Fatal("ERROR: Bot service is nil. Check your TELEGRAM_BOT_TOKEN.")
	}

	// Set Gin mode
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize router
	router := gin.Default()

	// Initialize handlers
	botHandler := handlers.NewBotHandler(cfg, botService)

	// Webhook endpoint for Telegram (only used in production)
	if !cfg.UsePolling {
		router.POST("/webhook", botHandler.HandleWebhook)
		log.Println("Webhook mode enabled. Set webhook URL in Telegram.")
	}

	// API endpoints to serve JSON files for Hugo app
	router.GET("/api/meals.json", botHandler.ServeMealsJSON)
	router.GET("/api/menu.json", botHandler.ServeMenuJSON)

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Start bot in polling mode (for local development)
	if cfg.UsePolling {
		if botService == nil {
			log.Println("Warning: Bot service is nil. Cannot start polling. Check your TELEGRAM_BOT_TOKEN.")
		} else {
			go func() {
				log.Println("Starting polling goroutine...")
				if err := botService.StartPolling(botHandler.ProcessUpdate); err != nil {
					log.Printf("Error starting polling: %v", err)
				}
			}()
			log.Println("Bot is running in polling mode (local development)")
		}
	} else {
		log.Println("Polling mode disabled. Using webhook mode.")
	}

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

