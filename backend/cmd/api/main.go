package main

import (
	stdlog "log"
	"os"
	"runtime"
	"runtime/debug"

	"github.com/cleberrangel/clickup-excel-api/internal/client"
	"github.com/cleberrangel/clickup-excel-api/internal/config"
	"github.com/cleberrangel/clickup-excel-api/internal/handler"
	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/middleware"
	"github.com/cleberrangel/clickup-excel-api/internal/service"
	"github.com/gin-gonic/gin"
)

const Version = "1.4.2"

func main() {
	// Carrega configurações
	cfg, err := config.Load()
	if err != nil {
		stdlog.Fatalf("Erro ao carregar configurações: %v", err)
	}

	// Inicializa logger estruturado
	logger.Init(cfg.LogLevel, cfg.LogJSON)
	log := logger.Global()
	log.Info().
		Str("version", Version).
		Str("port", cfg.Port).
		Str("log_level", cfg.LogLevel).
		Bool("log_json", cfg.LogJSON).
		Msg("ClickUp Excel API iniciando")

	// Inicializa dependências
	clickupClient := client.NewClient(cfg.TokenClickUp)
	reportService := service.NewReportService(clickupClient)
	webhookService := service.NewWebhookService()
	reportHandler := handler.NewReportHandler(reportService, webhookService)

	// Configura modo do Gin
	gin.SetMode(cfg.GinMode)

	// Inicializa router
	r := gin.New()
	r.Use(middleware.RequestID()) // Request ID + logging estruturado
	r.Use(gin.Recovery())

	// Health check (público)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Debug memory endpoint (público)
	r.GET("/debug/memory", func(c *gin.Context) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		c.JSON(200, gin.H{
			"alloc_mb":       m.Alloc / 1024 / 1024,
			"total_alloc_mb": m.TotalAlloc / 1024 / 1024,
			"sys_mb":         m.Sys / 1024 / 1024,
			"heap_alloc_mb":  m.HeapAlloc / 1024 / 1024,
			"heap_inuse_mb":  m.HeapInuse / 1024 / 1024,
			"heap_objects":   m.HeapObjects,
			"goroutines":     runtime.NumGoroutine(),
			"gc_runs":        m.NumGC,
			"gc_pause_total": m.PauseTotalNs / 1000000, // ms
		})
	})

	// Force GC endpoint (público)
	r.POST("/debug/gc", func(c *gin.Context) {
		runtime.GC()
		debug.FreeOSMemory()
		c.JSON(200, gin.H{"status": "gc_completed"})
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
	log.Info().Str("port", port).Msg("Servidor iniciando")

	if err := r.Run(":" + port); err != nil {
		log.Fatal().Err(err).Msg("Erro ao iniciar servidor")
		os.Exit(1)
	}
}
