package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/repository"
	"github.com/cleberrangel/clickup-excel-api/internal/websocket"
)

// Job status constants
const (
	JobStatusPending    = "pending"
	JobStatusProcessing = "processing"
	JobStatusCompleted  = "completed"
	JobStatusFailed     = "failed"
)

// Queue service errors
var (
	ErrJobNotFound     = errors.New("job não encontrado")
	ErrQueueFull       = errors.New("fila de processamento cheia")
	ErrInvalidJobState = errors.New("estado do job inválido para esta operação")
)

// QueueService manages job queue processing
type QueueService struct {
	queueRepo *repository.QueueRepository
	wsHub     *websocket.Hub
	
	// Background processor control
	processorCtx    context.Context
	processorCancel context.CancelFunc
	processorWg     sync.WaitGroup
	
	// Job processor callback (to be set by task update service)
	jobProcessor func(ctx context.Context, job *repository.UpdateJob) error
	
	// Cleanup interval
	cleanupInterval time.Duration
}

// NewQueueService creates a new queue service
func NewQueueService(queueRepo *repository.QueueRepository, wsHub *websocket.Hub) *QueueService {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &QueueService{
		queueRepo:       queueRepo,
		wsHub:           wsHub,
		processorCtx:    ctx,
		processorCancel: cancel,
		cleanupInterval: 1 * time.Hour,
	}
}

// SetJobProcessor sets the callback function for processing jobs
func (s *QueueService) SetJobProcessor(processor func(ctx context.Context, job *repository.UpdateJob) error) {
	s.jobProcessor = processor
}

// Start starts the background job processor and cleanup goroutines
func (s *QueueService) Start() {
	log := logger.Global()
	log.Info().Msg("Iniciando QueueService")
	
	// Start job processor
	s.processorWg.Add(1)
	go s.processJobsLoop()
	
	// Start cleanup routine
	s.processorWg.Add(1)
	go s.cleanupLoop()
}

// Stop stops the background processors gracefully
func (s *QueueService) Stop() {
	log := logger.Global()
	log.Info().Msg("Parando QueueService")
	
	s.processorCancel()
	s.processorWg.Wait()
	
	log.Info().Msg("QueueService parado")
}


// CreateJob creates a new job in the queue
func (s *QueueService) CreateJob(userID, title, filePath string, mapping map[string]string, totalRows int) (*repository.UpdateJob, error) {
	log := logger.Global()
	
	job := repository.UpdateJob{
		UserID:        userID,
		Title:         title,
		Status:        JobStatusPending,
		FilePath:      filePath,
		Mapping:       mapping,
		TotalRows:     totalRows,
		ProcessedRows: 0,
		SuccessCount:  0,
		ErrorCount:    0,
		ErrorDetails:  []string{},
	}
	
	createdJob, err := s.queueRepo.CreateJob(job)
	if err != nil {
		log.Error().Err(err).Str("user_id", userID).Msg("Erro ao criar job")
		return nil, err
	}
	
	log.Info().
		Int("job_id", createdJob.ID).
		Str("user_id", userID).
		Str("title", title).
		Int("total_rows", totalRows).
		Msg("Job criado com sucesso")
	
	// Send WebSocket notification
	if s.wsHub != nil {
		s.wsHub.SendProgress(userID, websocket.ProgressUpdate{
			JobID:     createdJob.ID,
			Status:    JobStatusPending,
			TotalRows: totalRows,
			Message:   "Job adicionado à fila",
		})
	}
	
	return createdJob, nil
}

// GetJobByID retrieves a job by its ID
func (s *QueueService) GetJobByID(jobID int) (*repository.UpdateJob, error) {
	job, err := s.queueRepo.GetJobByID(jobID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, ErrJobNotFound
	}
	return job, nil
}

// GetJobsByUser retrieves all jobs for a user
func (s *QueueService) GetJobsByUser(userID string) ([]repository.UpdateJob, error) {
	return s.queueRepo.GetJobsByUser(userID)
}

// UpdateJobProgress updates job progress and sends WebSocket notification
func (s *QueueService) UpdateJobProgress(jobID int, processedRows, successCount, errorCount int, errorDetails []string) error {
	log := logger.Global()
	
	err := s.queueRepo.UpdateJobProgress(jobID, processedRows, successCount, errorCount, errorDetails)
	if err != nil {
		return err
	}
	
	// Get job to send WebSocket update
	job, err := s.queueRepo.GetJobByID(jobID)
	if err != nil {
		log.Warn().Err(err).Int("job_id", jobID).Msg("Erro ao buscar job para WebSocket update")
		return nil // Don't fail the progress update
	}
	
	if job != nil && s.wsHub != nil {
		s.wsHub.SendProgress(job.UserID, websocket.ProgressUpdate{
			JobID:         jobID,
			Status:        job.Status,
			ProcessedRows: processedRows,
			TotalRows:     job.TotalRows,
			SuccessCount:  successCount,
			ErrorCount:    errorCount,
			Message:       "Processando...",
		})
	}
	
	return nil
}

// CompleteJob marks a job as completed
func (s *QueueService) CompleteJob(jobID int) error {
	log := logger.Global()
	
	err := s.queueRepo.UpdateJobStatus(jobID, JobStatusCompleted)
	if err != nil {
		return err
	}
	
	// Get job to send WebSocket update
	job, err := s.queueRepo.GetJobByID(jobID)
	if err != nil {
		log.Warn().Err(err).Int("job_id", jobID).Msg("Erro ao buscar job para WebSocket update")
		return nil
	}
	
	if job != nil && s.wsHub != nil {
		s.wsHub.SendProgress(job.UserID, websocket.ProgressUpdate{
			JobID:         jobID,
			Status:        JobStatusCompleted,
			ProcessedRows: job.ProcessedRows,
			TotalRows:     job.TotalRows,
			SuccessCount:  job.SuccessCount,
			ErrorCount:    job.ErrorCount,
			Message:       "Processamento concluído",
		})
	}
	
	log.Info().Int("job_id", jobID).Msg("Job concluído")
	return nil
}

// FailJob marks a job as failed
func (s *QueueService) FailJob(jobID int, errorMsg string) error {
	log := logger.Global()
	
	// Get current job to append error
	job, err := s.queueRepo.GetJobByID(jobID)
	if err != nil {
		return err
	}
	if job == nil {
		return ErrJobNotFound
	}
	
	// Append error to details
	errorDetails := append(job.ErrorDetails, errorMsg)
	if err := s.queueRepo.UpdateJobProgress(jobID, job.ProcessedRows, job.SuccessCount, job.ErrorCount+1, errorDetails); err != nil {
		return err
	}
	
	// Update status to failed
	if err := s.queueRepo.UpdateJobStatus(jobID, JobStatusFailed); err != nil {
		return err
	}
	
	// Send WebSocket notification
	if s.wsHub != nil {
		s.wsHub.SendProgress(job.UserID, websocket.ProgressUpdate{
			JobID:         jobID,
			Status:        JobStatusFailed,
			ProcessedRows: job.ProcessedRows,
			TotalRows:     job.TotalRows,
			SuccessCount:  job.SuccessCount,
			ErrorCount:    job.ErrorCount + 1,
			Message:       errorMsg,
		})
	}
	
	log.Error().Int("job_id", jobID).Str("error", errorMsg).Msg("Job falhou")
	return nil
}


// processJobsLoop is the background goroutine that processes jobs in FIFO order
func (s *QueueService) processJobsLoop() {
	defer s.processorWg.Done()
	
	log := logger.Global()
	log.Info().Msg("Job processor iniciado")
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.processorCtx.Done():
			log.Info().Msg("Job processor parando")
			return
		case <-ticker.C:
			s.processNextJob()
		}
	}
}

// processNextJob processes the next pending job in the queue
func (s *QueueService) processNextJob() {
	log := logger.Global()
	
	// Get pending jobs (FIFO order)
	jobs, err := s.queueRepo.GetPendingJobs()
	if err != nil {
		log.Error().Err(err).Msg("Erro ao buscar jobs pendentes")
		return
	}
	
	if len(jobs) == 0 {
		return // No pending jobs
	}
	
	// Process first job (FIFO)
	job := jobs[0]
	
	log.Info().
		Int("job_id", job.ID).
		Str("user_id", job.UserID).
		Str("title", job.Title).
		Msg("Processando job")
	
	// Update status to processing
	if err := s.queueRepo.UpdateJobStatus(job.ID, JobStatusProcessing); err != nil {
		log.Error().Err(err).Int("job_id", job.ID).Msg("Erro ao atualizar status do job")
		return
	}
	
	// Send WebSocket notification
	if s.wsHub != nil {
		s.wsHub.SendProgress(job.UserID, websocket.ProgressUpdate{
			JobID:     job.ID,
			Status:    JobStatusProcessing,
			TotalRows: job.TotalRows,
			Message:   "Iniciando processamento",
		})
	}
	
	// Process the job
	if s.jobProcessor != nil {
		if err := s.jobProcessor(s.processorCtx, &job); err != nil {
			log.Error().Err(err).Int("job_id", job.ID).Msg("Erro ao processar job")
			s.FailJob(job.ID, err.Error())
			return
		}
	} else {
		// No processor set - mark as completed (for testing)
		log.Warn().Int("job_id", job.ID).Msg("Nenhum processador de job configurado")
	}
	
	// Mark as completed
	s.CompleteJob(job.ID)
}

// cleanupLoop is the background goroutine that cleans up old jobs
func (s *QueueService) cleanupLoop() {
	defer s.processorWg.Done()
	
	log := logger.Global()
	log.Info().Msg("Cleanup routine iniciada")
	
	ticker := time.NewTicker(s.cleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.processorCtx.Done():
			log.Info().Msg("Cleanup routine parando")
			return
		case <-ticker.C:
			s.runCleanup()
		}
	}
}

// runCleanup performs cleanup of completed and old failed jobs
func (s *QueueService) runCleanup() {
	log := logger.Global()
	log.Info().Msg("Executando limpeza de jobs")
	
	// Delete completed jobs
	if err := s.queueRepo.DeleteCompletedJobs(); err != nil {
		log.Error().Err(err).Msg("Erro ao deletar jobs concluídos")
	}
	
	// Delete old failed jobs (older than 24 hours)
	if err := s.queueRepo.DeleteOldFailedJobs(); err != nil {
		log.Error().Err(err).Msg("Erro ao deletar jobs antigos")
	}
	
	log.Info().Msg("Limpeza de jobs concluída")
}

// GetPendingJobsCount returns the number of pending jobs
func (s *QueueService) GetPendingJobsCount() (int, error) {
	jobs, err := s.queueRepo.GetPendingJobs()
	if err != nil {
		return 0, err
	}
	return len(jobs), nil
}

// ResumePendingJobs resumes processing of pending jobs after system restart
func (s *QueueService) ResumePendingJobs() error {
	log := logger.Global()
	
	jobs, err := s.queueRepo.GetPendingJobs()
	if err != nil {
		return err
	}
	
	log.Info().Int("count", len(jobs)).Msg("Jobs pendentes encontrados para retomar")
	
	// Jobs will be processed by the background processor
	return nil
}
