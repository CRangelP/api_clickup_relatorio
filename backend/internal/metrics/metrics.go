package metrics

import (
	"database/sql"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// EndpointMetrics tracks metrics for a specific endpoint
type EndpointMetrics struct {
	Requests     int64
	Errors       int64
	TotalLatency int64
}

// Metrics holds all application metrics
type Metrics struct {
	mu sync.RWMutex

	// Request metrics
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64

	// Request latency (in milliseconds)
	TotalLatency int64
	RequestCount int64

	// Job metrics
	JobsCreated    int64
	JobsCompleted  int64
	JobsFailed     int64
	JobsProcessing int64

	// Task update metrics
	TasksUpdated      int64
	TaskUpdateErrors  int64
	TaskUpdateLatency int64

	// File upload metrics
	FilesUploaded      int64
	TotalBytesUploaded int64

	// WebSocket metrics
	WSConnections int64
	WSMessagesIn  int64
	WSMessagesOut int64

	// Authentication metrics
	LoginAttempts  int64
	LoginSuccesses int64
	LoginFailures  int64

	// Metadata sync metrics
	MetadataSyncs      int64
	MetadataSyncErrors int64

	// Report generation metrics
	ReportsGenerated int64
	ReportErrors     int64

	// Mapping metrics
	MappingsCreated   int64
	MappingsValidated int64

	// Endpoint-specific metrics
	EndpointMetrics map[string]*EndpointMetrics

	// Start time for uptime calculation
	StartTime time.Time
}

// global metrics instance
var globalMetrics *Metrics
var once sync.Once

// Init initializes the global metrics instance
func Init() {
	once.Do(func() {
		globalMetrics = &Metrics{
			StartTime:       time.Now(),
			EndpointMetrics: make(map[string]*EndpointMetrics),
		}
	})
}

// Get returns the global metrics instance
func Get() *Metrics {
	if globalMetrics == nil {
		Init()
	}
	return globalMetrics
}

// IncrementRequests increments request counters
func (m *Metrics) IncrementRequests(success bool, latencyMs int64) {
	atomic.AddInt64(&m.TotalRequests, 1)
	atomic.AddInt64(&m.TotalLatency, latencyMs)
	atomic.AddInt64(&m.RequestCount, 1)
	
	if success {
		atomic.AddInt64(&m.SuccessfulRequests, 1)
	} else {
		atomic.AddInt64(&m.FailedRequests, 1)
	}
}

// IncrementJobCreated increments job created counter
func (m *Metrics) IncrementJobCreated() {
	atomic.AddInt64(&m.JobsCreated, 1)
	atomic.AddInt64(&m.JobsProcessing, 1)
}

// IncrementJobCompleted increments job completed counter
func (m *Metrics) IncrementJobCompleted() {
	atomic.AddInt64(&m.JobsCompleted, 1)
	atomic.AddInt64(&m.JobsProcessing, -1)
}

// IncrementJobFailed increments job failed counter
func (m *Metrics) IncrementJobFailed() {
	atomic.AddInt64(&m.JobsFailed, 1)
	atomic.AddInt64(&m.JobsProcessing, -1)
}

// IncrementFileUpload increments file upload counters
func (m *Metrics) IncrementFileUpload(bytes int64) {
	atomic.AddInt64(&m.FilesUploaded, 1)
	atomic.AddInt64(&m.TotalBytesUploaded, bytes)
}

// IncrementWSConnection increments WebSocket connection counter
func (m *Metrics) IncrementWSConnection() {
	atomic.AddInt64(&m.WSConnections, 1)
}

// DecrementWSConnection decrements WebSocket connection counter
func (m *Metrics) DecrementWSConnection() {
	atomic.AddInt64(&m.WSConnections, -1)
}

// IncrementWSMessageIn increments WebSocket incoming message counter
func (m *Metrics) IncrementWSMessageIn() {
	atomic.AddInt64(&m.WSMessagesIn, 1)
}

// IncrementWSMessageOut increments WebSocket outgoing message counter
func (m *Metrics) IncrementWSMessageOut() {
	atomic.AddInt64(&m.WSMessagesOut, 1)
}

// IncrementLogin increments login counters
func (m *Metrics) IncrementLogin(success bool) {
	atomic.AddInt64(&m.LoginAttempts, 1)
	if success {
		atomic.AddInt64(&m.LoginSuccesses, 1)
	} else {
		atomic.AddInt64(&m.LoginFailures, 1)
	}
}

// IncrementMetadataSync increments metadata sync counters
func (m *Metrics) IncrementMetadataSync(success bool) {
	atomic.AddInt64(&m.MetadataSyncs, 1)
	if !success {
		atomic.AddInt64(&m.MetadataSyncErrors, 1)
	}
}

// IncrementTaskUpdate increments task update counters
func (m *Metrics) IncrementTaskUpdate(success bool, latencyMs int64) {
	if success {
		atomic.AddInt64(&m.TasksUpdated, 1)
	} else {
		atomic.AddInt64(&m.TaskUpdateErrors, 1)
	}
	atomic.AddInt64(&m.TaskUpdateLatency, latencyMs)
}

// IncrementReportGenerated increments report generation counters
func (m *Metrics) IncrementReportGenerated(success bool) {
	if success {
		atomic.AddInt64(&m.ReportsGenerated, 1)
	} else {
		atomic.AddInt64(&m.ReportErrors, 1)
	}
}

// IncrementMappingCreated increments mapping creation counter
func (m *Metrics) IncrementMappingCreated() {
	atomic.AddInt64(&m.MappingsCreated, 1)
}

// IncrementMappingValidated increments mapping validation counter
func (m *Metrics) IncrementMappingValidated() {
	atomic.AddInt64(&m.MappingsValidated, 1)
}

// TrackEndpoint tracks metrics for a specific endpoint
func (m *Metrics) TrackEndpoint(path, method string, statusCode int, latencyMs int64) {
	key := method + " " + path

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.EndpointMetrics == nil {
		m.EndpointMetrics = make(map[string]*EndpointMetrics)
	}

	em, exists := m.EndpointMetrics[key]
	if !exists {
		em = &EndpointMetrics{}
		m.EndpointMetrics[key] = em
	}

	atomic.AddInt64(&em.Requests, 1)
	atomic.AddInt64(&em.TotalLatency, latencyMs)
	if statusCode >= 400 {
		atomic.AddInt64(&em.Errors, 1)
	}
}

// GetEndpointMetrics returns a copy of endpoint metrics
func (m *Metrics) GetEndpointMetrics() map[string]EndpointMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]EndpointMetrics)
	for k, v := range m.EndpointMetrics {
		result[k] = EndpointMetrics{
			Requests:     atomic.LoadInt64(&v.Requests),
			Errors:       atomic.LoadInt64(&v.Errors),
			TotalLatency: atomic.LoadInt64(&v.TotalLatency),
		}
	}
	return result
}


// GetAverageLatency returns average request latency in milliseconds
func (m *Metrics) GetAverageLatency() float64 {
	count := atomic.LoadInt64(&m.RequestCount)
	if count == 0 {
		return 0
	}
	total := atomic.LoadInt64(&m.TotalLatency)
	return float64(total) / float64(count)
}

// GetUptime returns the application uptime
func (m *Metrics) GetUptime() time.Duration {
	return time.Since(m.StartTime)
}

// EndpointMetricsSnapshot represents endpoint metrics in a snapshot
type EndpointMetricsSnapshot struct {
	Requests     int64   `json:"requests"`
	Errors       int64   `json:"errors"`
	ErrorRate    float64 `json:"error_rate"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

// MetricsSnapshot represents a point-in-time snapshot of all metrics
type MetricsSnapshot struct {
	// Uptime
	UptimeSeconds float64 `json:"uptime_seconds"`
	StartTime     string  `json:"start_time"`

	// Request metrics
	Requests struct {
		Total        int64   `json:"total"`
		Successful   int64   `json:"successful"`
		Failed       int64   `json:"failed"`
		AvgLatencyMs float64 `json:"avg_latency_ms"`
	} `json:"requests"`

	// Job metrics
	Jobs struct {
		Created    int64 `json:"created"`
		Completed  int64 `json:"completed"`
		Failed     int64 `json:"failed"`
		Processing int64 `json:"processing"`
	} `json:"jobs"`

	// Task update metrics
	TaskUpdates struct {
		Updated      int64   `json:"updated"`
		Errors       int64   `json:"errors"`
		AvgLatencyMs float64 `json:"avg_latency_ms"`
	} `json:"task_updates"`

	// File metrics
	Files struct {
		Uploaded   int64 `json:"uploaded"`
		TotalBytes int64 `json:"total_bytes"`
	} `json:"files"`

	// WebSocket metrics
	WebSocket struct {
		Connections int64 `json:"connections"`
		MessagesIn  int64 `json:"messages_in"`
		MessagesOut int64 `json:"messages_out"`
	} `json:"websocket"`

	// Auth metrics
	Auth struct {
		LoginAttempts  int64 `json:"login_attempts"`
		LoginSuccesses int64 `json:"login_successes"`
		LoginFailures  int64 `json:"login_failures"`
	} `json:"auth"`

	// Metadata metrics
	Metadata struct {
		Syncs  int64 `json:"syncs"`
		Errors int64 `json:"errors"`
	} `json:"metadata"`

	// Report metrics
	Reports struct {
		Generated int64 `json:"generated"`
		Errors    int64 `json:"errors"`
	} `json:"reports"`

	// Mapping metrics
	Mappings struct {
		Created   int64 `json:"created"`
		Validated int64 `json:"validated"`
	} `json:"mappings"`

	// System metrics
	System struct {
		Goroutines   int    `json:"goroutines"`
		HeapAllocMB  uint64 `json:"heap_alloc_mb"`
		HeapInUseMB  uint64 `json:"heap_inuse_mb"`
		StackInUseMB uint64 `json:"stack_inuse_mb"`
		NumGC        uint32 `json:"num_gc"`
	} `json:"system"`

	// Endpoint-specific metrics (top endpoints by request count)
	Endpoints map[string]EndpointMetricsSnapshot `json:"endpoints,omitempty"`
}

// Snapshot returns a point-in-time snapshot of all metrics
func (m *Metrics) Snapshot() MetricsSnapshot {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	snapshot := MetricsSnapshot{}

	// Uptime
	snapshot.UptimeSeconds = m.GetUptime().Seconds()
	snapshot.StartTime = m.StartTime.Format(time.RFC3339)

	// Request metrics
	snapshot.Requests.Total = atomic.LoadInt64(&m.TotalRequests)
	snapshot.Requests.Successful = atomic.LoadInt64(&m.SuccessfulRequests)
	snapshot.Requests.Failed = atomic.LoadInt64(&m.FailedRequests)
	snapshot.Requests.AvgLatencyMs = m.GetAverageLatency()

	// Job metrics
	snapshot.Jobs.Created = atomic.LoadInt64(&m.JobsCreated)
	snapshot.Jobs.Completed = atomic.LoadInt64(&m.JobsCompleted)
	snapshot.Jobs.Failed = atomic.LoadInt64(&m.JobsFailed)
	snapshot.Jobs.Processing = atomic.LoadInt64(&m.JobsProcessing)

	// Task update metrics
	tasksUpdated := atomic.LoadInt64(&m.TasksUpdated)
	taskLatency := atomic.LoadInt64(&m.TaskUpdateLatency)
	snapshot.TaskUpdates.Updated = tasksUpdated
	snapshot.TaskUpdates.Errors = atomic.LoadInt64(&m.TaskUpdateErrors)
	if tasksUpdated > 0 {
		snapshot.TaskUpdates.AvgLatencyMs = float64(taskLatency) / float64(tasksUpdated)
	}

	// File metrics
	snapshot.Files.Uploaded = atomic.LoadInt64(&m.FilesUploaded)
	snapshot.Files.TotalBytes = atomic.LoadInt64(&m.TotalBytesUploaded)

	// WebSocket metrics
	snapshot.WebSocket.Connections = atomic.LoadInt64(&m.WSConnections)
	snapshot.WebSocket.MessagesIn = atomic.LoadInt64(&m.WSMessagesIn)
	snapshot.WebSocket.MessagesOut = atomic.LoadInt64(&m.WSMessagesOut)

	// Auth metrics
	snapshot.Auth.LoginAttempts = atomic.LoadInt64(&m.LoginAttempts)
	snapshot.Auth.LoginSuccesses = atomic.LoadInt64(&m.LoginSuccesses)
	snapshot.Auth.LoginFailures = atomic.LoadInt64(&m.LoginFailures)

	// Metadata metrics
	snapshot.Metadata.Syncs = atomic.LoadInt64(&m.MetadataSyncs)
	snapshot.Metadata.Errors = atomic.LoadInt64(&m.MetadataSyncErrors)

	// Report metrics
	snapshot.Reports.Generated = atomic.LoadInt64(&m.ReportsGenerated)
	snapshot.Reports.Errors = atomic.LoadInt64(&m.ReportErrors)

	// Mapping metrics
	snapshot.Mappings.Created = atomic.LoadInt64(&m.MappingsCreated)
	snapshot.Mappings.Validated = atomic.LoadInt64(&m.MappingsValidated)

	// System metrics
	snapshot.System.Goroutines = runtime.NumGoroutine()
	snapshot.System.HeapAllocMB = memStats.HeapAlloc / 1024 / 1024
	snapshot.System.HeapInUseMB = memStats.HeapInuse / 1024 / 1024
	snapshot.System.StackInUseMB = memStats.StackInuse / 1024 / 1024
	snapshot.System.NumGC = memStats.NumGC

	// Endpoint metrics
	endpointMetrics := m.GetEndpointMetrics()
	if len(endpointMetrics) > 0 {
		snapshot.Endpoints = make(map[string]EndpointMetricsSnapshot)
		for k, v := range endpointMetrics {
			em := EndpointMetricsSnapshot{
				Requests: v.Requests,
				Errors:   v.Errors,
			}
			if v.Requests > 0 {
				em.ErrorRate = float64(v.Errors) / float64(v.Requests) * 100
				em.AvgLatencyMs = float64(v.TotalLatency) / float64(v.Requests)
			}
			snapshot.Endpoints[k] = em
		}
	}

	return snapshot
}

// HealthStatus represents the health status of a component
type HealthStatus struct {
	Status  string `json:"status"` // "healthy", "degraded", "unhealthy"
	Message string `json:"message,omitempty"`
	Latency int64  `json:"latency_ms,omitempty"`
}

// HealthCheck represents the overall health check response
type HealthCheck struct {
	Status     string                  `json:"status"` // "healthy", "degraded", "unhealthy"
	Version    string                  `json:"version"`
	Uptime     string                  `json:"uptime"`
	Timestamp  string                  `json:"timestamp"`
	Components map[string]HealthStatus `json:"components"`
}

// CheckDatabaseHealth checks database connectivity
func CheckDatabaseHealth(db *sql.DB) HealthStatus {
	start := time.Now()
	
	if db == nil {
		return HealthStatus{
			Status:  "unhealthy",
			Message: "database connection not initialized",
		}
	}

	err := db.Ping()
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return HealthStatus{
			Status:  "unhealthy",
			Message: err.Error(),
			Latency: latency,
		}
	}

	// Check if latency is acceptable (< 100ms)
	if latency > 100 {
		return HealthStatus{
			Status:  "degraded",
			Message: "high latency",
			Latency: latency,
		}
	}

	return HealthStatus{
		Status:  "healthy",
		Latency: latency,
	}
}

// CheckMemoryHealth checks memory usage
func CheckMemoryHealth(maxHeapMB uint64) HealthStatus {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	heapMB := memStats.HeapAlloc / 1024 / 1024

	if heapMB > maxHeapMB {
		return HealthStatus{
			Status:  "unhealthy",
			Message: "heap memory exceeds limit",
		}
	}

	// Warn if using more than 80% of limit
	if heapMB > (maxHeapMB * 80 / 100) {
		return HealthStatus{
			Status:  "degraded",
			Message: "heap memory usage high",
		}
	}

	return HealthStatus{
		Status: "healthy",
	}
}

// DetermineOverallStatus determines overall health from component statuses
func DetermineOverallStatus(components map[string]HealthStatus) string {
	hasUnhealthy := false
	hasDegraded := false

	for _, status := range components {
		switch status.Status {
		case "unhealthy":
			hasUnhealthy = true
		case "degraded":
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return "unhealthy"
	}
	if hasDegraded {
		return "degraded"
	}
	return "healthy"
}
