package main

import (
	"log"
	"os"

	"github.com/cleberrangel/clickup-excel-api/internal/client"
	"github.com/cleberrangel/clickup-excel-api/internal/config"
	"github.com/cleberrangel/clickup-excel-api/internal/handler"
	"github.com/cleberrangel/clickup-excel-api/internal/middleware"
	"github.com/cleberrangel/clickup-excel-api/internal/service"
	"github.com/gin-gonic/gin"
)

func main() {
	// Carrega configurações
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Erro ao carregar configurações: %v", err)
	}

	// Inicializa dependências
	clickupClient := client.NewClient(cfg.TokenClickUp)
	reportService := service.NewReportService(clickupClient)
	webhookService := service.NewWebhookService()
	reportHandler := handler.NewReportHandler(reportService, webhookService)

	// Configura modo do Gin
	gin.SetMode(cfg.GinMode)

	// Inicializa router
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// Health check (público)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Grupo de rotas protegidas
	api := r.Group("/api/v1")
	api.Use(middleware.BearerAuth(middleware.AuthConfig{
		TokenAPI: cfg.TokenAPI,
	}))
	{
		api.POST("/reports", reportHandler.GenerateReport)
	}

	// Inicia servidor
	port := cfg.Port
	log.Printf("Servidor iniciando na porta %s", port)

	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Erro ao iniciar servidor: %v", err)
		os.Exit(1)
	}
}
