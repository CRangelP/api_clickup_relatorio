package handler

import (
	"net/http"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/metrics"
	"github.com/cleberrangel/clickup-excel-api/internal/middleware"
	"github.com/cleberrangel/clickup-excel-api/internal/model"
	"github.com/cleberrangel/clickup-excel-api/internal/service"
	"github.com/cleberrangel/clickup-excel-api/internal/websocket"
	"github.com/gin-gonic/gin"
)

// MetadataHandler handles metadata synchronization requests
type MetadataHandler struct {
	metadataService *service.MetadataService
	wsHub           *websocket.Hub
}

// NewMetadataHandler creates a new metadata handler
func NewMetadataHandler(metadataService *service.MetadataService, wsHub *websocket.Hub) *MetadataHandler {
	return &MetadataHandler{
		metadataService: metadataService,
		wsHub:           wsHub,
	}
}

// SyncMetadata triggers metadata synchronization from ClickUp
// @Summary      Sync metadata from ClickUp
// @Description  Fetches workspaces, spaces, folders, lists and custom fields from ClickUp
// @Tags         metadata
// @Accept       json
// @Produce      json
// @Security     BasicAuth
// @Param        request body SyncMetadataRequest true "ClickUp token"
// @Success      200 {object} model.Response
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /api/web/metadata/sync [post]
func (h *MetadataHandler) SyncMetadata(c *gin.Context) {
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
	
	var req SyncMetadataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "payload inválido",
			Details: err.Error(),
		})
		return
	}
	
	if req.Token == "" {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "token é obrigatório",
		})
		return
	}

	// Sanitize and validate token
	req.Token = middleware.SanitizeToken(req.Token)
	if !middleware.ValidateToken(req.Token) {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "formato de token inválido",
			Details: "o token deve começar com 'pk_' e conter apenas caracteres alfanuméricos",
		})
		return
	}
	
	log.Info().Str("user_id", userID.(string)).Msg("Iniciando sincronização de metadados")
	
	// Send initial progress via WebSocket
	h.wsHub.SendToUser(userID.(string), websocket.Message{
		Type: "metadata_sync",
		Data: map[string]interface{}{
			"status":  "started",
			"message": "Iniciando sincronização...",
		},
	})
	
	// Get username for audit
	username, _ := c.Get("username")
	usernameStr := ""
	if username != nil {
		usernameStr = username.(string)
	}

	// Sync metadata
	err := h.metadataService.SyncMetadata(c.Request.Context(), userID.(string), req.Token)
	if err != nil {
		log.Error().Err(err).Str("user_id", userID.(string)).Msg("Erro na sincronização de metadados")
		
		// Audit failed sync
		logger.Audit(c.Request.Context(), logger.AuditEvent{
			Action:   logger.AuditActionMetadataSync,
			UserID:   userID.(string),
			Username: usernameStr,
			Resource: "metadata",
			ClientIP: c.ClientIP(),
			Success:  false,
			Error:    err.Error(),
		})
		metrics.Get().IncrementMetadataSync(false)
		
		// Send error via WebSocket
		h.wsHub.SendToUser(userID.(string), websocket.Message{
			Type: "metadata_sync",
			Data: map[string]interface{}{
				"status":  "error",
				"message": err.Error(),
			},
		})
		
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Success: false,
			Error:   "erro na sincronização de metadados",
			Details: err.Error(),
		})
		return
	}
	
	// Send completion via WebSocket
	h.wsHub.SendToUser(userID.(string), websocket.Message{
		Type: "metadata_sync",
		Data: map[string]interface{}{
			"status":  "completed",
			"message": "Sincronização concluída com sucesso",
		},
	})
	
	// Audit successful sync
	logger.Audit(c.Request.Context(), logger.AuditEvent{
		Action:   logger.AuditActionMetadataSync,
		UserID:   userID.(string),
		Username: usernameStr,
		Resource: "metadata",
		ClientIP: c.ClientIP(),
		Success:  true,
	})
	metrics.Get().IncrementMetadataSync(true)

	log.Info().Str("user_id", userID.(string)).Msg("Sincronização de metadados concluída")
	
	c.JSON(http.StatusOK, model.Response{
		Success: true,
	})
}

// GetHierarchy returns hierarchical metadata for the UI
// @Summary      Get hierarchical metadata
// @Description  Returns workspaces, spaces, folders, lists and custom fields in hierarchical structure
// @Tags         metadata
// @Produce      json
// @Security     BasicAuth
// @Success      200 {object} HierarchyResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /api/web/metadata/hierarchy [get]
func (h *MetadataHandler) GetHierarchy(c *gin.Context) {
	log := logger.FromGin(c)
	
	data, err := h.metadataService.GetHierarchicalData(c.Request.Context())
	if err != nil {
		log.Error().Err(err).Msg("Erro ao buscar dados hierárquicos")
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Success: false,
			Error:   "erro ao buscar dados hierárquicos",
			Details: err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, HierarchyResponse{
		Success: true,
		Data:    data,
	})
}

// SyncMetadataRequest represents the request to sync metadata
type SyncMetadataRequest struct {
	Token string `json:"token" binding:"required"`
}

// HierarchyResponse represents the response for hierarchical data
type HierarchyResponse struct {
	Success bool                      `json:"success"`
	Data    *service.HierarchicalData `json:"data"`
}
