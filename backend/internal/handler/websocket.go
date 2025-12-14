package handler

import (
	"net/http"

	"github.com/cleberrangel/clickup-excel-api/internal/websocket"
	"github.com/gin-gonic/gin"
)

// WebSocketHandler handles WebSocket-related HTTP requests
type WebSocketHandler struct {
	hub *websocket.Hub
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(hub *websocket.Hub) *WebSocketHandler {
	return &WebSocketHandler{
		hub: hub,
	}
}

// HandleConnection handles WebSocket connection upgrades
func (h *WebSocketHandler) HandleConnection(c *gin.Context) {
	h.hub.ServeWS(c)
}

// GetConnectionStats returns WebSocket connection statistics
func (h *WebSocketHandler) GetConnectionStats(c *gin.Context) {
	stats := map[string]interface{}{
		"total_connections": h.hub.GetConnectionCount(),
		"connected_users":   h.hub.GetConnectedUsers(),
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// GetUserConnections returns connection information for the current user
func (h *WebSocketHandler) GetUserConnections(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Usuário não autenticado",
			"code":    "USER_NOT_AUTHENTICATED",
		})
		return
	}

	connectionCount := h.hub.GetUserConnectionCount(userID.(string))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": map[string]interface{}{
			"user_id":          userID,
			"connection_count": connectionCount,
			"is_connected":     connectionCount > 0,
		},
	})
}

// SendTestMessage sends a test message to the current user (for debugging)
func (h *WebSocketHandler) SendTestMessage(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Usuário não autenticado",
			"code":    "USER_NOT_AUTHENTICATED",
		})
		return
	}

	var request struct {
		Message string `json:"message" binding:"required"`
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

	// Send test message
	testMessage := websocket.Message{
		Type: "test",
		Data: map[string]string{
			"message": request.Message,
		},
	}

	h.hub.SendToUser(userID.(string), testMessage)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Mensagem de teste enviada",
	})
}