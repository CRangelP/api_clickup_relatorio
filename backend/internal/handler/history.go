package handler

import (
	"net/http"
	"strconv"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/repository"
	"github.com/cleberrangel/clickup-excel-api/internal/service"
	"github.com/gin-gonic/gin"
)

// HistoryHandler handles operation history HTTP requests
type HistoryHandler struct {
	historyService *service.HistoryService
}

// NewHistoryHandler creates a new history handler
func NewHistoryHandler(historyService *service.HistoryService) *HistoryHandler {
	return &HistoryHandler{
		historyService: historyService,
	}
}

// HistoryResponse represents an operation history entry in API responses
type HistoryResponse struct {
	ID            int                    `json:"id"`
	UserID        string                 `json:"user_id"`
	OperationType string                 `json:"operation_type"`
	Title         string                 `json:"title"`
	Status        string                 `json:"status"`
	Details       map[string]interface{} `json:"details,omitempty"`
	CreatedAt     string                 `json:"created_at"`
}

// DeleteHistoryRequest represents the request body for deleting history
type DeleteHistoryRequest struct {
	Confirm bool `json:"confirm" binding:"required"`
}

// ListHistory returns operation history for the current user (last 50)
// @Summary List operation history
// @Description Returns the last 50 operation history entries for the authenticated user
// @Tags history
// @Produce json
// @Success 200 {object} []HistoryResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/web/history [get]
func (h *HistoryHandler) ListHistory(c *gin.Context) {
	log := logger.Get(c.Request.Context())

	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Usuário não autenticado",
		})
		return
	}

	history, err := h.historyService.GetHistoryByUser(userID.(string))
	if err != nil {
		log.Error().Err(err).Str("user_id", userID.(string)).Msg("Erro ao listar histórico")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Erro ao listar histórico",
			"details": err.Error(),
		})
		return
	}

	// Convert to response format
	responses := make([]HistoryResponse, len(history))
	for i := range history {
		responses[i] = toHistoryResponse(&history[i])
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    responses,
	})
}

// GetHistory returns a specific operation history entry by ID
// @Summary Get operation history by ID
// @Description Returns a specific operation history entry by its ID
// @Tags history
// @Produce json
// @Param id path int true "History ID"
// @Success 200 {object} HistoryResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/web/history/{id} [get]
func (h *HistoryHandler) GetHistory(c *gin.Context) {
	log := logger.Get(c.Request.Context())

	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Usuário não autenticado",
		})
		return
	}

	// Parse history ID
	historyIDStr := c.Param("id")
	historyID, err := strconv.Atoi(historyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "ID do histórico inválido",
		})
		return
	}

	history, err := h.historyService.GetHistoryByID(historyID)
	if err != nil {
		if err == service.ErrHistoryNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Registro de histórico não encontrado",
			})
			return
		}
		log.Error().Err(err).Int("history_id", historyID).Msg("Erro ao buscar histórico")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Erro ao buscar histórico",
			"details": err.Error(),
		})
		return
	}

	// Verify history belongs to user
	if history.UserID != userID.(string) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Registro de histórico não encontrado",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    toHistoryResponse(history),
	})
}


// DeleteAllHistory deletes all operation history for the current user
// @Summary Delete all operation history
// @Description Deletes all operation history entries for the authenticated user (requires double confirmation)
// @Tags history
// @Accept json
// @Produce json
// @Param request body DeleteHistoryRequest true "Confirmation request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/web/history [delete]
func (h *HistoryHandler) DeleteAllHistory(c *gin.Context) {
	log := logger.Get(c.Request.Context())

	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Usuário não autenticado",
		})
		return
	}

	var req DeleteHistoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warn().Err(err).Msg("Erro ao fazer bind do request")
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Dados inválidos",
			"details": err.Error(),
		})
		return
	}

	// Require explicit confirmation
	if !req.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Confirmação necessária para deletar histórico",
		})
		return
	}

	err := h.historyService.DeleteAllHistoryByUser(userID.(string))
	if err != nil {
		log.Error().Err(err).Str("user_id", userID.(string)).Msg("Erro ao deletar histórico")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Erro ao deletar histórico",
			"details": err.Error(),
		})
		return
	}

	// Get username for audit
	username, _ := c.Get("username")
	usernameStr := ""
	if username != nil {
		usernameStr = username.(string)
	}

	// Audit history clear
	logger.Audit(c.Request.Context(), logger.AuditEvent{
		Action:   logger.AuditActionHistoryClear,
		UserID:   userID.(string),
		Username: usernameStr,
		Resource: "history",
		ClientIP: c.ClientIP(),
		Success:  true,
	})

	log.Info().Str("user_id", userID.(string)).Msg("Histórico deletado com sucesso")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Histórico deletado com sucesso",
	})
}

// toHistoryResponse converts a repository.OperationHistory to HistoryResponse
func toHistoryResponse(history *repository.OperationHistory) HistoryResponse {
	return HistoryResponse{
		ID:            history.ID,
		UserID:        history.UserID,
		OperationType: history.OperationType,
		Title:         history.Title,
		Status:        history.Status,
		Details:       history.Details,
		CreatedAt:     history.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
