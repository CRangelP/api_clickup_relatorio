package model

// ReportRequest representa o payload de entrada para geração de relatório
type ReportRequest struct {
	ListIDs    []string `json:"list_ids" binding:"required,min=1"`
	Fields     []string `json:"fields" binding:"required,min=1"`
	WebhookURL string   `json:"webhook_url" binding:"omitempty,url"`
	Subtasks   *bool    `json:"subtasks,omitempty"` // nil = false (default: apenas main tasks)
}

// Response representa a resposta padrão da API
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
	Errors  []string    `json:"errors,omitempty"`
}

// Meta contém metadados da resposta
type Meta struct {
	TotalTasks   int `json:"total_tasks,omitempty"`
	TotalLists   int `json:"total_lists,omitempty"`
	TotalColumns int `json:"total_columns,omitempty"`
}

// ErrorResponse representa uma resposta de erro
type ErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// WebhookPayload representa o payload enviado para o webhook
type WebhookPayload struct {
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	FolderName string `json:"folder_name,omitempty"`
	TotalTasks int    `json:"total_tasks,omitempty"`
	TotalLists int    `json:"total_lists,omitempty"`
	FileName   string `json:"file_name,omitempty"`
	FileMime   string `json:"file_mime,omitempty"`
	FileBase64 string `json:"file_base64,omitempty"`
}
