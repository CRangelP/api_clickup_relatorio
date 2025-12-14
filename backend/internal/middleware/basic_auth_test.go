package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: clickup-field-updater, Property 1: Authentication state consistency**
// For any user session, when valid credentials are provided, the system should create a session 
// and maintain authentication state across all tabs and operations
// **Validates: Requirements 1.2, 1.4**
func TestAuthenticationStateConsistency(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("authentication state consistency", prop.ForAll(
		func(username, password string) bool {
			// Skip empty credentials as they are invalid by design
			if username == "" || password == "" {
				return true
			}

			// Create middleware with test user
			config := BasicAuthConfig{
				Users:           make(map[string]string),
				SessionDuration: 1 * time.Hour,
				CookieName:      "test_session",
				CookieSecure:    false,
				CookieHTTPOnly:  true,
			}

			middleware := NewBasicAuthMiddleware(config)

			// Hash password and add user
			hashedPassword, err := HashPassword(password)
			if err != nil {
				t.Logf("Failed to hash password: %v", err)
				return false
			}
			middleware.AddUser(username, hashedPassword)

			// Test 1: Valid credentials should create session
			sessionID, err := middleware.CreateSession(username)
			if err != nil {
				t.Logf("Failed to create session: %v", err)
				return false
			}

			// Test 2: Session should be retrievable and valid
			session, valid := middleware.GetSession(sessionID)
			if !valid || session == nil {
				t.Logf("Session not valid or not found")
				return false
			}

			// Test 3: Session should contain correct user information
			if session.Username != username || session.UserID != username {
				t.Logf("Session contains incorrect user information")
				return false
			}

			// Test 4: Session should not be expired
			if time.Now().After(session.ExpiresAt) {
				t.Logf("Session is expired")
				return false
			}

			// Test 5: Credential validation should work
			if !middleware.ValidateCredentials(username, password) {
				t.Logf("Credential validation failed")
				return false
			}

			// Test 6: Test complete login flow
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.POST("/login", middleware.Login)
			router.Use(middleware.RequireAuth())
			router.GET("/protected", func(c *gin.Context) {
				c.JSON(200, gin.H{"status": "ok"})
			})

			// Login request
			loginData := map[string]string{
				"username": username,
				"password": password,
			}
			jsonData, _ := json.Marshal(loginData)

			req, _ := http.NewRequest("POST", "/login", strings.NewReader(string(jsonData)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			// Should return 200 OK for valid credentials
			if w.Code != http.StatusOK {
				t.Logf("Login endpoint returned %d, expected 200", w.Code)
				return false
			}

			// Should set session cookie
			cookies := w.Result().Cookies()
			var sessionCookie *http.Cookie
			for _, cookie := range cookies {
				if cookie.Name == "test_session" {
					sessionCookie = cookie
					break
				}
			}

			if sessionCookie == nil {
				t.Logf("No session cookie set")
				return false
			}

			// Test 7: Protected endpoint should accept session from login
			req2, _ := http.NewRequest("GET", "/protected", nil)
			req2.AddCookie(sessionCookie)
			w2 := httptest.NewRecorder()

			router.ServeHTTP(w2, req2)

			if w2.Code != http.StatusOK {
				t.Logf("Protected endpoint returned %d, expected 200", w2.Code)
				return false
			}

			return true
		},
		gen.RegexMatch(`^[a-zA-Z]{3,20}$`),
		gen.RegexMatch(`^[a-zA-Z]{6,50}$`),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
// **Feature: clickup-field-updater, Property 2: Invalid input rejection**
// For any invalid input (credentials, files, tokens), the system should reject the input 
// and display appropriate error messages without affecting system state
// **Validates: Requirements 1.3**
func TestInvalidInputRejection(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("invalid input rejection", prop.ForAll(
		func(invalidUsername, invalidPassword, validUsername, validPassword string) bool {
			// Ensure we have valid credentials to test against
			if validUsername == "" || validPassword == "" || len(validPassword) < 6 {
				return true // Skip invalid test data
			}

			// Create middleware with one valid user
			config := BasicAuthConfig{
				Users:           make(map[string]string),
				SessionDuration: 1 * time.Hour,
				CookieName:      "test_session",
				CookieSecure:    false,
				CookieHTTPOnly:  true,
			}

			middleware := NewBasicAuthMiddleware(config)

			// Add valid user
			hashedPassword, err := HashPassword(validPassword)
			if err != nil {
				t.Logf("Failed to hash password: %v", err)
				return false
			}
			middleware.AddUser(validUsername, hashedPassword)

			gin.SetMode(gin.TestMode)

			// Test 1: Empty username should be rejected
			if invalidUsername == "" {
				if middleware.ValidateCredentials(invalidUsername, validPassword) {
					t.Logf("Empty username was accepted")
					return false
				}

				// Test login endpoint with empty username
				router := gin.New()
				router.POST("/login", middleware.Login)

				loginData := map[string]string{
					"username": invalidUsername,
					"password": validPassword,
				}
				jsonData, _ := json.Marshal(loginData)

				req, _ := http.NewRequest("POST", "/login", strings.NewReader(string(jsonData)))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				// Should return 400 Bad Request for empty username
				if w.Code == http.StatusOK {
					t.Logf("Login with empty username returned 200, should be rejected")
					return false
				}
			}

			// Test 2: Empty password should be rejected
			if invalidPassword == "" {
				if middleware.ValidateCredentials(validUsername, invalidPassword) {
					t.Logf("Empty password was accepted")
					return false
				}

				// Test login endpoint with empty password
				router := gin.New()
				router.POST("/login", middleware.Login)

				loginData := map[string]string{
					"username": validUsername,
					"password": invalidPassword,
				}
				jsonData, _ := json.Marshal(loginData)

				req, _ := http.NewRequest("POST", "/login", strings.NewReader(string(jsonData)))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				// Should return 400 Bad Request for empty password
				if w.Code == http.StatusOK {
					t.Logf("Login with empty password returned 200, should be rejected")
					return false
				}
			}

			// Test 3: Non-existent user should be rejected
			if invalidUsername != validUsername && invalidUsername != "" {
				if middleware.ValidateCredentials(invalidUsername, validPassword) {
					t.Logf("Non-existent user was accepted")
					return false
				}

				// Test login endpoint with non-existent user
				router := gin.New()
				router.POST("/login", middleware.Login)

				loginData := map[string]string{
					"username": invalidUsername,
					"password": validPassword,
				}
				jsonData, _ := json.Marshal(loginData)

				req, _ := http.NewRequest("POST", "/login", strings.NewReader(string(jsonData)))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				// Should return 401 Unauthorized for non-existent user
				if w.Code == http.StatusOK {
					t.Logf("Login with non-existent user returned 200, should be rejected")
					return false
				}
			}

			// Test 4: Wrong password should be rejected
			if invalidPassword != validPassword && invalidPassword != "" {
				if middleware.ValidateCredentials(validUsername, invalidPassword) {
					t.Logf("Wrong password was accepted")
					return false
				}

				// Test login endpoint with wrong password
				router := gin.New()
				router.POST("/login", middleware.Login)

				loginData := map[string]string{
					"username": validUsername,
					"password": invalidPassword,
				}
				jsonData, _ := json.Marshal(loginData)

				req, _ := http.NewRequest("POST", "/login", strings.NewReader(string(jsonData)))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				// Should return 401 Unauthorized for wrong password
				if w.Code == http.StatusOK {
					t.Logf("Login with wrong password returned 200, should be rejected")
					return false
				}
			}

			// Test 5: Invalid JSON should be rejected
			router := gin.New()
			router.POST("/login", middleware.Login)

			req, _ := http.NewRequest("POST", "/login", strings.NewReader("invalid json"))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			// Should return 400 Bad Request for invalid JSON
			if w.Code == http.StatusOK {
				t.Logf("Login with invalid JSON returned 200, should be rejected")
				return false
			}

			// Test 6: Missing session cookie should be rejected by RequireAuth
			router2 := gin.New()
			router2.Use(middleware.RequireAuth())
			router2.GET("/protected", func(c *gin.Context) {
				c.JSON(200, gin.H{"status": "ok"})
			})

			req2, _ := http.NewRequest("GET", "/protected", nil)
			w2 := httptest.NewRecorder()

			router2.ServeHTTP(w2, req2)

			// Should return 401 Unauthorized for missing session
			if w2.Code == http.StatusOK {
				t.Logf("Protected endpoint without session returned 200, should be rejected")
				return false
			}

			// Test 7: Invalid session ID should be rejected
			req3, _ := http.NewRequest("GET", "/protected", nil)
			req3.AddCookie(&http.Cookie{
				Name:  "test_session",
				Value: "invalid_session_id",
			})
			w3 := httptest.NewRecorder()

			router2.ServeHTTP(w3, req3)

			// Should return 401 Unauthorized for invalid session
			if w3.Code == http.StatusOK {
				t.Logf("Protected endpoint with invalid session returned 200, should be rejected")
				return false
			}

			return true
		},
		gen.AlphaString(),                  // invalidUsername (can be empty)
		gen.AlphaString(),                  // invalidPassword (can be empty)
		gen.RegexMatch(`^[a-zA-Z]{3,20}$`), // validUsername
		gen.RegexMatch(`^[a-zA-Z]{6,50}$`), // validPassword
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}