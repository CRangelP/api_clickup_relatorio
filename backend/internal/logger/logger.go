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
	RequestIDKey ctxKey = "request_id"
	LoggerKey    ctxKey = "logger"
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
		Logger()
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
