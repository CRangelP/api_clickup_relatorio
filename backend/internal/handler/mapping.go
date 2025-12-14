package handler

import (
	"net/http"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/middleware"
	"github.com/cleberrangel/clickup-excel-api/internal/model"
	"github.com/cleberrangel/clickup-excel-api/internal/service"
	"github.com/gin-gonic/gin"
)

// MappingHandler handles column mapping requests
type MappingHandler struct {
	mappingService *service.MappingService
	uploadService  *service.UploadService
}

// NewMappingHandler creates a new mapping handler
func NewMappingHandler(mappingService *service.MappingService, uploadService *service.UploadService) *MappingHandler {
	return &MappingHandler{
		mappingService: mappingService,
		uploadService:  uploadService,
	}
}

// SaveMappingRequest represents the request body for saving a mapping
type SaveMappingRequest struct {
	FilePath string                   `json:"file_path" binding:"required"`
	Mappings []service.ColumnMapping  `json:"mappings" binding:"required"`
	Title    string                   `json:"title" binding:"required"`
}

// MappingResponse represents the response for mapping operations
type MappingResponse struct {
	Success    bool                            `json:"success"`
	Data       *service.StoredMapping          `json:"data,omitempty"`
	Validation *service.MappingValidationResult `json:"validation,omitempty"`
}

// MappingListResponse represents the response for listing mappings
type MappingListResponse struct {
	Success bool                      `json:"success"`
	Data    []*service.StoredMapping  `json:"data"`
}


// SaveMapping handles POST /api/web/mapping - Save column mapping
// @Summary      Save column mapping
// @Description  Saves a column to custom field mapping for file processing
// @Tags         mapping
// @Accept       json
// @Produce      json
// @Security     BasicAuth
// @Param        request body SaveMappingRequest true "Mapping configuration"
// @Success      200 {object} MappingResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /api/web/mapping [post]
func (h *MappingHandler) SaveMapping(c *gin.Context) {
	log := logger.FromGin(c)

	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, model.ErrorResponse{
			Success: false,
			Error:   "usuário não autenticado",
		})
		return
	}

	var req SaveMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warn().Err(err).Msg("Payload inválido para mapeamento")
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "payload inválido",
			Details: err.Error(),
		})
		return
	}

	// Sanitize inputs
	req.Title = middleware.SanitizeTitle(req.Title)

	// Validate file path to prevent path traversal
	if !middleware.ValidateFilePath(req.FilePath, "") {
		log.Warn().Str("file_path", req.FilePath).Msg("Tentativa de path traversal detectada")
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "caminho de arquivo inválido",
		})
		return
	}

	// Sanitize mapping field IDs
	for i := range req.Mappings {
		req.Mappings[i].FieldID = middleware.SanitizeID(req.Mappings[i].FieldID)
		req.Mappings[i].FieldName = middleware.SanitizeTitle(req.Mappings[i].FieldName)
	}

	log.Info().
		Str("user_id", userID.(string)).
		Str("file_path", req.FilePath).
		Int("mappings_count", len(req.Mappings)).
		Msg("Processando salvamento de mapeamento")

	// Check for duplicate mappings first
	duplicates := h.mappingService.CheckDuplicateMappings(req.Mappings)
	if len(duplicates) > 0 {
		log.Warn().Strs("duplicates", duplicates).Msg("Mapeamentos duplicados detectados")
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "mapeamento duplicado detectado",
			Details: "campos duplicados: " + joinStrings(duplicates),
		})
		return
	}

	// Get file columns for validation
	columns, _, err := h.uploadService.GetFileData(req.FilePath)
	if err != nil {
		log.Error().Err(err).Str("file_path", req.FilePath).Msg("Erro ao ler arquivo para validação")
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "erro ao ler arquivo",
			Details: err.Error(),
		})
		return
	}

	// Convert request to service format
	mappingReq := &service.MappingRequest{
		FilePath: req.FilePath,
		Mappings: req.Mappings,
		Title:    req.Title,
	}

	// Validate and save mapping
	stored, validation, err := h.mappingService.ValidateAndSaveMapping(userID.(string), mappingReq, columns)
	if err != nil {
		log.Error().Err(err).Msg("Erro ao validar/salvar mapeamento")
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Success: false,
			Error:   "erro ao processar mapeamento",
			Details: err.Error(),
		})
		return
	}

	// If validation failed, return validation errors
	if !validation.Valid {
		log.Warn().Strs("errors", validation.Errors).Msg("Validação de mapeamento falhou")
		c.JSON(http.StatusBadRequest, MappingResponse{
			Success:    false,
			Validation: validation,
		})
		return
	}

	log.Info().
		Str("mapping_id", stored.ID).
		Str("user_id", userID.(string)).
		Msg("Mapeamento salvo com sucesso")

	c.JSON(http.StatusOK, MappingResponse{
		Success:    true,
		Data:       stored,
		Validation: validation,
	})
}


// GetMapping handles GET /api/web/mapping/:id - Get mapping by ID
// @Summary      Get mapping by ID
// @Description  Retrieves a saved column mapping by its ID
// @Tags         mapping
// @Produce      json
// @Security     BasicAuth
// @Param        id path string true "Mapping ID"
// @Success      200 {object} MappingResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      404 {object} model.ErrorResponse
// @Router       /api/web/mapping/{id} [get]
func (h *MappingHandler) GetMapping(c *gin.Context) {
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

	mappingID := middleware.SanitizeID(c.Param("id"))
	if mappingID == "" || !middleware.ValidateID(mappingID) {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "ID do mapeamento inválido",
		})
		return
	}

	log.Info().
		Str("mapping_id", mappingID).
		Str("user_id", userID.(string)).
		Msg("Buscando mapeamento")

	mapping, err := h.mappingService.GetMappingByUser(mappingID, userID.(string))
	if err != nil {
		if err == service.ErrMappingNotFound {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Success: false,
				Error:   "mapeamento não encontrado",
			})
			return
		}
		log.Error().Err(err).Str("mapping_id", mappingID).Msg("Erro ao buscar mapeamento")
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Success: false,
			Error:   "erro ao buscar mapeamento",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, MappingResponse{
		Success: true,
		Data:    mapping,
	})
}

// ListMappings handles GET /api/web/mapping - List user mappings
// @Summary      List user mappings
// @Description  Lists all mappings for the authenticated user
// @Tags         mapping
// @Produce      json
// @Security     BasicAuth
// @Success      200 {object} MappingListResponse
// @Failure      401 {object} model.ErrorResponse
// @Router       /api/web/mapping [get]
func (h *MappingHandler) ListMappings(c *gin.Context) {
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

	log.Info().Str("user_id", userID.(string)).Msg("Listando mapeamentos do usuário")

	mappings := h.mappingService.GetMappingsByUser(userID.(string))

	c.JSON(http.StatusOK, MappingListResponse{
		Success: true,
		Data:    mappings,
	})
}

// DeleteMapping handles DELETE /api/web/mapping/:id - Delete mapping
// @Summary      Delete mapping
// @Description  Deletes a saved column mapping
// @Tags         mapping
// @Produce      json
// @Security     BasicAuth
// @Param        id path string true "Mapping ID"
// @Success      200 {object} model.Response
// @Failure      401 {object} model.ErrorResponse
// @Failure      404 {object} model.ErrorResponse
// @Router       /api/web/mapping/{id} [delete]
func (h *MappingHandler) DeleteMapping(c *gin.Context) {
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

	mappingID := middleware.SanitizeID(c.Param("id"))
	if mappingID == "" || !middleware.ValidateID(mappingID) {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "ID do mapeamento inválido",
		})
		return
	}

	// Verify ownership first
	_, err := h.mappingService.GetMappingByUser(mappingID, userID.(string))
	if err != nil {
		if err == service.ErrMappingNotFound {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Success: false,
				Error:   "mapeamento não encontrado",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Success: false,
			Error:   "erro ao verificar mapeamento",
		})
		return
	}

	if err := h.mappingService.DeleteMapping(mappingID); err != nil {
		log.Error().Err(err).Str("mapping_id", mappingID).Msg("Erro ao deletar mapeamento")
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Success: false,
			Error:   "erro ao deletar mapeamento",
		})
		return
	}

	log.Info().
		Str("mapping_id", mappingID).
		Str("user_id", userID.(string)).
		Msg("Mapeamento deletado com sucesso")

	c.JSON(http.StatusOK, model.Response{
		Success: true,
	})
}

// ValidateMapping handles POST /api/web/mapping/validate - Validate mapping without saving
// @Summary      Validate mapping
// @Description  Validates a column mapping without saving it
// @Tags         mapping
// @Accept       json
// @Produce      json
// @Security     BasicAuth
// @Param        request body SaveMappingRequest true "Mapping configuration"
// @Success      200 {object} MappingResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Router       /api/web/mapping/validate [post]
func (h *MappingHandler) ValidateMapping(c *gin.Context) {
	log := logger.FromGin(c)

	var req SaveMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warn().Err(err).Msg("Payload inválido para validação")
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "payload inválido",
			Details: err.Error(),
		})
		return
	}

	// Check for duplicate mappings
	duplicates := h.mappingService.CheckDuplicateMappings(req.Mappings)
	if len(duplicates) > 0 {
		c.JSON(http.StatusOK, MappingResponse{
			Success: false,
			Validation: &service.MappingValidationResult{
				Valid:  false,
				Errors: []string{"mapeamento duplicado: " + joinStrings(duplicates)},
			},
		})
		return
	}

	// Get file columns for validation
	columns, _, err := h.uploadService.GetFileData(req.FilePath)
	if err != nil {
		log.Error().Err(err).Str("file_path", req.FilePath).Msg("Erro ao ler arquivo para validação")
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Success: false,
			Error:   "erro ao ler arquivo",
			Details: err.Error(),
		})
		return
	}

	// Convert and validate
	mappingReq := &service.MappingRequest{
		FilePath: req.FilePath,
		Mappings: req.Mappings,
		Title:    req.Title,
	}

	validation, err := h.mappingService.ValidateMapping(mappingReq, columns)
	if err != nil {
		log.Error().Err(err).Msg("Erro ao validar mapeamento")
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Success: false,
			Error:   "erro ao validar mapeamento",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, MappingResponse{
		Success:    validation.Valid,
		Validation: validation,
	})
}

// joinStrings joins a slice of strings with comma separator
func joinStrings(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += ", " + strs[i]
	}
	return result
}
