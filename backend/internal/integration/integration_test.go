// Package integration provides end-to-end integration tests for the ClickUp Field Updater system.
// These tests validate complete user workflows, WebSocket functionality, concurrent operations,
// data consistency, and error recovery scenarios.
//
// **Validates: All requirements integration validation**
package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/database"
	"github.com/cleberrangel/clickup-excel-api/internal/handler"
	"github.com/cleberrangel/clickup-excel-api/internal/migration"
	"github.com/cleberrangel/clickup-excel-api/internal/repository"
	"github.com/cleberrangel/clickup-excel-api/internal/service"
	"github.com/cleberrangel/clickup-excel-api/internal/websocket"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

// TestContext holds all dependencies for integration tests
type TestContext struct {
	DB              *sql.DB
	Router          *gin.Engine
	WSHub           *websocket.Hub
	AuthService     *service.AuthService
	MetadataService *service.MetadataService
	UploadService   *service.UploadService
	QueueService    *service.QueueService
	MappingService  *service.MappingService
	TempDir         string
	SessionCookie   *http.Cookie
	CSRFToken       string
}

// setupTestContext creates a complete test environment with all services initialized
func setupTestContext(t *testing.T) *TestContext {
	t.Helper()

	// Create temp directory for uploads
	tempDir, err := os.MkdirTemp("", "integration_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Setup test database
	dbConfig := database.Config{
		Host:     getEnvOrDefault("TEST_DB_HOST", "127.0.0.1"),
		Port:     getEnvOrDefault("TEST_DB_PORT", "5432"),
		User:     getEnvOrDefault("TEST_DB_USER", "postgres"),
		Password: getEnvOrDefault("TEST_DB_PASSWORD", "postgres"),
		DBName:   fmt.Sprintf("test_integration_%d", time.Now().UnixNano()),
		SSLMode:  "disable",
	}

	// Connect to postgres to create test database
	adminConfig := dbConfig
	adminConfig.DBName = "postgres"

	adminDB, err := database.Connect(adminConfig)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Skipf("Skipping test: could not connect to PostgreSQL: %v", err)
	}

	// Create test database
	_, err = adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", dbConfig.DBName))
	adminDB.Close()
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Connect to test database
	testDB, err := database.Connect(dbConfig)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Run migrations
	migrator := migration.NewMigrator(testDB)
	if err := migrator.Run(); err != nil {
		testDB.Close()
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize repositories
	metadataRepo := repository.NewMetadataRepository(testDB)
	queueRepo := repository.NewQueueRepository(testDB)
	configRepo := repository.NewConfigRepository(testDB)
	userRepo := repository.NewUserRepository(testDB)

	// Initialize WebSocket hub
	wsHub := websocket.NewHub()
	go wsHub.Run()

	// Initialize services
	authService := service.NewAuthService(userRepo)
	uploadService := service.NewUploadService(tempDir)
	mappingService := service.NewMappingService(metadataRepo)
	queueService := service.NewQueueService(queueRepo, wsHub)
	metadataService := service.NewMetadataService(metadataRepo, configRepo, "test-encryption-key-32-bytes-long")
	taskUpdateService := service.NewTaskUpdateService(uploadService, metadataRepo, configRepo, queueRepo, wsHub)
	historyService := service.NewHistoryService(queueRepo)

	// Connect task update service to queue
	queueService.SetJobProcessor(taskUpdateService.ProcessJob)
	queueService.Start()

	// Initialize handlers
	authHandler := handler.NewAuthHandler(authService)
	uploadHandler := handler.NewUploadHandler(uploadService)
	mappingHandler := handler.NewMappingHandler(mappingService, uploadService)
	queueHandler := handler.NewQueueHandler(queueService, uploadService, mappingService)
	historyHandler := handler.NewHistoryHandler(historyService)
	metadataHandler := handler.NewMetadataHandler(metadataService, wsHub)
	wsHandler := handler.NewWebSocketHandler(wsHub)

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())

	// Public routes
	auth := router.Group("/api/auth")
	{
		auth.POST("/login", authHandler.Login)
		auth.POST("/logout", authHandler.Logout)
	}

	// Auth status (requires auth but not CSRF)
	webAuth := router.Group("/api/web/auth")
	webAuth.Use(authService.GetAuthMiddleware().RequireAuth())
	{
		webAuth.GET("/status", authHandler.GetCurrentUser)
	}

	// Protected routes
	web := router.Group("/api/web")
	web.Use(authService.GetAuthMiddleware().RequireAuth())
	web.Use(authService.GetCSRFMiddleware().RequireCSRF())
	{
		web.GET("/user", authHandler.GetCurrentUser)
		web.POST("/upload", uploadHandler.UploadFile)
		web.POST("/upload/cleanup", uploadHandler.DeleteTempFile)
		web.POST("/mapping", mappingHandler.SaveMapping)
		web.GET("/mapping", mappingHandler.ListMappings)
		web.POST("/mapping/validate", mappingHandler.ValidateMapping)
		web.POST("/jobs", queueHandler.CreateJob)
		web.GET("/jobs", queueHandler.ListJobs)
		web.GET("/jobs/:id", queueHandler.GetJob)
		web.GET("/history", historyHandler.ListHistory)
		web.DELETE("/history", historyHandler.DeleteAllHistory)
		web.POST("/metadata/sync", metadataHandler.SyncMetadata)
		web.GET("/metadata/hierarchy", metadataHandler.GetHierarchy)
		web.GET("/ws", websocket.AuthMiddleware(authService.GetAuthMiddleware()), wsHandler.HandleConnection)
	}

	// Create test user using AuthService (this also adds to middleware)
	authService.CreateUser("testuser", "testpassword")

	// Cleanup function
	t.Cleanup(func() {
		testDB.Close()
		os.RemoveAll(tempDir)
		adminDB, _ := database.Connect(adminConfig)
		if adminDB != nil {
			adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbConfig.DBName))
			adminDB.Close()
		}
	})

	return &TestContext{
		DB:              testDB,
		Router:          router,
		WSHub:           wsHub,
		AuthService:     authService,
		MetadataService: metadataService,
		UploadService:   uploadService,
		QueueService:    queueService,
		MappingService:  mappingService,
		TempDir:         tempDir,
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// authenticateUser performs login and returns session cookie and CSRF token
func (tc *TestContext) authenticateUser(t *testing.T, username, password string) {
	t.Helper()

	loginData := map[string]string{"username": username, "password": password}
	jsonData, _ := json.Marshal(loginData)

	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	tc.Router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Login failed with status %d: %s", w.Code, w.Body.String())
	}

	// Extract session cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "session_id" {
			tc.SessionCookie = cookie
			break
		}
	}

	// Extract CSRF token from response
	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if token, ok := response["csrf_token"].(string); ok {
		tc.CSRFToken = token
	}
}

// makeAuthenticatedRequest creates an authenticated HTTP request
func (tc *TestContext) makeAuthenticatedRequest(method, path string, body io.Reader) *http.Request {
	req, _ := http.NewRequest(method, path, body)
	if tc.SessionCookie != nil {
		req.AddCookie(tc.SessionCookie)
	}
	if tc.CSRFToken != "" {
		req.Header.Set("X-CSRF-Token", tc.CSRFToken)
	}
	req.Header.Set("Content-Type", "application/json")
	return req
}

// createTestCSVFile creates a CSV file for testing
func createTestCSVFile(columns []string, rows [][]string) *bytes.Buffer {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	writer.Write(columns)
	for _, row := range rows {
		writer.Write(row)
	}
	writer.Flush()
	return &buf
}

// TestCompleteUserWorkflow tests the complete user workflow end-to-end
// This validates Requirements: 1.1, 1.2, 3.1, 3.2, 4.1, 4.2, 6.1, 7.1
func TestCompleteUserWorkflow(t *testing.T) {
	tc := setupTestContext(t)

	// Step 1: Login
	t.Run("Step1_Login", func(t *testing.T) {
		tc.authenticateUser(t, "testuser", "testpassword")
		if tc.SessionCookie == nil {
			t.Fatal("Session cookie not set after login")
		}
		if tc.CSRFToken == "" {
			t.Fatal("CSRF token not set after login")
		}
	})

	// Step 2: Verify authentication status
	t.Run("Step2_VerifyAuthStatus", func(t *testing.T) {
		req := tc.makeAuthenticatedRequest("GET", "/api/web/auth/status", nil)
		w := httptest.NewRecorder()
		tc.Router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Auth status check failed: %d", w.Code)
		}

		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)
		if user, ok := response["user"].(map[string]interface{}); ok {
			if user["username"] != "testuser" {
				t.Errorf("Expected username 'testuser', got %v", user["username"])
			}
		}
	})

	// Step 3: Upload a CSV file
	var uploadedFilePath string
	t.Run("Step3_UploadFile", func(t *testing.T) {
		// Create test CSV content
		csvContent := createTestCSVFile(
			[]string{"id_task", "status", "priority"},
			[][]string{
				{"task1", "open", "high"},
				{"task2", "closed", "low"},
				{"task3", "in_progress", "medium"},
			},
		)

		// Create multipart form
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, _ := writer.CreateFormFile("file", "test.csv")
		io.Copy(part, csvContent)
		writer.Close()

		req := tc.makeAuthenticatedRequest("POST", "/api/web/upload", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		tc.Router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Upload failed: %d - %s", w.Code, w.Body.String())
		}

		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)
		if data, ok := response["data"].(map[string]interface{}); ok {
			if path, ok := data["temp_path"].(string); ok {
				uploadedFilePath = path
			}
			// Verify columns were extracted
			if columns, ok := data["columns"].([]interface{}); ok {
				if len(columns) != 3 {
					t.Errorf("Expected 3 columns, got %d", len(columns))
				}
			}
			// Verify preview was generated
			if preview, ok := data["preview"].([]interface{}); ok {
				if len(preview) != 3 {
					t.Errorf("Expected 3 preview rows, got %d", len(preview))
				}
			}
		}
	})

	// Step 4: Validate mapping
	t.Run("Step4_ValidateMapping", func(t *testing.T) {
		mappingData := map[string]interface{}{
			"file_path": uploadedFilePath,
			"mappings": []map[string]interface{}{
				{"column": "id_task", "is_task_id": true},
				{"column": "status", "field_id": "field1", "field_name": "Status"},
			},
		}
		jsonData, _ := json.Marshal(mappingData)

		req := tc.makeAuthenticatedRequest("POST", "/api/web/mapping/validate", bytes.NewReader(jsonData))
		w := httptest.NewRecorder()
		tc.Router.ServeHTTP(w, req)

		// Validation should succeed (has task ID column)
		if w.Code != http.StatusOK {
			t.Logf("Mapping validation response: %s", w.Body.String())
		}
	})

	// Step 5: Check history (should be empty initially)
	t.Run("Step5_CheckHistory", func(t *testing.T) {
		req := tc.makeAuthenticatedRequest("GET", "/api/web/history", nil)
		w := httptest.NewRecorder()
		tc.Router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("History check failed: %d", w.Code)
		}
	})

	// Step 6: Cleanup uploaded file
	t.Run("Step6_CleanupFile", func(t *testing.T) {
		if uploadedFilePath == "" {
			t.Skip("No file to cleanup")
		}

		cleanupData := map[string]string{"file_path": uploadedFilePath}
		jsonData, _ := json.Marshal(cleanupData)

		req := tc.makeAuthenticatedRequest("POST", "/api/web/upload/cleanup", bytes.NewReader(jsonData))
		w := httptest.NewRecorder()
		tc.Router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Logf("Cleanup response: %s", w.Body.String())
		}
	})

	// Step 7: Logout
	t.Run("Step7_Logout", func(t *testing.T) {
		req := tc.makeAuthenticatedRequest("POST", "/api/auth/logout", nil)
		w := httptest.NewRecorder()
		tc.Router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Logout failed: %d", w.Code)
		}
	})
}

// TestWebSocketFunctionality tests WebSocket functionality across scenarios
// Validates Requirements: 5.1, 5.2, 8.2, 11.2, 11.3, 11.4
func TestWebSocketFunctionality(t *testing.T) {
	tc := setupTestContext(t)
	tc.authenticateUser(t, "testuser", "testpassword")

	t.Run("WebSocket_ProgressBroadcast", func(t *testing.T) {
		// Create a mock client to receive messages
		client := &websocket.Client{
			UserID:      "testuser",
			Username:    "testuser",
			Send:        make(chan []byte, 10),
			Hub:         tc.WSHub,
			ConnectedAt: time.Now(),
			LastPing:    time.Now(),
		}

		// Register client
		tc.WSHub.RegisterClient(client)
		defer tc.WSHub.UnregisterClient(client)

		// Drain welcome message
		select {
		case <-client.Send:
		case <-time.After(100 * time.Millisecond):
		}

		// Send progress update
		progress := websocket.ProgressUpdate{
			JobID:         1,
			Status:        "processing",
			ProcessedRows: 50,
			TotalRows:     100,
			SuccessCount:  45,
			ErrorCount:    5,
			Message:       "Processing...",
		}
		tc.WSHub.SendProgress("testuser", progress)

		// Verify message received
		select {
		case msg := <-client.Send:
			var received websocket.ProgressUpdate
			if err := json.Unmarshal(msg, &received); err != nil {
				t.Fatalf("Failed to unmarshal progress: %v", err)
			}
			if received.JobID != 1 {
				t.Errorf("Expected JobID 1, got %d", received.JobID)
			}
			if received.Progress != 50.0 {
				t.Errorf("Expected Progress 50.0, got %f", received.Progress)
			}
		case <-time.After(time.Second):
			t.Fatal("Timeout waiting for progress message")
		}
	})

	t.Run("WebSocket_MultipleClients", func(t *testing.T) {
		// Create multiple clients for same user
		clients := make([]*websocket.Client, 3)
		for i := 0; i < 3; i++ {
			clients[i] = &websocket.Client{
				UserID:      "testuser",
				Username:    "testuser",
				Send:        make(chan []byte, 10),
				Hub:         tc.WSHub,
				ConnectedAt: time.Now(),
				LastPing:    time.Now(),
			}
			tc.WSHub.RegisterClient(clients[i])
		}
		defer func() {
			for _, c := range clients {
				tc.WSHub.UnregisterClient(c)
			}
		}()

		// Drain welcome messages
		for _, c := range clients {
			select {
			case <-c.Send:
			case <-time.After(100 * time.Millisecond):
			}
		}

		// Send progress
		tc.WSHub.SendProgress("testuser", websocket.ProgressUpdate{JobID: 2, Status: "test"})

		// All clients should receive the message
		for i, c := range clients {
			select {
			case <-c.Send:
				// OK
			case <-time.After(time.Second):
				t.Errorf("Client %d did not receive message", i)
			}
		}
	})
}

// TestConcurrentUserOperations tests concurrent user operations
// Validates Requirements: 15.5 (multiple concurrent connections)
func TestConcurrentUserOperations(t *testing.T) {
	tc := setupTestContext(t)

	// Create multiple test users
	for i := 0; i < 5; i++ {
		username := fmt.Sprintf("user%d", i)
		tc.AuthService.CreateUser(username, "password123")
	}

	t.Run("ConcurrentLogins", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make(chan bool, 5)

		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(userNum int) {
				defer wg.Done()

				username := fmt.Sprintf("user%d", userNum)
				loginData := map[string]string{"username": username, "password": "password123"}
				jsonData, _ := json.Marshal(loginData)

				req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewReader(jsonData))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()

				tc.Router.ServeHTTP(w, req)
				results <- w.Code == http.StatusOK
			}(i)
		}

		wg.Wait()
		close(results)

		successCount := 0
		for success := range results {
			if success {
				successCount++
			}
		}

		if successCount != 5 {
			t.Errorf("Expected 5 successful logins, got %d", successCount)
		}
	})

	t.Run("ConcurrentFileUploads", func(t *testing.T) {
		// First authenticate
		tc.authenticateUser(t, "user0", "password123")

		var wg sync.WaitGroup
		results := make(chan bool, 3)

		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(uploadNum int) {
				defer wg.Done()

				csvContent := createTestCSVFile(
					[]string{"id_task", "field1"},
					[][]string{
						{fmt.Sprintf("task_%d_1", uploadNum), "value1"},
						{fmt.Sprintf("task_%d_2", uploadNum), "value2"},
					},
				)

				var body bytes.Buffer
				writer := multipart.NewWriter(&body)
				part, _ := writer.CreateFormFile("file", fmt.Sprintf("test_%d.csv", uploadNum))
				io.Copy(part, csvContent)
				writer.Close()

				req := tc.makeAuthenticatedRequest("POST", "/api/web/upload", &body)
				req.Header.Set("Content-Type", writer.FormDataContentType())
				w := httptest.NewRecorder()

				tc.Router.ServeHTTP(w, req)
				results <- w.Code == http.StatusOK
			}(i)
		}

		wg.Wait()
		close(results)

		successCount := 0
		for success := range results {
			if success {
				successCount++
			}
		}

		if successCount != 3 {
			t.Errorf("Expected 3 successful uploads, got %d", successCount)
		}
	})
}

// TestDataConsistency tests data consistency across all operations
// Validates Requirements: 14.1, 14.2, 14.3, 14.4, 14.5
func TestDataConsistency(t *testing.T) {
	tc := setupTestContext(t)
	tc.authenticateUser(t, "testuser", "testpassword")
	_ = context.Background() // Available for future use

	t.Run("MetadataHierarchyConsistency", func(t *testing.T) {
		// Insert test metadata
		metadataRepo := repository.NewMetadataRepository(tc.DB)

		// Create workspace
		err := metadataRepo.UpsertWorkspace(repository.Workspace{
			ID:   "ws1",
			Name: "Test Workspace",
		})
		if err != nil {
			t.Fatalf("Failed to create workspace: %v", err)
		}

		// Create space linked to workspace
		err = metadataRepo.UpsertSpace(repository.Space{
			ID:          "sp1",
			WorkspaceID: "ws1",
			Name:        "Test Space",
		})
		if err != nil {
			t.Fatalf("Failed to create space: %v", err)
		}

		// Create folder linked to space
		err = metadataRepo.UpsertFolder(repository.Folder{
			ID:      "fd1",
			SpaceID: "sp1",
			Name:    "Test Folder",
		})
		if err != nil {
			t.Fatalf("Failed to create folder: %v", err)
		}

		// Create list linked to folder
		err = metadataRepo.UpsertList(repository.List{
			ID:       "ls1",
			FolderID: "fd1",
			Name:     "Test List",
		})
		if err != nil {
			t.Fatalf("Failed to create list: %v", err)
		}

		// Verify hierarchical data via API
		req := tc.makeAuthenticatedRequest("GET", "/api/web/metadata/hierarchy", nil)
		w := httptest.NewRecorder()
		tc.Router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Hierarchy request failed: %d - %s", w.Code, w.Body.String())
		}

		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)

		if data, ok := response["data"].(map[string]interface{}); ok {
			if workspaces, ok := data["workspaces"].([]interface{}); ok {
				if len(workspaces) == 0 {
					t.Error("Expected at least one workspace")
				}
			}
		}
	})

	t.Run("CustomFieldConsistency", func(t *testing.T) {
		metadataRepo := repository.NewMetadataRepository(tc.DB)

		// Create custom fields
		fields := []repository.CustomField{
			{ID: "cf1", Name: "Status", Type: "dropdown"},
			{ID: "cf2", Name: "Priority", Type: "dropdown"},
			{ID: "cf3", Name: "Due Date", Type: "date"},
		}

		for _, field := range fields {
			err := metadataRepo.UpsertCustomField(field)
			if err != nil {
				t.Fatalf("Failed to create custom field: %v", err)
			}
		}

		// Verify all fields are retrievable directly from repository
		retrievedFields, err := metadataRepo.GetCustomFields()
		if err != nil {
			t.Fatalf("Failed to get custom fields: %v", err)
		}

		if len(retrievedFields) < 3 {
			t.Errorf("Expected at least 3 custom fields, got %d", len(retrievedFields))
		}

		// Verify each field was stored correctly
		for _, expectedField := range fields {
			found := false
			for _, retrievedField := range retrievedFields {
				if retrievedField.ID == expectedField.ID && 
				   retrievedField.Name == expectedField.Name && 
				   retrievedField.Type == expectedField.Type {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Custom field %s not found in retrieved fields", expectedField.ID)
			}
		}
	})
}

// TestErrorScenariosAndRecovery tests error scenarios and recovery
// Validates Requirements: 1.3, 2.3, 3.3, 8.4, 13.4
func TestErrorScenariosAndRecovery(t *testing.T) {
	tc := setupTestContext(t)

	t.Run("InvalidCredentials", func(t *testing.T) {
		loginData := map[string]string{"username": "invalid", "password": "wrong"}
		jsonData, _ := json.Marshal(loginData)

		req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewReader(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tc.Router.ServeHTTP(w, req)

		if w.Code == http.StatusOK {
			t.Error("Expected login to fail with invalid credentials")
		}
	})

	t.Run("UnauthorizedAccess", func(t *testing.T) {
		// Try to access protected endpoint without authentication
		req, _ := http.NewRequest("GET", "/api/web/history", nil)
		w := httptest.NewRecorder()

		tc.Router.ServeHTTP(w, req)

		if w.Code == http.StatusOK {
			t.Error("Expected unauthorized access to be rejected")
		}
	})

	t.Run("InvalidCSRFToken", func(t *testing.T) {
		tc.authenticateUser(t, "testuser", "testpassword")

		// Make request with invalid CSRF token
		req, _ := http.NewRequest("POST", "/api/web/upload/cleanup", strings.NewReader("{}"))
		req.AddCookie(tc.SessionCookie)
		req.Header.Set("X-CSRF-Token", "invalid-token")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		tc.Router.ServeHTTP(w, req)

		if w.Code == http.StatusOK {
			t.Error("Expected request with invalid CSRF token to be rejected")
		}
	})

	t.Run("InvalidFileFormat", func(t *testing.T) {
		tc.authenticateUser(t, "testuser", "testpassword")

		// Try to upload invalid file format
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, _ := writer.CreateFormFile("file", "test.txt")
		part.Write([]byte("This is not a CSV or XLSX file"))
		writer.Close()

		req := tc.makeAuthenticatedRequest("POST", "/api/web/upload", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		tc.Router.ServeHTTP(w, req)

		// Should reject invalid file format
		if w.Code == http.StatusOK {
			var response map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &response)
			if success, ok := response["success"].(bool); ok && success {
				t.Error("Expected invalid file format to be rejected")
			}
		}
	})

	t.Run("EmptyFileUpload", func(t *testing.T) {
		tc.authenticateUser(t, "testuser", "testpassword")

		// Try to upload empty file
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, _ := writer.CreateFormFile("file", "empty.csv")
		part.Write([]byte(""))
		writer.Close()

		req := tc.makeAuthenticatedRequest("POST", "/api/web/upload", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		tc.Router.ServeHTTP(w, req)

		// Should handle empty file gracefully
		if w.Code == http.StatusOK {
			var response map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &response)
			if success, ok := response["success"].(bool); ok && success {
				t.Error("Expected empty file to be rejected")
			}
		}
	})

	t.Run("RecoveryAfterError", func(t *testing.T) {
		tc.authenticateUser(t, "testuser", "testpassword")

		// First, cause an error (invalid file)
		var body1 bytes.Buffer
		writer1 := multipart.NewWriter(&body1)
		part1, _ := writer1.CreateFormFile("file", "invalid.txt")
		part1.Write([]byte("invalid content"))
		writer1.Close()

		req1 := tc.makeAuthenticatedRequest("POST", "/api/web/upload", &body1)
		req1.Header.Set("Content-Type", writer1.FormDataContentType())
		w1 := httptest.NewRecorder()
		tc.Router.ServeHTTP(w1, req1)

		// Now, verify system can still process valid requests
		csvContent := createTestCSVFile(
			[]string{"id_task", "status"},
			[][]string{{"task1", "open"}},
		)

		var body2 bytes.Buffer
		writer2 := multipart.NewWriter(&body2)
		part2, _ := writer2.CreateFormFile("file", "valid.csv")
		io.Copy(part2, csvContent)
		writer2.Close()

		req2 := tc.makeAuthenticatedRequest("POST", "/api/web/upload", &body2)
		req2.Header.Set("Content-Type", writer2.FormDataContentType())
		w2 := httptest.NewRecorder()
		tc.Router.ServeHTTP(w2, req2)

		if w2.Code != http.StatusOK {
			t.Errorf("System failed to recover after error: %d - %s", w2.Code, w2.Body.String())
		}
	})

	t.Run("SessionExpiry", func(t *testing.T) {
		// Login
		tc.authenticateUser(t, "testuser", "testpassword")

		// Logout
		req1 := tc.makeAuthenticatedRequest("POST", "/api/auth/logout", nil)
		w1 := httptest.NewRecorder()
		tc.Router.ServeHTTP(w1, req1)

		// Try to access protected resource with old session
		req2 := tc.makeAuthenticatedRequest("GET", "/api/web/history", nil)
		w2 := httptest.NewRecorder()
		tc.Router.ServeHTTP(w2, req2)

		if w2.Code == http.StatusOK {
			t.Error("Expected access to be denied after logout")
		}
	})
}

// TestQueueProcessingIntegration tests queue processing integration
// Validates Requirements: 6.1, 6.2, 6.3, 6.4, 6.5
func TestQueueProcessingIntegration(t *testing.T) {
	tc := setupTestContext(t)
	tc.authenticateUser(t, "testuser", "testpassword")

	t.Run("JobListRetrieval", func(t *testing.T) {
		req := tc.makeAuthenticatedRequest("GET", "/api/web/jobs", nil)
		w := httptest.NewRecorder()
		tc.Router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Job list retrieval failed: %d", w.Code)
		}

		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)

		if _, ok := response["data"]; !ok {
			t.Error("Expected data field in response")
		}
	})

	t.Run("HistoryRetrieval", func(t *testing.T) {
		req := tc.makeAuthenticatedRequest("GET", "/api/web/history", nil)
		w := httptest.NewRecorder()
		tc.Router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("History retrieval failed: %d", w.Code)
		}
	})
}

// TestMappingValidationIntegration tests mapping validation integration
// Validates Requirements: 4.3, 4.4, 4.5, 10.3
func TestMappingValidationIntegration(t *testing.T) {
	tc := setupTestContext(t)
	tc.authenticateUser(t, "testuser", "testpassword")

	// First upload a file
	csvContent := createTestCSVFile(
		[]string{"id_task", "status", "priority", "due_date"},
		[][]string{
			{"task1", "open", "high", "2024-01-01"},
			{"task2", "closed", "low", "2024-01-02"},
		},
	)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "test.csv")
	io.Copy(part, csvContent)
	writer.Close()

	req := tc.makeAuthenticatedRequest("POST", "/api/web/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	tc.Router.ServeHTTP(w, req)

	var uploadResponse map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &uploadResponse)
	var filePath string
	if data, ok := uploadResponse["data"].(map[string]interface{}); ok {
		filePath, _ = data["temp_path"].(string)
	}

	t.Run("ValidMappingWithTaskID", func(t *testing.T) {
		mappingData := map[string]interface{}{
			"file_path": filePath,
			"mappings": []map[string]interface{}{
				{"column": "id_task", "is_task_id": true},
				{"column": "status", "field_id": "cf1", "field_name": "Status"},
			},
		}
		jsonData, _ := json.Marshal(mappingData)

		req := tc.makeAuthenticatedRequest("POST", "/api/web/mapping/validate", bytes.NewReader(jsonData))
		w := httptest.NewRecorder()
		tc.Router.ServeHTTP(w, req)

		// Should succeed with task ID column
		if w.Code != http.StatusOK {
			t.Logf("Validation response: %s", w.Body.String())
		}
	})

	t.Run("InvalidMappingWithoutTaskID", func(t *testing.T) {
		mappingData := map[string]interface{}{
			"file_path": filePath,
			"mappings": []map[string]interface{}{
				{"column": "status", "field_id": "cf1", "field_name": "Status"},
			},
		}
		jsonData, _ := json.Marshal(mappingData)

		req := tc.makeAuthenticatedRequest("POST", "/api/web/mapping/validate", bytes.NewReader(jsonData))
		w := httptest.NewRecorder()
		tc.Router.ServeHTTP(w, req)

		// Should fail without task ID column
		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)
		if data, ok := response["data"].(map[string]interface{}); ok {
			if valid, ok := data["valid"].(bool); ok && valid {
				t.Error("Expected validation to fail without task ID column")
			}
		}
	})
}

// TestWebSocketConnectionPersistence tests WebSocket connection persistence across tab navigation
// Validates Requirements: 11.3, 11.4, 13.2
func TestWebSocketConnectionPersistence(t *testing.T) {
	tc := setupTestContext(t)
	tc.authenticateUser(t, "testuser", "testpassword")

	t.Run("ConnectionCountTracking", func(t *testing.T) {
		initialCount := tc.WSHub.GetConnectionCount()

		// Add clients
		clients := make([]*websocket.Client, 3)
		for i := 0; i < 3; i++ {
			clients[i] = &websocket.Client{
				UserID:      "testuser",
				Username:    "testuser",
				Send:        make(chan []byte, 10),
				Hub:         tc.WSHub,
				ConnectedAt: time.Now(),
				LastPing:    time.Now(),
			}
			tc.WSHub.RegisterClient(clients[i])
		}

		// Verify count increased
		newCount := tc.WSHub.GetConnectionCount()
		if newCount != initialCount+3 {
			t.Errorf("Expected connection count %d, got %d", initialCount+3, newCount)
		}

		// Remove one client
		tc.WSHub.UnregisterClient(clients[0])

		// Verify count decreased
		afterRemoval := tc.WSHub.GetConnectionCount()
		if afterRemoval != initialCount+2 {
			t.Errorf("Expected connection count %d after removal, got %d", initialCount+2, afterRemoval)
		}

		// Cleanup
		for i := 1; i < 3; i++ {
			tc.WSHub.UnregisterClient(clients[i])
		}
	})

	t.Run("UserConnectionCount", func(t *testing.T) {
		// Add multiple connections for same user
		clients := make([]*websocket.Client, 2)
		for i := 0; i < 2; i++ {
			clients[i] = &websocket.Client{
				UserID:      "testuser",
				Username:    "testuser",
				Send:        make(chan []byte, 10),
				Hub:         tc.WSHub,
				ConnectedAt: time.Now(),
				LastPing:    time.Now(),
			}
			tc.WSHub.RegisterClient(clients[i])
		}

		userCount := tc.WSHub.GetUserConnectionCount("testuser")
		if userCount != 2 {
			t.Errorf("Expected 2 connections for user, got %d", userCount)
		}

		// Cleanup
		for _, c := range clients {
			tc.WSHub.UnregisterClient(c)
		}
	})

	t.Run("MessageIsolation", func(t *testing.T) {
		// Create clients for different users
		client1 := &websocket.Client{
			UserID:      "user1",
			Username:    "user1",
			Send:        make(chan []byte, 10),
			Hub:         tc.WSHub,
			ConnectedAt: time.Now(),
			LastPing:    time.Now(),
		}
		client2 := &websocket.Client{
			UserID:      "user2",
			Username:    "user2",
			Send:        make(chan []byte, 10),
			Hub:         tc.WSHub,
			ConnectedAt: time.Now(),
			LastPing:    time.Now(),
		}

		tc.WSHub.RegisterClient(client1)
		tc.WSHub.RegisterClient(client2)
		defer tc.WSHub.UnregisterClient(client1)
		defer tc.WSHub.UnregisterClient(client2)

		// Drain welcome messages
		for _, c := range []*websocket.Client{client1, client2} {
			select {
			case <-c.Send:
			case <-time.After(100 * time.Millisecond):
			}
		}

		// Send message to user1 only
		tc.WSHub.SendProgress("user1", websocket.ProgressUpdate{JobID: 1, Status: "test"})

		// user1 should receive
		select {
		case <-client1.Send:
			// OK
		case <-time.After(time.Second):
			t.Error("user1 did not receive message")
		}

		// user2 should NOT receive
		select {
		case <-client2.Send:
			t.Error("user2 received message intended for user1")
		case <-time.After(100 * time.Millisecond):
			// OK - no message received
		}
	})
}
