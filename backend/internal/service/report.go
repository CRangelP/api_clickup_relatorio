package service

import (
	"bytes"
	"context"

	"github.com/cleberrangel/clickup-excel-api/internal/client"
	"github.com/cleberrangel/clickup-excel-api/internal/model"
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
	Buffer     *bytes.Buffer
	TotalTasks int
	TotalLists int
}

// GenerateReport gera um relatório Excel a partir das listas e campos solicitados
func (s *ReportService) GenerateReport(ctx context.Context, req model.ReportRequest) (*ReportResult, error) {
	// Busca tarefas de todas as listas
	tasks, err := s.clickupClient.GetTasksMultiple(ctx, req.ListIDs)
	if err != nil {
		return nil, err
	}

	// Gera Excel
	buf, err := s.excelGenerator.Generate(tasks, req.Fields)
	if err != nil {
		return nil, err
	}

	return &ReportResult{
		Buffer:     buf,
		TotalTasks: len(tasks),
		TotalLists: len(req.ListIDs),
	}, nil
}
