package main

import (
	"demenu_bot/config"
	"demenu_bot/storage"
	"demenu_bot/utils"
	"flag"
	"fmt"
	"log"
)

// This utility helps inspect the database contents
func main() {
	var (
		userID  = flag.Int64("user-id", 0, "Check if a specific user ID has menu data")
		listAll = flag.Bool("list-all", false, "List all user IDs in the database")
		hash    = flag.String("hash", "", "Check a hash (will decode to user ID)")
	)
	flag.Parse()

	cfg := config.Load()

	// Determine storage type
	if cfg.StorageType != "postgres" {
		log.Fatal("This utility only works with PostgreSQL storage. Set STORAGE_TYPE=postgres")
	}

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL not set. Required when STORAGE_TYPE=postgres")
	}

	// Initialize PostgreSQL storage
	pgStorage, err := storage.NewPostgresStorage(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pgStorage.Close()

	log.Println("Connected to PostgreSQL database")

	// Handle hash decoding
	if *hash != "" {
		log.Printf("Decoding hash: %s", *hash)
		decodedUserID, err := utils.DecodeUserID(*hash)
		if err != nil {
			log.Fatalf("Failed to decode hash: %v", err)
		}
		log.Printf("Hash decodes to user ID: %d", decodedUserID)
		checkUserID(pgStorage, decodedUserID)
		return
	}

	// Handle list all
	if *listAll {
		listAllUsers(pgStorage)
		return
	}

	// Handle specific user ID
	if *userID > 0 {
		checkUserID(pgStorage, *userID)
		return
	}

	// No action specified
	fmt.Println("Usage:")
	fmt.Println("  go run cmd/check_db.go --list-all")
	fmt.Println("  go run cmd/check_db.go --user-id 123456")
	fmt.Println("  go run cmd/check_db.go --hash 01fjZtL2bbMCi")
}

func listAllUsers(pgStorage *storage.PostgresStorage) {
	log.Println("Listing all users in database...")

	// Cast to PostgresStorage to access ListAllUserIDs
	// We need to check if it's actually PostgresStorage
	if pgStorage == nil {
		log.Fatal("PostgresStorage is nil")
	}

	userIDs, err := pgStorage.ListAllUserIDs()
	if err != nil {
		log.Fatalf("Failed to list users: %v", err)
	}

	if len(userIDs) == 0 {
		log.Println("No users found in database")
		return
	}

	log.Printf("Found %d user(s) in database:", len(userIDs))
	for _, userID := range userIDs {
		log.Printf("  - User ID: %d", userID)
	}
}

func checkUserID(pgStorage *storage.PostgresStorage, userID int64) {
	log.Printf("Checking user ID: %d", userID)

	exists := pgStorage.UserHasJSON(userID, "menu.json")
	if !exists {
		log.Printf("User %d does NOT have menu data", userID)
		return
	}

	log.Printf("User %d HAS menu data", userID)

	data, err := pgStorage.GetUserJSON(userID, "menu.json")
	if err != nil {
		log.Printf("Error reading menu data: %v", err)
		return
	}

	log.Printf("Menu data size: %d bytes", len(data))

	// Show preview (first 200 chars)
	previewLen := 200
	if len(data) < previewLen {
		previewLen = len(data)
	}
	log.Printf("Menu preview (first %d chars):\n%s", previewLen, string(data[:previewLen]))
}
