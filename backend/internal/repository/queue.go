package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
)

// QueueRepository gerencia operações da fila no banco
type QueueRepository struct {
	db *sql.DB
}

// NewQueueRepository cria um novo repositório de fila
func NewQueueRepository(db *sql.DB) *QueueRepository {
	return &QueueRepository{db: db}
}

// UpdateJob representa um job de atualização na fila
type UpdateJob struct {
	ID            int                    `json:"id" db:"id"`
	UserID        string                 `json:"user_id" db:"user_id"`
	Title         string                 `json:"title" db:"title"`
	Status        string                 `json:"status" db:"status"`
	FilePath      string                 `json:"file_path" db:"file_path"`
	Mapping       map[string]string      `json:"mapping" db:"mapping"`
	TotalRows     int                    `json:"total_rows" db:"total_rows"`
	ProcessedRows int                    `json:"processed_rows" db:"processed_rows"`
	SuccessCount  int                    `json:"success_count" db:"success_count"`
	ErrorCount    int                    `json:"error_count" db:"error_count"`
	ErrorDetails  []string               `json:"error_details" db:"error_details"`
	CreatedAt     time.Time             `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at" db:"updated_at"`
	CompletedAt   *time.Time            `json:"completed_at" db:"completed_at"`
}

// OperationHistory representa uma entrada no histórico de operações
type OperationHistory struct {
	ID            int                    `json:"id" db:"id"`
	UserID        string                 `json:"user_id" db:"user_id"`
	OperationType string                 `json:"operation_type" db:"operation_type"`
	Title         string                 `json:"title" db:"title"`
	Status        string                 `json:"status" db:"status"`
	Details       map[string]interface{} `json:"details" db:"details"`
	CreatedAt     time.Time             `json:"created_at" db:"created_at"`
}

// CreateJob cria um novo job na fila
func (r *QueueRepository) CreateJob(job UpdateJob) (*UpdateJob, error) {
	log := logger.Global()
	
	// Serializa mapping para JSONB
	mappingJSON, err := json.Marshal(job.Mapping)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar mapping: %w", err)
	}
	
	// Serializa error details para JSONB
	errorDetailsJSON, err := json.Marshal(job.ErrorDetails)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar error details: %w", err)
	}
	
	query := `
		INSERT INTO job_queue (user_id, title, status, file_path, mapping, total_rows, 
			processed_rows, success_count, error_count, error_details, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		RETURNING id, created_at, updated_at
	`
	
	var createdJob UpdateJob = job
	err = r.db.QueryRow(query, job.UserID, job.Title, job.Status, job.FilePath, 
		mappingJSON, job.TotalRows, job.ProcessedRows, job.SuccessCount, 
		job.ErrorCount, errorDetailsJSON).Scan(&createdJob.ID, &createdJob.CreatedAt, &createdJob.UpdatedAt)
	
	if err != nil {
		log.Error().Err(err).Str("user_id", job.UserID).Msg("Erro ao criar job")
		return nil, fmt.Errorf("erro ao criar job: %w", err)
	}
	
	log.Info().Int("job_id", createdJob.ID).Str("user_id", job.UserID).Msg("Job criado com sucesso")
	return &createdJob, nil
}

// UpdateJobProgress atualiza o progresso de um job
func (r *QueueRepository) UpdateJobProgress(jobID int, processedRows, successCount, errorCount int, errorDetails []string) error {
	log := logger.Global()
	
	// Serializa error details para JSONB
	errorDetailsJSON, err := json.Marshal(errorDetails)
	if err != nil {
		return fmt.Errorf("erro ao serializar error details: %w", err)
	}
	
	query := `
		UPDATE job_queue 
		SET processed_rows = $2, success_count = $3, error_count = $4, 
			error_details = $5, updated_at = NOW()
		WHERE id = $1
	`
	
	_, err = r.db.Exec(query, jobID, processedRows, successCount, errorCount, errorDetailsJSON)
	if err != nil {
		log.Error().Err(err).Int("job_id", jobID).Msg("Erro ao atualizar progresso do job")
		return fmt.Errorf("erro ao atualizar progresso do job: %w", err)
	}
	
	return nil
}

// UpdateJobStatus atualiza o status de um job
func (r *QueueRepository) UpdateJobStatus(jobID int, status string) error {
	log := logger.Global()
	
	var query string
	var args []interface{}
	
	if status == "completed" || status == "failed" {
		query = `
			UPDATE job_queue 
			SET status = $2, completed_at = NOW(), updated_at = NOW()
			WHERE id = $1
		`
		args = []interface{}{jobID, status}
	} else {
		query = `
			UPDATE job_queue 
			SET status = $2, updated_at = NOW()
			WHERE id = $1
		`
		args = []interface{}{jobID, status}
	}
	
	_, err := r.db.Exec(query, args...)
	if err != nil {
		log.Error().Err(err).Int("job_id", jobID).Str("status", status).Msg("Erro ao atualizar status do job")
		return fmt.Errorf("erro ao atualizar status do job: %w", err)
	}
	
	return nil
}

// GetJobByID retorna um job pelo ID
func (r *QueueRepository) GetJobByID(jobID int) (*UpdateJob, error) {
	query := `
		SELECT id, user_id, title, status, file_path, mapping, total_rows, 
			processed_rows, success_count, error_count, error_details, 
			created_at, updated_at, completed_at
		FROM job_queue 
		WHERE id = $1
	`
	
	var job UpdateJob
	var mappingJSON, errorDetailsJSON []byte
	
	err := r.db.QueryRow(query, jobID).Scan(&job.ID, &job.UserID, &job.Title, &job.Status, &job.FilePath,
		&mappingJSON, &job.TotalRows, &job.ProcessedRows, &job.SuccessCount,
		&job.ErrorCount, &errorDetailsJSON, &job.CreatedAt, &job.UpdatedAt, &job.CompletedAt)
	
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("erro ao buscar job: %w", err)
	}
	
	// Deserializa mapping
	if len(mappingJSON) > 0 {
		if err := json.Unmarshal(mappingJSON, &job.Mapping); err != nil {
			return nil, fmt.Errorf("erro ao deserializar mapping: %w", err)
		}
	}
	
	// Deserializa error details
	if len(errorDetailsJSON) > 0 {
		if err := json.Unmarshal(errorDetailsJSON, &job.ErrorDetails); err != nil {
			return nil, fmt.Errorf("erro ao deserializar error details: %w", err)
		}
	}
	
	return &job, nil
}

// GetPendingJobs retorna jobs pendentes na ordem FIFO
func (r *QueueRepository) GetPendingJobs() ([]UpdateJob, error) {
	query := `
		SELECT id, user_id, title, status, file_path, mapping, total_rows, 
			processed_rows, success_count, error_count, error_details, 
			created_at, updated_at, completed_at
		FROM job_queue 
		WHERE status = 'pending'
		ORDER BY created_at ASC
	`
	
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar jobs pendentes: %w", err)
	}
	defer rows.Close()
	
	var jobs []UpdateJob
	for rows.Next() {
		var job UpdateJob
		var mappingJSON, errorDetailsJSON []byte
		
		err := rows.Scan(&job.ID, &job.UserID, &job.Title, &job.Status, &job.FilePath,
			&mappingJSON, &job.TotalRows, &job.ProcessedRows, &job.SuccessCount,
			&job.ErrorCount, &errorDetailsJSON, &job.CreatedAt, &job.UpdatedAt, &job.CompletedAt)
		
		if err != nil {
			return nil, fmt.Errorf("erro ao escanear job: %w", err)
		}
		
		// Deserializa mapping
		if len(mappingJSON) > 0 {
			if err := json.Unmarshal(mappingJSON, &job.Mapping); err != nil {
				return nil, fmt.Errorf("erro ao deserializar mapping: %w", err)
			}
		}
		
		// Deserializa error details
		if len(errorDetailsJSON) > 0 {
			if err := json.Unmarshal(errorDetailsJSON, &job.ErrorDetails); err != nil {
				return nil, fmt.Errorf("erro ao deserializar error details: %w", err)
			}
		}
		
		jobs = append(jobs, job)
	}
	
	return jobs, nil
}

// GetJobsByUser retorna jobs de um usuário
func (r *QueueRepository) GetJobsByUser(userID string) ([]UpdateJob, error) {
	query := `
		SELECT id, user_id, title, status, file_path, mapping, total_rows, 
			processed_rows, success_count, error_count, error_details, 
			created_at, updated_at, completed_at
		FROM job_queue 
		WHERE user_id = $1
		ORDER BY created_at DESC
	`
	
	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar jobs do usuário: %w", err)
	}
	defer rows.Close()
	
	var jobs []UpdateJob
	for rows.Next() {
		var job UpdateJob
		var mappingJSON, errorDetailsJSON []byte
		
		err := rows.Scan(&job.ID, &job.UserID, &job.Title, &job.Status, &job.FilePath,
			&mappingJSON, &job.TotalRows, &job.ProcessedRows, &job.SuccessCount,
			&job.ErrorCount, &errorDetailsJSON, &job.CreatedAt, &job.UpdatedAt, &job.CompletedAt)
		
		if err != nil {
			return nil, fmt.Errorf("erro ao escanear job: %w", err)
		}
		
		// Deserializa mapping
		if len(mappingJSON) > 0 {
			if err := json.Unmarshal(mappingJSON, &job.Mapping); err != nil {
				return nil, fmt.Errorf("erro ao deserializar mapping: %w", err)
			}
		}
		
		// Deserializa error details
		if len(errorDetailsJSON) > 0 {
			if err := json.Unmarshal(errorDetailsJSON, &job.ErrorDetails); err != nil {
				return nil, fmt.Errorf("erro ao deserializar error details: %w", err)
			}
		}
		
		jobs = append(jobs, job)
	}
	
	return jobs, nil
}

// DeleteCompletedJobs remove jobs concluídos
func (r *QueueRepository) DeleteCompletedJobs() error {
	query := "DELETE FROM job_queue WHERE status = 'completed'"
	
	result, err := r.db.Exec(query)
	if err != nil {
		return fmt.Errorf("erro ao deletar jobs concluídos: %w", err)
	}
	
	rowsAffected, _ := result.RowsAffected()
	log := logger.Global()
	log.Info().Int64("rows_deleted", rowsAffected).Msg("Jobs concluídos removidos")
	
	return nil
}

// DeleteOldFailedJobs remove jobs que falharam há mais de 24 horas
func (r *QueueRepository) DeleteOldFailedJobs() error {
	query := `
		DELETE FROM job_queue 
		WHERE status = 'failed' AND updated_at < NOW() - INTERVAL '24 hours'
	`
	
	result, err := r.db.Exec(query)
	if err != nil {
		return fmt.Errorf("erro ao deletar jobs antigos: %w", err)
	}
	
	rowsAffected, _ := result.RowsAffected()
	log := logger.Global()
	log.Info().Int64("rows_deleted", rowsAffected).Msg("Jobs antigos removidos")
	
	return nil
}

// CreateOperationHistory cria uma entrada no histórico
func (r *QueueRepository) CreateOperationHistory(history OperationHistory) (*OperationHistory, error) {
	log := logger.Global()
	
	// Serializa details para JSONB
	detailsJSON, err := json.Marshal(history.Details)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar details: %w", err)
	}
	
	query := `
		INSERT INTO operation_history (user_id, operation_type, title, status, details, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		RETURNING id, created_at
	`
	
	var createdHistory OperationHistory = history
	err = r.db.QueryRow(query, history.UserID, history.OperationType, history.Title, 
		history.Status, detailsJSON).Scan(&createdHistory.ID, &createdHistory.CreatedAt)
	
	if err != nil {
		log.Error().Err(err).Str("user_id", history.UserID).Msg("Erro ao criar entrada no histórico")
		return nil, fmt.Errorf("erro ao criar entrada no histórico: %w", err)
	}
	
	return &createdHistory, nil
}

// GetOperationHistoryByUser retorna histórico de um usuário (últimas 50 entradas)
func (r *QueueRepository) GetOperationHistoryByUser(userID string) ([]OperationHistory, error) {
	query := `
		SELECT id, user_id, operation_type, title, status, details, created_at
		FROM operation_history 
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 50
	`
	
	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar histórico: %w", err)
	}
	defer rows.Close()
	
	var history []OperationHistory
	for rows.Next() {
		var h OperationHistory
		var detailsJSON []byte
		
		err := rows.Scan(&h.ID, &h.UserID, &h.OperationType, &h.Title, &h.Status, 
			&detailsJSON, &h.CreatedAt)
		
		if err != nil {
			return nil, fmt.Errorf("erro ao escanear histórico: %w", err)
		}
		
		// Deserializa details
		if len(detailsJSON) > 0 {
			if err := json.Unmarshal(detailsJSON, &h.Details); err != nil {
				return nil, fmt.Errorf("erro ao deserializar details: %w", err)
			}
		}
		
		history = append(history, h)
	}
	
	return history, nil
}

// DeleteAllHistoryByUser remove todo histórico de um usuário
func (r *QueueRepository) DeleteAllHistoryByUser(userID string) error {
	query := "DELETE FROM operation_history WHERE user_id = $1"
	
	result, err := r.db.Exec(query, userID)
	if err != nil {
		return fmt.Errorf("erro ao deletar histórico: %w", err)
	}
	
	rowsAffected, _ := result.RowsAffected()
	log := logger.Global()
	log.Info().Str("user_id", userID).Int64("rows_deleted", rowsAffected).Msg("Histórico do usuário removido")
	
	return nil
}

// CleanupOldHistory remove registros antigos mantendo apenas os últimos 1000
func (r *QueueRepository) CleanupOldHistory() error {
	query := `
		DELETE FROM operation_history 
		WHERE id NOT IN (
			SELECT id FROM operation_history 
			ORDER BY created_at DESC 
			LIMIT 1000
		)
	`
	
	result, err := r.db.Exec(query)
	if err != nil {
		return fmt.Errorf("erro ao limpar histórico antigo: %w", err)
	}
	
	rowsAffected, _ := result.RowsAffected()
	log := logger.Global()
	log.Info().Int64("rows_deleted", rowsAffected).Msg("Histórico antigo removido")
	
	return nil
}


// GetOperationHistoryByID retorna uma entrada específica do histórico pelo ID
func (r *QueueRepository) GetOperationHistoryByID(historyID int) (*OperationHistory, error) {
	query := `
		SELECT id, user_id, operation_type, title, status, details, created_at
		FROM operation_history 
		WHERE id = $1
	`
	
	var h OperationHistory
	var detailsJSON []byte
	
	err := r.db.QueryRow(query, historyID).Scan(&h.ID, &h.UserID, &h.OperationType, 
		&h.Title, &h.Status, &detailsJSON, &h.CreatedAt)
	
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("erro ao buscar histórico: %w", err)
	}
	
	// Deserializa details
	if len(detailsJSON) > 0 {
		if err := json.Unmarshal(detailsJSON, &h.Details); err != nil {
			return nil, fmt.Errorf("erro ao deserializar details: %w", err)
		}
	}
	
	return &h, nil
}

// UpdateOperationHistoryStatus atualiza o status de uma entrada do histórico
func (r *QueueRepository) UpdateOperationHistoryStatus(historyID int, status string, details map[string]interface{}) error {
	log := logger.Global()
	
	// Serializa details para JSONB
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("erro ao serializar details: %w", err)
	}
	
	query := `
		UPDATE operation_history 
		SET status = $2, details = $3
		WHERE id = $1
	`
	
	result, err := r.db.Exec(query, historyID, status, detailsJSON)
	if err != nil {
		log.Error().Err(err).Int("history_id", historyID).Str("status", status).Msg("Erro ao atualizar status do histórico")
		return fmt.Errorf("erro ao atualizar status do histórico: %w", err)
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("registro de histórico não encontrado")
	}
	
	return nil
}

// GetOperationHistoryCount retorna a contagem total de registros no histórico
func (r *QueueRepository) GetOperationHistoryCount() (int, error) {
	query := "SELECT COUNT(*) FROM operation_history"
	
	var count int
	err := r.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("erro ao contar histórico: %w", err)
	}
	
	return count, nil
}
