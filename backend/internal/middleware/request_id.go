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

		// Adiciona ao contexto e header de resposta
		ctx := logger.WithRequestID(c.Request.Context(), requestID)
		c.Request = c.Request.WithContext(ctx)
		c.Header(HeaderRequestID, requestID)

		// Log da requisição
		log := logger.Get(ctx)
		log.Info().
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Str("client_ip", c.ClientIP()).
			Msg("Request iniciado")

		c.Next()

		// Log da resposta
		log.Info().
			Int("status", c.Writer.Status()).
			Int("size", c.Writer.Size()).
			Dur("latency", time.Since(start)).
			Msg("Request finalizado")
	}
}
