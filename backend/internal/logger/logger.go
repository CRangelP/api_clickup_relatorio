package logger

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type ctxKey string

const (
	RequestIDKey   ctxKey = "request_id"
	LoggerKey      ctxKey = "logger"
	UserIDKey      ctxKey = "user_id"
	UsernameKey    ctxKey = "username"
	OperationIDKey ctxKey = "operation_id"
	TraceIDKey     ctxKey = "trace_id"
)

var globalLogger zerolog.Logger

// Init inicializa o logger global
func Init(level string, jsonFormat bool) {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil || lvl == zerolog.NoLevel {
		lvl = zerolog.InfoLevel
	}

	var output io.Writer = os.Stdout
	if !jsonFormat {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
	}

	globalLogger = zerolog.New(output).
		Level(lvl).
		With().
		Timestamp().
		Str("service", "clickup-field-updater").
		Logger()

	// Initialize audit logger
	InitAudit()
}

// Global retorna o logger global
func Global() *zerolog.Logger {
	return &globalLogger
}

// Get retorna logger do contexto ou global
func Get(ctx context.Context) *zerolog.Logger {
	if ctx == nil {
		return &globalLogger
	}
	if l, ok := ctx.Value(LoggerKey).(*zerolog.Logger); ok {
		return l
	}
	return &globalLogger
}

// FromGin extrai o logger do contexto Gin
func FromGin(c *gin.Context) *zerolog.Logger {
	return Get(c.Request.Context())
}

// WithRequestID adiciona request_id ao logger e contexto
func WithRequestID(ctx context.Context, requestID string) context.Context {
	l := globalLogger.With().Str("request_id", requestID).Logger()
	ctx = context.WithValue(ctx, RequestIDKey, requestID)
	ctx = context.WithValue(ctx, LoggerKey, &l)
	return ctx
}

// WithUserInfo adiciona informações do usuário ao contexto e logger
func WithUserInfo(ctx context.Context, userID, username string) context.Context {
	existingLogger := Get(ctx)
	l := existingLogger.With().
		Str("user_id", userID).
		Str("username", username).
		Logger()
	ctx = context.WithValue(ctx, UserIDKey, userID)
	ctx = context.WithValue(ctx, UsernameKey, username)
	ctx = context.WithValue(ctx, LoggerKey, &l)
	return ctx
}

// WithOperationID adiciona um ID de operação ao contexto para rastreamento
func WithOperationID(ctx context.Context, operationID string) context.Context {
	existingLogger := Get(ctx)
	l := existingLogger.With().Str("operation_id", operationID).Logger()
	ctx = context.WithValue(ctx, OperationIDKey, operationID)
	ctx = context.WithValue(ctx, LoggerKey, &l)
	return ctx
}

// WithTraceID adiciona um trace ID para rastreamento distribuído
func WithTraceID(ctx context.Context, traceID string) context.Context {
	existingLogger := Get(ctx)
	l := existingLogger.With().Str("trace_id", traceID).Logger()
	ctx = context.WithValue(ctx, TraceIDKey, traceID)
	ctx = context.WithValue(ctx, LoggerKey, &l)
	return ctx
}

// GetRequestID extrai request_id do contexto
func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// GetUserID extrai user_id do contexto
func GetUserID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(UserIDKey).(string); ok {
		return id
	}
	return ""
}

// GetUsername extrai username do contexto
func GetUsername(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if name, ok := ctx.Value(UsernameKey).(string); ok {
		return name
	}
	return ""
}

// GetOperationID extrai operation_id do contexto
func GetOperationID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(OperationIDKey).(string); ok {
		return id
	}
	return ""
}

// TraceContext retorna todas as informações de rastreamento do contexto
func TraceContext(ctx context.Context) map[string]string {
	return map[string]string{
		"request_id":   GetRequestID(ctx),
		"user_id":      GetUserID(ctx),
		"username":     GetUsername(ctx),
		"operation_id": GetOperationID(ctx),
	}
}
