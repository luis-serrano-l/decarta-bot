package services

import (
	"demenu_bot/config"
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type BotService struct {
	bot    *tgbotapi.BotAPI
	config *config.Config
}

func NewBotService(cfg *config.Config) (*BotService, error) {
	if cfg.TelegramBotToken == "" {
		return nil, fmt.Errorf("telegram bot token is empty - check your .env file")
	}

	// Validate token format (should be numbers:letters)
	if len(cfg.TelegramBotToken) < 10 {
		return nil, fmt.Errorf("token seems too short - check your .env file")
	}

	// Log token info (first 10 chars only for security)
	tokenPreview := cfg.TelegramBotToken
	if len(tokenPreview) > 10 {
		tokenPreview = tokenPreview[:10] + "..."
	}
	log.Printf("Attempting to connect with token: %s", tokenPreview)

	bot, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot API (token might be invalid): %w. Check your TELEGRAM_BOT_TOKEN in .env file", err)
	}

	bot.Debug = cfg.Environment == "development"

	log.Printf("Authorized on account %s (ID: %d)", bot.Self.UserName, bot.Self.ID)

	// If using polling, clear any existing webhook
	if cfg.UsePolling {
		log.Println("Clearing any existing webhook for polling mode...")
		_, err = bot.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: false})
		if err != nil {
			log.Printf("Warning: Could not clear webhook (might not exist): %v", err)
		} else {
			log.Println("Webhook cleared - ready for polling")
		}
	}

	return &BotService{
		bot:    bot,
		config: cfg,
	}, nil
}

func (s *BotService) GetBot() *tgbotapi.BotAPI {
	return s.bot
}

func (s *BotService) SendMessage(chatID int64, text string) error {
	if s.bot == nil {
		log.Printf("Warning: Bot is nil, cannot send message to chat %d", chatID)
		return fmt.Errorf("bot is nil")
	}

	msg := tgbotapi.NewMessage(chatID, text)
	_, err := s.bot.Send(msg)
	if err != nil {
		log.Printf("Error sending message to chat %d: %v", chatID, err)
		return err
	}
	log.Printf("Message sent successfully to chat %d", chatID)
	return nil
}

func (s *BotService) IsUserAllowed(userID int64) bool {
	// If no allowed users specified, deny access
	if len(s.config.AllowedUserIDs) == 0 {
		log.Printf("Access denied: ALLOWED_USER_IDS is empty. No users are authorized.")
		return false
	}

	// Check if user ID is in allowed list
	for _, allowedID := range s.config.AllowedUserIDs {
		if userID == allowedID {
			return true
		}
	}

	return false
}

func (s *BotService) IsAdmin(userID int64) bool {
	// Check if user ID is in admin list
	for _, adminID := range s.config.AdminUserIDs {
		if userID == adminID {
			return true
		}
	}

	return false
}

func (s *BotService) GetAdminUserIDs() []int64 {
	return s.config.AdminUserIDs
}

// SetWebhook sets the Telegram webhook URL
func (s *BotService) SetWebhook(webhookURL string) error {
	if s.bot == nil {
		return fmt.Errorf("bot is nil")
	}

	if webhookURL == "" {
		return fmt.Errorf("webhook URL is empty")
	}

	log.Printf("Setting webhook URL: %s", webhookURL)
	wh, err := tgbotapi.NewWebhook(webhookURL)
	if err != nil {
		return fmt.Errorf("failed to create webhook config: %w", err)
	}

	_, err = s.bot.Request(wh)
	if err != nil {
		return fmt.Errorf("failed to set webhook: %w", err)
	}

	log.Println("Webhook set successfully")
	return nil
}

// StartPolling starts long polling for updates (useful for local development)
func (s *BotService) StartPolling(updateHandler func(*tgbotapi.Update)) error {
	if s.bot == nil {
		return nil
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := s.bot.GetUpdatesChan(u)

	log.Println("Bot is polling for updates...")

	for update := range updates {
		go updateHandler(&update)
	}

	return nil
}
