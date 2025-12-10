package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/model"
	"github.com/cleberrangel/clickup-excel-api/internal/repository"
	"golang.org/x/time/rate"
)

const (
	baseURL = "https://api.clickup.com/api/v2"

	// MaxConcurrentRequests limita requisições simultâneas
	MaxConcurrentRequests = 5

	// RequestsPerMinute limite conservador (ClickUp permite 10k/min)
	RequestsPerMinute = 2000

	// DefaultTimeout timeout padrão para requisições
	DefaultTimeout = 60 * time.Second

	// PageSize tamanho padrão da página do ClickUp
	PageSize = 100

	// RetryMaxAttempts número máximo de tentativas por página
	RetryMaxAttempts = 3

	// RetryBackoff tempo de espera entre retries
	RetryBackoff = 30 * time.Second
)

// Client é o cliente HTTP para a API do ClickUp
type Client struct {
	token      string
	httpClient *http.Client
	limiter    *rate.Limiter
}

// NewClient cria um novo cliente ClickUp
func NewClient(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     30 * time.Second,
			},
		},
		limiter: rate.NewLimiter(rate.Every(time.Minute/RequestsPerMinute), 50),
	}
}

// buildTaskURL constrói a URL para buscar tarefas de uma lista
func buildTaskURL(listID string, page int, subtasks, includeClosed bool) string {
	return fmt.Sprintf("%s/list/%s/task?page=%d&subtasks=%t&include_closed=%t",
		baseURL, listID, page, subtasks, includeClosed)
}

// GetTasks busca todas as tarefas de uma lista com paginação automática e retry
func (c *Client) GetTasks(ctx context.Context, listID string, subtasks, includeClosed bool) ([]model.Task, error) {
	var allTasks []model.Task
	page := 0
	totalCollected := 0

	for {
		// Aguarda rate limiter
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter: %w", err)
		}

		url := buildTaskURL(listID, page, subtasks, includeClosed)

		// Executa request com retry
		resp, err := c.doRequestWithRetry(ctx, url, listID, page)
		if err != nil {
			// Se falhou após todos os retries, retorna o que já coletou + erro
			logger.Get(ctx).Error().
				Str("list_id", listID).
				Int("page", page).
				Int("attempts", RetryMaxAttempts).
				Int("collected", totalCollected).
				Err(err).
				Msg("Falha definitiva na coleta")
			return allTasks, fmt.Errorf("lista %s página %d: %w (coletadas %d tarefas antes do erro)", 
				listID, page, err, totalCollected)
		}

		allTasks = append(allTasks, resp.Tasks...)
		totalCollected = len(allTasks)

		logger.Get(ctx).Info().
			Str("list_id", listID).
			Int("page", page).
			Int("tasks", len(resp.Tasks)).
			Int("total", totalCollected).
			Bool("last_page", resp.LastPage).
			Msg("Tasks coletadas")

		// Condição de parada: última página ou menos que PageSize
		if resp.LastPage || len(resp.Tasks) < PageSize {
			break
		}

		page++
	}

	logger.Get(ctx).Info().
		Str("list_id", listID).
		Int("tasks", len(allTasks)).
		Int("pages", page+1).
		Msg("Lista concluída")
	return allTasks, nil
}

// doRequestWithRetry executa request com retry e backoff
func (c *Client) doRequestWithRetry(ctx context.Context, url, listID string, page int) (*model.TaskResponse, error) {
	var lastErr error

	for attempt := 1; attempt <= RetryMaxAttempts; attempt++ {
		resp, err := c.doRequest(ctx, url)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Se é erro de contexto cancelado, não faz retry
		if ctx.Err() != nil {
			return nil, err
		}

		// Se é rate limit ou não autorizado, não faz retry
		if err == model.ErrRateLimited || err == model.ErrUnauthorized || err == model.ErrNotFound {
			return nil, err
		}

		// Se ainda tem tentativas, aguarda e tenta novamente
		if attempt < RetryMaxAttempts {
			logger.Get(ctx).Warn().
			Str("list_id", listID).
			Int("page", page).
			Int("attempt", attempt).
			Int("max_attempts", RetryMaxAttempts).
			Err(err).
			Dur("backoff", RetryBackoff).
			Msg("Tentativa falhou, aguardando retry")

			select {
			case <-time.After(RetryBackoff):
				logger.Get(ctx).Info().
					Str("list_id", listID).
					Int("page", page).
					Int("attempt", attempt+1).
					Msg("Retomando tentativa")
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	return nil, lastErr
}

// GetTasksMultiple busca tarefas de múltiplas listas com concorrência controlada
// DEPRECATED: Use GetTasksToStorage para grandes volumes
func (c *Client) GetTasksMultiple(ctx context.Context, listIDs []string, subtasks, includeClosed bool) ([]model.Task, error) {
	storage, err := repository.NewTaskStorage()
	if err != nil {
		return nil, fmt.Errorf("criar storage: %w", err)
	}
	defer storage.Close()

	if err := c.GetTasksToStorage(ctx, listIDs, storage, subtasks, includeClosed); err != nil {
		return nil, err
	}

	return storage.ReadAllTasks()
}

// GetTasksToStorage busca tarefas e salva diretamente no storage (baixo consumo de memória)
func (c *Client) GetTasksToStorage(ctx context.Context, listIDs []string, storage *repository.TaskStorage, subtasks, includeClosed bool) error {
	totalTasks := 0

	for i, listID := range listIDs {
		logger.Get(ctx).Info().
			Int("current", i+1).
			Int("total", len(listIDs)).
			Str("list_id", listID).
			Msg("Processando lista")

		page := 0
		listTasks := 0

		for {
			// Aguarda rate limiter
			if err := c.limiter.Wait(ctx); err != nil {
				return fmt.Errorf("rate limiter: %w", err)
			}

			url := buildTaskURL(listID, page, subtasks, includeClosed)

			// Executa request com retry
			resp, err := c.doRequestWithRetry(ctx, url, listID, page)
			if err != nil {
				logger.Get(ctx).Warn().
				Str("list_id", listID).
				Int("page", page).
				Err(err).
				Int("collected", listTasks).
				Msg("Falha na lista, continuando")
				break // Continua para próxima lista
			}

			// Salva tasks no storage (não acumula em memória)
			if err := storage.AppendTasks(resp.Tasks); err != nil {
				return fmt.Errorf("salvar tasks no storage: %w", err)
			}

			listTasks += len(resp.Tasks)
			totalTasks += len(resp.Tasks)

			logger.Get(ctx).Info().
				Str("list_id", listID).
				Int("page", page).
				Int("page_tasks", len(resp.Tasks)).
				Int("list_tasks", listTasks).
				Int("total_tasks", totalTasks).
				Bool("last_page", resp.LastPage).
				Msg("Tasks coletadas")

			// Condição de parada: última página
			if resp.LastPage {
				break
			}

			page++
		}

		logger.Get(ctx).Info().
			Str("list_id", listID).
			Int("tasks", listTasks).
			Int("pages", page+1).
			Msg("Lista concluída")
	}

	logger.Get(ctx).Info().
		Int("total_tasks", totalTasks).
		Int("total_lists", len(listIDs)).
		Msg("Todas as listas processadas")
	return nil
}

// doRequest executa uma requisição HTTP para a API do ClickUp
func (c *Client) doRequest(ctx context.Context, url string) (*model.TaskResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("criar request: %w", err)
	}

	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, model.ErrTimeout
		}
		return nil, fmt.Errorf("executar request: %w", err)
	}
	defer resp.Body.Close()

	// Tratamento de erros HTTP
	switch resp.StatusCode {
	case http.StatusOK:
		// OK, continua
	case http.StatusTooManyRequests:
		return nil, model.ErrRateLimited
	case http.StatusUnauthorized:
		return nil, model.ErrUnauthorized
	case http.StatusNotFound:
		return nil, model.ErrNotFound
	default:
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	// Parse da resposta
	var taskResp model.TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&taskResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &taskResp, nil
}

