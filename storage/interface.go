package storage

// Storage defines the interface for storing and retrieving user menu data
type Storage interface {
	// SaveUserJSON saves a JSON file for a specific user
	SaveUserJSON(userID int64, filename string, data interface{}) error

	// GetUserJSON retrieves a JSON file for a specific user
	GetUserJSON(userID int64, filename string) ([]byte, error)

	// UserHasJSON checks if a user has a specific JSON file
	UserHasJSON(userID int64, filename string) bool

	// ValidateJSON validates the JSON structure based on filename
	ValidateJSON(filename string, data interface{}) error

	// GetJSON retrieves a global JSON file (legacy method, mainly for file storage)
	GetJSON(filename string) ([]byte, error)
}

