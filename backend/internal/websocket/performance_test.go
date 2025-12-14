package websocket

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestWebSocketConnectionLimits tests that the system can handle the required number
// of concurrent WebSocket connections (Requirements 15.5)
func TestWebSocketConnectionLimits(t *testing.T) {
	hub := NewHub()

	// Requirement 15.5: Support up to 10 concurrent WebSocket connections
	maxConnections := 10

	// Create clients
	clients := make([]*Client, maxConnections)
	for i := 0; i < maxConnections; i++ {
		clients[i] = &Client{
			UserID:      "user" + string(rune('0'+i)),
			Username:    "testuser" + string(rune('0'+i)),
			Send:        make(chan []byte, 256),
			Hub:         hub,
			ConnectedAt: time.Now(),
			LastPing:    time.Now(),
		}
		hub.registerClient(clients[i])
	}

	// Verify all connections are registered
	if hub.GetConnectionCount() != maxConnections {
		t.Errorf("Expected %d connections, got %d", maxConnections, hub.GetConnectionCount())
	}

	// Verify each user has exactly one connection
	for i := 0; i < maxConnections; i++ {
		userID := "user" + string(rune('0'+i))
		if hub.GetUserConnectionCount(userID) != 1 {
			t.Errorf("Expected 1 connection for %s, got %d", userID, hub.GetUserConnectionCount(userID))
		}
	}

	// Test sending messages to all connections concurrently
	var wg sync.WaitGroup
	for i := 0; i < maxConnections; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			userID := "user" + string(rune('0'+idx))
			progress := ProgressUpdate{
				JobID:         idx,
				Status:        "processing",
				ProcessedRows: 50,
				TotalRows:     100,
			}
			hub.SendProgress(userID, progress)
		}(i)
	}
	wg.Wait()

	// Verify all clients received messages (including welcome message)
	for i, client := range clients {
		// Drain welcome message first
		select {
		case <-client.Send:
		case <-time.After(100 * time.Millisecond):
		}

		// Check for progress message
		select {
		case <-client.Send:
			// Message received successfully
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Client %d did not receive progress message", i)
		}
	}

	// Cleanup
	for _, client := range clients {
		hub.unregisterClient(client)
	}

	if hub.GetConnectionCount() != 0 {
		t.Errorf("Expected 0 connections after cleanup, got %d", hub.GetConnectionCount())
	}
}

// TestWebSocketMemoryUsageWithConnections tests memory usage with multiple connections
func TestWebSocketMemoryUsageWithConnections(t *testing.T) {
	hub := NewHub()

	// Create 10 connections (requirement 15.5)
	numConnections := 10
	clients := make([]*Client, numConnections)

	for i := 0; i < numConnections; i++ {
		clients[i] = &Client{
			UserID:      "user" + string(rune('0'+i%10)) + string(rune('0'+i/10)),
			Username:    "testuser",
			Send:        make(chan []byte, 256),
			Hub:         hub,
			ConnectedAt: time.Now(),
			LastPing:    time.Now(),
		}
		hub.registerClient(clients[i])
	}

	// Send multiple messages to each client
	numMessages := 100
	for m := 0; m < numMessages; m++ {
		for i := 0; i < numConnections; i++ {
			progress := ProgressUpdate{
				JobID:         m,
				Status:        "processing",
				ProcessedRows: m,
				TotalRows:     numMessages,
			}
			hub.SendProgress(clients[i].UserID, progress)
		}
	}

	// Drain all messages from clients
	for _, client := range clients {
		for {
			select {
			case <-client.Send:
			default:
				goto nextClient
			}
		}
	nextClient:
	}

	// Force GC and measure memory
	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// Use HeapAlloc for current heap usage
	memUsedMB := float64(memAfter.HeapAlloc) / (1024 * 1024)

	// Memory usage should be reasonable for 10 connections with 100 messages each
	maxMemoryMB := 100.0
	if memUsedMB > maxMemoryMB {
		t.Errorf("Memory usage exceeded limit: %.2f MB (limit: %.2f MB)", memUsedMB, maxMemoryMB)
	}

	// Cleanup
	for _, client := range clients {
		hub.unregisterClient(client)
	}

	t.Logf("Handled %d connections with %d messages each, heap memory: %.2f MB", numConnections, numMessages, memUsedMB)
}

// TestConcurrentClientRegistration tests thread safety of client registration
func TestConcurrentClientRegistration(t *testing.T) {
	hub := NewHub()

	numClients := 10
	var wg sync.WaitGroup

	// Register clients concurrently
	clients := make([]*Client, numClients)
	for i := 0; i < numClients; i++ {
		clients[i] = &Client{
			UserID:      "concurrent_user",
			Username:    "testuser",
			Send:        make(chan []byte, 256),
			Hub:         hub,
			ConnectedAt: time.Now(),
			LastPing:    time.Now(),
		}
	}

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			hub.registerClient(clients[idx])
		}(i)
	}
	wg.Wait()

	// All clients should be registered for the same user
	if hub.GetUserConnectionCount("concurrent_user") != numClients {
		t.Errorf("Expected %d connections for concurrent_user, got %d",
			numClients, hub.GetUserConnectionCount("concurrent_user"))
	}

	// Unregister clients concurrently
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			hub.unregisterClient(clients[idx])
		}(i)
	}
	wg.Wait()

	// All clients should be unregistered
	if hub.GetConnectionCount() != 0 {
		t.Errorf("Expected 0 connections after concurrent unregister, got %d", hub.GetConnectionCount())
	}
}
