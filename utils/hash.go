package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// Secret key for hashing (in production, this should be from config)
// For now, using a simple secret. In production, use a secure random key from env
var hashSecret = []byte("demenu-secret-key-change-in-production")

// SetHashSecret allows setting a custom secret key from config
func SetHashSecret(secret string) {
	if secret != "" {
		hashSecret = []byte(secret)
	}
}

// EncodeUserID encodes a user ID into a hash string (uses simple reversible encoding)
func EncodeUserID(userID int64) string {
	return SimpleEncodeUserID(userID)
}

// DecodeUserID decodes a hash string back to user ID
func DecodeUserID(hash string) (int64, error) {
	return SimpleDecodeUserID(hash)
}

// SimpleEncodeUserID creates a simpler reversible encoding
// Format: base64(userID_bytes + HMAC_signature)
func SimpleEncodeUserID(userID int64) string {
	// Convert user ID to bytes
	userIDBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		userIDBytes[i] = byte(userID >> (56 - i*8))
	}
	
	// Create HMAC signature
	h := hmac.New(sha256.New, hashSecret)
	h.Write(userIDBytes)
	signature := h.Sum(nil)[:8] // Use first 8 bytes of signature
	
	// Combine userID + signature
	combined := append(userIDBytes, signature...)
	
	// Encode to base64 URL-safe
	return base64.URLEncoding.EncodeToString(combined)
}

// SimpleDecodeUserID decodes the simple encoding back to user ID
func SimpleDecodeUserID(hash string) (int64, error) {
	// Decode base64
	decoded, err := base64.URLEncoding.DecodeString(hash)
	if err != nil {
		return 0, fmt.Errorf("invalid hash format: %w", err)
	}
	
	// Should be 16 bytes: 8 bytes userID + 8 bytes signature
	if len(decoded) != 16 {
		return 0, fmt.Errorf("invalid hash length")
	}
	
	// Extract userID bytes
	userIDBytes := decoded[:8]
	signature := decoded[8:]
	
	// Verify signature
	h := hmac.New(sha256.New, hashSecret)
	h.Write(userIDBytes)
	expectedSignature := h.Sum(nil)[:8]
	
	if !hmac.Equal(signature, expectedSignature) {
		return 0, fmt.Errorf("invalid hash signature")
	}
	
	// Convert bytes to int64
	userID := int64(userIDBytes[0])<<56 | int64(userIDBytes[1])<<48 | int64(userIDBytes[2])<<40 | int64(userIDBytes[3])<<32 |
		int64(userIDBytes[4])<<24 | int64(userIDBytes[5])<<16 | int64(userIDBytes[6])<<8 | int64(userIDBytes[7])
	
	return userID, nil
}

