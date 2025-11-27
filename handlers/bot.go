package handlers

import (
	"demenu_bot/config"
	"demenu_bot/services"
	"demenu_bot/storage"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type BotHandler struct {
	config     *config.Config
	storage    *storage.Storage
	botService *services.BotService
}

func NewBotHandler(cfg *config.Config, botService *services.BotService) *BotHandler {
	return &BotHandler{
		config:     cfg,
		storage:    storage.NewStorage(cfg.StoragePath),
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

	log.Printf("Received update: %+v", update)

	// Handle commands
	if update.Message != nil && update.Message.IsCommand() {
		log.Printf("Processing command: %s from user %d", update.Message.Command(), update.Message.From.ID)
		h.handleCommand(update.Message)
		return
	}

	// Handle edited messages (for JSON template editing)
	if update.EditedMessage != nil {
		log.Printf("Processing edited message from user %d", update.EditedMessage.From.ID)
		h.handleJSONMessage(update.EditedMessage)
		return
	}

	// Handle regular messages (for JSON template editing)
	if update.Message != nil && update.Message.Text != "" {
		log.Printf("Processing text message from user %d", update.Message.From.ID)
		h.handleJSONMessage(update.Message)
		return
	}

	// Handle other messages
	if update.Message != nil {
		log.Printf("Processing message from user %d", update.Message.From.ID)
		h.handleMessage(update.Message)
	}
}

func (h *BotHandler) handleCommand(message *tgbotapi.Message) {
	userID := message.From.ID

	switch message.Command() {
	case "start":
		text := "👋 Welcome to the Demenu Bot!\n\n" +
			"Admins can use /template to get JSON templates, edit them, and send them back as text messages to update the menu.\n\n" +
			"Commands:\n" +
			"/start - Show this message\n" +
			"/help - Show help information\n" +
			"/template - Get JSON templates (Admin only)\n" +
			"/request_access - Request access to update menu\n" +
			"/status - Check bot status (Admin only)\n" +
			"/myid - Get your Telegram user ID (Admin only)"
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

	default:
		text := "❓ Unknown command. Use /help to see available commands."
		h.botService.SendMessage(message.Chat.ID, text)
	}

	log.Printf("Command %s from user %d", message.Command(), userID)
}

func (h *BotHandler) handleHelp(message *tgbotapi.Message) {
	userID := message.From.ID
	isAdmin := h.botService.IsAdmin(userID)
	hasAccess := h.botService.IsUserAllowed(userID)

	text := "📋 Available Commands\n\n"

	// Common commands for everyone
	text += "• /start - Show welcome message\n"
	text += "• /help - Show this help message\n"

	// Commands based on user permissions
	if isAdmin {
		text += "• /status - Check bot status\n"
		text += "• /myid - Get your Telegram user ID\n"
		text += "• /template - Get JSON templates for meals.json and menu.json (Admin only)\n"
	}

	if !hasAccess {
		text += "• /request_access - Request access to update menu\n"
	}

	text += "\n📄 Menu Updates:\n"
	if isAdmin {
		text += "• Use /template to get JSON templates\n"
		text += "• Edit the template and send it back as a text message to update the menu.\n"
		text += "• JSON must be sent as a text message (file uploads are not supported).\n"
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
		log.Printf("⚠️  Access request from user %d, but no admin users configured", userID)
		return
	}

	// Send notification to each admin
	notificationText := fmt.Sprintf("🔔 Access requested from user:\n\n%s", userInfo)
	for _, adminID := range adminUserIDs {
		if err := h.botService.SendMessage(adminID, notificationText); err != nil {
			log.Printf("Error sending access request notification to admin %d: %v", adminID, err)
		} else {
			log.Printf("Access request notification sent to admin %d", adminID)
		}
	}

	log.Printf("Access request processed from user %d, notifications sent to %d admin(s)", userID, len(adminUserIDs))
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

	text := "📋 JSON Templates\n\n" +
		"Edit one of these templates and send it back to update the menu:\n\n" +
		"━━━━━━━━━━━━━━━━━━━━\n" +
		"🍽️ MEALS TEMPLATE:\n" +
		"━━━━━━━━━━━━━━━━━━━━\n" +
		"```json\n" +
		"{\n" +
		"  \"meals\": [\n" +
		"    {\n" +
		"      \"name\": \"Meal Name\",\n" +
		"      \"price\": 24.99,\n" +
		"      \"description\": \"Meal description\",\n" +
		"      \"dietary\": \"Optional dietary info\",\n" +
		"      \"image\": \"/images/meal.jpg\"\n" +
		"    }\n" +
		"  ]\n" +
		"}\n" +
		"```\n\n" +
		"━━━━━━━━━━━━━━━━━━━━\n" +
		"📋 MENU TEMPLATE:\n" +
		"━━━━━━━━━━━━━━━━━━━━\n" +
		"```json\n" +
		"{\n" +
		"  \"categories\": [\n" +
		"    {\n" +
		"      \"id\": \"category-id\",\n" +
		"      \"name\": \"Category Name\",\n" +
		"      \"description\": \"Category description\",\n" +
		"      \"items\": [\n" +
		"        {\n" +
		"          \"name\": \"Item Name\",\n" +
		"          \"description\": \"Item description\",\n" +
		"          \"price\": 12.99\n" +
		"        }\n" +
		"      ]\n" +
		"    }\n" +
		"  ]\n" +
		"}\n" +
		"```\n\n" +
		"💡 Tip: Copy one template, edit it with your data, and send it back as a message."

	h.botService.SendMessage(message.Chat.ID, text)
}

func (h *BotHandler) handleJSONMessage(message *tgbotapi.Message) {
	userID := message.From.ID
	text := message.Text

	// Check if user is admin
	if !h.botService.IsAdmin(userID) {
		errorMsg := "❌ You are not authorized to send JSON data. Only administrators can update the menu."
		h.botService.SendMessage(message.Chat.ID, errorMsg)
		log.Printf("Unauthorized JSON submission attempt from user %d", userID)
		return
	}

	// Check if message contains text (JSON)
	if text == "" {
		return
	}

	// Try to parse JSON
	var jsonData interface{}
	if err := json.Unmarshal([]byte(text), &jsonData); err != nil {
		errorMsg := fmt.Sprintf("❌ Invalid JSON format:\n\n`%v`\n\nPlease check your JSON syntax and try again.", err)
		h.botService.SendMessage(message.Chat.ID, errorMsg)
		log.Printf("Invalid JSON from user %d: %v", userID, err)
		return
	}

	// Determine file type based on JSON structure
	dataMap, ok := jsonData.(map[string]interface{})
	if !ok {
		errorMsg := "❌ Invalid format: JSON must be an object with either 'meals' or 'categories' field."
		h.botService.SendMessage(message.Chat.ID, errorMsg)
		return
	}

	var filename string
	if _, hasMeals := dataMap["meals"]; hasMeals {
		filename = "meals.json"
	} else if _, hasCategories := dataMap["categories"]; hasCategories {
		filename = "menu.json"
	} else {
		errorMsg := "❌ Invalid format: JSON must contain either 'meals' or 'categories' field.\n\n" +
			"Use /template to see the correct format."
		h.botService.SendMessage(message.Chat.ID, errorMsg)
		return
	}

	// Validate JSON structure
	if err := h.storage.ValidateJSON(filename, jsonData); err != nil {
		errorMsg := fmt.Sprintf("❌ Invalid JSON structure for %s:\n\n`%v`\n\nUse /template to see the correct format.", filename, err)
		h.botService.SendMessage(message.Chat.ID, errorMsg)
		log.Printf("Invalid JSON structure from user %d: %v", userID, err)
		return
	}

	// Save JSON
	if err := h.storage.SaveJSON(filename, jsonData); err != nil {
		errorMsg := fmt.Sprintf("❌ Error saving %s: %v", filename, err)
		h.botService.SendMessage(message.Chat.ID, errorMsg)
		log.Printf("Error saving JSON from user %d: %v", userID, err)
		return
	}

	// Success message
	successMsg := fmt.Sprintf("✅ Successfully saved %s!", filename)
	h.botService.SendMessage(message.Chat.ID, successMsg)
	log.Printf("JSON %s saved successfully from user %d", filename, userID)
}

func (h *BotHandler) handleMessage(message *tgbotapi.Message) {
	// Provide helpful information
	text := "📄 Use /template to get JSON templates, then edit and send them back.\n\n" +
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
