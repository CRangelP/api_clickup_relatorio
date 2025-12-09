package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/cleberrangel/clickup-excel-api/internal/client"
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
	Buffer     *bytes.Buffer
	TotalTasks int
	TotalLists int
	FolderName string
}

// GenerateReport gera um relatório Excel a partir das listas e campos solicitados
// Usa streaming para baixo consumo de memória
func (s *ReportService) GenerateReport(ctx context.Context, req model.ReportRequest) (*ReportResult, error) {
	log.Printf("[Report] Iniciando geração com streaming - Listas: %d, Campos: %d", len(req.ListIDs), len(req.Fields))

	// 1. Cria storage temporário
	storage, err := repository.NewTaskStorage()
	if err != nil {
		return nil, fmt.Errorf("criar storage: %w", err)
	}
	defer storage.Close() // Cleanup automático

	// 2. Coleta tasks e salva no storage (não acumula em memória)
	log.Printf("[Report] Fase 1: Coletando tasks do ClickUp...")
	if err := s.clickupClient.GetTasksToStorage(ctx, req.ListIDs, storage); err != nil {
		return nil, fmt.Errorf("coletar tasks: %w", err)
	}

	totalTasks := storage.GetTaskCount()
	folderName := storage.GetFolderName()

	log.Printf("[Report] Fase 1 concluída: %d tasks coletadas, folder: %s", totalTasks, folderName)

	// 3. Gera Excel via streaming do storage
	log.Printf("[Report] Fase 2: Gerando Excel via streaming...")
	excelPath, err := s.excelGenerator.GenerateFromStorage(storage, req.Fields)
	if err != nil {
		return nil, fmt.Errorf("gerar excel: %w", err)
	}
	defer os.Remove(excelPath) // Cleanup do arquivo Excel

	log.Printf("[Report] Fase 2 concluída: Excel gerado em %s", excelPath)

	// 4. Lê o arquivo Excel para buffer (necessário para enviar via webhook)
	log.Printf("[Report] Fase 3: Carregando Excel para envio...")
	excelFile, err := os.Open(excelPath)
	if err != nil {
		return nil, fmt.Errorf("abrir excel: %w", err)
	}
	defer excelFile.Close()

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, excelFile); err != nil {
		return nil, fmt.Errorf("ler excel: %w", err)
	}

	log.Printf("[Report] Fase 3 concluída: Excel carregado (%d bytes)", buf.Len())

	return &ReportResult{
		Buffer:     buf,
		TotalTasks: totalTasks,
		TotalLists: len(req.ListIDs),
		FolderName: folderName,
	}, nil
}
