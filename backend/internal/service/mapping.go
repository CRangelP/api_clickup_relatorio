package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cleberrangel/clickup-excel-api/internal/repository"
)

// Mapping errors
var (
	ErrInvalidMapping   = errors.New("mapeamento inválido ou incompleto")
	ErrMissingTaskID    = errors.New("coluna 'id task' é obrigatória")
	ErrDuplicateMapping = errors.New("mapeamento duplicado detectado")
	ErrInvalidFieldType = errors.New("tipo de campo incompatível com dados da coluna")
	ErrFieldNotFound    = errors.New("campo personalizado não encontrado")
	ErrMappingNotFound  = errors.New("mapeamento não encontrado")
)

// ColumnMapping represents a mapping between a file column and a custom field
type ColumnMapping struct {
	Column     string `json:"column"`
	FieldID    string `json:"field_id"`
	FieldName  string `json:"field_name"`
	FieldType  string `json:"field_type"`
	IsRequired bool   `json:"is_required"`
	IsTaskID   bool   `json:"is_task_id"`
}

// MappingRequest represents a request to create a mapping
type MappingRequest struct {
	FilePath string          `json:"file_path" binding:"required"`
	Mappings []ColumnMapping `json:"mappings" binding:"required"`
	Title    string          `json:"title" binding:"required"`
}

// MappingValidationResult represents the result of mapping validation
type MappingValidationResult struct {
	Valid       bool     `json:"valid"`
	Errors      []string `json:"errors,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
	HasTaskID   bool     `json:"has_task_id"`
	TotalFields int      `json:"total_fields"`
}

// StoredMapping represents a mapping stored in memory (temporary)
type StoredMapping struct {
	ID        string          `json:"id"`
	UserID    string          `json:"user_id"`
	FilePath  string          `json:"file_path"`
	Title     string          `json:"title"`
	Mappings  []ColumnMapping `json:"mappings"`
	Validated bool            `json:"validated"`
}

// MappingService handles column to custom field mapping operations
type MappingService struct {
	metadataRepo *repository.MetadataRepository
	mappings     map[string]*StoredMapping // In-memory storage for temporary mappings
}

// NewMappingService creates a new mapping service
func NewMappingService(metadataRepo *repository.MetadataRepository) *MappingService {
	return &MappingService{
		metadataRepo: metadataRepo,
		mappings:     make(map[string]*StoredMapping),
	}
}


// ValidateMapping validates a mapping request
func (s *MappingService) ValidateMapping(req *MappingRequest, fileColumns []string) (*MappingValidationResult, error) {
	result := &MappingValidationResult{
		Valid:       true,
		Errors:      []string{},
		Warnings:    []string{},
		HasTaskID:   false,
		TotalFields: len(req.Mappings),
	}

	if len(req.Mappings) == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, "nenhum mapeamento definido")
		return result, nil
	}

	// Get available custom fields from database
	customFields, err := s.metadataRepo.GetCustomFields()
	if err != nil {
		return nil, err
	}

	// Create a map for quick field lookup
	fieldMap := make(map[string]repository.CustomField)
	for _, f := range customFields {
		fieldMap[f.ID] = f
	}

	// Create a map for quick column lookup
	columnSet := make(map[string]bool)
	for _, col := range fileColumns {
		columnSet[strings.ToLower(strings.TrimSpace(col))] = true
	}

	// Track mapped fields to detect duplicates
	mappedFields := make(map[string]string) // fieldID -> column

	for _, mapping := range req.Mappings {
		// Check if this is the task ID column
		if mapping.IsTaskID {
			result.HasTaskID = true
			continue
		}

		// Skip empty mappings
		if mapping.FieldID == "" {
			continue
		}

		// Validate column exists in file
		colLower := strings.ToLower(strings.TrimSpace(mapping.Column))
		if !columnSet[colLower] {
			result.Valid = false
			result.Errors = append(result.Errors, "coluna '"+mapping.Column+"' não encontrada no arquivo")
			continue
		}

		// Validate field exists
		field, exists := fieldMap[mapping.FieldID]
		if !exists {
			result.Valid = false
			result.Errors = append(result.Errors, "campo '"+mapping.FieldID+"' não encontrado")
			continue
		}

		// Check for duplicate mappings (same field mapped to multiple columns)
		if existingCol, isDuplicate := mappedFields[mapping.FieldID]; isDuplicate {
			result.Valid = false
			result.Errors = append(result.Errors, "campo '"+field.Name+"' já mapeado para coluna '"+existingCol+"'")
			continue
		}
		mappedFields[mapping.FieldID] = mapping.Column

		// Validate type compatibility
		if !s.isTypeCompatible(mapping.FieldType, field.Type) {
			result.Warnings = append(result.Warnings, "tipo do campo '"+field.Name+"' pode ser incompatível com dados da coluna")
		}
	}

	// Check for required task ID column
	if !result.HasTaskID {
		result.Valid = false
		result.Errors = append(result.Errors, "coluna 'id task' é obrigatória para identificar as tarefas")
	}

	return result, nil
}

// isTypeCompatible checks if a column type is compatible with a field type
func (s *MappingService) isTypeCompatible(columnType, fieldType string) bool {
	// Most types are compatible with string data from CSV/XLSX
	// This is a basic compatibility check
	compatibleTypes := map[string][]string{
		"text":          {"text", "short_text", "email", "url", "phone"},
		"number":        {"number", "currency", "percentage"},
		"date":          {"date"},
		"dropdown":      {"drop_down", "labels"},
		"checkbox":      {"checkbox"},
		"users":         {"users"},
		"automatic":     {"automatic_progress", "tasks", "formula"},
	}

	// If no specific type is provided, assume compatible
	if columnType == "" {
		return true
	}

	// Check if types are directly equal
	if columnType == fieldType {
		return true
	}

	// Check compatibility groups
	for _, types := range compatibleTypes {
		hasColumn := false
		hasField := false
		for _, t := range types {
			if t == columnType {
				hasColumn = true
			}
			if t == fieldType {
				hasField = true
			}
		}
		if hasColumn && hasField {
			return true
		}
	}

	// Default to compatible for flexibility
	return true
}


// SaveMapping saves a mapping temporarily
func (s *MappingService) SaveMapping(userID string, req *MappingRequest) (*StoredMapping, error) {
	// Generate a unique ID for the mapping
	id := generateMappingID()

	stored := &StoredMapping{
		ID:        id,
		UserID:    userID,
		FilePath:  req.FilePath,
		Title:     req.Title,
		Mappings:  req.Mappings,
		Validated: false,
	}

	s.mappings[id] = stored
	return stored, nil
}

// GetMapping retrieves a mapping by ID
func (s *MappingService) GetMapping(id string) (*StoredMapping, error) {
	mapping, exists := s.mappings[id]
	if !exists {
		return nil, ErrMappingNotFound
	}
	return mapping, nil
}

// GetMappingByUser retrieves a mapping by ID and validates user ownership
func (s *MappingService) GetMappingByUser(id, userID string) (*StoredMapping, error) {
	mapping, exists := s.mappings[id]
	if !exists {
		return nil, ErrMappingNotFound
	}
	if mapping.UserID != userID {
		return nil, ErrMappingNotFound
	}
	return mapping, nil
}

// DeleteMapping removes a mapping
func (s *MappingService) DeleteMapping(id string) error {
	if _, exists := s.mappings[id]; !exists {
		return ErrMappingNotFound
	}
	delete(s.mappings, id)
	return nil
}

// GetMappingsByUser returns all mappings for a user
func (s *MappingService) GetMappingsByUser(userID string) []*StoredMapping {
	var result []*StoredMapping
	for _, m := range s.mappings {
		if m.UserID == userID {
			result = append(result, m)
		}
	}
	return result
}

// ValidateAndSaveMapping validates and saves a mapping
func (s *MappingService) ValidateAndSaveMapping(userID string, req *MappingRequest, fileColumns []string) (*StoredMapping, *MappingValidationResult, error) {
	// First validate the mapping
	validationResult, err := s.ValidateMapping(req, fileColumns)
	if err != nil {
		return nil, nil, err
	}

	// If validation failed, return the result without saving
	if !validationResult.Valid {
		return nil, validationResult, nil
	}

	// Save the mapping
	stored, err := s.SaveMapping(userID, req)
	if err != nil {
		return nil, validationResult, err
	}

	stored.Validated = true
	return stored, validationResult, nil
}

// CheckDuplicateMappings checks for duplicate field mappings in a request
func (s *MappingService) CheckDuplicateMappings(mappings []ColumnMapping) []string {
	var duplicates []string
	fieldCount := make(map[string]int)
	fieldNames := make(map[string]string)

	for _, m := range mappings {
		if m.FieldID == "" || m.IsTaskID {
			continue
		}
		fieldCount[m.FieldID]++
		fieldNames[m.FieldID] = m.FieldName
	}

	for fieldID, count := range fieldCount {
		if count > 1 {
			name := fieldNames[fieldID]
			if name == "" {
				name = fieldID
			}
			duplicates = append(duplicates, name)
		}
	}

	return duplicates
}

// FindTaskIDColumn finds the task ID column in mappings
func (s *MappingService) FindTaskIDColumn(mappings []ColumnMapping) (string, bool) {
	for _, m := range mappings {
		if m.IsTaskID {
			return m.Column, true
		}
	}
	return "", false
}

// ConvertToJobMapping converts column mappings to a simple map for job processing
func (s *MappingService) ConvertToJobMapping(mappings []ColumnMapping) map[string]string {
	result := make(map[string]string)
	for _, m := range mappings {
		if m.FieldID != "" && !m.IsTaskID {
			result[m.Column] = m.FieldID
		}
	}
	return result
}

// generateMappingID generates a unique ID for a mapping
func generateMappingID() string {
	// Simple ID generation using timestamp
	return fmt.Sprintf("map_%d", time.Now().UnixNano())
}
