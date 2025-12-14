package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// CSRFTokenHeader is the header name for CSRF token
	CSRFTokenHeader = "X-CSRF-Token"
	// CSRFCookieName is the cookie name for CSRF token
	CSRFCookieName = "csrf_token"
	// CSRFTokenLength is the length of the CSRF token in bytes
	CSRFTokenLength = 32
)

// CSRFToken represents a CSRF token with expiration
type CSRFToken struct {
	Token     string
	ExpiresAt time.Time
}

// CSRFConfig contains configuration for CSRF protection
type CSRFConfig struct {
	TokenDuration time.Duration // How long tokens are valid
	CookieDomain  string        // Cookie domain
	CookieSecure  bool          // Secure cookie flag
	CookiePath    string        // Cookie path
}

// CSRFMiddleware handles CSRF protection
type CSRFMiddleware struct {
	config CSRFConfig
	tokens map[string]*CSRFToken // sessionID -> CSRFToken
	mu     sync.RWMutex
}

// NewCSRFMiddleware creates a new CSRF middleware
func NewCSRFMiddleware(config CSRFConfig) *CSRFMiddleware {
	// Set defaults
	if config.TokenDuration == 0 {
		config.TokenDuration = 24 * time.Hour
	}
	if config.CookiePath == "" {
		config.CookiePath = "/"
	}

	return &CSRFMiddleware{
		config: config,
		tokens: make(map[string]*CSRFToken),
	}
}


// GenerateToken generates a new CSRF token for a session
func (m *CSRFMiddleware) GenerateToken(sessionID string) (string, error) {
	bytes := make([]byte, CSRFTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := base64.URLEncoding.EncodeToString(bytes)

	m.mu.Lock()
	m.tokens[sessionID] = &CSRFToken{
		Token:     token,
		ExpiresAt: time.Now().Add(m.config.TokenDuration),
	}
	m.mu.Unlock()

	return token, nil
}

// ValidateToken validates a CSRF token for a session
func (m *CSRFMiddleware) ValidateToken(sessionID, token string) bool {
	m.mu.RLock()
	csrfToken, exists := m.tokens[sessionID]
	m.mu.RUnlock()

	if !exists {
		return false
	}

	// Check if token is expired
	if time.Now().After(csrfToken.ExpiresAt) {
		m.mu.Lock()
		delete(m.tokens, sessionID)
		m.mu.Unlock()
		return false
	}

	return csrfToken.Token == token
}

// DeleteToken removes a CSRF token for a session
func (m *CSRFMiddleware) DeleteToken(sessionID string) {
	m.mu.Lock()
	delete(m.tokens, sessionID)
	m.mu.Unlock()
}

// GetToken retrieves the current CSRF token for a session
func (m *CSRFMiddleware) GetToken(sessionID string) (string, bool) {
	m.mu.RLock()
	csrfToken, exists := m.tokens[sessionID]
	m.mu.RUnlock()

	if !exists || time.Now().After(csrfToken.ExpiresAt) {
		return "", false
	}

	return csrfToken.Token, true
}

// SetTokenCookie sets the CSRF token as a cookie
func (m *CSRFMiddleware) SetTokenCookie(c *gin.Context, token string) {
	c.SetCookie(
		CSRFCookieName,
		token,
		int(m.config.TokenDuration.Seconds()),
		m.config.CookiePath,
		m.config.CookieDomain,
		m.config.CookieSecure,
		false, // Not HTTPOnly - JavaScript needs to read it
	)
}

// ClearTokenCookie clears the CSRF token cookie
func (m *CSRFMiddleware) ClearTokenCookie(c *gin.Context) {
	c.SetCookie(
		CSRFCookieName,
		"",
		-1,
		m.config.CookiePath,
		m.config.CookieDomain,
		m.config.CookieSecure,
		false,
	)
}

// RequireCSRF middleware that validates CSRF tokens for state-changing requests
func (m *CSRFMiddleware) RequireCSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only validate for state-changing methods
		method := c.Request.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
		}

		// Get session ID from context (set by RequireAuth middleware)
		sessionID, exists := c.Get("user_id")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Sessão não encontrada",
				"code":    "SESSION_NOT_FOUND",
			})
			return
		}

		// Get CSRF token from header
		token := c.GetHeader(CSRFTokenHeader)
		if token == "" {
			// Also check cookie as fallback
			token, _ = c.Cookie(CSRFCookieName)
		}

		if token == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "Token CSRF ausente",
				"code":    "CSRF_TOKEN_MISSING",
			})
			return
		}

		// Validate token
		if !m.ValidateToken(sessionID.(string), token) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "Token CSRF inválido ou expirado",
				"code":    "CSRF_TOKEN_INVALID",
			})
			return
		}

		c.Next()
	}
}

// CleanupExpiredTokens removes expired CSRF tokens
func (m *CSRFMiddleware) CleanupExpiredTokens() {
	now := time.Now()
	m.mu.Lock()
	for sessionID, token := range m.tokens {
		if now.After(token.ExpiresAt) {
			delete(m.tokens, sessionID)
		}
	}
	m.mu.Unlock()
}
