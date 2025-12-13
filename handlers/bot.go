package handlers

import (
	"demenu_bot/config"
	"demenu_bot/services"
	"demenu_bot/storage"
	"demenu_bot/utils"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type BotHandler struct {
	config     *config.Config
	storage    storage.Storage // For validation and user JSON storage (carta_{userID}.json)
	botService *services.BotService
}

func NewBotHandler(cfg *config.Config, botService *services.BotService, storage storage.Storage) *BotHandler {
	return &BotHandler{
		config:     cfg,
		storage:    storage,
		botService: botService,
	}
}

func (h *BotHandler) HandleWebhook(c *gin.Context) {
	var update tgbotapi.Update
	if err := c.ShouldBindJSON(&update); err != nil {
		log.Printf("Error parsing update: %v", err)
		c.JSON(400, gin.H{"error": "Invalid update format"})
		return
	}

	// Handle the update asynchronously
	go h.ProcessUpdate(&update)

	c.JSON(200, gin.H{"status": "ok"})
}

// ProcessUpdate processes a Telegram update (used by both webhook and polling)
func (h *BotHandler) ProcessUpdate(update *tgbotapi.Update) {
	if update == nil {
		return
	}

	// Handle commands
	if update.Message != nil && update.Message.IsCommand() {
		h.handleCommand(update.Message)
		return
	}

	// Handle edited messages (for JSON template editing)
	if update.EditedMessage != nil {
		h.handleJSONMessage(update.EditedMessage)
		return
	}

	// Handle regular messages (for JSON template editing)
	if update.Message != nil && update.Message.Text != "" {
		h.handleJSONMessage(update.Message)
		return
	}

	// Handle other messages
	if update.Message != nil {
		h.handleMessage(update.Message)
	}
}

func (h *BotHandler) handleCommand(message *tgbotapi.Message) {
	userID := message.From.ID

	switch message.Command() {
	case "start":
		text := "👋 Welcome to the Demenu Bot!\n\n" +
			"Create and manage your restaurant menu! Admins can use /template to get JSON templates, edit them, and send them back as text messages to update the menu.\n\n" +
			"📋 Commands:\n" +
			"/start - Show this message\n" +
			"/help - Show help information\n" +
			"/myurl - Get your personal menu URL (share with customers)\n" +
			"/template - Get JSON templates (Admin only)\n" +
			"/request_access - Request access to update menu\n" +
			"/status - Check bot status (Admin only)\n" +
			"/myid - Get your Telegram user ID (Admin only)\n\n" +
			"💡 Tip: Use /myurl to get your unique menu link after uploading content!"
		h.botService.SendMessage(message.Chat.ID, text)

	case "help":
		h.handleHelp(message)

	case "status":
		if !h.botService.IsAdmin(userID) {
			text := "❌ You are not authorized to use this command. Admin access required."
			h.botService.SendMessage(message.Chat.ID, text)
			log.Printf("Unauthorized command attempt: /status from user %d", userID)
			return
		}
		text := "✅ Bot is running!\n\n" +
			"Use /template to get JSON templates, edit them, and send them back as messages to update the menu."
		h.botService.SendMessage(message.Chat.ID, text)

	case "myid":
		if !h.botService.IsAdmin(userID) {
			text := "❌ You are not authorized to use this command. Admin access required."
			h.botService.SendMessage(message.Chat.ID, text)
			log.Printf("Unauthorized command attempt: /myid from user %d", userID)
			return
		}
		text := fmt.Sprintf("🆔 Your Telegram User ID is: `%d`\n\n"+
			"Add this to your `.env` file:\n"+
			"`ALLOWED_USER_IDS=%d`\n"+
			"`ADMIN_USER_IDS=%d`", userID, userID, userID)
		h.botService.SendMessage(message.Chat.ID, text)

	case "request_access":
		h.handleRequestAccess(message)

	case "template":
		h.handleTemplate(message)

	case "myurl", "link", "menuurl":
		h.handleMyURL(message)

	default:
		text := "❓ Unknown command. Use /help to see available commands."
		h.botService.SendMessage(message.Chat.ID, text)
	}
}

func (h *BotHandler) handleHelp(message *tgbotapi.Message) {
	userID := message.From.ID
	isAdmin := h.botService.IsAdmin(userID)
	hasAccess := h.botService.IsUserAllowed(userID)

	text := "📋 Available Commands\n\n"

	// Common commands for everyone
	text += "• /start - Welcome message\n"
	text += "• /help - Show this help message\n"
	text += "• /myurl - Get your personal menu URL (share with customers)\n"

	// Commands based on user permissions
	if isAdmin {
		text += "• /status - Check bot status\n"
		text += "• /myid - Get your Telegram user ID\n"
		text += "• /template - Get menu template (Admin only)\n"
	}

	if !hasAccess {
		text += "• /request_access - Request access to update menu\n"
	}

	text += "\n📄 Menu Updates:\n"
	if isAdmin {
		text += "• Use /template to get menu templates\n"
		text += "• Edit the template and send it back as a text message to update the menu.\n"
		text += "• Use the simple list format (see /template for examples).\n"
		text += "• After saving, use /myurl to get your menu link to share with customers.\n"
	} else {
		text += "Only administrators can update the menu.\n"
		text += "Use /request_access to request permission from administrators."
	}

	h.botService.SendMessage(message.Chat.ID, text)
}

func (h *BotHandler) handleRequestAccess(message *tgbotapi.Message) {
	userID := message.From.ID
	userName := message.From.UserName
	firstName := message.From.FirstName
	lastName := message.From.LastName

	// Build user info string
	userInfo := fmt.Sprintf("User ID: `%d`", userID)
	if userName != "" {
		userInfo += fmt.Sprintf("\nUsername: @%s", userName)
	}
	if firstName != "" {
		userInfo += fmt.Sprintf("\nName: %s", firstName)
		if lastName != "" {
			userInfo += fmt.Sprintf(" %s", lastName)
		}
	}

	// Send confirmation to requesting user
	confirmText := "✅ Your access request has been sent to the administrators. You will be notified once your request is reviewed."
	h.botService.SendMessage(message.Chat.ID, confirmText)

	// Get all admin user IDs and send notification
	adminUserIDs := h.botService.GetAdminUserIDs()
	if len(adminUserIDs) == 0 {
		log.Printf("WARNING: Access request from user %d, but no admin users configured", userID)
		return
	}

	// Send notification to each admin
	notificationText := fmt.Sprintf("🔔 Access requested from user:\n\n%s", userInfo)
	for _, adminID := range adminUserIDs {
		if err := h.botService.SendMessage(adminID, notificationText); err != nil {
			log.Printf("Error sending access request notification to admin %d: %s", adminID, err.Error())
		}
	}

	log.Printf("Access request: user %d → %d admin(s)", userID, len(adminUserIDs))
}

func (h *BotHandler) handleTemplate(message *tgbotapi.Message) {
	userID := message.From.ID

	// Check if user is admin
	if !h.botService.IsAdmin(userID) {
		text := "❌ You are not authorized to use this command. Admin access required."
		h.botService.SendMessage(message.Chat.ID, text)
		log.Printf("Unauthorized command attempt: /template from user %d", userID)
		return
	}

	// Send instructions first
	instructions := "📋 Menu Template\n\n" +
		"Edit the template below and send it back to update the menu.\n\n" +
		"💡 Tip: Use dashes (-) for items. Empty lines separate items.\n" +
		"After saving, use /myurl to get your menu link!"

	h.botService.SendMessage(message.Chat.ID, instructions)

	// Send template in a separate message for easy copying
	template :=
		"CATEGORY: Appetizers\n" +
			"DESCRIPTION: Start your meal\n" +
			"\n" +
			"- ITEM: Bruschetta\n" +
			"  PRICE: 12.99\n" +
			"  DESCRIPTION: Classic tomato basil\n" +
			"\n" +
			"- ITEM: Calamari\n" +
			"  PRICE: 14.99\n" +
			"  DESCRIPTION: Golden fried squid\n" +
			"\n" +
			"CATEGORY: Salads\n" +
			"DESCRIPTION: Fresh options\n" +
			"\n" +
			"- ITEM: Caesar Salad\n" +
			"  PRICE: 9.99\n" +
			"  DESCRIPTION: Crisp romaine\n"

	h.botService.SendMessage(message.Chat.ID, template)
}

func (h *BotHandler) handleMyURL(message *tgbotapi.Message) {
	userID := message.From.ID

	// Generate hash from user ID
	hash := utils.EncodeUserID(userID)

	// Get base URL from config
	baseURL := h.config.HugoBaseURL
	if baseURL == "" {
		h.botService.SendMessage(message.Chat.ID, "❌ Error: Hugo base URL not configured. Please set HUGO_BASE_URL in bot configuration.")
		log.Printf("Error: HugoBaseURL not set for user %d", userID)
		return
	}

	// Build menu URL: /menu/{hash}
	menuURL := fmt.Sprintf("%s/menu/%s", baseURL, hash)

	// Check if user has any content stored
	hasMenu := h.storage.UserHasJSON(userID, "menu.json")

	url := fmt.Sprintf("%s\n\n", menuURL)
	var text string
	if hasMenu {
		text = fmt.Sprintf("Updated url %s for user %d", url, userID)
	} else {
		text = fmt.Sprintf("New url %s for user %d", url, userID)
	}

	h.botService.SendMessage(message.Chat.ID, text)
}

func (h *BotHandler) handleJSONMessage(message *tgbotapi.Message) {
	userID := message.From.ID
	text := message.Text

	// Check if user is admin
	if !h.botService.IsAdmin(userID) {
		errorMsg := "❌ You are not authorized to send menu data. Only administrators can update the menu."
		h.botService.SendMessage(message.Chat.ID, errorMsg)
		log.Printf("Unauthorized menu submission attempt from user %d", userID)
		return
	}

	// Check if message contains text
	if text == "" {
		return
	}

	// Detect format (list or JSON)
	format := utils.DetectListFormat(text)
	var jsonData interface{}
	var filename string
	var err error

	if format == "json" {
		// Try to parse JSON (backward compatibility)
		if err := json.Unmarshal([]byte(text), &jsonData); err != nil {
			errorMsg := fmt.Sprintf("❌ Invalid JSON format:\n\n`%v`\n\nPlease check your JSON syntax and try again.", err)
			h.botService.SendMessage(message.Chat.ID, errorMsg)
			log.Printf("Invalid JSON from user %d", userID)
			return
		}

		// Determine file type based on JSON structure
		dataMap, ok := jsonData.(map[string]interface{})
		if !ok {
			errorMsg := "❌ Invalid format: JSON must be an object with either 'meals' or 'categories' field."
			h.botService.SendMessage(message.Chat.ID, errorMsg)
			return
		}

		if _, hasCategories := dataMap["categories"]; hasCategories {
			filename = "menu.json"
		} else {
			errorMsg := "❌ Invalid format: JSON must contain 'categories' field.\n\n" +
				"Use /template to see the correct format."
			h.botService.SendMessage(message.Chat.ID, errorMsg)
			return
		}
	} else {
		// Parse list format
		jsonData, err = utils.ParseMenuList(text)
		if err != nil {
			errorMsg := fmt.Sprintf("❌ Invalid list format:\n\n`%v`\n\nUse /template to see the correct format.", err)
			h.botService.SendMessage(message.Chat.ID, errorMsg)
			log.Printf("Invalid list format from user %d", userID)
			return
		}
		filename = "menu.json"
	}

	// Validate JSON structure
	if err := h.storage.ValidateJSON(filename, jsonData); err != nil {
		errorMsg := fmt.Sprintf("❌ Invalid JSON structure for %s:\n\n`%v`\n\nUse /template to see the correct format.", filename, err)
		h.botService.SendMessage(message.Chat.ID, errorMsg)
		log.Printf("Invalid JSON structure from user %d: %s", userID, err.Error())
		return
	}

	// Save JSON to file as carta_{userID}.json
	if err := h.storage.SaveUserJSON(userID, filename, jsonData); err != nil {
		errorMsg := fmt.Sprintf("❌ Error saving %s: %v", filename, err)
		h.botService.SendMessage(message.Chat.ID, errorMsg)
		log.Printf("Error saving JSON file for user %d: %s", userID, err.Error())
		return
	}

	// Success message with hash link
	hash := utils.EncodeUserID(userID)
	baseURL := h.config.HugoBaseURL
	if baseURL == "" {
		successMsg := fmt.Sprintf("✅ Successfully saved %s!\n\n⚠️ Warning: Hugo base URL not configured. Set HUGO_BASE_URL in bot configuration to get your menu link.\n\n💡 Use /myurl to get your menu URL once configured.", filename)
		h.botService.SendMessage(message.Chat.ID, successMsg)
		log.Printf("Menu saved: user %d (hash: %s) - HugoBaseURL not set", userID, hash)
		return
	}
	menuURL := fmt.Sprintf("%s/menu/%s", baseURL, hash)
	successMsg := fmt.Sprintf("✅ Successfully saved %s!\n\n🔗 Your personal menu URL:\n%s\n\n✨ Your menu is now live! Share this link with your customers.\n\n💡 Use /myurl anytime to get your menu link.", filename, menuURL)
	h.botService.SendMessage(message.Chat.ID, successMsg)
	log.Printf("Menu saved: user %d (hash: %s)", userID, hash)
}

func (h *BotHandler) handleMessage(message *tgbotapi.Message) {
	// Provide helpful information
	text := "📄 Use /template to get menu templates, then edit and send them back.\n\n" +
		"Use /help for more information."
	h.botService.SendMessage(message.Chat.ID, text)
}

// ServeMealsJSON serves the meals.json file for the Hugo app
func (h *BotHandler) ServeMealsJSON(c *gin.Context) {
	data, err := h.storage.GetJSON("meals.json")
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(404, gin.H{"error": "meals.json not found"})
			return
		}
		c.JSON(500, gin.H{"error": "Failed to read meals.json"})
		return
	}

	c.Data(200, "application/json", data)
}

// ServeMenuJSON serves the menu.json file for the Hugo app
func (h *BotHandler) ServeMenuJSON(c *gin.Context) {
	data, err := h.storage.GetJSON("menu.json")
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(404, gin.H{"error": "menu.json not found"})
			return
		}
		c.JSON(500, gin.H{"error": "Failed to read menu.json"})
		return
	}

	c.Data(200, "application/json", data)
}

// getClientIP extracts the client IP from the request, handling proxy headers
func getClientIP(c *gin.Context) string {
	// Check X-Forwarded-For header (first IP in chain)
	forwarded := c.GetHeader("X-Forwarded-For")
	if forwarded != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(forwarded, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if ip != "" {
				return ip
			}
		}
	}

	// Check X-Real-IP header
	realIP := c.GetHeader("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	return c.ClientIP()
}

// ServeUserMenuJSON serves the menu.json file for a specific user by hash (reads from carta_{userID}.json)
// Security: Only serves files matching carta_{userID}.json pattern from storage directory
func (h *BotHandler) ServeUserMenuJSON(c *gin.Context) {
	hash := c.Param("hash")
	clientIP := getClientIP(c)

	// Add security headers
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "DENY")
	c.Header("X-XSS-Protection", "1; mode=block")

	// Decode hash to user ID
	userID, err := utils.DecodeUserID(hash)
	if err != nil {
		log.Printf("Invalid hash: %s (IP: %s)", hash, clientIP)
		c.JSON(400, gin.H{"error": "Invalid hash"})
		return
	}

	// Security: Only allow reading carta_*.json files (validated in GetUserJSON)
	// GetUserJSON ensures the filename is carta_{userID}.json and reads from storage directory only
	data, err := h.storage.GetUserJSON(userID, "menu.json")
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Menu not found: hash %s (user %d, IP: %s)", hash, userID, clientIP)
			c.JSON(404, gin.H{"error": "menu.json not found for this user. Please send your menu JSON via Telegram first."})
			return
		}
		log.Printf("Error reading menu: user %d (IP: %s) - %s", userID, clientIP, err.Error())
		c.JSON(500, gin.H{"error": "Failed to read menu.json"})
		return
	}

	// Success - middleware will log the request details
	c.Data(200, "application/json", data)
}
