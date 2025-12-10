package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type Storage struct {
	basePath string
}

func NewStorage(basePath string) *Storage {
	// Create storage directory if it doesn't exist
	if err := os.MkdirAll(basePath, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create storage directory: %v", err))
	}

	return &Storage{
		basePath: basePath,
	}
}

// SaveJSON saves a JSON file to storage
func (s *Storage) SaveJSON(filename string, data interface{}) error {
	filePath := filepath.Join(s.basePath, filename)

	// Validate JSON structure before saving
	if err := s.validateJSON(filename, data); err != nil {
		return fmt.Errorf("invalid JSON structure: %w", err)
	}

	// Marshal JSON with indentation
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// ValidateJSON validates the JSON structure based on filename (public method)
func (s *Storage) ValidateJSON(filename string, data interface{}) error {
	return s.validateJSON(filename, data)
}

// validateJSON validates the JSON structure based on filename
func (s *Storage) validateJSON(filename string, data interface{}) error {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("JSON must be an object")
	}

	switch filename {
	case "menu.json":
		return s.validateMenuJSON(dataMap)
	default:
		return fmt.Errorf("unknown file type: %s", filename)
	}
}

// validateMenuJSON validates the structure of menu.json
func (s *Storage) validateMenuJSON(data map[string]interface{}) error {
	categories, ok := data["categories"]
	if !ok {
		return fmt.Errorf("missing 'categories' field")
	}

	categoriesSlice, ok := categories.([]interface{})
	if !ok {
		return fmt.Errorf("'categories' must be an array")
	}

	for i, category := range categoriesSlice {
		categoryMap, ok := category.(map[string]interface{})
		if !ok {
			return fmt.Errorf("category at index %d must be an object", i)
		}

		// Required fields for category
		if _, ok := categoryMap["id"]; !ok {
			return fmt.Errorf("category at index %d missing required field 'id'", i)
		}
		if _, ok := categoryMap["name"]; !ok {
			return fmt.Errorf("category at index %d missing required field 'name'", i)
		}
		if _, ok := categoryMap["items"]; !ok {
			return fmt.Errorf("category at index %d missing required field 'items'", i)
		}

		// Validate items array
		items, ok := categoryMap["items"].([]interface{})
		if !ok {
			return fmt.Errorf("category at index %d: 'items' must be an array", i)
		}

		for j, item := range items {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				return fmt.Errorf("category at index %d, item at index %d must be an object", i, j)
			}

			// Required fields for item
			if _, ok := itemMap["name"]; !ok {
				return fmt.Errorf("category at index %d, item at index %d missing required field 'name'", i, j)
			}
			if _, ok := itemMap["description"]; !ok {
				return fmt.Errorf("category at index %d, item at index %d missing required field 'description'", i, j)
			}
			if _, ok := itemMap["price"]; !ok {
				return fmt.Errorf("category at index %d, item at index %d missing required field 'price'", i, j)
			}

			// Validate price is a number
			if price, ok := itemMap["price"]; ok {
				if !isNumber(price) {
					return fmt.Errorf("category at index %d, item at index %d: 'price' must be a number", i, j)
				}
			}
		}
	}

	return nil
}

// isNumber checks if a value is a number (int, int64, float32, float64)
func isNumber(v interface{}) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	default:
		// Check if it's a JSON number (which Go unmarshals as float64)
		kind := reflect.TypeOf(v).Kind()
		return kind == reflect.Float64 || kind == reflect.Int || kind == reflect.Int64
	}
}

// GetJSON retrieves a JSON file from storage
func (s *Storage) GetJSON(filename string) ([]byte, error) {
	filePath := filepath.Join(s.basePath, filename)
	return os.ReadFile(filePath)
}

// SaveUserJSON saves a JSON file for a specific user as carta_{userID}.json
func (s *Storage) SaveUserJSON(userID int64, filename string, data interface{}) error {
	// Validate JSON structure before saving
	if err := s.validateJSON(filename, data); err != nil {
		return fmt.Errorf("invalid JSON structure: %w", err)
	}

	// Save as carta_{userID}.json (only for menu.json, meals.json uses same pattern)
	// Determine the carta filename based on the original filename
	var cartaFilename string
	if filename == "menu.json" {
		cartaFilename = fmt.Sprintf("carta_%d.json", userID)
	} else if filename == "meals.json" {
		cartaFilename = fmt.Sprintf("carta_%d_meals.json", userID)
	} else {
		return fmt.Errorf("unknown file type: %s", filename)
	}

	filePath := filepath.Join(s.basePath, cartaFilename)

	// Marshal JSON with indentation
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// GetUserJSON retrieves a JSON file for a specific user from carta_{userID}.json
// Security: Only allows reading files matching carta_{userID}.json pattern from storage directory
func (s *Storage) GetUserJSON(userID int64, filename string) ([]byte, error) {
	// Determine the carta filename based on the original filename
	var cartaFilename string
	if filename == "menu.json" {
		cartaFilename = fmt.Sprintf("carta_%d.json", userID)
	} else {
		return nil, fmt.Errorf("unknown file type: %s", filename)
	}

	// Security: Validate filename pattern to prevent directory traversal
	// Only allow carta_*.json pattern (numbers and underscores only in userID part)
	if !isValidCartaFilename(cartaFilename) {
		return nil, fmt.Errorf("invalid filename pattern")
	}

	// Build file path - ensure it's within storage directory
	filePath := filepath.Join(s.basePath, cartaFilename)

	// Security: Verify the resolved path is still within storage directory (prevent directory traversal)
	absBasePath, err := filepath.Abs(s.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base path: %w", err)
	}
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve file path: %w", err)
	}

	// Check if the file path is within the base path
	relPath, err := filepath.Rel(absBasePath, absFilePath)
	if err != nil || relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("invalid file path: directory traversal detected")
	}

	return os.ReadFile(filePath)
}

// isValidCartaFilename validates that the filename matches carta_{number}.json pattern
func isValidCartaFilename(filename string) bool {
	// Pattern: carta_<digits>.json
	matched, _ := filepath.Match("carta_*.json", filename)
	return matched
}

// UserHasJSON checks if a user has a specific JSON file (carta_{userID}.json)
func (s *Storage) UserHasJSON(userID int64, filename string) bool {
	// Determine the carta filename based on the original filename
	var cartaFilename string
	if filename == "menu.json" {
		cartaFilename = fmt.Sprintf("carta_%d.json", userID)
	} else {
		return false
	}

	filePath := filepath.Join(s.basePath, cartaFilename)
	_, err := os.Stat(filePath)
	return err == nil
}
