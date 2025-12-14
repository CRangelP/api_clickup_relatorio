package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// Session represents a user session
type Session struct {
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// BasicAuthConfig contains configuration for basic authentication
type BasicAuthConfig struct {
	Users           map[string]string // username -> password hash
	SessionDuration time.Duration     // session duration
	CookieName      string           // session cookie name
	CookieDomain    string           // cookie domain
	CookieSecure    bool             // secure cookie flag
	CookieHTTPOnly  bool             // httponly cookie flag
}

// BasicAuthMiddleware handles basic authentication with sessions
type BasicAuthMiddleware struct {
	config   BasicAuthConfig
	sessions map[string]*Session // sessionID -> Session
}

// AddUser adds a user to the middleware's user map
func (m *BasicAuthMiddleware) AddUser(username, passwordHash string) {
	m.config.Users[username] = passwordHash
}

// RemoveUser removes a user from the middleware's user map
func (m *BasicAuthMiddleware) RemoveUser(username string) {
	delete(m.config.Users, username)
}

// ClearUsers clears all users from the middleware
func (m *BasicAuthMiddleware) ClearUsers() {
	m.config.Users = make(map[string]string)
}

// NewBasicAuthMiddleware creates a new basic auth middleware
func NewBasicAuthMiddleware(config BasicAuthConfig) *BasicAuthMiddleware {
	// Set defaults
	if config.SessionDuration == 0 {
		config.SessionDuration = 24 * time.Hour
	}
	if config.CookieName == "" {
		config.CookieName = "session_id"
	}
	if config.Users == nil {
		config.Users = make(map[string]string)
	}

	return &BasicAuthMiddleware{
		config:   config,
		sessions: make(map[string]*Session),
	}
}

// HashPassword creates a bcrypt hash of the password
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword compares a password with its hash
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// generateSessionID generates a secure random session ID
func (m *BasicAuthMiddleware) generateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// CreateSession creates a new session for the user
func (m *BasicAuthMiddleware) CreateSession(username string) (string, error) {
	sessionID, err := m.generateSessionID()
	if err != nil {
		return "", err
	}

	session := &Session{
		UserID:    username, // Using username as userID for simplicity
		Username:  username,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(m.config.SessionDuration),
	}

	m.sessions[sessionID] = session
	return sessionID, nil
}

// GetSession retrieves a session by ID
func (m *BasicAuthMiddleware) GetSession(sessionID string) (*Session, bool) {
	session, exists := m.sessions[sessionID]
	if !exists {
		return nil, false
	}

	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		delete(m.sessions, sessionID)
		return nil, false
	}

	return session, true
}

// DeleteSession removes a session
func (m *BasicAuthMiddleware) DeleteSession(sessionID string) {
	delete(m.sessions, sessionID)
}

// ValidateCredentials checks if username and password are valid
func (m *BasicAuthMiddleware) ValidateCredentials(username, password string) bool {
	hash, exists := m.config.Users[username]
	if !exists {
		return false
	}
	return CheckPassword(password, hash)
}

// RequireAuth middleware that requires authentication
func (m *BasicAuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check for session cookie
		sessionID, err := c.Cookie(m.config.CookieName)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Sessão não encontrada",
				"code":    "SESSION_NOT_FOUND",
			})
			return
		}

		// Validate session
		session, valid := m.GetSession(sessionID)
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

// Login handles user login
func (m *BasicAuthMiddleware) Login(c *gin.Context) {
	var loginRequest struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&loginRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Dados de login inválidos",
			"details": err.Error(),
			"code":    "INVALID_INPUT",
		})
		return
	}

	// Validate credentials
	if !m.ValidateCredentials(loginRequest.Username, loginRequest.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Credenciais inválidas",
			"code":    "INVALID_CREDENTIALS",
		})
		return
	}

	// Create session
	sessionID, err := m.CreateSession(loginRequest.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Erro interno do servidor",
			"code":    "SESSION_CREATE_ERROR",
		})
		return
	}

	// Set session cookie
	c.SetCookie(
		m.config.CookieName,
		sessionID,
		int(m.config.SessionDuration.Seconds()),
		"/",
		m.config.CookieDomain,
		m.config.CookieSecure,
		m.config.CookieHTTPOnly,
	)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Login realizado com sucesso",
		"user": gin.H{
			"username": loginRequest.Username,
		},
	})
}

// Logout handles user logout
func (m *BasicAuthMiddleware) Logout(c *gin.Context) {
	// Get session ID from cookie
	sessionID, err := c.Cookie(m.config.CookieName)
	if err == nil {
		// Delete session
		m.DeleteSession(sessionID)
	}

	// Clear cookie
	c.SetCookie(
		m.config.CookieName,
		"",
		-1,
		"/",
		m.config.CookieDomain,
		m.config.CookieSecure,
		m.config.CookieHTTPOnly,
	)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logout realizado com sucesso",
	})
}

// CleanupExpiredSessions removes expired sessions (should be called periodically)
func (m *BasicAuthMiddleware) CleanupExpiredSessions() {
	now := time.Now()
	for sessionID, session := range m.sessions {
		if now.After(session.ExpiresAt) {
			delete(m.sessions, sessionID)
		}
	}
}