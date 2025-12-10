package main

import (
	"demenu_bot/utils"
	"fmt"
	"os"
	"strconv"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run get_hash.go <userID>")
		os.Exit(1)
	}

	userID, err := strconv.ParseInt(os.Args[1], 10, 64)
	if err != nil {
		fmt.Printf("Error: Invalid user ID: %v\n", err)
		os.Exit(1)
	}

	hash := utils.EncodeUserID(userID)
	fmt.Printf("User ID: %d\n", userID)
	fmt.Printf("Hash: %s\n", hash)
	fmt.Printf("\nTest URL: http://localhost:1313/menu/%s\n", hash)
	fmt.Printf("API URL: http://localhost:8080/api/user/%s/menu.json\n", hash)
}
