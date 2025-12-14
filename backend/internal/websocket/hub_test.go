package websocket

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// drainWelcomeMessage drains the welcome message sent during client registration
func drainWelcomeMessage(client *Client) {
	select {
	case <-client.Send:
		// Welcome message drained
	case <-time.After(100 * time.Millisecond):
		// No welcome message (shouldn't happen)
	}
}

// **Feature: clickup-field-updater, Property 8: WebSocket progress consistency**
// **Validates: Requirements 5.1, 5.2, 8.2, 11.2, 11.3, 11.4**
// For any active operation, the system should maintain WebSocket connection,
// send progress updates, and preserve connection state across tab navigation
func TestWebSocketProgressConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.MaxSize = 1000

	properties := gopter.NewProperties(parameters)

	// Property: For any valid progress update, the WebSocket hub should deliver
	// the message with correct data and calculated progress percentage
	properties.Property("progress updates are delivered with correct data", prop.ForAll(
		func(jobID int, totalRows int, processedRows int, userIDLen int) bool {
			// Generate userID from length (ensures non-empty)
			userID := string(make([]byte, userIDLen))
			for i := range userID {
				userID = userID[:i] + "u" + userID[i+1:]
			}
			if userID == "" {
				userID = "user"
			}

			// Create hub
			hub := NewHub()

			// Create client with buffered channel
			client := &Client{
				UserID:      userID,
				Username:    userID,
				Send:        make(chan []byte, 10),
				Hub:         hub,
				ConnectedAt: time.Now(),
				LastPing:    time.Now(),
			}

			// Register client
			hub.registerClient(client)

			// Drain the welcome message sent during registration
			drainWelcomeMessage(client)

			// Calculate expected counts
			successCount := processedRows / 2
			errorCount := processedRows - successCount

			// Create progress update
			progress := ProgressUpdate{
				JobID:         jobID,
				Status:        "processing",
				ProcessedRows: processedRows,
				TotalRows:     totalRows,
				SuccessCount:  successCount,
				ErrorCount:    errorCount,
				Message:       "Processing data",
			}

			// Send progress
			hub.SendProgress(userID, progress)

			// Verify message received
			select {
			case msg := <-client.Send:
				var received ProgressUpdate
				err := json.Unmarshal(msg, &received)
				if err != nil {
					return false
				}

				// Check data consistency
				if received.JobID != jobID {
					return false
				}
				if received.ProcessedRows != processedRows {
					return false
				}
				if received.TotalRows != totalRows {
					return false
				}
				if received.Type != "progress" {
					return false
				}
				if received.SuccessCount != successCount {
					return false
				}
				if received.ErrorCount != errorCount {
					return false
				}

				// Check progress calculation
				if totalRows > 0 {
					expectedProgress := float64(processedRows) / float64(totalRows) * 100
					if received.Progress != expectedProgress {
						return false
					}
				}

				return true

			case <-time.After(100 * time.Millisecond):
				return false
			}
		},
		gen.IntRange(1, 10000),  // jobID: 1 to 10000
		gen.IntRange(1, 10000),  // totalRows: 1 to 10000 (avoid division by zero)
		gen.IntRange(0, 10000),  // processedRows: 0 to 10000
		gen.IntRange(1, 20),     // userIDLen: 1 to 20 (for generating userID)
	))

	// Property: Progress percentage is always between 0 and 100 (or more if processedRows > totalRows)
	properties.Property("progress percentage is calculated correctly", prop.ForAll(
		func(totalRows int, processedRows int) bool {
			hub := NewHub()

			userID := "testuser"
			client := &Client{
				UserID:      userID,
				Username:    userID,
				Send:        make(chan []byte, 10),
				Hub:         hub,
				ConnectedAt: time.Now(),
				LastPing:    time.Now(),
			}

			hub.registerClient(client)

			// Drain the welcome message sent during registration
			drainWelcomeMessage(client)

			progress := ProgressUpdate{
				JobID:         1,
				Status:        "processing",
				ProcessedRows: processedRows,
				TotalRows:     totalRows,
			}

			hub.SendProgress(userID, progress)

			select {
			case msg := <-client.Send:
				var received ProgressUpdate
				if err := json.Unmarshal(msg, &received); err != nil {
					return false
				}

				expectedProgress := float64(processedRows) / float64(totalRows) * 100
				return received.Progress == expectedProgress

			case <-time.After(100 * time.Millisecond):
				return false
			}
		},
		gen.IntRange(1, 10000), // totalRows: 1 to 10000 (avoid division by zero)
		gen.IntRange(0, 10000), // processedRows: 0 to 10000
	))

	// Property: Messages are only delivered to the correct user
	properties.Property("messages are delivered only to the target user", prop.ForAll(
		func(targetUserNum int, otherUserNum int) bool {
			// Generate distinct user IDs from numbers
			targetUserID := "user" + string(rune('A'+targetUserNum%26))
			otherUserID := "user" + string(rune('a'+otherUserNum%26))

			// Ensure users are different
			if targetUserID == otherUserID {
				otherUserID = otherUserID + "2"
			}

			hub := NewHub()

			// Create target client
			targetClient := &Client{
				UserID:      targetUserID,
				Username:    targetUserID,
				Send:        make(chan []byte, 10),
				Hub:         hub,
				ConnectedAt: time.Now(),
				LastPing:    time.Now(),
			}

			// Create other client
			otherClient := &Client{
				UserID:      otherUserID,
				Username:    otherUserID,
				Send:        make(chan []byte, 10),
				Hub:         hub,
				ConnectedAt: time.Now(),
				LastPing:    time.Now(),
			}

			hub.registerClient(targetClient)
			hub.registerClient(otherClient)

			// Drain welcome messages from both clients
			drainWelcomeMessage(targetClient)
			drainWelcomeMessage(otherClient)

			progress := ProgressUpdate{
				JobID:  1,
				Status: "processing",
			}

			// Send to target user only
			hub.SendProgress(targetUserID, progress)

			// Target should receive message
			targetReceived := false
			select {
			case <-targetClient.Send:
				targetReceived = true
			case <-time.After(100 * time.Millisecond):
				targetReceived = false
			}

			// Other user should NOT receive message
			otherReceived := false
			select {
			case <-otherClient.Send:
				otherReceived = true
			case <-time.After(10 * time.Millisecond):
				otherReceived = false
			}

			return targetReceived && !otherReceived
		},
		gen.IntRange(0, 25),  // targetUserNum: 0 to 25 (for generating targetUserID)
		gen.IntRange(0, 25),  // otherUserNum: 0 to 25 (for generating otherUserID)
	))

	properties.TestingRun(t)
}

// Test basic WebSocket progress functionality
func TestBasicWebSocketProgress(t *testing.T) {
	hub := NewHub()
	
	// Create a mock client
	receivedMessages := make([][]byte, 0)
	client := &Client{
		UserID:      "testuser",
		Username:    "testuser",
		Send:        make(chan []byte, 256),
		Hub:         hub,
		ConnectedAt: time.Now(),
		LastPing:    time.Now(),
	}

	// Register client manually
	hub.mutex.Lock()
	if hub.clients[client.UserID] == nil {
		hub.clients[client.UserID] = make(map[*Client]bool)
	}
	hub.clients[client.UserID][client] = true
	hub.mutex.Unlock()

	// Start goroutine to capture messages
	go func() {
		for msg := range client.Send {
			receivedMessages = append(receivedMessages, msg)
		}
	}()

	// Send progress update
	progress := ProgressUpdate{
		JobID:         1,
		Status:        "processing",
		ProcessedRows: 50,
		TotalRows:     100,
		SuccessCount:  45,
		ErrorCount:    5,
		Message:       "Processing data",
	}

	hub.SendProgress("testuser", progress)
	time.Sleep(10 * time.Millisecond)

	// Verify message was received
	if len(receivedMessages) == 0 {
		t.Fatal("No message received")
	}

	// Parse the received message
	var receivedProgress ProgressUpdate
	if err := json.Unmarshal(receivedMessages[0], &receivedProgress); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify progress consistency
	if receivedProgress.JobID != 1 {
		t.Errorf("Expected JobID 1, got %d", receivedProgress.JobID)
	}
	if receivedProgress.ProcessedRows != 50 {
		t.Errorf("Expected ProcessedRows 50, got %d", receivedProgress.ProcessedRows)
	}
	if receivedProgress.TotalRows != 100 {
		t.Errorf("Expected TotalRows 100, got %d", receivedProgress.TotalRows)
	}
	if receivedProgress.Progress != 50.0 {
		t.Errorf("Expected Progress 50.0, got %f", receivedProgress.Progress)
	}
	if receivedProgress.Type != "progress" {
		t.Errorf("Expected Type 'progress', got %s", receivedProgress.Type)
	}

	close(client.Send)
}

// Test WebSocket connection management
func TestWebSocketConnectionManagement(t *testing.T) {
	hub := NewHub()

	// Test initial state
	if hub.GetConnectionCount() != 0 {
		t.Errorf("Initial connection count should be 0, got %d", hub.GetConnectionCount())
	}

	// Create mock clients
	client1 := &Client{
		UserID:   "user1",
		Username: "testuser1",
		Send:     make(chan []byte, 256),
		Hub:      hub,
	}

	client2 := &Client{
		UserID:   "user1", // Same user, different connection
		Username: "testuser1",
		Send:     make(chan []byte, 256),
		Hub:      hub,
	}

	client3 := &Client{
		UserID:   "user2",
		Username: "testuser2",
		Send:     make(chan []byte, 256),
		Hub:      hub,
	}

	// Register clients manually
	hub.registerClient(client1)
	hub.registerClient(client2)
	hub.registerClient(client3)

	// Verify connection counts
	if hub.GetConnectionCount() != 3 {
		t.Errorf("Total connection count should be 3, got %d", hub.GetConnectionCount())
	}

	if hub.GetUserConnectionCount("user1") != 2 {
		t.Errorf("User1 connection count should be 2, got %d", hub.GetUserConnectionCount("user1"))
	}

	if hub.GetUserConnectionCount("user2") != 1 {
		t.Errorf("User2 connection count should be 1, got %d", hub.GetUserConnectionCount("user2"))
	}

	// Unregister one client
	hub.unregisterClient(client1)

	if hub.GetConnectionCount() != 2 {
		t.Errorf("Total connection count should be 2 after unregistering, got %d", hub.GetConnectionCount())
	}

	if hub.GetUserConnectionCount("user1") != 1 {
		t.Errorf("User1 connection count should be 1 after unregistering, got %d", hub.GetUserConnectionCount("user1"))
	}

	// Unregister all user1 clients
	hub.unregisterClient(client2)

	if hub.GetUserConnectionCount("user1") != 0 {
		t.Errorf("User1 connection count should be 0 after unregistering all, got %d", hub.GetUserConnectionCount("user1"))
	}

	connectedUsers := hub.GetConnectedUsers()
	if len(connectedUsers) != 1 || connectedUsers[0] != "user2" {
		t.Errorf("Connected users should only contain user2, got %v", connectedUsers)
	}
}