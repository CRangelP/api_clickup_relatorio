package websocket

import (
	"net/http"
	"net/url"

	"github.com/cleberrangel/clickup-excel-api/internal/middleware"
	"github.com/gin-gonic/gin"
)

// AuthMiddleware creates a WebSocket authentication middleware
// It checks for session authentication via cookie or query parameter
func AuthMiddleware(authMiddleware *middleware.BasicAuthMiddleware) gin.HandlerFunc {
	return func(c *gin.Context) {
		var sessionID string
		var err error

		// First try to get session from cookie
		sessionID, err = c.Cookie("session_id")
		if err != nil {
			// If cookie is not available, try query parameter (for WebSocket connections)
			sessionID = c.Query("session_id")
			if sessionID == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"error":   "Sessão não encontrada",
					"code":    "SESSION_NOT_FOUND",
				})
				return
			}
		}

		// Validate session
		session, valid := authMiddleware.GetSession(sessionID)
		if !valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Sessão inválida ou expirada",
				"code":    "SESSION_INVALID",
			})
			return
		}

		// Add session info to context
		c.Set("session", session)
		c.Set("user_id", session.UserID)
		c.Set("username", session.Username)

		c.Next()
	}
}

// ExtractSessionFromURL extracts session ID from WebSocket URL
// This is useful for client-side WebSocket connections that need to pass session info
func ExtractSessionFromURL(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	return parsedURL.Query().Get("session_id"), nil
}

// BuildWebSocketURL builds a WebSocket URL with session authentication
func BuildWebSocketURL(baseURL, sessionID string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}

	q := u.Query()
	q.Set("session_id", sessionID)
	u.RawQuery = q.Encode()

	return u.String()
}