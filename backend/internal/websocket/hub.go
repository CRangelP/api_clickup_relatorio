package websocket

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/metrics"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// Hub maintains the set of active clients and broadcasts messages to the clients
type Hub struct {
	// Registered clients by user ID
	clients map[string]map[*Client]bool

	// Inbound messages from the clients
	broadcast chan []byte

	// Register requests from the clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Mutex for thread-safe operations
	mutex sync.RWMutex

	// Logger
	logger *zerolog.Logger
}

// Client is a middleman between the websocket connection and the hub
type Client struct {
	// The websocket connection
	conn *websocket.Conn

	// Buffered channel of outbound messages
	Send chan []byte

	// User identification
	UserID   string
	Username string

	// Hub reference
	Hub *Hub

	// Connection metadata
	ConnectedAt time.Time
	LastPing    time.Time
}

// ProgressUpdate represents a progress update message
type ProgressUpdate struct {
	Type          string    `json:"type"`
	JobID         int       `json:"job_id,omitempty"`
	Status        string    `json:"status"`
	ProcessedRows int       `json:"processed_rows,omitempty"`
	TotalRows     int       `json:"total_rows,omitempty"`
	SuccessCount  int       `json:"success_count,omitempty"`
	ErrorCount    int       `json:"error_count,omitempty"`
	Message       string    `json:"message,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
	Progress      float64   `json:"progress,omitempty"` // 0-100 percentage
}

// Message represents a generic WebSocket message
type Message struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections from any origin for now
		// In production, you should validate the origin
		return true
	},
}

// NewHub creates a new WebSocket hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]map[*Client]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		logger:     logger.Global(),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)

		case message := <-h.broadcast:
			h.broadcastMessage(message)
		}
	}
}

// registerClient registers a new client
func (h *Hub) registerClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.clients[client.UserID] == nil {
		h.clients[client.UserID] = make(map[*Client]bool)
	}
	h.clients[client.UserID][client] = true

	// Track metrics
	metrics.Get().IncrementWSConnection()

	h.logger.Info().
		Str("user_id", client.UserID).
		Str("username", client.Username).
		Int("user_connections", len(h.clients[client.UserID])).
		Msg("WebSocket client registered")

	// Send welcome message
	welcome := Message{
		Type:      "connection",
		Data:      map[string]string{"status": "connected"},
		Timestamp: time.Now(),
	}
	client.SendMessage(welcome)
}

// unregisterClient unregisters a client
func (h *Hub) unregisterClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if clients, ok := h.clients[client.UserID]; ok {
		if _, ok := clients[client]; ok {
			delete(clients, client)
			close(client.Send)

			// Track metrics
			metrics.Get().DecrementWSConnection()

			// Remove user entry if no more clients
			if len(clients) == 0 {
				delete(h.clients, client.UserID)
			}

			h.logger.Info().
				Str("user_id", client.UserID).
				Str("username", client.Username).
				Int("remaining_connections", len(clients)).
				Msg("WebSocket client unregistered")
		}
	}
}

// broadcastMessage broadcasts a message to all connected clients
func (h *Hub) broadcastMessage(message []byte) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	for userID, clients := range h.clients {
		for client := range clients {
			select {
			case client.Send <- message:
			default:
				h.logger.Warn().
					Str("user_id", userID).
					Msg("Failed to send message to client, closing connection")
				close(client.Send)
				delete(clients, client)
				if len(clients) == 0 {
					delete(h.clients, userID)
				}
			}
		}
	}
}

// SendToUser sends a message to all connections of a specific user
func (h *Hub) SendToUser(userID string, message interface{}) {
	h.mutex.RLock()
	clients, exists := h.clients[userID]
	h.mutex.RUnlock()

	if !exists {
		h.logger.Debug().
			Str("user_id", userID).
			Msg("No WebSocket connections found for user")
		return
	}

	data, err := json.Marshal(message)
	if err != nil {
		h.logger.Error().
			Err(err).
			Str("user_id", userID).
			Msg("Failed to marshal message for user")
		return
	}

	h.mutex.RLock()
	defer h.mutex.RUnlock()

	for client := range clients {
		select {
		case client.Send <- data:
			// Track outgoing message
			metrics.Get().IncrementWSMessageOut()
		default:
			h.logger.Warn().
				Str("user_id", userID).
				Msg("Failed to send message to user client, closing connection")
			close(client.Send)
			delete(clients, client)
			metrics.Get().DecrementWSConnection()
		}
	}

	// Clean up empty user entry
	if len(clients) == 0 {
		delete(h.clients, userID)
	}
}

// SendProgress sends a progress update to a specific user
func (h *Hub) SendProgress(userID string, progress ProgressUpdate) {
	progress.Type = "progress"
	progress.Timestamp = time.Now()
	
	// Calculate progress percentage if total rows is available
	if progress.TotalRows > 0 {
		progress.Progress = float64(progress.ProcessedRows) / float64(progress.TotalRows) * 100
	}

	h.SendToUser(userID, progress)
}

// GetConnectedUsers returns a list of currently connected user IDs
func (h *Hub) GetConnectedUsers() []string {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	users := make([]string, 0, len(h.clients))
	for userID := range h.clients {
		users = append(users, userID)
	}
	return users
}

// GetConnectionCount returns the total number of active connections
func (h *Hub) GetConnectionCount() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	count := 0
	for _, clients := range h.clients {
		count += len(clients)
	}
	return count
}

// GetUserConnectionCount returns the number of connections for a specific user
func (h *Hub) GetUserConnectionCount(userID string) int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if clients, exists := h.clients[userID]; exists {
		return len(clients)
	}
	return 0
}

// RegisterClient is a public method to register a client (for testing)
func (h *Hub) RegisterClient(client *Client) {
	h.registerClient(client)
}

// UnregisterClient is a public method to unregister a client (for testing)
func (h *Hub) UnregisterClient(client *Client) {
	h.unregisterClient(client)
}