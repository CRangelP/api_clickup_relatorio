package service

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/model"
)

// NativeFields mapeia campos nativos do ClickUp para nomes de coluna
var NativeFields = map[string]string{
	"name":         "NOME DA TAREFA",
	"id":           "ID",
	"description":  "DESCRIÇÃO",
	"status":       "STATUS",
	"date_created": "DATA CRIAÇÃO",
	"date_updated": "DATA ATUALIZAÇÃO",
	"date_closed":  "DATA FECHAMENTO",
	"due_date":     "DATA VENCIMENTO",
	"start_date":   "DATA INÍCIO",
	"priority":     "PRIORIDADE",
	"assignees":    "RESPONSÁVEIS",
	"tags":         "TAGS",
	"list":         "LISTA",
	"folder":       "PASTA",
	"url":          "URL",
}

// Extractor é o serviço de extração de valores
type Extractor struct {
	location *time.Location
}

// NewExtractor cria um novo extrator
func NewExtractor() *Extractor {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Printf("[WARN] Não foi possível carregar timezone, usando UTC: %v", err)
		loc = time.UTC
	}

	return &Extractor{
		location: loc,
	}
}

// ResolveHeader retorna o nome da coluna para um campo
func (e *Extractor) ResolveHeader(fieldKey string, task model.Task) string {
	// Primeiro verifica se é campo nativo
	if name, ok := NativeFields[fieldKey]; ok {
		return name
	}

	// Se não é nativo, procura nos campos personalizados
	for _, cf := range task.CustomFields {
		if cf.ID == fieldKey {
			return cf.Name
		}
	}

	// Fallback: retorna o próprio ID
	return fieldKey
}

// ExtractNativeValue extrai valor de um campo nativo
func (e *Extractor) ExtractNativeValue(fieldKey string, task model.Task) string {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[WARN] Panic ao extrair campo nativo %s: %v", fieldKey, r)
		}
	}()

	switch fieldKey {
	case "name":
		return task.Name
	case "id":
		return task.ID
	case "description":
		return task.Description
	case "status":
		return task.Status.Status
	case "date_created":
		return e.formatEpochMs(task.DateCreated)
	case "date_updated":
		return e.formatEpochMs(task.DateUpdated)
	case "date_closed":
		return e.formatEpochMs(task.DateClosed)
	case "due_date":
		return e.formatEpochMs(task.DueDate)
	case "start_date":
		return e.formatEpochMs(task.StartDate)
	case "priority":
		if task.Priority != nil {
			return task.Priority.Priority
		}
		return ""
	case "assignees":
		return e.formatAssignees(task.Assignees)
	case "tags":
		return e.formatTags(task.Tags)
	case "list":
		return task.List.Name
	case "folder":
		return task.Folder.Name
	case "url":
		return task.URL
	default:
		return ""
	}
}

// ExtractCustomFieldValue extrai valor de um campo personalizado
func (e *Extractor) ExtractCustomFieldValue(fieldID string, task model.Task) string {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[WARN] Panic ao extrair campo personalizado %s: %v", fieldID, r)
		}
	}()

	// Encontra o campo personalizado
	var field *model.CustomField
	for i := range task.CustomFields {
		if task.CustomFields[i].ID == fieldID {
			field = &task.CustomFields[i]
			break
		}
	}

	if field == nil {
		return ""
	}

	if field.Value == nil {
		return ""
	}

	// Extrai baseado no tipo
	switch field.Type {
	case "drop_down":
		return e.extractDropdown(*field)
	case "labels":
		return e.extractLabels(*field)
	case "date":
		return e.extractDate(*field)
	case "currency":
		return e.extractCurrency(*field)
	case "number":
		return e.extractNumber(*field)
	case "email":
		return e.extractText(*field)
	case "phone":
		return e.extractText(*field)
	case "url":
		return e.extractText(*field)
	case "text", "short_text":
		return e.extractText(*field)
	case "checkbox":
		return e.extractCheckbox(*field)
	case "users":
		return e.extractUsers(*field)
	default:
		return e.extractText(*field)
	}
}

// ExtractValue extrai valor de qualquer campo (nativo ou personalizado)
func (e *Extractor) ExtractValue(fieldKey string, task model.Task) string {
	// Verifica se é campo nativo
	if _, isNative := NativeFields[fieldKey]; isNative {
		return e.ExtractNativeValue(fieldKey, task)
	}

	// Campo personalizado
	return e.ExtractCustomFieldValue(fieldKey, task)
}

// extractDropdown extrai valor de dropdown
func (e *Extractor) extractDropdown(field model.CustomField) string {
	// Pode vir como índice numérico ou como objeto com orderindex
	switch v := field.Value.(type) {
	case float64:
		idx := int(v)
		if field.TypeConfig == nil || idx < 0 || idx >= len(field.TypeConfig.Options) {
			log.Printf("[WARN] Dropdown %s: índice %d fora do range", field.ID, idx)
			return ""
		}
		return field.TypeConfig.Options[idx].Name

	case map[string]interface{}:
		// Formato alternativo: {"orderindex": 0}
		if orderIdx, ok := v["orderindex"].(float64); ok {
			idx := int(orderIdx)
			if field.TypeConfig != nil && idx >= 0 && idx < len(field.TypeConfig.Options) {
				return field.TypeConfig.Options[idx].Name
			}
		}
		// Ou pode ter o nome direto
		if name, ok := v["name"].(string); ok {
			return name
		}
		return ""

	default:
		log.Printf("[WARN] Dropdown %s: tipo inesperado %T", field.ID, field.Value)
		return ""
	}
}

// extractLabels extrai valores de labels (múltipla escolha)
func (e *Extractor) extractLabels(field model.CustomField) string {
	arr, ok := field.Value.([]interface{})
	if !ok {
		log.Printf("[WARN] Labels %s: tipo inesperado %T", field.ID, field.Value)
		return ""
	}

	var labels []string
	for _, item := range arr {
		switch v := item.(type) {
		case string:
			// Pode ser o ID da opção
			for _, opt := range field.TypeConfig.Options {
				if opt.ID == v {
					labels = append(labels, opt.Name)
					break
				}
			}
		case map[string]interface{}:
			if name, ok := v["name"].(string); ok {
				labels = append(labels, name)
			}
		case float64:
			idx := int(v)
			if field.TypeConfig != nil && idx >= 0 && idx < len(field.TypeConfig.Options) {
				labels = append(labels, field.TypeConfig.Options[idx].Name)
			}
		}
	}

	return strings.Join(labels, ", ")
}

// extractDate extrai valor de data
func (e *Extractor) extractDate(field model.CustomField) string {
	var epochMs int64

	switch v := field.Value.(type) {
	case float64:
		epochMs = int64(v)
	case string:
		if v == "" {
			return "" // Campo de data vazio no ClickUp
		}
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			log.Printf("[WARN] Date %s: não conseguiu parsear string: %v", field.ID, err)
			return ""
		}
		epochMs = parsed
	case map[string]interface{}:
		// Formato alternativo: {"date": "timestamp"}
		if dateStr, ok := v["date"].(string); ok {
			parsed, err := strconv.ParseInt(dateStr, 10, 64)
			if err == nil {
				epochMs = parsed
			}
		}
	default:
		log.Printf("[WARN] Date %s: tipo inesperado %T", field.ID, field.Value)
		return ""
	}

	if epochMs == 0 {
		return ""
	}

	t := time.UnixMilli(epochMs).In(e.location)
	return t.Format("02/01/2006")
}

// extractCurrency extrai valor de moeda
func (e *Extractor) extractCurrency(field model.CustomField) string {
	var value float64

	switch v := field.Value.(type) {
	case float64:
		value = v
	case string:
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			log.Printf("[WARN] Currency %s: não conseguiu parsear: %v", field.ID, err)
			return ""
		}
		value = parsed
	default:
		log.Printf("[WARN] Currency %s: tipo inesperado %T", field.ID, field.Value)
		return ""
	}

	// Formata como moeda brasileira
	return fmt.Sprintf("R$ %.2f", value)
}

// extractNumber extrai valor numérico
func (e *Extractor) extractNumber(field model.CustomField) string {
	switch v := field.Value.(type) {
	case float64:
		// Remove decimais se for inteiro
		if v == float64(int64(v)) {
			return fmt.Sprintf("%.0f", v)
		}
		return fmt.Sprintf("%.2f", v)
	case string:
		return v
	default:
		log.Printf("[WARN] Number %s: tipo inesperado %T", field.ID, field.Value)
		return ""
	}
}

// extractText extrai valor de texto
func (e *Extractor) extractText(field model.CustomField) string {
	switch v := field.Value.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%v", v)
	case map[string]interface{}:
		if text, ok := v["text"].(string); ok {
			return text
		}
		if value, ok := v["value"].(string); ok {
			return value
		}
		return ""
	default:
		return fmt.Sprintf("%v", field.Value)
	}
}

// extractCheckbox extrai valor de checkbox
func (e *Extractor) extractCheckbox(field model.CustomField) string {
	switch v := field.Value.(type) {
	case bool:
		if v {
			return "SIM"
		}
		return "NÃO"
	case string:
		if v == "true" || v == "1" {
			return "SIM"
		}
		return "NÃO"
	default:
		log.Printf("[WARN] Checkbox %s: tipo inesperado %T", field.ID, field.Value)
		return ""
	}
}

// extractUsers extrai usuários
func (e *Extractor) extractUsers(field model.CustomField) string {
	arr, ok := field.Value.([]interface{})
	if !ok {
		log.Printf("[WARN] Users %s: tipo inesperado %T", field.ID, field.Value)
		return ""
	}

	var users []string
	for _, item := range arr {
		if user, ok := item.(map[string]interface{}); ok {
			if username, ok := user["username"].(string); ok {
				users = append(users, username)
			} else if email, ok := user["email"].(string); ok {
				users = append(users, email)
			}
		}
	}

	return strings.Join(users, ", ")
}

// formatEpochMs formata timestamp em milissegundos para data
func (e *Extractor) formatEpochMs(epochStr string) string {
	if epochStr == "" {
		return ""
	}

	epochMs, err := strconv.ParseInt(epochStr, 10, 64)
	if err != nil {
		return ""
	}

	t := time.UnixMilli(epochMs).In(e.location)
	return t.Format("02/01/2006")
}

// formatAssignees formata lista de responsáveis
func (e *Extractor) formatAssignees(assignees []model.Assignee) string {
	if len(assignees) == 0 {
		return ""
	}

	var names []string
	for _, a := range assignees {
		names = append(names, a.Username)
	}

	return strings.Join(names, ", ")
}

// formatTags formata lista de tags
func (e *Extractor) formatTags(tags []model.Tag) string {
	if len(tags) == 0 {
		return ""
	}

	var names []string
	for _, t := range tags {
		names = append(names, t.Name)
	}

	return strings.Join(names, ", ")
}
