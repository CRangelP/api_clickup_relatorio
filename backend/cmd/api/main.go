package main

import (
	stdlog "log"
	"os"
	"runtime"
	"runtime/debug"

	"github.com/cleberrangel/clickup-excel-api/internal/client"
	"github.com/cleberrangel/clickup-excel-api/internal/config"
	"github.com/cleberrangel/clickup-excel-api/internal/database"
	"github.com/cleberrangel/clickup-excel-api/internal/handler"
	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/metrics"
	"github.com/cleberrangel/clickup-excel-api/internal/middleware"
	"github.com/cleberrangel/clickup-excel-api/internal/migration"
	"github.com/cleberrangel/clickup-excel-api/internal/repository"
	"github.com/cleberrangel/clickup-excel-api/internal/service"
	"github.com/cleberrangel/clickup-excel-api/internal/websocket"
	"github.com/gin-gonic/gin"
)

const Version = "1.4.3"

func main() {
	// Carrega configurações
	cfg, err := config.Load()
	if err != nil {
		stdlog.Fatalf("Erro ao carregar configurações: %v", err)
	}

	// Inicializa logger estruturado
	logger.Init(cfg.LogLevel, cfg.LogJSON)
	log := logger.Global()
	
	// Inicializa métricas
	metrics.Init()
	
	log.Info().
		Str("version", Version).
		Str("port", cfg.Port).
		Str("log_level", cfg.LogLevel).
		Bool("log_json", cfg.LogJSON).
		Msg("ClickUp Excel API iniciando")

	// Inicializa conexão com banco de dados
	dbConfig := database.Config{
		Host:            cfg.DBHost,
		Port:            cfg.DBPort,
		User:            cfg.DBUser,
		Password:        cfg.DBPassword,
		DBName:          cfg.DBName,
		SSLMode:         cfg.DBSSLMode,
		MaxOpenConns:    cfg.DBMaxOpenConns,
		MaxIdleConns:    cfg.DBMaxIdleConns,
		ConnMaxLifetime: cfg.DBConnMaxLifetime,
		ConnMaxIdleTime: cfg.DBConnMaxIdleTime,
	}
	
	db, err := database.Connect(dbConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("Erro ao conectar com o banco de dados")
		os.Exit(1)
	}
	defer database.Close(db)

	// Executa migrações
	migrator := migration.NewMigrator(db)
	if err := migrator.Run(); err != nil {
		log.Fatal().Err(err).Msg("Erro ao executar migrações")
		os.Exit(1)
	}

	// Inicializa repositórios
	metadataRepo := repository.NewMetadataRepository(db)
	queueRepo := repository.NewQueueRepository(db)
	configRepo := repository.NewConfigRepository(db)
	userRepo := repository.NewUserRepository(db)

	// Inicializa WebSocket hub
	wsHub := websocket.NewHub()
	go wsHub.Run() // Start hub in background

	// Inicializa dependências
	clickupClient := client.NewClient(cfg.TokenClickUp)
	reportService := service.NewReportService(clickupClient)
	webhookService := service.NewWebhookService()
	authService := service.NewAuthService(userRepo)
	uploadService := service.NewUploadService("")
	mappingService := service.NewMappingService(metadataRepo)
	
	// Inicializa QueueService
	queueService := service.NewQueueService(queueRepo, wsHub)
	
	// Inicializa TaskUpdateService e conecta ao QueueService
	taskUpdateService := service.NewTaskUpdateService(uploadService, metadataRepo, configRepo, queueRepo, wsHub)
	queueService.SetJobProcessor(taskUpdateService.ProcessJob)
	
	// Inicializa HistoryService
	historyService := service.NewHistoryService(queueRepo)
	
	// Inicializa MetadataService
	metadataService := service.NewMetadataService(metadataRepo, configRepo, cfg.EncryptionKey)
	
	// Inicializa handlers
	reportHandler := handler.NewReportHandler(reportService, webhookService)
	authHandler := handler.NewAuthHandler(authService)
	wsHandler := handler.NewWebSocketHandler(wsHub)
	uploadHandler := handler.NewUploadHandler(uploadService)
	mappingHandler := handler.NewMappingHandler(mappingService, uploadService)
	queueHandler := handler.NewQueueHandler(queueService, uploadService, mappingService)
	historyHandler := handler.NewHistoryHandler(historyService)
	metadataHandler := handler.NewMetadataHandler(metadataService, wsHub)
	configHandler := handler.NewConfigHandler(configRepo)
	webReportHandler := handler.NewWebReportHandler(metadataService)
	healthHandler := handler.NewHealthHandlerWithWebSocket(db, wsHub, Version)

	// Inicia limpeza de sessões expiradas
	authService.StartSessionCleanup()
	
	// Inicia processador de jobs em background
	queueService.Start()
	
	// Resume pending jobs after restart
	if err := queueService.ResumePendingJobs(); err != nil {
		log.Warn().Err(err).Msg("Erro ao retomar jobs pendentes")
	}

	// Log dos repositórios inicializados
	log.Info().Msg("Repositórios e serviços inicializados com sucesso")

	// Configura modo do Gin
	gin.SetMode(cfg.GinMode)

	// Inicializa router
	r := gin.New()
	r.Use(middleware.RequestID())        // Request ID + logging estruturado
	r.Use(middleware.MetricsMiddleware()) // Metrics collection
	r.Use(middleware.AuditMiddleware())   // Audit logging for sensitive operations
	r.Use(gin.Recovery())

	// Health check endpoints (públicos)
	r.GET("/health", healthHandler.DetailedHealthCheck)
	r.GET("/health/live", healthHandler.LivenessCheck)
	r.GET("/health/ready", healthHandler.ReadinessCheck)
	
	// Metrics endpoints (públicos)
	r.GET("/metrics", healthHandler.GetMetrics)
	r.GET("/metrics/summary", healthHandler.GetMetricsSummary)
	r.GET("/metrics/endpoints", healthHandler.GetEndpointMetrics)

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

	// Database pool stats endpoint (público)
	r.GET("/debug/db-pool", func(c *gin.Context) {
		stats := database.GetPoolStats(db)
		c.JSON(200, stats)
	})

	// Rotas de autenticação (públicas)
	auth := r.Group("/api/auth")
	{
		auth.POST("/login", authHandler.Login)
		auth.POST("/logout", authHandler.Logout)
	}

	// Auth status endpoint (requires auth but not CSRF for initial check)
	webAuth := r.Group("/api/web/auth")
	webAuth.Use(authService.GetAuthMiddleware().RequireAuth())
	{
		webAuth.GET("/status", authHandler.GetCurrentUser)
	}

	// Grupo de rotas protegidas por autenticação básica
	web := r.Group("/api/web")
	web.Use(authService.GetAuthMiddleware().RequireAuth())
	web.Use(authService.GetCSRFMiddleware().RequireCSRF())
	{
		web.GET("/user", authHandler.GetCurrentUser)
		web.POST("/user/password", authHandler.UpdatePassword)
		
		// WebSocket routes
		web.GET("/ws", websocket.AuthMiddleware(authService.GetAuthMiddleware()), wsHandler.HandleConnection)
		web.GET("/ws/stats", wsHandler.GetConnectionStats)
		web.GET("/ws/connections", wsHandler.GetUserConnections)
		web.POST("/ws/test", wsHandler.SendTestMessage)
		
		// Upload routes
		web.POST("/upload", uploadHandler.UploadFile)
		web.POST("/upload/cleanup", uploadHandler.DeleteTempFile)
		
		// Mapping routes
		web.POST("/mapping", mappingHandler.SaveMapping)
		web.GET("/mapping", mappingHandler.ListMappings)
		web.GET("/mapping/:id", mappingHandler.GetMapping)
		web.DELETE("/mapping/:id", mappingHandler.DeleteMapping)
		web.POST("/mapping/validate", mappingHandler.ValidateMapping)
		
		// Job queue routes
		web.POST("/jobs", queueHandler.CreateJob)
		web.GET("/jobs", queueHandler.ListJobs)
		web.GET("/jobs/:id", queueHandler.GetJob)
		
		// History routes
		web.GET("/history", historyHandler.ListHistory)
		web.GET("/history/:id", historyHandler.GetHistory)
		web.DELETE("/history", historyHandler.DeleteAllHistory)
		
		// Metadata routes
		web.POST("/metadata/sync", metadataHandler.SyncMetadata)
		web.GET("/metadata/hierarchy", metadataHandler.GetHierarchy)
		
		// Config routes
		web.GET("/config", configHandler.GetConfig)
		web.POST("/config", configHandler.SaveConfig)
		
		// Web report routes
		web.POST("/reports", webReportHandler.GenerateReport)
	}

	// Grupo de rotas protegidas por Bearer token (API externa)
	api := r.Group("/api/v1")
	api.Use(middleware.BearerAuth(middleware.AuthConfig{
		TokenAPI: cfg.TokenAPI,
	}))
	{
		api.POST("/reports", reportHandler.GenerateReport)
		// Endpoint para criar usuários (admin)
		api.POST("/users", authHandler.CreateUser)
	}

	// Inicia servidor
	port := cfg.Port
	log.Info().Str("port", port).Msg("Servidor iniciando")

	if err := r.Run(":" + port); err != nil {
		log.Fatal().Err(err).Msg("Erro ao iniciar servidor")
		os.Exit(1)
	}
}
