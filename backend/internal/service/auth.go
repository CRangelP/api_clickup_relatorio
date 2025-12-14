package service

import (
	"errors"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/middleware"
	"github.com/cleberrangel/clickup-excel-api/internal/repository"
)

var (
	ErrUserNotFound      = errors.New("usuário não encontrado")
	ErrInvalidCredentials = errors.New("credenciais inválidas")
	ErrUserAlreadyExists = errors.New("usuário já existe")
)

// AuthService handles authentication business logic
type AuthService struct {
	userRepo       *repository.UserRepository
	authMiddleware *middleware.BasicAuthMiddleware
	csrfMiddleware *middleware.CSRFMiddleware
}

// NewAuthService creates a new authentication service
func NewAuthService(userRepo *repository.UserRepository) *AuthService {
	// Configure basic auth middleware
	config := middleware.BasicAuthConfig{
		Users:           make(map[string]string),
		SessionDuration: 24 * time.Hour,
		CookieName:      "session_id",
		CookieDomain:    "",
		CookieSecure:    false, // Set to true in production with HTTPS
		CookieHTTPOnly:  true,
	}

	authMiddleware := middleware.NewBasicAuthMiddleware(config)

	// Configure CSRF middleware
	csrfConfig := middleware.CSRFConfig{
		TokenDuration: 24 * time.Hour,
		CookieDomain:  "",
		CookieSecure:  false, // Set to true in production with HTTPS
		CookiePath:    "/",
	}

	csrfMiddleware := middleware.NewCSRFMiddleware(csrfConfig)

	service := &AuthService{
		userRepo:       userRepo,
		authMiddleware: authMiddleware,
		csrfMiddleware: csrfMiddleware,
	}

	// Load existing users into middleware
	service.loadUsersIntoMiddleware()

	return service
}

// GetAuthMiddleware returns the authentication middleware
func (s *AuthService) GetAuthMiddleware() *middleware.BasicAuthMiddleware {
	return s.authMiddleware
}

// GetCSRFMiddleware returns the CSRF middleware
func (s *AuthService) GetCSRFMiddleware() *middleware.CSRFMiddleware {
	return s.csrfMiddleware
}

// loadUsersIntoMiddleware loads all users from database into middleware
func (s *AuthService) loadUsersIntoMiddleware() error {
	users, err := s.userRepo.List()
	if err != nil {
		return err
	}

	// Clear existing users in middleware
	s.authMiddleware.ClearUsers()

	// Load users into middleware
	for _, user := range users {
		// Get full user with password hash
		fullUser, err := s.userRepo.GetByUsername(user.Username)
		if err != nil {
			continue // Skip users that can't be loaded
		}
		s.authMiddleware.AddUser(user.Username, fullUser.PasswordHash)
	}

	return nil
}

// CreateUser creates a new user with hashed password
func (s *AuthService) CreateUser(username, password string) error {
	// Check if user already exists
	existingUser, err := s.userRepo.GetByUsername(username)
	if err != nil {
		return err
	}
	if existingUser != nil {
		return ErrUserAlreadyExists
	}

	// Hash password
	passwordHash, err := middleware.HashPassword(password)
	if err != nil {
		return err
	}

	// Create user in database
	_, err = s.userRepo.Create(username, passwordHash)
	if err != nil {
		return err
	}

	// Add user to middleware
	s.authMiddleware.AddUser(username, passwordHash)

	return nil
}

// UpdateUserPassword updates a user's password
func (s *AuthService) UpdateUserPassword(username, newPassword string) error {
	// Check if user exists
	user, err := s.userRepo.GetByUsername(username)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	// Hash new password
	passwordHash, err := middleware.HashPassword(newPassword)
	if err != nil {
		return err
	}

	// Update password in database
	err = s.userRepo.UpdatePassword(username, passwordHash)
	if err != nil {
		return err
	}

	// Update password in middleware
	s.authMiddleware.AddUser(username, passwordHash)

	return nil
}

// DeleteUser removes a user
func (s *AuthService) DeleteUser(username string) error {
	// Delete from database
	err := s.userRepo.Delete(username)
	if err != nil {
		return err
	}

	// Remove from middleware
	s.authMiddleware.RemoveUser(username)

	return nil
}

// ValidateCredentials validates username and password
func (s *AuthService) ValidateCredentials(username, password string) bool {
	return s.authMiddleware.ValidateCredentials(username, password)
}

// StartSessionCleanup starts a goroutine to periodically clean up expired sessions
func (s *AuthService) StartSessionCleanup() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour) // Clean up every hour
		defer ticker.Stop()

		for range ticker.C {
			s.authMiddleware.CleanupExpiredSessions()
		}
	}()
}