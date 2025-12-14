package handler

import (
	"net/http"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/metrics"
	"github.com/cleberrangel/clickup-excel-api/internal/middleware"
	"github.com/cleberrangel/clickup-excel-api/internal/model"
	"github.com/cleberrangel/clickup-excel-api/internal/service"
	"github.com/gin-gonic/gin"
)

// UploadHandler handles file upload requests
type UploadHandler struct {
	uploadService *service.UploadService
}

// NewUploadHandler creates a new upload handler
func NewUploadHandler(uploadService *service.UploadService) *UploadHandler {
	return &UploadHandler{
		uploadService: uploadService,
	}
}

// UploadFile handles file upload and returns preview
// @Summary      Upload file for processing
// @Description  Uploads a CSV or XLSX file and returns column list and preview
// @Tags         upload
// @Accept       multipart/form-data
// @Produce      json
// @Security     BasicAuth
// @Param        file formance file true "CSV or XLSX file to upload"
// @Success      200 {object} FileUploadResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      413 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /api/v1/upload [post]
func (h *UploadHandler) UploadFile(c *gin.Context) {
	log := logger.FromGin(c)
	
	// Get uploaded file
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		log.Error().Err(err).Msg("Erro ao obter arquivo do formulário")
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "arquivo não encontrado no formulário",
			Details: "use o campo 'file' para enviar o arquivo",
		})
		return
	}
	defer file.Close()

	// Sanitize filename to prevent path traversal and other attacks
	sanitizedFilename := middleware.SanitizeFilename(header.Filename)
	
	// Validate file format first
	if err := h.uploadService.ValidateFileFormat(sanitizedFilename); err != nil {
		log.Warn().Str("filename", sanitizedFilename).Msg("Formato de arquivo inválido")
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "formato de arquivo não suportado",
			Details: "apenas arquivos CSV e XLSX são aceitos",
		})
		return
	}
	
	log.Info().
		Str("filename", sanitizedFilename).
		Int64("size", header.Size).
		Msg("Processando upload de arquivo")
	
	// Process file with sanitized filename
	result, err := h.uploadService.ProcessFile(sanitizedFilename, file, header.Size)
	if err != nil {
		log.Error().Err(err).Str("filename", header.Filename).Msg("Erro ao processar arquivo")
		
		switch err {
		case service.ErrFileTooLarge:
			c.JSON(http.StatusRequestEntityTooLarge, model.ErrorResponse{
				Success: false,
				Error:   "arquivo muito grande",
				Details: "o limite máximo é 10MB",
			})
		case service.ErrEmptyFile:
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Success: false,
				Error:   "arquivo vazio",
				Details: "o arquivo não contém dados",
			})
		case service.ErrNoColumns:
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Success: false,
				Error:   "arquivo sem colunas",
				Details: "o arquivo não contém cabeçalhos de coluna",
			})
		case service.ErrUnsupportedType:
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Success: false,
				Error:   "formato não suportado",
				Details: "apenas arquivos CSV e XLSX são aceitos",
			})
		default:
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Success: false,
				Error:   "erro ao processar arquivo",
				Details: err.Error(),
			})
		}
		return
	}
	
	log.Info().
		Str("filename", result.Filename).
		Int("columns", len(result.Columns)).
		Int("total_rows", result.TotalRows).
		Msg("Arquivo processado com sucesso")
	
	// Get user info for audit
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	userIDStr := ""
	usernameStr := ""
	if userID != nil {
		userIDStr = userID.(string)
	}
	if username != nil {
		usernameStr = username.(string)
	}

	// Audit file upload
	logger.Audit(c.Request.Context(), logger.AuditEvent{
		Action:     logger.AuditActionFileUpload,
		UserID:     userIDStr,
		Username:   usernameStr,
		Resource:   "file",
		ResourceID: result.TempPath,
		ClientIP:   c.ClientIP(),
		Success:    true,
		Details: map[string]interface{}{
			"filename":   result.Filename,
			"size":       result.Size,
			"columns":    len(result.Columns),
			"total_rows": result.TotalRows,
		},
	})
	metrics.Get().IncrementFileUpload(result.Size)

	c.JSON(http.StatusOK, FileUploadResponse{
		Success: true,
		Data: FileUploadData{
			Filename:    result.Filename,
			Size:        result.Size,
			ContentType: result.ContentType,
			Columns:     result.Columns,
			Preview:     result.Preview,
			TempPath:    result.TempPath,
			TotalRows:   result.TotalRows,
		},
	})
}

// FileUploadResponse represents the response for file upload
type FileUploadResponse struct {
	Success bool           `json:"success"`
	Data    FileUploadData `json:"data"`
}

// FileUploadData contains the uploaded file information
type FileUploadData struct {
	Filename    string     `json:"filename"`
	Size        int64      `json:"size"`
	ContentType string     `json:"content_type"`
	Columns     []string   `json:"columns"`
	Preview     [][]string `json:"preview"`
	TempPath    string     `json:"temp_path"`
	TotalRows   int        `json:"total_rows"`
}

// DeleteTempFile handles deletion of temporary files
// @Summary      Delete temporary file
// @Description  Deletes a temporary file after processing is complete
// @Tags         upload
// @Accept       json
// @Produce      json
// @Security     BasicAuth
// @Param        request body DeleteTempFileRequest true "Temp file path"
// @Success      200 {object} model.Response
// @Failure      400 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /api/v1/upload/cleanup [post]
func (h *UploadHandler) DeleteTempFile(c *gin.Context) {
	log := logger.FromGin(c)
	
	var req DeleteTempFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "payload inválido",
			Details: err.Error(),
		})
		return
	}

	// Validate path to prevent path traversal attacks
	if !middleware.ValidateFilePath(req.TempPath, "") {
		log.Warn().Str("path", req.TempPath).Msg("Tentativa de path traversal detectada")
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "caminho de arquivo inválido",
			Details: "o caminho contém caracteres não permitidos",
		})
		return
	}
	
	if err := h.uploadService.RemoveTempFile(req.TempPath); err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Success: false,
			Error:   "erro ao remover arquivo temporário",
			Details: err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, model.Response{
		Success: true,
	})
}

// DeleteTempFileRequest represents the request to delete a temp file
type DeleteTempFileRequest struct {
	TempPath string `json:"temp_path" binding:"required"`
}
