package middleware

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// AccessLog represents a structured access log entry
type AccessLog struct {
	Timestamp    time.Time `json:"timestamp"`
	IP           string    `json:"ip"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	Hash         string    `json:"hash,omitempty"` // extracted from path if present
	StatusCode   int       `json:"status_code"`
	ResponseSize int64     `json:"response_size"`
	UserAgent    string    `json:"user_agent"`
	Duration     string    `json:"duration_ms"`
}

// getClientIP extracts the client IP from the request, handling proxy headers
func getClientIP(c *gin.Context) string {
	// Check X-Forwarded-For header (first IP in chain)
	forwarded := c.GetHeader("X-Forwarded-For")
	if forwarded != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(forwarded, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if ip != "" {
				return ip
			}
		}
	}

	// Check X-Real-IP header
	realIP := c.GetHeader("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	ip := c.ClientIP()
	return ip
}

// extractHashFromPath extracts the hash from paths like /api/user/:hash/menu.json
func extractHashFromPath(path string) string {
	// Pattern: /api/user/{hash}/menu.json
	parts := strings.Split(path, "/")
	if len(parts) >= 4 && parts[1] == "api" && parts[2] == "user" {
		return parts[3]
	}
	return ""
}

// AccessLoggingMiddleware creates a middleware that logs all HTTP requests
func AccessLoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Create a custom response writer to capture status code and size
		blw := &bodyLogWriter{body: &strings.Builder{}, ResponseWriter: c.Writer}
		c.Writer = blw

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(start)

		// Extract client IP
		clientIP := getClientIP(c)

		// Extract hash from path if present
		hash := extractHashFromPath(c.Request.URL.Path)

		// Create access log entry
		accessLog := AccessLog{
			Timestamp:    start,
			IP:           clientIP,
			Method:       c.Request.Method,
			Path:         c.Request.URL.Path,
			Hash:         hash,
			StatusCode:   c.Writer.Status(),
			ResponseSize: int64(blw.body.Len()),
			UserAgent:    c.Request.UserAgent(),
			Duration:     duration.String(),
		}

		// Log as JSON for easy parsing
		logJSON, err := json.Marshal(accessLog)
		if err != nil {
			log.Printf("Error marshaling access log: %v", err)
			// Fallback to simple log format
			log.Printf("[ACCESS] %s - %s %s %d %d bytes %s (hash: %s)",
				clientIP, c.Request.Method, c.Request.URL.Path,
				c.Writer.Status(), blw.body.Len(), duration, hash)
		} else {
			log.Printf("[ACCESS] %s", string(logJSON))
		}
	}
}

// bodyLogWriter wraps gin.ResponseWriter to capture response body size
type bodyLogWriter struct {
	gin.ResponseWriter
	body *strings.Builder
}

func (w *bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *bodyLogWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

