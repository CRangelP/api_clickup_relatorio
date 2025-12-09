package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/model"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

const (
	baseURL = "https://api.clickup.com/api/v2"

	// MaxConcurrentRequests limita requisições simultâneas
	MaxConcurrentRequests = 5

	// RequestsPerMinute limite conservador (ClickUp permite 10k/min)
	RequestsPerMinute = 100

	// DefaultTimeout timeout padrão para requisições
	DefaultTimeout = 60 * time.Second

	// PageSize tamanho padrão da página do ClickUp
	PageSize = 100

	// RetryMaxAttempts número máximo de tentativas por página
	RetryMaxAttempts = 3

	// RetryBackoff tempo de espera entre retries (1m30s)
	RetryBackoff = 90 * time.Second
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
		limiter: rate.NewLimiter(rate.Every(time.Minute/RequestsPerMinute), 1),
	}
}

// GetTasks busca todas as tarefas de uma lista com paginação automática e retry
func (c *Client) GetTasks(ctx context.Context, listID string) ([]model.Task, error) {
	var allTasks []model.Task
	page := 0
	totalCollected := 0

	for {
		// Aguarda rate limiter
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter: %w", err)
		}

		url := fmt.Sprintf("%s/list/%s/task?page=%d&subtasks=true&include_closed=true",
			baseURL, listID, page)

		// Executa request com retry
		resp, err := c.doRequestWithRetry(ctx, url, listID, page)
		if err != nil {
			// Se falhou após todos os retries, retorna o que já coletou + erro
			log.Printf("[ClickUp] Lista %s - Falha definitiva na página %d após %d tentativas. Coletadas: %d tarefas",
				listID, page, RetryMaxAttempts, totalCollected)
			return allTasks, fmt.Errorf("lista %s página %d: %w (coletadas %d tarefas antes do erro)", 
				listID, page, err, totalCollected)
		}

		allTasks = append(allTasks, resp.Tasks...)
		totalCollected = len(allTasks)

		log.Printf("[ClickUp] Lista %s - Página %d: %d tarefas (total coletado: %d, last_page=%v)",
			listID, page, len(resp.Tasks), totalCollected, resp.LastPage)

		// Condição de parada: última página ou menos que PageSize
		if resp.LastPage || len(resp.Tasks) < PageSize {
			break
		}

		page++
	}

	log.Printf("[ClickUp] Lista %s - Concluída: %d tarefas em %d páginas", listID, len(allTasks), page+1)
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
			log.Printf("[ClickUp] Lista %s página %d - Tentativa %d/%d falhou: %v. Aguardando %v...",
				listID, page, attempt, RetryMaxAttempts, err, RetryBackoff)

			select {
			case <-time.After(RetryBackoff):
				log.Printf("[ClickUp] Lista %s página %d - Retomando tentativa %d/%d",
					listID, page, attempt+1, RetryMaxAttempts)
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	return nil, lastErr
}

// GetTasksMultiple busca tarefas de múltiplas listas com concorrência controlada
func (c *Client) GetTasksMultiple(ctx context.Context, listIDs []string) ([]model.Task, error) {
	var (
		allTasks []model.Task
		mu       sync.Mutex
	)

	g, gCtx := errgroup.WithContext(ctx)

	// Semáforo para limitar concorrência
	sem := make(chan struct{}, MaxConcurrentRequests)

	for _, listID := range listIDs {
		listID := listID // capture loop variable

		g.Go(func() error {
			// Adquire slot no semáforo
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-gCtx.Done():
				return gCtx.Err()
			}

			tasks, err := c.GetTasks(gCtx, listID)
			if err != nil {
				return err
			}

			mu.Lock()
			allTasks = append(allTasks, tasks...)
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	log.Printf("[ClickUp] Total: %d tarefas de %d listas", len(allTasks), len(listIDs))

	return allTasks, nil
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
