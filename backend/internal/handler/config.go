package handler

import (
	"net/http"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/model"
	"github.com/cleberrangel/clickup-excel-api/internal/repository"
	"github.com/gin-gonic/gin"
)

// ConfigHandler handles user configuration requests
type ConfigHandler struct {
	configRepo *repository.ConfigRepository
}

// NewConfigHandler creates a new config handler
func NewConfigHandler(configRepo *repository.ConfigRepository) *ConfigHandler {
	return &ConfigHandler{
		configRepo: configRepo,
	}
}

// GetConfig returns the user's configuration
// @Summary      Get user configuration
// @Description  Returns the current user's configuration settings
// @Tags         config
// @Produce      json
// @Security     BasicAuth
// @Success      200 {object} ConfigResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /api/web/config [get]
func (h *ConfigHandler) GetConfig(c *gin.Context) {
	log := logger.FromGin(c)
	
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, model.ErrorResponse{
			Success: false,
			Error:   "usuário não autenticado",
		})
		return
	}
	
	config, err := h.configRepo.GetUserConfig(userID.(string))
	if err != nil {
		log.Error().Err(err).Str("user_id", userID.(string)).Msg("Erro ao buscar configuração")
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Success: false,
			Error:   "erro ao buscar configuração",
			Details: err.Error(),
		})
		return
	}
	
	// Return default config if none exists
	if config == nil {
		c.JSON(http.StatusOK, ConfigResponse{
			Success: true,
			Data: ConfigData{
				HasToken:           false,
				RateLimitPerMinute: 2000, // Default value
			},
		})
		return
	}
	
	c.JSON(http.StatusOK, ConfigResponse{
		Success: true,
		Data: ConfigData{
			HasToken:           config.ClickUpTokenEncrypted != "",
			RateLimitPerMinute: config.RateLimitPerMinute,
		},
	})
}

// SaveConfig saves the user's configuration
// @Summary      Save user configuration
// @Description  Saves the user's configuration settings
// @Tags         config
// @Accept       json
// @Produce      json
// @Security     BasicAuth
// @Param        request body SaveConfigRequest true "Configuration to save"
// @Success      200 {object} model.Response
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /api/web/config [post]
func (h *ConfigHandler) SaveConfig(c *gin.Context) {
	log := logger.FromGin(c)
	
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, model.ErrorResponse{
			Success: false,
			Error:   "usuário não autenticado",
		})
		return
	}
	
	var req SaveConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "payload inválido",
			Details: err.Error(),
		})
		return
	}
	
	// Validate rate limit range (10-10000)
	if req.RateLimitPerMinute != nil {
		if *req.RateLimitPerMinute < 10 || *req.RateLimitPerMinute > 10000 {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Success: false,
				Error:   "rate limit inválido",
				Details: "rate limit deve estar entre 10 e 10000",
			})
			return
		}
		
		if err := h.configRepo.UpdateRateLimit(userID.(string), *req.RateLimitPerMinute); err != nil {
			log.Error().Err(err).Str("user_id", userID.(string)).Msg("Erro ao atualizar rate limit")
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Success: false,
				Error:   "erro ao salvar configuração",
				Details: err.Error(),
			})
			return
		}
	}
	
	// Get username for audit
	username, _ := c.Get("username")
	usernameStr := ""
	if username != nil {
		usernameStr = username.(string)
	}

	// Audit config update
	logger.Audit(c.Request.Context(), logger.AuditEvent{
		Action:   logger.AuditActionConfigUpdate,
		UserID:   userID.(string),
		Username: usernameStr,
		Resource: "config",
		ClientIP: c.ClientIP(),
		Success:  true,
		Details: map[string]interface{}{
			"rate_limit_per_minute": req.RateLimitPerMinute,
		},
	})

	log.Info().Str("user_id", userID.(string)).Msg("Configuração salva com sucesso")
	
	c.JSON(http.StatusOK, model.Response{
		Success: true,
	})
}

// ConfigResponse represents the response for getting configuration
type ConfigResponse struct {
	Success bool       `json:"success"`
	Data    ConfigData `json:"data"`
}

// ConfigData contains the user's configuration
type ConfigData struct {
	HasToken           bool `json:"has_token"`
	RateLimitPerMinute int  `json:"rate_limit_per_minute"`
}

// SaveConfigRequest represents the request to save configuration
type SaveConfigRequest struct {
	RateLimitPerMinute *int `json:"rate_limit_per_minute,omitempty"`
}
