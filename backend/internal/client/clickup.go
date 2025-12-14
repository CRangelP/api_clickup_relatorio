package client

import (
	"bytes"
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

// GetWorkspaces busca todos os workspaces do usuário
func (c *Client) GetWorkspaces(ctx context.Context) ([]model.Workspace, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	url := fmt.Sprintf("%s/team", baseURL)
	
	var resp model.WorkspaceResponse
	if err := c.doGenericRequest(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("buscar workspaces: %w", err)
	}

	return resp.Teams, nil
}

// GetSpaces busca todos os spaces de um workspace
func (c *Client) GetSpaces(ctx context.Context, workspaceID string) ([]model.Space, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	url := fmt.Sprintf("%s/team/%s/space", baseURL, workspaceID)
	
	var resp model.SpaceResponse
	if err := c.doGenericRequest(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("buscar spaces: %w", err)
	}

	return resp.Spaces, nil
}

// GetFolders busca todos os folders de um space
func (c *Client) GetFolders(ctx context.Context, spaceID string) ([]model.Folder, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	url := fmt.Sprintf("%s/space/%s/folder", baseURL, spaceID)
	
	var resp model.FolderResponse
	if err := c.doGenericRequest(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("buscar folders: %w", err)
	}

	return resp.Folders, nil
}

// GetLists busca todas as listas de um folder
func (c *Client) GetLists(ctx context.Context, folderID string) ([]model.List, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	url := fmt.Sprintf("%s/folder/%s/list", baseURL, folderID)
	
	var resp model.ListResponse
	if err := c.doGenericRequest(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("buscar listas: %w", err)
	}

	return resp.Lists, nil
}

// GetCustomFields busca todos os campos personalizados de uma lista
func (c *Client) GetCustomFields(ctx context.Context, listID string) ([]model.CustomFieldMetadata, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	url := fmt.Sprintf("%s/list/%s/field", baseURL, listID)
	
	var resp model.CustomFieldResponse
	if err := c.doGenericRequest(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("buscar campos personalizados: %w", err)
	}

	return resp.Fields, nil
}

// ValidateToken valida se o token é válido fazendo uma requisição simples
func (c *Client) ValidateToken(ctx context.Context) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limiter: %w", err)
	}

	url := fmt.Sprintf("%s/user", baseURL)
	
	var resp model.UserResponse
	if err := c.doGenericRequest(ctx, url, &resp); err != nil {
		return fmt.Errorf("validar token: %w", err)
	}

	return nil
}

// doGenericRequest executa uma requisição HTTP genérica para a API do ClickUp
func (c *Client) doGenericRequest(ctx context.Context, url string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("criar request: %w", err)
	}

	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return model.ErrTimeout
		}
		return fmt.Errorf("executar request: %w", err)
	}
	defer resp.Body.Close()

	// Tratamento de erros HTTP
	switch resp.StatusCode {
	case http.StatusOK:
		// OK, continua
	case http.StatusTooManyRequests:
		return model.ErrRateLimited
	case http.StatusUnauthorized:
		return model.ErrUnauthorized
	case http.StatusNotFound:
		return model.ErrNotFound
	default:
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	// Parse da resposta
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

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

// SetCustomFieldValue updates a custom field value for a task
// The value is transformed based on the field type before sending to the API
func (c *Client) SetCustomFieldValue(ctx context.Context, taskID, fieldID string, value interface{}, fieldType string) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limiter: %w", err)
	}

	url := fmt.Sprintf("%s/task/%s/field/%s", baseURL, taskID, fieldID)
	
	// Transform value based on field type
	transformedValue := TransformFieldValue(value, fieldType)
	
	// Build request body
	body := map[string]interface{}{
		"value": transformedValue,
	}
	
	return c.doPostRequest(ctx, url, body)
}

// TransformFieldValue transforms a value based on the custom field type
// This handles the different value formats required by ClickUp's API
func TransformFieldValue(value interface{}, fieldType string) interface{} {
	strValue := fmt.Sprintf("%v", value)
	
	switch fieldType {
	case "text", "short_text", "email", "url", "phone":
		// Text-based fields use string value directly
		return strValue
		
	case "number", "currency", "percentage":
		// Numeric fields - try to parse as number
		return parseNumericValue(strValue)
		
	case "checkbox":
		// Boolean field
		return parseBooleanValue(strValue)
		
	case "date":
		// Date field - expects Unix timestamp in milliseconds
		return parseDateValue(strValue)
		
	case "drop_down":
		// Dropdown expects the option ID or name
		// If it's already an ID (UUID format), use it directly
		// Otherwise, return as-is and let the API handle it
		return strValue
		
	case "labels":
		// Labels field expects an array of label IDs or names
		return parseLabelsValue(strValue)
		
	case "rating":
		// Rating field expects an integer
		return parseNumericValue(strValue)
		
	case "users":
		// Users field expects an array of user IDs
		return parseUsersValue(strValue)
		
	case "location":
		// Location field expects a location object
		return map[string]interface{}{
			"location": map[string]interface{}{
				"formatted_address": strValue,
			},
		}
		
	default:
		// For unknown types, return as string
		return strValue
	}
}

// parseNumericValue attempts to parse a string as a number
func parseNumericValue(s string) interface{} {
	// Try integer first
	var intVal int64
	if _, err := fmt.Sscanf(s, "%d", &intVal); err == nil {
		return intVal
	}
	
	// Try float
	var floatVal float64
	if _, err := fmt.Sscanf(s, "%f", &floatVal); err == nil {
		return floatVal
	}
	
	// Return 0 if parsing fails
	return 0
}

// parseBooleanValue parses a string as a boolean
func parseBooleanValue(s string) bool {
	switch s {
	case "true", "1", "yes", "sim", "TRUE", "True", "YES", "Yes", "SIM", "Sim":
		return true
	default:
		return false
	}
}

// parseDateValue parses a date string and returns Unix timestamp in milliseconds
func parseDateValue(s string) interface{} {
	// Common date formats to try
	formats := []string{
		"2006-01-02",
		"02/01/2006",
		"01/02/2006",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
	}
	
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t.UnixMilli()
		}
	}
	
	// If it's already a number (timestamp), return it
	var timestamp int64
	if _, err := fmt.Sscanf(s, "%d", &timestamp); err == nil {
		return timestamp
	}
	
	// Return nil if parsing fails
	return nil
}

// parseLabelsValue parses a comma-separated string into an array
func parseLabelsValue(s string) []string {
	if s == "" {
		return []string{}
	}
	
	parts := splitAndTrim(s, ",")
	return parts
}

// parseUsersValue parses a comma-separated string of user IDs
func parseUsersValue(s string) []interface{} {
	if s == "" {
		return []interface{}{}
	}
	
	parts := splitAndTrim(s, ",")
	result := make([]interface{}, len(parts))
	for i, p := range parts {
		// Try to parse as integer (user IDs are typically integers)
		var userID int64
		if _, err := fmt.Sscanf(p, "%d", &userID); err == nil {
			result[i] = userID
		} else {
			result[i] = p
		}
	}
	return result
}

// splitAndTrim splits a string and trims whitespace from each part
func splitAndTrim(s, sep string) []string {
	parts := make([]string, 0)
	for _, part := range splitString(s, sep) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

// splitString splits a string by separator (simple implementation)
func splitString(s, sep string) []string {
	result := make([]string, 0)
	current := ""
	sepLen := len(sep)
	
	for i := 0; i < len(s); i++ {
		if i+sepLen <= len(s) && s[i:i+sepLen] == sep {
			result = append(result, current)
			current = ""
			i += sepLen - 1
		} else {
			current += string(s[i])
		}
	}
	result = append(result, current)
	return result
}

// trimSpace removes leading and trailing whitespace
func trimSpace(s string) string {
	start := 0
	end := len(s)
	
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	
	return s[start:end]
}

// doPostRequest executes a POST request to the ClickUp API
func (c *Client) doPostRequest(ctx context.Context, url string, body interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("criar request: %w", err)
	}

	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return model.ErrTimeout
		}
		return fmt.Errorf("executar request: %w", err)
	}
	defer resp.Body.Close()

	// Tratamento de erros HTTP
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusTooManyRequests:
		return model.ErrRateLimited
	case http.StatusUnauthorized:
		return model.ErrUnauthorized
	case http.StatusNotFound:
		return model.ErrNotFound
	case http.StatusBadRequest:
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bad request: %s", string(respBody))
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}
}

// SetCustomFieldValueWithRetry updates a custom field with retry logic
func (c *Client) SetCustomFieldValueWithRetry(ctx context.Context, taskID, fieldID string, value interface{}, fieldType string) error {
	var lastErr error

	for attempt := 1; attempt <= RetryMaxAttempts; attempt++ {
		err := c.SetCustomFieldValue(ctx, taskID, fieldID, value, fieldType)
		if err == nil {
			return nil
		}

		lastErr = err

		// If context is cancelled, don't retry
		if ctx.Err() != nil {
			return err
		}

		// If it's rate limited, wait and retry
		if err == model.ErrRateLimited {
			logger.Get(ctx).Warn().
				Str("task_id", taskID).
				Str("field_id", fieldID).
				Int("attempt", attempt).
				Dur("backoff", RetryBackoff).
				Msg("Rate limited, aguardando retry")

			select {
			case <-time.After(RetryBackoff):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// For other errors, don't retry
		if err == model.ErrUnauthorized || err == model.ErrNotFound {
			return err
		}

		// For transient errors, retry with backoff
		if attempt < RetryMaxAttempts {
			logger.Get(ctx).Warn().
				Str("task_id", taskID).
				Str("field_id", fieldID).
				Int("attempt", attempt).
				Err(err).
				Dur("backoff", RetryBackoff).
				Msg("Tentativa falhou, aguardando retry")

			select {
			case <-time.After(RetryBackoff):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return lastErr
}

