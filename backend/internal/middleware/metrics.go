package middleware

import (
	"strings"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/metrics"
	"github.com/gin-gonic/gin"
)

// MetricsMiddleware tracks request metrics
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start).Milliseconds()

		// Determine success based on status code
		statusCode := c.Writer.Status()
		success := statusCode < 400

		// Record metrics
		metrics.Get().IncrementRequests(success, latency)

		// Track endpoint-specific metrics
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		metrics.Get().TrackEndpoint(path, c.Request.Method, statusCode, latency)
	}
}

// AuditMiddleware logs audit events for sensitive operations
func AuditMiddleware() gin.HandlerFunc {
	// Paths that should be audited
	auditPaths := map[string]bool{
		"/api/web/jobs":          true,
		"/api/web/mapping":       true,
		"/api/web/upload":        true,
		"/api/web/config":        true,
		"/api/web/history":       true,
		"/api/web/metadata/sync": true,
		"/api/auth/login":        true,
		"/api/auth/logout":       true,
	}

	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		// Check if this path should be audited
		shouldAudit := false
		for auditPath := range auditPaths {
			if strings.HasPrefix(path, auditPath) {
				shouldAudit = true
				break
			}
		}

		c.Next()

		// Only audit if path matches and it's a state-changing operation
		if shouldAudit && (c.Request.Method == "POST" || c.Request.Method == "PUT" || c.Request.Method == "DELETE") {
			duration := time.Since(start).Milliseconds()
			userID := ""
			if uid, exists := c.Get("user_id"); exists {
				userID = uid.(string)
			}

			logger.AuditRequest(
				c.Request.Context(),
				c.Request.Method,
				path,
				c.Writer.Status(),
				duration,
				userID,
				c.ClientIP(),
			)
		}
	}
}
