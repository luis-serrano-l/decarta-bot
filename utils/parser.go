package utils

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// generateCategoryID creates a unique ID from a category name
// Converts to lowercase, replaces spaces and special chars with hyphens
func generateCategoryID(name string) string {
	// Convert to lowercase
	id := strings.ToLower(name)

	// Replace spaces and special characters with hyphens
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	id = reg.ReplaceAllString(id, "-")

	// Remove leading/trailing hyphens
	id = strings.Trim(id, "-")

	// Ensure it's not empty
	if id == "" {
		id = "category"
	}

	return id
}

// ParseMenuList parses a list format into menu.json structure
// Format:
// CATEGORY: Category Name
// DESCRIPTION: Category description
//
//   - ITEM: Item Name
//     PRICE: 12.99
//     DESCRIPTION: Item description
//
// (empty line separates items, new CATEGORY starts a new category)
// Category IDs are auto-generated from category names
func ParseMenuList(text string) (map[string]interface{}, error) {
	lines := strings.Split(text, "\n")

	var categories []map[string]interface{}
	var currentCategory map[string]interface{}
	var currentItems []map[string]interface{}

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Parse CATEGORY
		if strings.HasPrefix(line, "CATEGORY:") {
			// Save previous category if exists
			if currentCategory != nil {
				// Convert items to []interface{} for validation
				itemsInterface := make([]interface{}, len(currentItems))
				for i, item := range currentItems {
					itemsInterface[i] = item
				}
				currentCategory["items"] = itemsInterface
				categories = append(categories, currentCategory)
			}

			// Start new category
			categoryName := strings.TrimSpace(strings.TrimPrefix(line, "CATEGORY:"))
			if categoryName == "" {
				return nil, fmt.Errorf("line %d: CATEGORY name is required", i+1)
			}

			// Generate unique ID from category name
			categoryID := generateCategoryID(categoryName)

			// Ensure ID is unique (add number suffix if duplicate)
			baseID := categoryID
			counter := 1
			for {
				isDuplicate := false
				for _, cat := range categories {
					if cat["id"] == categoryID {
						isDuplicate = true
						break
					}
				}
				if !isDuplicate {
					break
				}
				categoryID = fmt.Sprintf("%s-%d", baseID, counter)
				counter++
			}

			currentCategory = map[string]interface{}{
				"id":   categoryID,
				"name": categoryName,
			}
			currentItems = []map[string]interface{}{}
			continue
		}

		// Parse DESCRIPTION (can be category or item description, supports indented format)
		if strings.Contains(line, "DESCRIPTION:") {
			// Remove leading spaces/dashes and "DESCRIPTION:" prefix
			descLine := strings.TrimSpace(line)
			if strings.HasPrefix(descLine, "- DESCRIPTION:") {
				descLine = strings.TrimSpace(strings.TrimPrefix(descLine, "- DESCRIPTION:"))
			} else if strings.HasPrefix(descLine, "DESCRIPTION:") {
				descLine = strings.TrimSpace(strings.TrimPrefix(descLine, "DESCRIPTION:"))
			} else {
				// Try to extract description if line contains DESCRIPTION: anywhere
				parts := strings.Split(descLine, "DESCRIPTION:")
				if len(parts) > 1 {
					descLine = strings.TrimSpace(parts[1])
				}
			}

			if currentCategory != nil && len(currentItems) == 0 {
				// Category description
				currentCategory["description"] = descLine
			} else if len(currentItems) > 0 {
				// Item description
				lastItem := currentItems[len(currentItems)-1]
				lastItem["description"] = descLine
			} else {
				return nil, fmt.Errorf("line %d: DESCRIPTION must come after CATEGORY or ITEM", i+1)
			}
			continue
		}

		// Parse ITEM (supports both "ITEM:" and "- ITEM:" formats)
		if strings.HasPrefix(line, "- ITEM:") || strings.HasPrefix(line, "ITEM:") {
			if currentCategory == nil {
				return nil, fmt.Errorf("line %d: ITEM must come after CATEGORY", i+1)
			}

			// Remove "- " prefix if present, then "ITEM:" prefix
			itemLine := strings.TrimSpace(line)
			if strings.HasPrefix(itemLine, "- ITEM:") {
				itemLine = strings.TrimSpace(strings.TrimPrefix(itemLine, "- ITEM:"))
			} else if strings.HasPrefix(itemLine, "ITEM:") {
				itemLine = strings.TrimSpace(strings.TrimPrefix(itemLine, "ITEM:"))
			}

			if itemLine == "" {
				return nil, fmt.Errorf("line %d: ITEM name is required", i+1)
			}

			item := map[string]interface{}{
				"name": itemLine,
			}
			currentItems = append(currentItems, item)
			continue
		}

		// Parse PRICE (supports indented format with leading spaces)
		if strings.Contains(line, "PRICE:") {
			if len(currentItems) == 0 {
				return nil, fmt.Errorf("line %d: PRICE must come after ITEM", i+1)
			}

			// Remove leading spaces/dashes and "PRICE:" prefix
			priceLine := strings.TrimSpace(line)
			if strings.HasPrefix(priceLine, "- PRICE:") {
				priceLine = strings.TrimSpace(strings.TrimPrefix(priceLine, "- PRICE:"))
			} else if strings.HasPrefix(priceLine, "PRICE:") {
				priceLine = strings.TrimSpace(strings.TrimPrefix(priceLine, "PRICE:"))
			} else {
				// Try to extract price if line contains PRICE: anywhere
				parts := strings.Split(priceLine, "PRICE:")
				if len(parts) > 1 {
					priceLine = strings.TrimSpace(parts[1])
				}
			}

			price, err := strconv.ParseFloat(priceLine, 64)
			if err != nil {
				return nil, fmt.Errorf("line %d: invalid PRICE format: %v", i+1, err)
			}

			lastItem := currentItems[len(currentItems)-1]
			lastItem["price"] = price
			continue
		}

		// Unknown line format
		return nil, fmt.Errorf("line %d: unknown format: %s", i+1, line)
	}

	// Save last category
	if currentCategory != nil {
		// Convert items to []interface{} for validation
		itemsInterface := make([]interface{}, len(currentItems))
		for i, item := range currentItems {
			itemsInterface[i] = item
		}
		currentCategory["items"] = itemsInterface
		categories = append(categories, currentCategory)
	}

	if len(categories) == 0 {
		return nil, fmt.Errorf("no categories found")
	}

	// Convert []map[string]interface{} to []interface{} for validation
	categoriesInterface := make([]interface{}, len(categories))
	for i, cat := range categories {
		categoriesInterface[i] = cat
	}

	return map[string]interface{}{
		"categories": categoriesInterface,
	}, nil
}

// DetectListFormat detects if the text is in list format or JSON format
func DetectListFormat(text string) string {
	text = strings.TrimSpace(text)

	// Check if it starts with JSON
	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		return "json"
	}

	// Check for list format keywords
	keywords := []string{"CATEGORY:", "ITEM:", "PRICE:", "DESCRIPTION:"}
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return "list"
		}
	}

	// Default to list format if unclear
	return "list"
}
