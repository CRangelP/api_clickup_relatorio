package handler

import (
	"net/http"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/metrics"
	"github.com/cleberrangel/clickup-excel-api/internal/middleware"
	"github.com/cleberrangel/clickup-excel-api/internal/service"
	"github.com/gin-gonic/gin"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	authService *service.AuthService
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// Login handles user login requests
func (h *AuthHandler) Login(c *gin.Context) {
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

	// Sanitize inputs
	loginRequest.Username = middleware.SanitizeUsername(loginRequest.Username)
	loginRequest.Password = middleware.SanitizePassword(loginRequest.Password)

	// Validate username format
	if !middleware.ValidateUsername(loginRequest.Username) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Nome de usuário inválido",
			"code":    "INVALID_USERNAME",
		})
		return
	}

	// Validate credentials
	authMiddleware := h.authService.GetAuthMiddleware()
	if !authMiddleware.ValidateCredentials(loginRequest.Username, loginRequest.Password) {
		// Audit failed login
		logger.Audit(c.Request.Context(), logger.AuditEvent{
			Action:   logger.AuditActionLoginFailed,
			Username: loginRequest.Username,
			Resource: "auth",
			ClientIP: c.ClientIP(),
			Success:  false,
			Error:    "invalid credentials",
		})
		metrics.Get().IncrementLogin(false)
		
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Credenciais inválidas",
			"code":    "INVALID_CREDENTIALS",
		})
		return
	}

	// Create session
	sessionID, err := authMiddleware.CreateSession(loginRequest.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Erro interno do servidor",
			"code":    "SESSION_CREATE_ERROR",
		})
		return
	}

	// Generate CSRF token
	csrfMiddleware := h.authService.GetCSRFMiddleware()
	csrfToken, err := csrfMiddleware.GenerateToken(loginRequest.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Erro ao gerar token CSRF",
			"code":    "CSRF_TOKEN_ERROR",
		})
		return
	}

	// Set session cookie
	c.SetCookie(
		"session_id",
		sessionID,
		int(24*time.Hour.Seconds()),
		"/",
		"",
		false,
		true,
	)

	// Set CSRF token cookie (readable by JavaScript)
	csrfMiddleware.SetTokenCookie(c, csrfToken)

	// Audit successful login
	logger.Audit(c.Request.Context(), logger.AuditEvent{
		Action:   logger.AuditActionLogin,
		Username: loginRequest.Username,
		Resource: "auth",
		ClientIP: c.ClientIP(),
		Success:  true,
	})
	metrics.Get().IncrementLogin(true)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Login realizado com sucesso",
		"csrf_token": csrfToken,
		"user": gin.H{
			"username": loginRequest.Username,
		},
	})
}

// Logout handles user logout requests
func (h *AuthHandler) Logout(c *gin.Context) {
	// Get session ID from cookie
	sessionID, err := c.Cookie("session_id")
	if err == nil {
		// Delete session
		h.authService.GetAuthMiddleware().DeleteSession(sessionID)
	}

	// Get user ID for CSRF cleanup
	userID, exists := c.Get("user_id")
	username, _ := c.Get("username")
	if exists {
		h.authService.GetCSRFMiddleware().DeleteToken(userID.(string))
		
		// Audit logout
		usernameStr := ""
		if username != nil {
			usernameStr = username.(string)
		}
		logger.Audit(c.Request.Context(), logger.AuditEvent{
			Action:   logger.AuditActionLogout,
			UserID:   userID.(string),
			Username: usernameStr,
			Resource: "auth",
			ClientIP: c.ClientIP(),
			Success:  true,
		})
	}

	// Clear session cookie
	c.SetCookie(
		"session_id",
		"",
		-1,
		"/",
		"",
		false,
		true,
	)

	// Clear CSRF cookie
	h.authService.GetCSRFMiddleware().ClearTokenCookie(c)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logout realizado com sucesso",
	})
}

// GetCurrentUser returns information about the currently authenticated user
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	// Get user info from context (set by RequireAuth middleware)
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Sessão não encontrada",
			"code":    "SESSION_NOT_FOUND",
		})
		return
	}

	userID, _ := c.Get("user_id")

	// Get or regenerate CSRF token
	csrfMiddleware := h.authService.GetCSRFMiddleware()
	csrfToken, exists := csrfMiddleware.GetToken(userID.(string))
	if !exists {
		// Generate new token if not exists
		var err error
		csrfToken, err = csrfMiddleware.GenerateToken(userID.(string))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Erro ao gerar token CSRF",
				"code":    "CSRF_TOKEN_ERROR",
			})
			return
		}
		csrfMiddleware.SetTokenCookie(c, csrfToken)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"csrf_token": csrfToken,
		"user": gin.H{
			"username": username.(string),
			"user_id":  userID.(string),
		},
	})
}

// CreateUser creates a new user (admin endpoint)
func (h *AuthHandler) CreateUser(c *gin.Context) {
	var request struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Dados inválidos",
			"details": err.Error(),
			"code":    "INVALID_INPUT",
		})
		return
	}

	// Sanitize inputs
	request.Username = middleware.SanitizeUsername(request.Username)
	request.Password = middleware.SanitizePassword(request.Password)

	// Validate username format
	if !middleware.ValidateUsername(request.Username) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Nome de usuário inválido (use apenas letras, números, _ e -)",
			"code":    "INVALID_USERNAME",
		})
		return
	}

	// Validate password
	if !middleware.ValidatePassword(request.Password) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Senha deve ter entre 6 e 128 caracteres",
			"code":    "INVALID_PASSWORD",
		})
		return
	}

	err := h.authService.CreateUser(request.Username, request.Password)
	if err != nil {
		if err == service.ErrUserAlreadyExists {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Usuário já existe",
				"code":    "USER_ALREADY_EXISTS",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Erro interno do servidor",
			"code":    "INTERNAL_ERROR",
		})
		return
	}

	// Audit user creation
	logger.Audit(c.Request.Context(), logger.AuditEvent{
		Action:   logger.AuditActionUserCreate,
		Username: request.Username,
		Resource: "user",
		ClientIP: c.ClientIP(),
		Success:  true,
	})

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Usuário criado com sucesso",
		"user": gin.H{
			"username": request.Username,
		},
	})
}

// UpdatePassword updates the current user's password
func (h *AuthHandler) UpdatePassword(c *gin.Context) {
	var request struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Dados inválidos",
			"details": err.Error(),
			"code":    "INVALID_INPUT",
		})
		return
	}

	// Sanitize inputs
	request.CurrentPassword = middleware.SanitizePassword(request.CurrentPassword)
	request.NewPassword = middleware.SanitizePassword(request.NewPassword)

	// Validate new password
	if !middleware.ValidatePassword(request.NewPassword) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Nova senha deve ter entre 6 e 128 caracteres",
			"code":    "INVALID_PASSWORD",
		})
		return
	}

	// Get current user from context
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Usuário não autenticado",
			"code":    "NOT_AUTHENTICATED",
		})
		return
	}

	// Validate current password
	if !h.authService.ValidateCredentials(username.(string), request.CurrentPassword) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Senha atual incorreta",
			"code":    "INVALID_CURRENT_PASSWORD",
		})
		return
	}

	// Update password
	err := h.authService.UpdateUserPassword(username.(string), request.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Erro interno do servidor",
			"code":    "INTERNAL_ERROR",
		})
		return
	}

	// Audit password change
	logger.Audit(c.Request.Context(), logger.AuditEvent{
		Action:   logger.AuditActionPasswordChange,
		Username: username.(string),
		Resource: "user",
		ClientIP: c.ClientIP(),
		Success:  true,
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Senha atualizada com sucesso",
	})
}