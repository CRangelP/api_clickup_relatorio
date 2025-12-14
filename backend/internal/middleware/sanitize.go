package middleware

import (
	"html"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// SanitizeConfig contains configuration for input sanitization
type SanitizeConfig struct {
	MaxStringLength int  // Maximum allowed string length
	AllowHTML       bool // Whether to allow HTML in strings
}

// DefaultSanitizeConfig returns default sanitization configuration
func DefaultSanitizeConfig() SanitizeConfig {
	return SanitizeConfig{
		MaxStringLength: 10000,
		AllowHTML:       false,
	}
}

// SanitizeString sanitizes a string input by:
// - Trimming whitespace
// - Escaping HTML entities (if AllowHTML is false)
// - Removing null bytes
// - Truncating to max length
func SanitizeString(input string, config SanitizeConfig) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")

	// Trim whitespace
	input = strings.TrimSpace(input)

	// Escape HTML if not allowed
	if !config.AllowHTML {
		input = html.EscapeString(input)
	}

	// Truncate to max length
	if config.MaxStringLength > 0 && len(input) > config.MaxStringLength {
		input = input[:config.MaxStringLength]
	}

	return input
}

// SanitizeFilename sanitizes a filename by:
// - Removing path traversal attempts
// - Removing dangerous characters
// - Ensuring valid extension
func SanitizeFilename(filename string) string {
	// Get just the base name (remove any path components)
	filename = filepath.Base(filename)

	// Remove null bytes
	filename = strings.ReplaceAll(filename, "\x00", "")

	// Remove path traversal sequences
	filename = strings.ReplaceAll(filename, "..", "")
	filename = strings.ReplaceAll(filename, "/", "")
	filename = strings.ReplaceAll(filename, "\\", "")

	// Remove control characters
	filename = removeControlChars(filename)

	// Trim whitespace
	filename = strings.TrimSpace(filename)

	// If filename is empty after sanitization, return a default
	if filename == "" {
		return "unnamed_file"
	}

	return filename
}


// ValidateFilePath validates that a file path is safe and within allowed directories
func ValidateFilePath(path string, allowedDir string) bool {
	// Clean the path
	cleanPath := filepath.Clean(path)

	// Check for path traversal
	if strings.Contains(cleanPath, "..") {
		return false
	}

	// If allowedDir is specified, ensure path is within it
	if allowedDir != "" {
		absPath, err := filepath.Abs(cleanPath)
		if err != nil {
			return false
		}
		absAllowed, err := filepath.Abs(allowedDir)
		if err != nil {
			return false
		}
		if !strings.HasPrefix(absPath, absAllowed) {
			return false
		}
	}

	return true
}

// SanitizeToken sanitizes a ClickUp API token
func SanitizeToken(token string) string {
	// Remove whitespace
	token = strings.TrimSpace(token)

	// Remove null bytes
	token = strings.ReplaceAll(token, "\x00", "")

	// Remove any non-printable characters
	token = removeControlChars(token)

	return token
}

// ValidateToken validates a ClickUp API token format
func ValidateToken(token string) bool {
	// Token should start with pk_
	if !strings.HasPrefix(token, "pk_") {
		return false
	}

	// Token should be at least 10 characters
	if len(token) < 10 {
		return false
	}

	// Token should only contain alphanumeric characters and underscores
	validToken := regexp.MustCompile(`^pk_[a-zA-Z0-9_]+$`)
	return validToken.MatchString(token)
}

// SanitizeID sanitizes an ID string (for ClickUp IDs)
func SanitizeID(id string) string {
	// Remove whitespace
	id = strings.TrimSpace(id)

	// Remove null bytes
	id = strings.ReplaceAll(id, "\x00", "")

	// Remove any non-alphanumeric characters except hyphens and underscores
	validID := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	id = validID.ReplaceAllString(id, "")

	return id
}

// ValidateID validates that an ID is in a valid format
func ValidateID(id string) bool {
	if id == "" {
		return false
	}

	// ID should only contain alphanumeric characters, hyphens, and underscores
	validID := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	return validID.MatchString(id)
}

// SanitizeUsername sanitizes a username
func SanitizeUsername(username string) string {
	// Remove whitespace
	username = strings.TrimSpace(username)

	// Remove null bytes
	username = strings.ReplaceAll(username, "\x00", "")

	// Remove control characters
	username = removeControlChars(username)

	// Limit length
	if len(username) > 100 {
		username = username[:100]
	}

	return username
}

// ValidateUsername validates a username format
func ValidateUsername(username string) bool {
	if username == "" {
		return false
	}

	// Username should be 3-100 characters
	if len(username) < 3 || len(username) > 100 {
		return false
	}

	// Username should only contain alphanumeric characters, underscores, and hyphens
	validUsername := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	return validUsername.MatchString(username)
}

// SanitizePassword sanitizes a password (minimal sanitization to preserve special chars)
func SanitizePassword(password string) string {
	// Remove null bytes only
	password = strings.ReplaceAll(password, "\x00", "")

	// Remove control characters except common ones
	var result strings.Builder
	for _, r := range password {
		if !unicode.IsControl(r) || r == '\t' || r == '\n' || r == '\r' {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// ValidatePassword validates password requirements
func ValidatePassword(password string) bool {
	// Password should be at least 6 characters
	if len(password) < 6 {
		return false
	}

	// Password should be at most 128 characters
	if len(password) > 128 {
		return false
	}

	return true
}

// removeControlChars removes control characters from a string
func removeControlChars(s string) string {
	var result strings.Builder
	for _, r := range s {
		if !unicode.IsControl(r) {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// SanitizeJSON sanitizes a JSON string value
func SanitizeJSON(input string) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")

	// Escape special JSON characters that could cause issues
	input = strings.ReplaceAll(input, "\u2028", "\\u2028") // Line separator
	input = strings.ReplaceAll(input, "\u2029", "\\u2029") // Paragraph separator

	return input
}

// ValidateRateLimit validates rate limit value
func ValidateRateLimit(rateLimit int) bool {
	return rateLimit >= 10 && rateLimit <= 10000
}

// SanitizeTitle sanitizes a title/name string
func SanitizeTitle(title string) string {
	config := DefaultSanitizeConfig()
	config.MaxStringLength = 255

	return SanitizeString(title, config)
}
