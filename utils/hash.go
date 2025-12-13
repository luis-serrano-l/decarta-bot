package utils

import (
	"crypto/hmac"
	"crypto/sha256"
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

// base62Chars is the character set for base62 encoding (URL-safe, no padding needed)
const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// encodeBase62 encodes a number to base62 string with optional padding
func encodeBase62(n uint64, minLength int) string {
	if n == 0 {
		result := "0"
		for len(result) < minLength {
			result = "0" + result
		}
		return result
	}
	var result []byte
	for n > 0 {
		result = append([]byte{base62Chars[n%62]}, result...)
		n /= 62
	}
	// Pad to minimum length
	for len(result) < minLength {
		result = append([]byte{'0'}, result...)
	}
	return string(result)
}

// decodeBase62 decodes a base62 string to a number
func decodeBase62(s string) (uint64, error) {
	var result uint64
	for _, char := range s {
		idx := -1
		for i, c := range base62Chars {
			if c == char {
				idx = i
				break
			}
		}
		if idx == -1 {
			return 0, fmt.Errorf("invalid base62 character: %c", char)
		}
		result = result*62 + uint64(idx)
	}
	return result, nil
}

// SimpleEncodeUserID creates a shorter reversible encoding
// Format: base62(userID, 7 chars) + base62(signature, 6 chars) = 13 chars total
// Uses 4-byte signature for shorter hash while maintaining security
func SimpleEncodeUserID(userID int64) string {
	// Convert user ID to bytes
	userIDBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		userIDBytes[i] = byte(userID >> (56 - i*8))
	}

	// Create HMAC signature (use 4 bytes for shorter hash)
	h := hmac.New(sha256.New, hashSecret)
	h.Write(userIDBytes)
	signature := h.Sum(nil)[:4] // Use first 4 bytes of signature

	// Convert userID to uint64 for base62 encoding
	var userIDUint uint64
	for i := 0; i < 8; i++ {
		userIDUint = userIDUint<<8 | uint64(userIDBytes[i])
	}

	// Convert signature to uint32 for base62 encoding
	var sigUint uint32
	for i := 0; i < 4; i++ {
		sigUint = sigUint<<8 | uint32(signature[i])
	}

	// Encode both to base62 with fixed lengths (7 chars for userID, 6 for signature)
	userIDStr := encodeBase62(userIDUint, 7)
	sigStr := encodeBase62(uint64(sigUint), 6)

	// Combine: total 13 characters (much shorter than 24!)
	return userIDStr + sigStr
}

// SimpleDecodeUserID decodes the shorter encoding back to user ID
// Format: 7 chars (userID) + 6 chars (signature) = 13 chars total
func SimpleDecodeUserID(hash string) (int64, error) {
	if len(hash) != 13 {
		return 0, fmt.Errorf("invalid hash length: expected 13 characters, got %d", len(hash))
	}

	// Split: first 7 chars = userID, last 6 chars = signature
	userIDStr := hash[:7]
	sigStr := hash[7:]

	// Decode userID
	userID, err := decodeBase62(userIDStr)
	if err != nil {
		return 0, fmt.Errorf("invalid userID encoding: %w", err)
	}

	// Decode signature
	sig, err := decodeBase62(sigStr)
	if err != nil {
		return 0, fmt.Errorf("invalid signature encoding: %w", err)
	}

	// Convert userID back to bytes
	userIDBytes := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		userIDBytes[i] = byte(userID)
		userID >>= 8
	}

	// Verify signature
	h := hmac.New(sha256.New, hashSecret)
	h.Write(userIDBytes)
	expectedSignature := h.Sum(nil)[:4]

	var expectedSig uint32
	for i := 0; i < 4; i++ {
		expectedSig = expectedSig<<8 | uint32(expectedSignature[i])
	}

	if uint32(sig) != expectedSig {
		return 0, fmt.Errorf("invalid hash: signature verification failed")
	}

	// Convert back to int64
	var result int64
	for i := 0; i < 8; i++ {
		result = result<<8 | int64(userIDBytes[i])
	}

	return result, nil
}
