package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/cleberrangel/clickup-excel-api/internal/client"
	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/model"
	"github.com/cleberrangel/clickup-excel-api/internal/service"
	"github.com/gin-gonic/gin"
)

// WebReportHandler handles report generation for web interface
type WebReportHandler struct {
	metadataService *service.MetadataService
}

// NewWebReportHandler creates a new web report handler
func NewWebReportHandler(metadataService *service.MetadataService) *WebReportHandler {
	return &WebReportHandler{
		metadataService: metadataService,
	}
}

// GenerateReport generates an Excel report using the user's stored ClickUp token
// @Summary      Generate Excel report (web)
// @Description  Generates an Excel report using the user's stored ClickUp token
// @Tags         reports
// @Accept       json
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Security     BasicAuth
// @Param        request body model.ReportRequest true "Report configuration"
// @Success      200 {file} binary "Excel file"
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /api/web/reports [post]
func (h *WebReportHandler) GenerateReport(c *gin.Context) {
	log := logger.FromGin(c)

	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, model.ErrorResponse{
			Success: false,
			Error:   "usuário não autenticado",
		})
		return
	}

	// Parse request
	var req model.ReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "payload inválido",
			Details: err.Error(),
		})
		return
	}

	// Validate request
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

	// Get user's ClickUp token
	token, err := h.metadataService.GetUserToken(c.Request.Context(), userID.(string))
	if err != nil {
		log.Error().Err(err).Str("user_id", userID.(string)).Msg("Erro ao obter token do usuário")
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "token ClickUp não configurado",
			Details: "Configure seu token na aba Configurações",
		})
		return
	}

	log.Info().
		Str("user_id", userID.(string)).
		Int("lists", len(req.ListIDs)).
		Int("fields", len(req.Fields)).
		Msg("Iniciando geração de relatório web")

	// Create ClickUp client with user's token
	clickupClient := client.NewClient(token)
	reportService := service.NewReportService(clickupClient)

	// Generate report
	result, err := reportService.GenerateReport(c.Request.Context(), req)
	if err != nil {
		log.Error().Err(err).Str("user_id", userID.(string)).Msg("Erro ao gerar relatório")
		h.handleError(c, err)
		return
	}

	log.Info().
		Str("user_id", userID.(string)).
		Int("tasks", result.TotalTasks).
		Int("lists", result.TotalLists).
		Msg("Relatório gerado com sucesso")

	// Configure response headers
	filename := fmt.Sprintf("%s.xlsx", result.FolderName)

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
		log.Error().Err(err).Msg("Erro ao enviar arquivo")
	}
}

// handleError handles errors and returns appropriate response
func (h *WebReportHandler) handleError(c *gin.Context, err error) {
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
			Details: "verifique seu token na aba Configurações",
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
