package main

import (
	"log"
	"os"

	"demenu_bot/config"
	"demenu_bot/handlers"
	"demenu_bot/middleware"
	"demenu_bot/services"
	"demenu_bot/utils"

	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Set hash secret if provided in config
	if cfg.HashSecret != "" {
		utils.SetHashSecret(cfg.HashSecret)
	}

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

	// Add access logging middleware (logs all requests with IP, path, hash, etc.)
	router.Use(middleware.AccessLoggingMiddleware())

	// Add CORS middleware for Hugo app
	router.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Allow requests from GitHub Pages and localhost for development
		allowedOrigins := []string{
			"https://luis-serrano-l.github.io",
			"http://localhost:1313",
			"http://localhost:8080",
		}

		// Check if origin is allowed, or allow all for development
		allowOrigin := "*"
		for _, allowed := range allowedOrigins {
			if origin == allowed {
				allowOrigin = origin
				break
			}
		}

		c.Writer.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		// Note: Cannot use Access-Control-Allow-Credentials: true with Access-Control-Allow-Origin: *
		// Removing credentials header to fix Brave browser blocking
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Max-Age", "3600")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Initialize handlers
	botHandler := handlers.NewBotHandler(cfg, botService)

	// Webhook endpoint for Telegram (only used in production)
	if !cfg.UsePolling {
		router.POST("/webhook", botHandler.HandleWebhook)
		log.Println("Webhook mode enabled. Set webhook URL in Telegram.")
	}

	// API endpoints to serve JSON files for Hugo app
	router.GET("/api/menu.json", botHandler.ServeMenuJSON)

	// API endpoints to serve user-specific JSON files by hash
	router.GET("/api/user/:hash/menu.json", botHandler.ServeUserMenuJSON)

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
