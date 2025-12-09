package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SendSuccess envia o resultado de sucesso para o webhook
func (w *WebhookService) SendSuccess(ctx context.Context, webhookURL string, result *ReportResult) error {
	// Converte o arquivo para base64
	fileBase64 := base64.StdEncoding.EncodeToString(result.Buffer.Bytes())

	payload := model.WebhookPayload{
		Success:    true,
		FolderName: result.FolderName,
		TotalTasks: result.TotalTasks,
		TotalLists: result.TotalLists,
		FileName:   fmt.Sprintf("relatorio_%s.xlsx", time.Now().Format("2006-01-02_15-04-05")),
		FileMime:   "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		FileBase64: fileBase64,
	}

	return w.send(ctx, webhookURL, payload)
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
