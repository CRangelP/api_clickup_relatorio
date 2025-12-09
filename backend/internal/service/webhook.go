package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

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
	// Abre arquivo para stream
	file, err := os.Open(result.FilePath)
	if err != nil {
		return fmt.Errorf("abrir arquivo: %w", err)
	}
	defer file.Close()

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// escreve multipart em streaming
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

		filename := fmt.Sprintf("relatorio_%s.xlsx", time.Now().Format("2006-01-02_15-04-05"))
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

	log.Printf("[Webhook] Enviado com sucesso para %s (status: %d)", webhookURL, resp.StatusCode)
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

	log.Printf("[Webhook] Enviado com sucesso para %s (status: %d)", webhookURL, resp.StatusCode)

	return nil
}
