package handler

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/model"
	"github.com/cleberrangel/clickup-excel-api/internal/service"
	"github.com/gin-gonic/gin"
)

// forceGC força garbage collection e libera memória ao OS
func forceGC() {
	runtime.GC()
	debug.FreeOSMemory()
	log.Printf("[GC] Memória liberada")
}

// ReportHandler manipula requisições de relatório
type ReportHandler struct {
	reportService  *service.ReportService
	webhookService *service.WebhookService
}

// NewReportHandler cria um novo handler de relatórios
func NewReportHandler(reportService *service.ReportService, webhookService *service.WebhookService) *ReportHandler {
	return &ReportHandler{
		reportService:  reportService,
		webhookService: webhookService,
	}
}

// GenerateReport gera um relatório Excel
// @Summary      Gera relatório Excel
// @Description  Busca tarefas do ClickUp e retorna um arquivo Excel ou processa via webhook
// @Tags         reports
// @Accept       json
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Security     BearerAuth
// @Param        request body model.ReportRequest true "Configuração do relatório"
// @Success      200 {object} model.Response "Quando webhook_url é fornecido"
// @Success      200 {file} binary "Arquivo Excel quando webhook_url não é fornecido"
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /api/v1/reports [post]
func (h *ReportHandler) GenerateReport(c *gin.Context) {
	var req model.ReportRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "payload inválido",
			Details: err.Error(),
		})
		return
	}

	// Validações adicionais
	if len(req.ListIDs) == 0 {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "list_ids não pode estar vazio",
		})
		return
	}

	if len(req.Fields) == 0 {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "fields não pode estar vazio",
		})
		return
	}

	log.Printf("[Report] Iniciando geração - Listas: %d, Campos: %d, Webhook: %s",
		len(req.ListIDs), len(req.Fields), req.WebhookURL)

	// Se webhook_url foi fornecido, processa de forma assíncrona
	if req.WebhookURL != "" {
		go h.processAsync(req)

		c.JSON(http.StatusOK, model.Response{
			Success: true,
		})
		return
	}

	// Processamento síncrono (sem webhook)
	result, err := h.reportService.GenerateReport(c.Request.Context(), req)
	if err != nil {
		forceGC() // Libera memória mesmo em caso de erro
		h.handleError(c, err)
		return
	}

	log.Printf("[Report] Relatório gerado - Tarefas: %d, Listas: %d",
		result.TotalTasks, result.TotalLists)

	// Configura headers de resposta
	filename := fmt.Sprintf("relatorio_%s.xlsx", time.Now().Format("2006-01-02_15-04-05"))

	file, err := os.Open(result.FilePath)
	if err != nil {
		h.handleError(c, err)
		return
	}
	defer file.Close()
	defer os.Remove(result.FilePath)

	stat, _ := file.Stat()

	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	if stat != nil {
		c.Header("Content-Length", fmt.Sprintf("%d", stat.Size()))
	}
	c.Header("X-Total-Tasks", fmt.Sprintf("%d", result.TotalTasks))
	c.Header("X-Total-Lists", fmt.Sprintf("%d", result.TotalLists))

	if _, err := io.Copy(c.Writer, file); err != nil {
		h.handleError(c, err)
		return
	}
}

// processAsync processa o relatório de forma assíncrona e envia para o webhook
func (h *ReportHandler) processAsync(req model.ReportRequest) {
	// Garante liberação de memória ao final
	defer forceGC()

	// Timeout de 30 minutos para processar até 35k+ tasks com retries
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	log.Printf("[Report Async] Iniciando processamento para webhook: %s (listas: %d, campos: %d)",
		req.WebhookURL, len(req.ListIDs), len(req.Fields))

	result, err := h.reportService.GenerateReport(ctx, req)
	if err != nil {
		log.Printf("[Report Async] Erro ao gerar relatório: %v", err)
		// Contexto separado para webhook de erro (5 minutos)
		webhookCtx, webhookCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer webhookCancel()
		if webhookErr := h.webhookService.SendError(webhookCtx, req.WebhookURL, err); webhookErr != nil {
			log.Printf("[Report Async] Erro ao enviar webhook de erro: %v", webhookErr)
		}
		return
	}

	log.Printf("[Report Async] Relatório gerado com sucesso - Tarefas: %d, Listas: %d, Folder: %s",
		result.TotalTasks, result.TotalLists, result.FolderName)

	stat, _ := os.Stat(result.FilePath)
	size := int64(0)
	if stat != nil {
		size = stat.Size()
	}

	log.Printf("[Report Async] Enviando para webhook: %s (tamanho: %d bytes)",
		req.WebhookURL, size)

	// Contexto separado para webhook de sucesso (10 minutos para upload de arquivo grande)
	webhookCtx, webhookCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer webhookCancel()

	if err := h.webhookService.SendSuccess(webhookCtx, req.WebhookURL, result); err != nil {
		log.Printf("[Report Async] Erro ao enviar webhook de sucesso: %v", err)
	} else {
		log.Printf("[Report Async] Webhook enviado com sucesso!")
	}
}

// handleError trata erros e retorna resposta apropriada
func (h *ReportHandler) handleError(c *gin.Context, err error) {
	log.Printf("[Report] Erro ao gerar relatório: %v", err)

	switch err {
	case model.ErrRateLimited:
		c.JSON(http.StatusTooManyRequests, model.ErrorResponse{
			Success: false,
			Error:   "rate limit excedido",
			Details: "aguarde alguns segundos e tente novamente",
		})
	case model.ErrUnauthorized:
		c.JSON(http.StatusUnauthorized, model.ErrorResponse{
			Success: false,
			Error:   "token do ClickUp inválido",
			Details: "verifique a variável TOKEN_CLICKUP",
		})
	case model.ErrNotFound:
		c.JSON(http.StatusNotFound, model.ErrorResponse{
			Success: false,
			Error:   "lista não encontrada",
			Details: "verifique os IDs das listas",
		})
	case model.ErrTimeout:
		c.JSON(http.StatusGatewayTimeout, model.ErrorResponse{
			Success: false,
			Error:   "timeout na requisição",
			Details: "a API do ClickUp demorou muito para responder",
		})
	default:
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Success: false,
			Error:   "erro interno",
			Details: err.Error(),
		})
	}
}
