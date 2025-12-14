package middleware

import (
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// HeaderRequestID é o header HTTP para request ID
	HeaderRequestID = "X-Request-ID"
	// HeaderTraceID é o header HTTP para trace ID (distributed tracing)
	HeaderTraceID = "X-Trace-ID"
	// HeaderSpanID é o header HTTP para span ID
	HeaderSpanID = "X-Span-ID"
)

// RequestID adiciona request_id único a cada requisição
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Usa ID do header se existir, senão gera novo (8 chars)
		requestID := c.GetHeader(HeaderRequestID)
		if requestID == "" {
			requestID = uuid.New().String()[:8]
		}

		// Trace ID para rastreamento distribuído
		traceID := c.GetHeader(HeaderTraceID)
		if traceID == "" {
			traceID = uuid.New().String()
		}

		// Adiciona ao contexto e header de resposta
		ctx := logger.WithRequestID(c.Request.Context(), requestID)
		ctx = logger.WithTraceID(ctx, traceID)
		c.Request = c.Request.WithContext(ctx)
		c.Header(HeaderRequestID, requestID)
		c.Header(HeaderTraceID, traceID)

		// Log da requisição com informações detalhadas
		log := logger.Get(ctx)
		log.Info().
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Str("query", c.Request.URL.RawQuery).
			Str("client_ip", c.ClientIP()).
			Str("user_agent", c.Request.UserAgent()).
			Str("referer", c.Request.Referer()).
			Int64("content_length", c.Request.ContentLength).
			Msg("Request started")

		c.Next()

		// Calcula duração
		duration := time.Since(start)
		statusCode := c.Writer.Status()

		// Log da resposta com métricas
		logEvent := log.Info()
		if statusCode >= 400 {
			logEvent = log.Warn()
		}
		if statusCode >= 500 {
			logEvent = log.Error()
		}

		logEvent.
			Int("status", statusCode).
			Int("size", c.Writer.Size()).
			Dur("latency", duration).
			Float64("latency_ms", float64(duration.Microseconds())/1000).
			Msg("Request completed")
	}
}

// EnhancedRequestLogging adiciona logging detalhado para operações críticas
func EnhancedRequestLogging() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Adiciona informações do usuário ao contexto se disponível
		userID, exists := c.Get("user_id")
		if exists {
			username, _ := c.Get("username")
			usernameStr := ""
			if username != nil {
				usernameStr = username.(string)
			}
			ctx := logger.WithUserInfo(c.Request.Context(), userID.(string), usernameStr)
			c.Request = c.Request.WithContext(ctx)
		}

		c.Next()
	}
}
