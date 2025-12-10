package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/logger"
	"github.com/cleberrangel/clickup-excel-api/internal/model"
)

// WebhookService envia resultados para webhooks
type WebhookService struct {
	httpClient *http.Client
}

// NewWebhookService cria um novo serviço de webhook
func NewWebhookService() *WebhookService {
	return &WebhookService{
		// Timeout controlado pelo contexto do processAsync (30min)
		httpClient: &http.Client{},
	}
}

// SendSuccess envia o resultado de sucesso para o webhook
func (w *WebhookService) SendSuccess(ctx context.Context, webhookURL string, result *ReportResult) error {
	// Abre arquivo
	file, err := os.Open(result.FilePath)
	if err != nil {
		return fmt.Errorf("abrir arquivo: %w", err)
	}
	defer file.Close()

	stat, _ := file.Stat()
	const smallFileThreshold = 20 * 1024 * 1024 // 20 MB

	// Para arquivos pequenos, monta multipart em memória e envia com Content-Length (evita chunked)
	if stat != nil && stat.Size() <= smallFileThreshold {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)

		if err := writer.WriteField("success", "true"); err != nil {
			return fmt.Errorf("write success: %w", err)
		}
		if err := writer.WriteField("folder_name", result.FolderName); err != nil {
			return fmt.Errorf("write folder_name: %w", err)
		}
		if err := writer.WriteField("total_tasks", fmt.Sprintf("%d", result.TotalTasks)); err != nil {
			return fmt.Errorf("write total_tasks: %w", err)
		}
		if err := writer.WriteField("total_lists", fmt.Sprintf("%d", result.TotalLists)); err != nil {
			return fmt.Errorf("write total_lists: %w", err)
		}
		if err := writer.WriteField("file_mime", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"); err != nil {
			return fmt.Errorf("write file_mime: %w", err)
		}

		filename := fmt.Sprintf("%s.xlsx", result.FolderName)
		part, err := writer.CreateFormFile("file", filepath.Base(filename))
		if err != nil {
			return fmt.Errorf("criar form file: %w", err)
		}
		if _, err := io.Copy(part, file); err != nil {
			return fmt.Errorf("copiar arquivo: %w", err)
		}

		if err := writer.Close(); err != nil {
			return fmt.Errorf("fechar writer: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, &body)
		if err != nil {
			return fmt.Errorf("criar request: %w", err)
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())

		resp, err := w.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("enviar webhook: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("webhook retornou status %d: %s", resp.StatusCode, string(respBody))
		}

		logger.Get(ctx).Info().
			Str("url", webhookURL).
			Int("status", resp.StatusCode).
			Str("mode", "buffer").
			Int64("size_bytes", stat.Size()).
			Msg("Webhook enviado com sucesso")
		return nil
	}

	// Para arquivos maiores, usa streaming via pipe (chunked)
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer file.Close()

		if err := writer.WriteField("success", "true"); err != nil {
			pw.CloseWithError(fmt.Errorf("write success: %w", err))
			return
		}
		if err := writer.WriteField("folder_name", result.FolderName); err != nil {
			pw.CloseWithError(fmt.Errorf("write folder_name: %w", err))
			return
		}
		if err := writer.WriteField("total_tasks", fmt.Sprintf("%d", result.TotalTasks)); err != nil {
			pw.CloseWithError(fmt.Errorf("write total_tasks: %w", err))
			return
		}
		if err := writer.WriteField("total_lists", fmt.Sprintf("%d", result.TotalLists)); err != nil {
			pw.CloseWithError(fmt.Errorf("write total_lists: %w", err))
			return
		}
		if err := writer.WriteField("file_mime", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"); err != nil {
			pw.CloseWithError(fmt.Errorf("write file_mime: %w", err))
			return
		}

		filename := fmt.Sprintf("%s.xlsx", result.FolderName)
		part, err := writer.CreateFormFile("file", filepath.Base(filename))
		if err != nil {
			pw.CloseWithError(fmt.Errorf("criar form file: %w", err))
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			pw.CloseWithError(fmt.Errorf("copiar arquivo: %w", err))
			return
		}

		if err := writer.Close(); err != nil {
			pw.CloseWithError(fmt.Errorf("fechar writer: %w", err))
			return
		}
		_ = pw.Close()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, pr)
	if err != nil {
		return fmt.Errorf("criar request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("enviar webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook retornou status %d: %s", resp.StatusCode, string(respBody))
	}

	logger.Get(ctx).Info().
		Str("url", webhookURL).
		Int("status", resp.StatusCode).
		Str("mode", "streaming").
		Msg("Webhook enviado com sucesso")
	return nil
}

// SendError envia o resultado de erro para o webhook
func (w *WebhookService) SendError(ctx context.Context, webhookURL string, err error) error {
	payload := model.WebhookPayload{
		Success: false,
		Error:   err.Error(),
	}

	return w.send(ctx, webhookURL, payload)
}

// send envia o payload para o webhook
func (w *WebhookService) send(ctx context.Context, webhookURL string, payload model.WebhookPayload) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("criar request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("enviar webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook retornou status %d", resp.StatusCode)
	}

	logger.Get(ctx).Info().
		Str("url", webhookURL).
		Int("status", resp.StatusCode).
		Msg("Webhook enviado com sucesso")

	return nil
}
