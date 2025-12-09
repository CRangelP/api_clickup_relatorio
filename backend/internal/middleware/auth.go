package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthConfig contém a configuração do middleware de autenticação
type AuthConfig struct {
	TokenAPI string
}

// BearerAuth retorna um middleware que valida o token Bearer
func BearerAuth(cfg AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")

		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "header Authorization ausente",
			})
			return
		}

		// Extrai o token do formato "Bearer {token}"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "formato inválido, esperado: Bearer {token}",
			})
			return
		}

		token := parts[1]

		if token != cfg.TokenAPI {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "token inválido",
			})
			return
		}

		c.Next()
	}
}
