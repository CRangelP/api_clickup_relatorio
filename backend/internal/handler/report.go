package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/model"
	"github.com/cleberrangel/clickup-excel-api/internal/service"
	"github.com/gin-gonic/gin"
)

// forceGC força garbage collection e libera memória ao OS
func forceGC(ctx context.Context) {
	runtime.GC()
	debug.FreeOSMemory()
	logger.Get(ctx).Debug().Msg("GC executado, memória liberada")
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

	log := logger.FromGin(c)
	log.Info().
		Int("lists", len(req.ListIDs)).
		Int("fields", len(req.Fields)).
		Str("webhook", req.WebhookURL).
		Msg("Iniciando geração de relatório")

	// Se webhook_url foi fornecido, processa de forma assíncrona
	if req.WebhookURL != "" {
		// Faz estimativa rápida antes de iniciar processamento
		subtasks := false
		if req.Subtasks != nil {
			subtasks = *req.Subtasks
		}
		includeClosed := false
		if req.IncludeClosed != nil {
			includeClosed = *req.IncludeClosed
		}
		
		estimate, err := h.reportService.EstimateTasks(c.Request.Context(), req.ListIDs, subtasks, includeClosed)
		if err != nil {
			log.Warn().Err(err).Msg("Falha ao estimar tasks, continuando sem estimativa")
		}

		requestID := logger.GetRequestID(c.Request.Context())
		go h.processAsync(req, requestID)

		// Retorna resposta com estimativa (se disponível)
		response := gin.H{
			"success": true,
			"message": "Processamento iniciado",
		}
		if estimate != nil {
			response["estimate"] = gin.H{
				"lists":          len(estimate.Lists),
				"tasks_min":      estimate.TotalMin,
				"tasks_max":      estimate.TotalMax,
				"tasks_avg":      estimate.EstimatedAvg,
				"estimated_time": estimate.EstimatedTime,
			}
		}
		c.JSON(http.StatusOK, response)
		return
	}

	// Processamento síncrono (sem webhook)
	result, err := h.reportService.GenerateReport(c.Request.Context(), req)
	if err != nil {
		forceGC(c.Request.Context()) // Libera memória mesmo em caso de erro
		h.handleError(c, err)
		return
	}

	log.Info().
		Int("tasks", result.TotalTasks).
		Int("lists", result.TotalLists).
		Msg("Relatório gerado com sucesso")

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
func (h *ReportHandler) processAsync(req model.ReportRequest, requestID string) {
	// Timeout de 90 minutos para processar até 200k+ tasks com retries
	baseCtx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
	defer cancel()

	// Propaga request_id para o contexto async
	ctx := logger.WithRequestID(baseCtx, requestID)
	log := logger.Get(ctx)

	// Garante liberação de memória ao final
	defer forceGC(ctx)

	log.Info().
		Str("webhook", req.WebhookURL).
		Int("lists", len(req.ListIDs)).
		Int("fields", len(req.Fields)).
		Msg("Iniciando processamento assíncrono")

	result, err := h.reportService.GenerateReport(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("Erro ao gerar relatório")
		// Contexto separado para webhook de erro (5 minutos)
		webhookCtx, webhookCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer webhookCancel()
		if webhookErr := h.webhookService.SendError(webhookCtx, req.WebhookURL, err); webhookErr != nil {
			log.Error().Err(webhookErr).Msg("Erro ao enviar webhook de erro")
		}
		return
	}

	log.Info().
		Int("tasks", result.TotalTasks).
		Int("lists", result.TotalLists).
		Str("folder", result.FolderName).
		Msg("Relatório gerado com sucesso")

	stat, _ := os.Stat(result.FilePath)
	size := int64(0)
	if stat != nil {
		size = stat.Size()
	}

	log.Info().
		Str("webhook", req.WebhookURL).
		Int64("size_bytes", size).
		Msg("Enviando para webhook")

	// Contexto separado para webhook de sucesso (10 minutos para upload de arquivo grande)
	webhookCtx, webhookCancel := context.WithTimeout(ctx, 10*time.Minute)
	defer webhookCancel()

	if err := h.webhookService.SendSuccess(webhookCtx, req.WebhookURL, result); err != nil {
		log.Error().Err(err).Msg("Erro ao enviar webhook de sucesso")
	} else {
		log.Info().Msg("Webhook enviado com sucesso")
	}
}

// handleError trata erros e retorna resposta apropriada
func (h *ReportHandler) handleError(c *gin.Context, err error) {
	logger.FromGin(c).Error().Err(err).Msg("Erro ao gerar relatório")

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
