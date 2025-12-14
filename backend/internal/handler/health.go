package handler

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/metrics"
	"github.com/cleberrangel/clickup-excel-api/internal/websocket"
	"github.com/gin-gonic/gin"
)

// HealthHandler handles health check and metrics endpoints
type HealthHandler struct {
	db        *sql.DB
	wsHub     *websocket.Hub
	version   string
	startTime time.Time
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(db *sql.DB, version string) *HealthHandler {
	return &HealthHandler{
		db:        db,
		version:   version,
		startTime: time.Now(),
	}
}

// NewHealthHandlerWithWebSocket creates a new health handler with WebSocket hub
func NewHealthHandlerWithWebSocket(db *sql.DB, wsHub *websocket.Hub, version string) *HealthHandler {
	return &HealthHandler{
		db:        db,
		wsHub:     wsHub,
		version:   version,
		startTime: time.Now(),
	}
}

// LivenessCheck returns basic liveness status
// @Summary Liveness check
// @Description Returns basic liveness status for Kubernetes probes
// @Tags health
// @Produce json
// @Success 200 {object} map[string]string
// @Router /health/live [get]
func (h *HealthHandler) LivenessCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

// ReadinessCheck returns readiness status including dependencies
// @Summary Readiness check
// @Description Returns readiness status including database connectivity
// @Tags health
// @Produce json
// @Success 200 {object} metrics.HealthCheck
// @Failure 503 {object} metrics.HealthCheck
// @Router /health/ready [get]
func (h *HealthHandler) ReadinessCheck(c *gin.Context) {
	components := make(map[string]metrics.HealthStatus)

	// Check database
	components["database"] = metrics.CheckDatabaseHealth(h.db)

	// Check memory (512MB limit as per requirements)
	components["memory"] = metrics.CheckMemoryHealth(512)

	// Determine overall status
	overallStatus := metrics.DetermineOverallStatus(components)

	healthCheck := metrics.HealthCheck{
		Status:     overallStatus,
		Version:    h.version,
		Uptime:     time.Since(h.startTime).String(),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Components: components,
	}

	statusCode := http.StatusOK
	if overallStatus == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, healthCheck)
}

// DetailedHealthCheck returns comprehensive health information
// @Summary Detailed health check
// @Description Returns comprehensive health information including all components
// @Tags health
// @Produce json
// @Success 200 {object} metrics.HealthCheck
// @Failure 503 {object} metrics.HealthCheck
// @Router /health [get]
func (h *HealthHandler) DetailedHealthCheck(c *gin.Context) {
	components := make(map[string]metrics.HealthStatus)

	// Check database
	components["database"] = metrics.CheckDatabaseHealth(h.db)

	// Check memory
	components["memory"] = metrics.CheckMemoryHealth(512)

	// Check WebSocket hub if available
	if h.wsHub != nil {
		components["websocket"] = h.checkWebSocketHealth()
	}

	// Check queue processor
	components["queue"] = h.checkQueueHealth()

	// Determine overall status
	overallStatus := metrics.DetermineOverallStatus(components)

	healthCheck := metrics.HealthCheck{
		Status:     overallStatus,
		Version:    h.version,
		Uptime:     time.Since(h.startTime).String(),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Components: components,
	}

	statusCode := http.StatusOK
	if overallStatus == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, healthCheck)
}

// checkWebSocketHealth checks WebSocket hub health
func (h *HealthHandler) checkWebSocketHealth() metrics.HealthStatus {
	if h.wsHub == nil {
		return metrics.HealthStatus{
			Status:  "unhealthy",
			Message: "WebSocket hub not initialized",
		}
	}

	// Get connection count from metrics
	wsConnections := metrics.Get().Snapshot().WebSocket.Connections

	// Check if connections are within limit (10 as per requirements)
	if wsConnections > 10 {
		return metrics.HealthStatus{
			Status:  "degraded",
			Message: "WebSocket connections near limit",
		}
	}

	return metrics.HealthStatus{
		Status: "healthy",
	}
}

// checkQueueHealth checks queue processor health
func (h *HealthHandler) checkQueueHealth() metrics.HealthStatus {
	snapshot := metrics.Get().Snapshot()

	// Check if there are too many jobs processing
	if snapshot.Jobs.Processing > 10 {
		return metrics.HealthStatus{
			Status:  "degraded",
			Message: "High number of jobs processing",
		}
	}

	// Check failure rate
	totalJobs := snapshot.Jobs.Completed + snapshot.Jobs.Failed
	if totalJobs > 0 {
		failureRate := float64(snapshot.Jobs.Failed) / float64(totalJobs) * 100
		if failureRate > 50 {
			return metrics.HealthStatus{
				Status:  "degraded",
				Message: "High job failure rate",
			}
		}
	}

	return metrics.HealthStatus{
		Status: "healthy",
	}
}

// GetMetrics returns application metrics
// @Summary Get application metrics
// @Description Returns all application metrics including request counts, job stats, etc.
// @Tags metrics
// @Produce json
// @Success 200 {object} metrics.MetricsSnapshot
// @Router /metrics [get]
func (h *HealthHandler) GetMetrics(c *gin.Context) {
	snapshot := metrics.Get().Snapshot()
	c.JSON(http.StatusOK, snapshot)
}

// GetMetricsSummary returns a summary of key metrics
// @Summary Get metrics summary
// @Description Returns a summary of key application metrics
// @Tags metrics
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /metrics/summary [get]
func (h *HealthHandler) GetMetricsSummary(c *gin.Context) {
	snapshot := metrics.Get().Snapshot()

	// Calculate success rates
	requestSuccessRate := float64(0)
	if snapshot.Requests.Total > 0 {
		requestSuccessRate = float64(snapshot.Requests.Successful) / float64(snapshot.Requests.Total) * 100
	}

	loginSuccessRate := float64(0)
	if snapshot.Auth.LoginAttempts > 0 {
		loginSuccessRate = float64(snapshot.Auth.LoginSuccesses) / float64(snapshot.Auth.LoginAttempts) * 100
	}

	jobSuccessRate := float64(0)
	totalJobs := snapshot.Jobs.Completed + snapshot.Jobs.Failed
	if totalJobs > 0 {
		jobSuccessRate = float64(snapshot.Jobs.Completed) / float64(totalJobs) * 100
	}

	summary := gin.H{
		"uptime_seconds": snapshot.UptimeSeconds,
		"version":        h.version,
		"requests": gin.H{
			"total":        snapshot.Requests.Total,
			"success_rate": requestSuccessRate,
			"avg_latency":  snapshot.Requests.AvgLatencyMs,
		},
		"jobs": gin.H{
			"processing":   snapshot.Jobs.Processing,
			"completed":    snapshot.Jobs.Completed,
			"failed":       snapshot.Jobs.Failed,
			"success_rate": jobSuccessRate,
		},
		"auth": gin.H{
			"login_attempts": snapshot.Auth.LoginAttempts,
			"success_rate":   loginSuccessRate,
		},
		"websocket": gin.H{
			"connections": snapshot.WebSocket.Connections,
		},
		"system": gin.H{
			"goroutines":  snapshot.System.Goroutines,
			"heap_mb":     snapshot.System.HeapAllocMB,
			"heap_use_mb": snapshot.System.HeapInUseMB,
		},
	}

	c.JSON(http.StatusOK, summary)
}

// GetEndpointMetrics returns metrics for specific endpoints
// @Summary Get endpoint metrics
// @Description Returns metrics broken down by endpoint
// @Tags metrics
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /metrics/endpoints [get]
func (h *HealthHandler) GetEndpointMetrics(c *gin.Context) {
	snapshot := metrics.Get().Snapshot()

	c.JSON(http.StatusOK, gin.H{
		"endpoints": snapshot.Endpoints,
	})
}
