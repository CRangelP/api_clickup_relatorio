package websocket

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ServeWS handles websocket requests from the peer
func (h *Hub) ServeWS(c *gin.Context) {
	// Get user information from context (set by auth middleware)
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Usuário não autenticado",
			"code":    "USER_NOT_AUTHENTICATED",
		})
		return
	}

	username, _ := c.Get("username")

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error().
			Err(err).
			Str("user_id", userID.(string)).
			Msg("Failed to upgrade WebSocket connection")
		return
	}

	client := &Client{
		conn:        conn,
		Send:        make(chan []byte, 256),
		UserID:      userID.(string),
		Username:    username.(string),
		Hub:         h,
		ConnectedAt: time.Now(),
		LastPing:    time.Now(),
	}

	client.Hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines
	go client.writePump()
	go client.readPump()
}

// readPump pumps messages from the websocket connection to the hub
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		c.Hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.LastPing = time.Now()
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.Hub.logger.Error().
					Err(err).
					Str("user_id", c.UserID).
					Msg("WebSocket connection closed unexpectedly")
			}
			break
		}

		// Handle incoming messages from client
		c.handleMessage(message)
	}
}

// writePump pumps messages from the hub to the websocket connection
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current websocket message
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes incoming messages from the client
func (c *Client) handleMessage(data []byte) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		c.Hub.logger.Error().
			Err(err).
			Str("user_id", c.UserID).
			Msg("Failed to unmarshal client message")
		return
	}

	switch msg.Type {
	case "ping":
		// Respond to ping with pong
		pong := Message{
			Type:      "pong",
			Timestamp: time.Now(),
		}
		c.SendMessage(pong)

	case "subscribe":
		// Handle subscription requests (for future use)
		c.Hub.logger.Debug().
			Str("user_id", c.UserID).
			Str("message_type", msg.Type).
			Msg("Client subscription request received")

	default:
		c.Hub.logger.Debug().
			Str("user_id", c.UserID).
			Str("message_type", msg.Type).
			Msg("Unknown message type received from client")
	}
}

// SendMessage sends a message to this specific client
func (c *Client) SendMessage(message interface{}) {
	data, err := json.Marshal(message)
	if err != nil {
		c.Hub.logger.Error().
			Err(err).
			Str("user_id", c.UserID).
			Msg("Failed to marshal message for client")
		return
	}

	select {
	case c.Send <- data:
	default:
		c.Hub.logger.Warn().
			Str("user_id", c.UserID).
			Msg("Client send channel is full, closing connection")
		close(c.Send)
	}
}

// IsConnected returns true if the client connection is still active
func (c *Client) IsConnected() bool {
	return c.conn != nil
}

// GetConnectionInfo returns information about this client connection
func (c *Client) GetConnectionInfo() map[string]interface{} {
	return map[string]interface{}{
		"user_id":      c.UserID,
		"username":     c.Username,
		"connected_at": c.ConnectedAt,
		"last_ping":    c.LastPing,
	}
}