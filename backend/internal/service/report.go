package service

import (
	"context"
	"fmt"

	"github.com/cleberrangel/clickup-excel-api/internal/client"
	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/model"
	"github.com/cleberrangel/clickup-excel-api/internal/repository"
)

// ReportService orquestra a geração de relatórios
type ReportService struct {
	clickupClient  *client.Client
	excelGenerator *ExcelGenerator
}

// NewReportService cria um novo serviço de relatórios
func NewReportService(clickupClient *client.Client) *ReportService {
	return &ReportService{
		clickupClient:  clickupClient,
		excelGenerator: NewExcelGenerator(),
	}
}

// ReportResult contém o resultado da geração do relatório
type ReportResult struct {
	FilePath   string
	TotalTasks int
	TotalLists int
	FolderName string
}

// EstimateTasks faz uma estimativa rápida das tasks antes de coletar
func (s *ReportService) EstimateTasks(ctx context.Context, listIDs []string, subtasks bool) (*model.EstimateResult, error) {
	return s.clickupClient.EstimateTaskCount(ctx, listIDs, subtasks)
}

// GenerateReport gera um relatório Excel a partir das listas e campos solicitados
// Usa streaming para baixo consumo de memória
func (s *ReportService) GenerateReport(ctx context.Context, req model.ReportRequest) (*ReportResult, error) {
	log := logger.Get(ctx)
	log.Info().
		Int("lists", len(req.ListIDs)).
		Int("fields", len(req.Fields)).
		Msg("Iniciando geração de relatório")

	// 1. Cria storage temporário
	storage, err := repository.NewTaskStorage()
	if err != nil {
		return nil, fmt.Errorf("criar storage: %w", err)
	}
	defer storage.Close() // Cleanup automático

	// 2. Coleta tasks e salva no storage (não acumula em memória)
	// Default: false (apenas main tasks, sem subtasks)
	subtasks := false
	if req.Subtasks != nil {
		subtasks = *req.Subtasks
	}

	log.Info().Bool("subtasks", subtasks).Msg("Fase 1: Coletando tasks do ClickUp")
	if err := s.clickupClient.GetTasksToStorage(ctx, req.ListIDs, storage, subtasks); err != nil {
		return nil, fmt.Errorf("coletar tasks: %w", err)
	}

	totalTasks := storage.GetTaskCount()
	folderName := storage.GetFolderName()

	log.Info().
		Int("tasks", totalTasks).
		Str("folder", folderName).
		Msg("Fase 1 concluída: tasks coletadas")

	// 3. Gera Excel via streaming do storage
	log.Info().Msg("Fase 2: Gerando Excel via streaming")
	excelPath, err := s.excelGenerator.GenerateFromStorage(storage, req.Fields)
	if err != nil {
		return nil, fmt.Errorf("gerar excel: %w", err)
	}

	log.Info().Str("path", excelPath).Msg("Fase 2 concluída: Excel gerado")

	return &ReportResult{
		FilePath:   excelPath,
		TotalTasks: totalTasks,
		TotalLists: len(req.ListIDs),
		FolderName: folderName,
	}, nil
}
