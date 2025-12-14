package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/client"
	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/metrics"
	"github.com/cleberrangel/clickup-excel-api/internal/repository"
	"github.com/cleberrangel/clickup-excel-api/internal/websocket"
	"golang.org/x/time/rate"
)

// TaskUpdateService handles batch processing of task updates
type TaskUpdateService struct {
	uploadService  *UploadService
	metadataRepo   *repository.MetadataRepository
	configRepo     *repository.ConfigRepository
	queueRepo      *repository.QueueRepository
	wsHub          *websocket.Hub
}

// TaskUpdateResult represents the result of a single task update
type TaskUpdateResult struct {
	TaskID  string `json:"task_id"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// BatchUpdateResult represents the result of a batch update operation
type BatchUpdateResult struct {
	TotalRows     int                `json:"total_rows"`
	ProcessedRows int                `json:"processed_rows"`
	SuccessCount  int                `json:"success_count"`
	ErrorCount    int                `json:"error_count"`
	Errors        []TaskUpdateResult `json:"errors,omitempty"`
}

// NewTaskUpdateService creates a new task update service
func NewTaskUpdateService(
	uploadService *UploadService,
	metadataRepo *repository.MetadataRepository,
	configRepo *repository.ConfigRepository,
	queueRepo *repository.QueueRepository,
	wsHub *websocket.Hub,
) *TaskUpdateService {
	return &TaskUpdateService{
		uploadService: uploadService,
		metadataRepo:  metadataRepo,
		configRepo:    configRepo,
		queueRepo:     queueRepo,
		wsHub:         wsHub,
	}
}

// ProcessJob processes a job from the queue
// This is the main entry point called by QueueService
func (s *TaskUpdateService) ProcessJob(ctx context.Context, job *repository.UpdateJob) error {
	log := logger.Get(ctx)
	log.Info().
		Int("job_id", job.ID).
		Str("user_id", job.UserID).
		Str("title", job.Title).
		Int("total_rows", job.TotalRows).
		Msg("Iniciando processamento do job")

	// Get user configuration for token and rate limit
	config, err := s.configRepo.GetUserConfig(job.UserID)
	if err != nil {
		return fmt.Errorf("erro ao buscar configuração do usuário: %w", err)
	}
	if config == nil {
		return fmt.Errorf("configuração do usuário não encontrada")
	}
	if config.ClickUpTokenEncrypted == "" {
		return fmt.Errorf("token do ClickUp não configurado")
	}

	// Create ClickUp client with user's token
	clickupClient := client.NewClient(config.ClickUpTokenEncrypted)

	// Get custom fields for type information
	customFields, err := s.metadataRepo.GetCustomFields()
	if err != nil {
		return fmt.Errorf("erro ao buscar campos personalizados: %w", err)
	}

	// Build field type map
	fieldTypeMap := make(map[string]string)
	for _, field := range customFields {
		fieldTypeMap[field.ID] = field.Type
	}

	// Read file data
	columns, data, err := s.uploadService.GetFileData(job.FilePath)
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo: %w", err)
	}

	// Find task ID column index
	taskIDColumnIndex := s.findTaskIDColumnIndex(columns, job.Mapping)
	if taskIDColumnIndex < 0 {
		return fmt.Errorf("coluna 'id task' não encontrada no mapeamento")
	}

	// Process rows with rate limiting
	result, err := s.processBatch(ctx, clickupClient, job, columns, data, taskIDColumnIndex, fieldTypeMap, config.RateLimitPerMinute)
	if err != nil {
		return err
	}

	// Track job completion metrics
	if result.ErrorCount > 0 && result.SuccessCount == 0 {
		metrics.Get().IncrementJobFailed()
	} else {
		metrics.Get().IncrementJobCompleted()
	}

	log.Info().
		Int("job_id", job.ID).
		Int("success_count", result.SuccessCount).
		Int("error_count", result.ErrorCount).
		Msg("Job processado com sucesso")

	return nil
}


// findTaskIDColumnIndex finds the index of the task ID column
func (s *TaskUpdateService) findTaskIDColumnIndex(columns []string, mapping map[string]string) int {
	// Look for "id task" or similar column names in the mapping
	taskIDKeys := []string{"id task", "id_task", "task_id", "taskid", "id"}
	
	for i, col := range columns {
		colLower := strings.ToLower(strings.TrimSpace(col))
		for _, key := range taskIDKeys {
			if colLower == key {
				return i
			}
		}
	}
	
	// Also check if any column is mapped to a special "task_id" field
	for colName, fieldID := range mapping {
		if strings.ToLower(fieldID) == "task_id" || strings.ToLower(fieldID) == "id_task" {
			for i, col := range columns {
				if col == colName {
					return i
				}
			}
		}
	}
	
	return -1
}

// processBatch processes all rows in the batch with rate limiting
func (s *TaskUpdateService) processBatch(
	ctx context.Context,
	clickupClient *client.Client,
	job *repository.UpdateJob,
	columns []string,
	data [][]string,
	taskIDColumnIndex int,
	fieldTypeMap map[string]string,
	rateLimitPerMinute int,
) (*BatchUpdateResult, error) {
	log := logger.Get(ctx)
	
	// Create rate limiter based on user configuration
	limiter := rate.NewLimiter(rate.Every(time.Minute/time.Duration(rateLimitPerMinute)), 50)
	
	result := &BatchUpdateResult{
		TotalRows: len(data),
		Errors:    make([]TaskUpdateResult, 0),
	}
	
	errorDetails := make([]string, 0)
	
	// Build column index map for quick lookup
	columnIndexMap := make(map[string]int)
	for i, col := range columns {
		columnIndexMap[col] = i
	}
	
	for rowIndex, row := range data {
		// Check context cancellation
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		
		// Get task ID from row
		if taskIDColumnIndex >= len(row) {
			errorMsg := fmt.Sprintf("linha %d: índice da coluna task_id fora do range", rowIndex+1)
			errorDetails = append(errorDetails, errorMsg)
			result.Errors = append(result.Errors, TaskUpdateResult{
				TaskID:  "",
				Success: false,
				Error:   errorMsg,
			})
			result.ErrorCount++
			result.ProcessedRows++
			continue
		}
		
		taskID := strings.TrimSpace(row[taskIDColumnIndex])
		if taskID == "" {
			errorMsg := fmt.Sprintf("linha %d: task_id vazio", rowIndex+1)
			errorDetails = append(errorDetails, errorMsg)
			result.Errors = append(result.Errors, TaskUpdateResult{
				TaskID:  "",
				Success: false,
				Error:   errorMsg,
			})
			result.ErrorCount++
			result.ProcessedRows++
			continue
		}
		
		// Process each mapped field for this row
		rowSuccess := true
		var rowError string
		
		for columnName, fieldID := range job.Mapping {
			// Skip task_id column mapping
			if strings.ToLower(fieldID) == "task_id" || strings.ToLower(fieldID) == "id_task" {
				continue
			}
			
			// Get column index
			colIndex, exists := columnIndexMap[columnName]
			if !exists {
				continue
			}
			
			// Get value from row
			if colIndex >= len(row) {
				continue
			}
			value := row[colIndex]
			
			// Skip empty values
			if strings.TrimSpace(value) == "" {
				continue
			}
			
			// Get field type
			fieldType := fieldTypeMap[fieldID]
			if fieldType == "" {
				fieldType = "text" // Default to text if type unknown
			}
			
			// Wait for rate limiter
			if err := limiter.Wait(ctx); err != nil {
				return result, fmt.Errorf("rate limiter: %w", err)
			}
			
			// Update the custom field
			err := clickupClient.SetCustomFieldValueWithRetry(ctx, taskID, fieldID, value, fieldType)
			if err != nil {
				rowSuccess = false
				rowError = fmt.Sprintf("linha %d, task %s, campo %s: %v", rowIndex+1, taskID, fieldID, err)
				log.Warn().
					Str("task_id", taskID).
					Str("field_id", fieldID).
					Err(err).
					Msg("Erro ao atualizar campo")
				break // Stop processing this row on first error
			}
		}
		
		result.ProcessedRows++
		
		if rowSuccess {
			result.SuccessCount++
		} else {
			result.ErrorCount++
			errorDetails = append(errorDetails, rowError)
			result.Errors = append(result.Errors, TaskUpdateResult{
				TaskID:  taskID,
				Success: false,
				Error:   rowError,
			})
		}
		
		// Update job progress periodically (every 10 rows or on last row)
		if result.ProcessedRows%10 == 0 || result.ProcessedRows == result.TotalRows {
			s.updateJobProgress(job.ID, job.UserID, result, errorDetails)
		}
		
		// Log progress periodically
		if result.ProcessedRows%100 == 0 {
			log.Info().
				Int("job_id", job.ID).
				Int("processed", result.ProcessedRows).
				Int("total", result.TotalRows).
				Int("success", result.SuccessCount).
				Int("errors", result.ErrorCount).
				Msg("Progresso do processamento")
		}
	}
	
	// Final progress update
	s.updateJobProgress(job.ID, job.UserID, result, errorDetails)
	
	return result, nil
}

// updateJobProgress updates job progress in database and sends WebSocket notification
func (s *TaskUpdateService) updateJobProgress(jobID int, userID string, result *BatchUpdateResult, errorDetails []string) {
	// Update database
	if s.queueRepo != nil {
		s.queueRepo.UpdateJobProgress(jobID, result.ProcessedRows, result.SuccessCount, result.ErrorCount, errorDetails)
	}
	
	// Send WebSocket notification
	if s.wsHub != nil {
		progress := websocket.ProgressUpdate{
			JobID:         jobID,
			Status:        "processing",
			ProcessedRows: result.ProcessedRows,
			TotalRows:     result.TotalRows,
			SuccessCount:  result.SuccessCount,
			ErrorCount:    result.ErrorCount,
			Message:       fmt.Sprintf("Processando... %d/%d", result.ProcessedRows, result.TotalRows),
		}
		
		if result.ProcessedRows == result.TotalRows {
			progress.Status = "completed"
			progress.Message = fmt.Sprintf("Concluído: %d sucesso, %d erros", result.SuccessCount, result.ErrorCount)
		}
		
		s.wsHub.SendProgress(userID, progress)
	}
}

// GetFieldTypeMap returns a map of field ID to field type
func (s *TaskUpdateService) GetFieldTypeMap() (map[string]string, error) {
	customFields, err := s.metadataRepo.GetCustomFields()
	if err != nil {
		return nil, err
	}
	
	fieldTypeMap := make(map[string]string)
	for _, field := range customFields {
		fieldTypeMap[field.ID] = field.Type
	}
	
	return fieldTypeMap, nil
}
