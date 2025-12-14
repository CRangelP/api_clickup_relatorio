package handler

import (
	"net/http"
	"strconv"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/metrics"
	"github.com/cleberrangel/clickup-excel-api/internal/repository"
	"github.com/cleberrangel/clickup-excel-api/internal/service"
	"github.com/gin-gonic/gin"
)

// QueueHandler handles queue-related HTTP requests
type QueueHandler struct {
	queueService   *service.QueueService
	uploadService  *service.UploadService
	mappingService *service.MappingService
}

// NewQueueHandler creates a new queue handler
func NewQueueHandler(queueService *service.QueueService, uploadService *service.UploadService, mappingService *service.MappingService) *QueueHandler {
	return &QueueHandler{
		queueService:   queueService,
		uploadService:  uploadService,
		mappingService: mappingService,
	}
}

// CreateJobRequest represents the request body for creating a job
type CreateJobRequest struct {
	MappingID string `json:"mapping_id" binding:"required"`
	Title     string `json:"title" binding:"required"`
}

// JobResponse represents a job in API responses
type JobResponse struct {
	ID            int      `json:"id"`
	UserID        string   `json:"user_id"`
	Title         string   `json:"title"`
	Status        string   `json:"status"`
	TotalRows     int      `json:"total_rows"`
	ProcessedRows int      `json:"processed_rows"`
	SuccessCount  int      `json:"success_count"`
	ErrorCount    int      `json:"error_count"`
	ErrorDetails  []string `json:"error_details,omitempty"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	CompletedAt   *string  `json:"completed_at,omitempty"`
}

// CreateJob creates a new update job
// @Summary Create update job
// @Description Creates a new job to update ClickUp tasks based on a mapping
// @Tags jobs
// @Accept json
// @Produce json
// @Param request body CreateJobRequest true "Job creation request"
// @Success 201 {object} JobResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/web/jobs [post]
func (h *QueueHandler) CreateJob(c *gin.Context) {
	log := logger.Get(c.Request.Context())
	
	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Usuário não autenticado",
		})
		return
	}
	
	var req CreateJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warn().Err(err).Msg("Erro ao fazer bind do request")
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Dados inválidos",
			"details": err.Error(),
		})
		return
	}
	
	// Get mapping to retrieve file path and mapping data
	mapping, err := h.mappingService.GetMappingByUser(req.MappingID, userID.(string))
	if err != nil {
		log.Error().Err(err).Str("mapping_id", req.MappingID).Msg("Erro ao buscar mapping")
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Mapeamento não encontrado",
		})
		return
	}
	
	// Get file data to count total rows
	_, data, err := h.uploadService.GetFileData(mapping.FilePath)
	if err != nil {
		log.Error().Err(err).Str("file_path", mapping.FilePath).Msg("Erro ao ler arquivo")
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Erro ao ler arquivo de dados",
			"details": err.Error(),
		})
		return
	}
	
	totalRows := len(data)
	
	// Convert mapping to map[string]string
	mappingMap := h.mappingService.ConvertToJobMapping(mapping.Mappings)
	
	// Create job
	job, err := h.queueService.CreateJob(userID.(string), req.Title, mapping.FilePath, mappingMap, totalRows)
	if err != nil {
		log.Error().Err(err).Msg("Erro ao criar job")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Erro ao criar job",
			"details": err.Error(),
		})
		return
	}
	
	log.Info().Int("job_id", job.ID).Str("user_id", userID.(string)).Msg("Job criado com sucesso")
	
	// Get username for audit
	username, _ := c.Get("username")
	usernameStr := ""
	if username != nil {
		usernameStr = username.(string)
	}

	// Audit job creation
	logger.Audit(c.Request.Context(), logger.AuditEvent{
		Action:     logger.AuditActionJobCreate,
		UserID:     userID.(string),
		Username:   usernameStr,
		Resource:   "job",
		ResourceID: strconv.Itoa(job.ID),
		ClientIP:   c.ClientIP(),
		Success:    true,
		Details: map[string]interface{}{
			"title":      req.Title,
			"total_rows": totalRows,
			"mapping_id": req.MappingID,
		},
	})
	metrics.Get().IncrementJobCreated()

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    toJobResponse(job),
	})
}

// ListJobs lists all jobs for the current user
// @Summary List user jobs
// @Description Returns all jobs for the authenticated user
// @Tags jobs
// @Produce json
// @Success 200 {object} []JobResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/web/jobs [get]
func (h *QueueHandler) ListJobs(c *gin.Context) {
	log := logger.Get(c.Request.Context())
	
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Usuário não autenticado",
		})
		return
	}
	
	jobs, err := h.queueService.GetJobsByUser(userID.(string))
	if err != nil {
		log.Error().Err(err).Str("user_id", userID.(string)).Msg("Erro ao listar jobs")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Erro ao listar jobs",
			"details": err.Error(),
		})
		return
	}
	
	// Convert to response format
	responses := make([]JobResponse, len(jobs))
	for i := range jobs {
		responses[i] = toJobResponseFromRepo(&jobs[i])
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    responses,
	})
}

// GetJob gets a specific job by ID
// @Summary Get job by ID
// @Description Returns a specific job by its ID
// @Tags jobs
// @Produce json
// @Param id path int true "Job ID"
// @Success 200 {object} JobResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/web/jobs/{id} [get]
func (h *QueueHandler) GetJob(c *gin.Context) {
	log := logger.Get(c.Request.Context())
	
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Usuário não autenticado",
		})
		return
	}
	
	// Parse job ID
	jobIDStr := c.Param("id")
	jobID, err := strconv.Atoi(jobIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "ID do job inválido",
		})
		return
	}
	
	job, err := h.queueService.GetJobByID(jobID)
	if err != nil {
		if err == service.ErrJobNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Job não encontrado",
			})
			return
		}
		log.Error().Err(err).Int("job_id", jobID).Msg("Erro ao buscar job")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Erro ao buscar job",
			"details": err.Error(),
		})
		return
	}
	
	// Verify job belongs to user
	if job.UserID != userID.(string) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Job não encontrado",
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    toJobResponseFromRepo(job),
	})
}

// toJobResponse converts a repository.UpdateJob pointer to JobResponse
func toJobResponse(job *repository.UpdateJob) JobResponse {
	return toJobResponseFromRepo(job)
}

// toJobResponseFromRepo converts a repository.UpdateJob to JobResponse
func toJobResponseFromRepo(job *repository.UpdateJob) JobResponse {
	resp := JobResponse{
		ID:            job.ID,
		UserID:        job.UserID,
		Title:         job.Title,
		Status:        job.Status,
		TotalRows:     job.TotalRows,
		ProcessedRows: job.ProcessedRows,
		SuccessCount:  job.SuccessCount,
		ErrorCount:    job.ErrorCount,
		ErrorDetails:  job.ErrorDetails,
		CreatedAt:     job.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     job.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	
	if job.CompletedAt != nil {
		completedStr := job.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.CompletedAt = &completedStr
	}
	
	return resp
}
