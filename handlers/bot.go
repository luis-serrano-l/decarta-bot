package handlers

import (
	"demenu_bot/config"
	"demenu_bot/services"
	"demenu_bot/storage"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

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

	// Handle document messages
	if update.Message != nil && update.Message.Document != nil {
		log.Printf("Processing document: %s from user %d", update.Message.Document.FileName, update.Message.From.ID)
		h.handleDocument(update.Message)
		return
	}

	// Handle regular messages
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
			"Send me a JSON file (meals.json or menu.json) to update the menu.\n\n" +
			"Commands:\n" +
			"/start - Show this message\n" +
			"/help - Show help information\n" +
			"/status - Check bot status\n" +
			"/myid - Get your Telegram user ID"
		h.botService.SendMessage(message.Chat.ID, text)

	case "help":
		text := "📋 Help\n\n" +
			"This bot accepts JSON files to update the menu:\n\n" +
			"• meals.json - Contains meal items\n" +
			"• menu.json - Contains menu categories and items\n\n" +
			"Just send the JSON file as a document and I'll process it!"
		h.botService.SendMessage(message.Chat.ID, text)

	case "status":
		text := "✅ Bot is running and ready to receive JSON files!"
		h.botService.SendMessage(message.Chat.ID, text)

	case "myid":
		userID := message.From.ID
		text := fmt.Sprintf("🆔 Your Telegram User ID is: `%d`\n\n"+
			"Add this to your `.env` file:\n"+
			"`ALLOWED_USER_IDS=%d`", userID, userID)
		h.botService.SendMessage(message.Chat.ID, text)

	default:
		text := "❓ Unknown command. Use /help to see available commands."
		h.botService.SendMessage(message.Chat.ID, text)
	}

	log.Printf("Command %s from user %d", message.Command(), userID)
}

func (h *BotHandler) handleMessage(message *tgbotapi.Message) {
	// If message is not a document, provide helpful information
	if message.Document == nil {
		text := "📄 Please send a JSON file (meals.json or menu.json) as a document.\n\n" +
			"Use /help for more information."
		h.botService.SendMessage(message.Chat.ID, text)
	}
}

func (h *BotHandler) handleDocument(message *tgbotapi.Message) {
	userID := message.From.ID
	document := message.Document

	// Check if user is allowed
	if !h.botService.IsUserAllowed(userID) {
		text := "❌ You are not authorized to send files to this bot."
		h.botService.SendMessage(message.Chat.ID, text)
		log.Printf("Unauthorized access attempt from user %d", userID)
		return
	}

	// Check if file is JSON
	fileName := document.FileName
	if !strings.HasSuffix(strings.ToLower(fileName), ".json") {
		text := "❌ Please send a JSON file (.json extension)."
		h.botService.SendMessage(message.Chat.ID, text)
		return
	}

	// Check if filename is meals.json or menu.json
	if fileName != "meals.json" && fileName != "menu.json" {
		text := "❌ File must be named 'meals.json' or 'menu.json'."
		h.botService.SendMessage(message.Chat.ID, text)
		return
	}

	// Download file from Telegram
	text := "⏳ Processing your file..."
	h.botService.SendMessage(message.Chat.ID, text)

	fileData, err := h.downloadFile(document.FileID)
	if err != nil {
		log.Printf("Error downloading file: %v", err)
		text := "❌ Error downloading file. Please try again."
		h.botService.SendMessage(message.Chat.ID, text)
		return
	}

	// Parse JSON
	var jsonData interface{}
	if err := json.Unmarshal(fileData, &jsonData); err != nil {
		log.Printf("Error parsing JSON: %v", err)
		text := "❌ Invalid JSON format. Please check your file."
		h.botService.SendMessage(message.Chat.ID, text)
		return
	}

	// Validate and save
	if err := h.storage.SaveJSON(fileName, jsonData); err != nil {
		log.Printf("Error saving file: %v", err)
		text := fmt.Sprintf("❌ Error saving file: %v", err)
		h.botService.SendMessage(message.Chat.ID, text)
		return
	}

	// Success message
	text = fmt.Sprintf("✅ Successfully saved %s!", fileName)
	h.botService.SendMessage(message.Chat.ID, text)
	log.Printf("File %s saved successfully from user %d", fileName, userID)
}

func (h *BotHandler) downloadFile(fileID string) ([]byte, error) {
	if h.botService.GetBot() == nil {
		return nil, fmt.Errorf("bot not initialized")
	}

	file, err := h.botService.GetBot().GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return nil, err
	}

	fileURL := file.Link(h.botService.GetBot().Token)
	resp, err := http.Get(fileURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
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
