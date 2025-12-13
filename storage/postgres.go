package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"time"

	_ "github.com/lib/pq"
)

// PostgresStorage implements Storage interface using PostgreSQL
type PostgresStorage struct {
	db *sql.DB
}

// NewPostgresStorage creates a new PostgreSQL-based storage instance
func NewPostgresStorage(databaseURL string) (*PostgresStorage, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Ensure table exists
	if err := ensureTableExists(db); err != nil {
		return nil, fmt.Errorf("failed to ensure table exists: %w", err)
	}

	return &PostgresStorage{db: db}, nil
}

// ensureTableExists creates the user_menus table if it doesn't exist
func ensureTableExists(db *sql.DB) error {
	query := `
		CREATE TABLE IF NOT EXISTS user_menus (
			user_id BIGINT PRIMARY KEY,
			menu_data JSONB NOT NULL,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		);
		
		CREATE INDEX IF NOT EXISTS idx_user_menus_user_id ON user_menus(user_id);
		CREATE INDEX IF NOT EXISTS idx_user_menus_menu_data ON user_menus USING GIN (menu_data);
	`
	_, err := db.Exec(query)
	return err
}

// SaveUserJSON saves a JSON file for a specific user
func (s *PostgresStorage) SaveUserJSON(userID int64, filename string, data interface{}) error {
	// Validate JSON structure before saving
	if err := s.validateJSON(filename, data); err != nil {
		return fmt.Errorf("invalid JSON structure: %w", err)
	}

	// Only support menu.json for now
	if filename != "menu.json" {
		return fmt.Errorf("unknown file type: %s", filename)
	}

	// Marshal JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Upsert: INSERT or UPDATE
	query := `
		INSERT INTO user_menus (user_id, menu_data, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (user_id) 
		DO UPDATE SET menu_data = $2, updated_at = NOW()
	`

	_, err = s.db.Exec(query, userID, jsonData)
	if err != nil {
		return fmt.Errorf("failed to save user menu: %w", err)
	}

	return nil
}

// GetUserJSON retrieves a JSON file for a specific user
func (s *PostgresStorage) GetUserJSON(userID int64, filename string) ([]byte, error) {
	// Only support menu.json for now
	if filename != "menu.json" {
		return nil, fmt.Errorf("unknown file type: %s", filename)
	}

	var jsonData []byte
	query := `SELECT menu_data FROM user_menus WHERE user_id = $1`
	err := s.db.QueryRow(query, userID).Scan(&jsonData)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("failed to get user menu: %w", err)
	}

	// Format JSON with indentation for consistency with file storage
	var data interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	formattedJSON, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format JSON: %w", err)
	}

	return formattedJSON, nil
}

// UserHasJSON checks if a user has a specific JSON file
func (s *PostgresStorage) UserHasJSON(userID int64, filename string) bool {
	// Only support menu.json for now
	if filename != "menu.json" {
		return false
	}

	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM user_menus WHERE user_id = $1)`
	err := s.db.QueryRow(query, userID).Scan(&exists)
	if err != nil {
		return false
	}

	return exists
}

// ValidateJSON validates the JSON structure based on filename
func (s *PostgresStorage) ValidateJSON(filename string, data interface{}) error {
	return s.validateJSON(filename, data)
}

// validateJSON validates the JSON structure based on filename
func (s *PostgresStorage) validateJSON(filename string, data interface{}) error {
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
func (s *PostgresStorage) validateMenuJSON(data map[string]interface{}) error {
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
// Note: This uses the same validation logic as file storage
func (s *PostgresStorage) isNumber(v interface{}) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	default:
		// Check if it's a JSON number (which Go unmarshals as float64)
		kind := reflect.TypeOf(v).Kind()
		return kind == reflect.Float64 || kind == reflect.Int || kind == reflect.Int64
	}
}

// GetJSON retrieves a global JSON file (not supported in PostgreSQL storage)
// This is a legacy method for file storage compatibility
func (s *PostgresStorage) GetJSON(filename string) ([]byte, error) {
	// PostgreSQL storage only supports user-specific menus
	// Return error to indicate this method is not supported
	return nil, os.ErrNotExist
}

// ListAllUserIDs returns all user IDs that have menu data
func (s *PostgresStorage) ListAllUserIDs() ([]int64, error) {
	query := `SELECT user_id FROM user_menus ORDER BY user_id`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query user IDs: %w", err)
	}
	defer rows.Close()

	var userIDs []int64
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, fmt.Errorf("failed to scan user ID: %w", err)
		}
		userIDs = append(userIDs, userID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return userIDs, nil
}

// Ping checks the database connection
func (s *PostgresStorage) Ping() error {
	if s.db == nil {
		return fmt.Errorf("database connection is nil")
	}
	return s.db.Ping()
}

// Close closes the database connection
func (s *PostgresStorage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
