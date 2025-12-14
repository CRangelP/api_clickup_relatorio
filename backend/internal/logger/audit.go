package logger

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// AuditAction represents the type of action being audited
type AuditAction string

const (
	// Authentication actions
	AuditActionLogin          AuditAction = "LOGIN"
	AuditActionLogout         AuditAction = "LOGOUT"
	AuditActionLoginFailed    AuditAction = "LOGIN_FAILED"
	AuditActionPasswordChange AuditAction = "PASSWORD_CHANGE"
	AuditActionUserCreate     AuditAction = "USER_CREATE"
	AuditActionSessionExpired AuditAction = "SESSION_EXPIRED"

	// File operations
	AuditActionFileUpload  AuditAction = "FILE_UPLOAD"
	AuditActionFileDelete  AuditAction = "FILE_DELETE"
	AuditActionFileProcess AuditAction = "FILE_PROCESS"

	// Job operations
	AuditActionJobCreate   AuditAction = "JOB_CREATE"
	AuditActionJobStart    AuditAction = "JOB_START"
	AuditActionJobProgress AuditAction = "JOB_PROGRESS"
	AuditActionJobComplete AuditAction = "JOB_COMPLETE"
	AuditActionJobFailed   AuditAction = "JOB_FAILED"
	AuditActionJobRetry    AuditAction = "JOB_RETRY"

	// Configuration operations
	AuditActionConfigUpdate AuditAction = "CONFIG_UPDATE"
	AuditActionTokenUpdate  AuditAction = "TOKEN_UPDATE"
	AuditActionMetadataSync AuditAction = "METADATA_SYNC"

	// History operations
	AuditActionHistoryClear AuditAction = "HISTORY_CLEAR"

	// Mapping operations
	AuditActionMappingCreate   AuditAction = "MAPPING_CREATE"
	AuditActionMappingDelete   AuditAction = "MAPPING_DELETE"
	AuditActionMappingValidate AuditAction = "MAPPING_VALIDATE"

	// Report operations
	AuditActionReportGenerate AuditAction = "REPORT_GENERATE"
	AuditActionReportDownload AuditAction = "REPORT_DOWNLOAD"

	// Task operations
	AuditActionTaskUpdate AuditAction = "TASK_UPDATE"
	AuditActionTaskBatch  AuditAction = "TASK_BATCH"

	// WebSocket operations
	AuditActionWSConnect    AuditAction = "WS_CONNECT"
	AuditActionWSDisconnect AuditAction = "WS_DISCONNECT"

	// API operations
	AuditActionAPIRequest AuditAction = "API_REQUEST"
	AuditActionAPIError   AuditAction = "API_ERROR"
)

// AuditEvent represents an audit log entry
type AuditEvent struct {
	Action      AuditAction
	UserID      string
	Username    string
	Resource    string
	ResourceID  string
	Details     map[string]interface{}
	ClientIP    string
	RequestID   string
	OperationID string
	Success     bool
	Error       string
	Duration    int64 // Duration in milliseconds
	Method      string
	Path        string
	StatusCode  int
}

// auditLogger is a specialized logger for audit events
var auditLogger zerolog.Logger

// InitAudit initializes the audit logger
func InitAudit() {
	auditLogger = globalLogger.With().Str("log_type", "audit").Logger()
}

// Audit logs an audit event
func Audit(ctx context.Context, event AuditEvent) {
	requestID := GetRequestID(ctx)
	if requestID != "" && event.RequestID == "" {
		event.RequestID = requestID
	}

	operationID := GetOperationID(ctx)
	if operationID != "" && event.OperationID == "" {
		event.OperationID = operationID
	}

	// Get user info from context if not provided
	if event.UserID == "" {
		event.UserID = GetUserID(ctx)
	}
	if event.Username == "" {
		event.Username = GetUsername(ctx)
	}

	logEvent := auditLogger.Info()
	if !event.Success {
		logEvent = auditLogger.Warn()
	}

	logEvent.
		Str("action", string(event.Action)).
		Str("user_id", event.UserID).
		Str("username", event.Username).
		Str("resource", event.Resource).
		Str("resource_id", event.ResourceID).
		Str("client_ip", event.ClientIP).
		Str("request_id", event.RequestID).
		Bool("success", event.Success).
		Time("timestamp", time.Now().UTC())

	if event.OperationID != "" {
		logEvent.Str("operation_id", event.OperationID)
	}

	if event.Error != "" {
		logEvent.Str("error", event.Error)
	}

	if event.Duration > 0 {
		logEvent.Int64("duration_ms", event.Duration)
	}

	if event.Method != "" {
		logEvent.Str("method", event.Method)
	}

	if event.Path != "" {
		logEvent.Str("path", event.Path)
	}

	if event.StatusCode > 0 {
		logEvent.Int("status_code", event.StatusCode)
	}

	if len(event.Details) > 0 {
		logEvent.Interface("details", event.Details)
	}

	logEvent.Msg("Audit event")
}

// AuditFromGin creates an audit event with context from Gin
func AuditFromGin(ctx context.Context, action AuditAction, userID, username, resource, resourceID string, success bool) {
	Audit(ctx, AuditEvent{
		Action:     action,
		UserID:     userID,
		Username:   username,
		Resource:   resource,
		ResourceID: resourceID,
		Success:    success,
	})
}

// AuditRequest logs an API request audit event
func AuditRequest(ctx context.Context, method, path string, statusCode int, duration int64, userID, clientIP string) {
	success := statusCode < 400
	action := AuditActionAPIRequest
	if !success {
		action = AuditActionAPIError
	}

	Audit(ctx, AuditEvent{
		Action:     action,
		UserID:     userID,
		Resource:   "api",
		ResourceID: path,
		Method:     method,
		Path:       path,
		StatusCode: statusCode,
		Duration:   duration,
		ClientIP:   clientIP,
		Success:    success,
	})
}

// AuditJobProgress logs job progress for tracking
func AuditJobProgress(ctx context.Context, jobID int, userID string, processed, total, success, errors int) {
	Audit(ctx, AuditEvent{
		Action:     AuditActionJobProgress,
		UserID:     userID,
		Resource:   "job",
		ResourceID: string(rune(jobID)),
		Success:    true,
		Details: map[string]interface{}{
			"job_id":        jobID,
			"processed":     processed,
			"total":         total,
			"success_count": success,
			"error_count":   errors,
			"progress_pct":  float64(processed) / float64(total) * 100,
		},
	})
}

// AuditWebSocket logs WebSocket connection events
func AuditWebSocket(ctx context.Context, action AuditAction, userID, clientIP string, details map[string]interface{}) {
	Audit(ctx, AuditEvent{
		Action:   action,
		UserID:   userID,
		Resource: "websocket",
		ClientIP: clientIP,
		Success:  true,
		Details:  details,
	})
}
