package service

import (
	"errors"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/repository"
)

// Operation type constants
const (
	OperationTypeReportGeneration = "report_generation"
	OperationTypeFieldUpdate      = "field_update"
)

// Operation status constants
const (
	OperationStatusPending    = "pending"
	OperationStatusProcessing = "processing"
	OperationStatusCompleted  = "completed"
	OperationStatusFailed     = "failed"
)

// History service errors
var (
	ErrHistoryNotFound = errors.New("registro de histórico não encontrado")
	ErrInvalidOperation = errors.New("tipo de operação inválido")
)

// HistoryService manages operation history tracking
type HistoryService struct {
	queueRepo       *repository.QueueRepository
	maxRecords      int
	cleanupInterval time.Duration
}

// NewHistoryService creates a new history service
func NewHistoryService(queueRepo *repository.QueueRepository) *HistoryService {
	return &HistoryService{
		queueRepo:       queueRepo,
		maxRecords:      1000,
		cleanupInterval: 1 * time.Hour,
	}
}

// CreateHistoryRecord creates a new operation history record on operation start
func (s *HistoryService) CreateHistoryRecord(userID, operationType, title string, details map[string]interface{}) (*repository.OperationHistory, error) {
	log := logger.Global()

	// Validate operation type
	if operationType != OperationTypeReportGeneration && operationType != OperationTypeFieldUpdate {
		return nil, ErrInvalidOperation
	}

	history := repository.OperationHistory{
		UserID:        userID,
		OperationType: operationType,
		Title:         title,
		Status:        OperationStatusPending,
		Details:       details,
	}

	createdHistory, err := s.queueRepo.CreateOperationHistory(history)
	if err != nil {
		log.Error().Err(err).Str("user_id", userID).Str("operation_type", operationType).Msg("Erro ao criar registro de histórico")
		return nil, err
	}

	log.Info().
		Int("history_id", createdHistory.ID).
		Str("user_id", userID).
		Str("operation_type", operationType).
		Str("title", title).
		Msg("Registro de histórico criado")

	// Trigger cleanup if needed (async)
	go s.cleanupIfNeeded()

	return createdHistory, nil
}


// UpdateHistoryStatus updates the status of an operation history record
func (s *HistoryService) UpdateHistoryStatus(historyID int, status string, details map[string]interface{}) error {
	log := logger.Global()

	// Validate status
	if status != OperationStatusPending && status != OperationStatusProcessing &&
		status != OperationStatusCompleted && status != OperationStatusFailed {
		return errors.New("status inválido")
	}

	err := s.queueRepo.UpdateOperationHistoryStatus(historyID, status, details)
	if err != nil {
		log.Error().Err(err).Int("history_id", historyID).Str("status", status).Msg("Erro ao atualizar status do histórico")
		return err
	}

	log.Info().Int("history_id", historyID).Str("status", status).Msg("Status do histórico atualizado")
	return nil
}

// GetHistoryByUser retrieves operation history for a user (last 50 records)
func (s *HistoryService) GetHistoryByUser(userID string) ([]repository.OperationHistory, error) {
	return s.queueRepo.GetOperationHistoryByUser(userID)
}

// GetHistoryByID retrieves a specific operation history record by ID
func (s *HistoryService) GetHistoryByID(historyID int) (*repository.OperationHistory, error) {
	history, err := s.queueRepo.GetOperationHistoryByID(historyID)
	if err != nil {
		return nil, err
	}
	if history == nil {
		return nil, ErrHistoryNotFound
	}
	return history, nil
}

// DeleteAllHistoryByUser removes all history records for a user
func (s *HistoryService) DeleteAllHistoryByUser(userID string) error {
	log := logger.Global()

	err := s.queueRepo.DeleteAllHistoryByUser(userID)
	if err != nil {
		log.Error().Err(err).Str("user_id", userID).Msg("Erro ao deletar histórico do usuário")
		return err
	}

	log.Info().Str("user_id", userID).Msg("Histórico do usuário deletado")
	return nil
}

// cleanupIfNeeded checks if cleanup is needed and performs it
func (s *HistoryService) cleanupIfNeeded() {
	log := logger.Global()

	count, err := s.queueRepo.GetOperationHistoryCount()
	if err != nil {
		log.Warn().Err(err).Msg("Erro ao verificar contagem de histórico")
		return
	}

	if count > s.maxRecords {
		log.Info().Int("count", count).Int("max", s.maxRecords).Msg("Iniciando limpeza de histórico")
		if err := s.queueRepo.CleanupOldHistory(); err != nil {
			log.Error().Err(err).Msg("Erro ao limpar histórico antigo")
		}
	}
}

// GetHistoryCount returns the total count of history records
func (s *HistoryService) GetHistoryCount() (int, error) {
	return s.queueRepo.GetOperationHistoryCount()
}
