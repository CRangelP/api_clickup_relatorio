package service

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xuri/excelize/v2"
)

// File upload errors
var (
	ErrInvalidFile     = errors.New("arquivo inválido ou corrompido")
	ErrFileTooLarge    = errors.New("arquivo excede limite de 10MB")
	ErrUnsupportedType = errors.New("formato de arquivo não suportado (use CSV ou XLSX)")
	ErrEmptyFile       = errors.New("arquivo está vazio")
	ErrNoColumns       = errors.New("arquivo não contém colunas")
)

const (
	// MaxFileSize is the maximum allowed file size (10MB)
	MaxFileSize = 10 * 1024 * 1024
	// PreviewRows is the number of rows to show in preview
	PreviewRows = 5
	// TempFileExpiry is how long temp files are kept before cleanup
	TempFileExpiry = 1 * time.Hour
)

// FileUpload represents an uploaded file with extracted metadata
type FileUpload struct {
	Filename    string     `json:"filename"`
	Size        int64      `json:"size"`
	ContentType string     `json:"content_type"`
	Columns     []string   `json:"columns"`
	Preview     [][]string `json:"preview"`
	TempPath    string     `json:"temp_path"`
	TotalRows   int        `json:"total_rows"`
}

// UploadService handles file upload and processing
type UploadService struct {
	tempDir     string
	tempFiles   map[string]time.Time
	tempFilesMu sync.RWMutex
}

// NewUploadService creates a new upload service
func NewUploadService(tempDir string) *UploadService {
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	
	service := &UploadService{
		tempDir:   tempDir,
		tempFiles: make(map[string]time.Time),
	}
	
	// Start cleanup goroutine
	go service.cleanupLoop()
	
	return service
}

// ProcessFile processes an uploaded file and extracts columns and preview
func (s *UploadService) ProcessFile(filename string, reader io.Reader, size int64) (*FileUpload, error) {
	// Validate file size
	if size > MaxFileSize {
		return nil, ErrFileTooLarge
	}
	
	if size == 0 {
		return nil, ErrEmptyFile
	}
	
	// Determine file type
	ext := strings.ToLower(filepath.Ext(filename))
	contentType := s.getContentType(ext)
	
	if contentType == "" {
		return nil, ErrUnsupportedType
	}
	
	// Save to temp file
	tempPath, err := s.saveTempFile(filename, reader)
	if err != nil {
		return nil, fmt.Errorf("erro ao salvar arquivo temporário: %w", err)
	}
	
	// Process based on file type
	var columns []string
	var preview [][]string
	var totalRows int
	
	switch ext {
	case ".csv":
		columns, preview, totalRows, err = s.processCSV(tempPath)
	case ".xlsx":
		columns, preview, totalRows, err = s.processXLSX(tempPath)
	default:
		os.Remove(tempPath)
		return nil, ErrUnsupportedType
	}
	
	if err != nil {
		os.Remove(tempPath)
		return nil, err
	}
	
	if len(columns) == 0 {
		os.Remove(tempPath)
		return nil, ErrNoColumns
	}
	
	// Track temp file for cleanup
	s.trackTempFile(tempPath)
	
	return &FileUpload{
		Filename:    filename,
		Size:        size,
		ContentType: contentType,
		Columns:     columns,
		Preview:     preview,
		TempPath:    tempPath,
		TotalRows:   totalRows,
	}, nil
}


// processCSV processes a CSV file and extracts columns and preview
func (s *UploadService) processCSV(filePath string) ([]string, [][]string, int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("erro ao abrir arquivo: %w", err)
	}
	defer file.Close()
	
	reader := csv.NewReader(file)
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	
	// Read header (first row)
	header, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return nil, nil, 0, ErrEmptyFile
		}
		return nil, nil, 0, fmt.Errorf("erro ao ler cabeçalho: %w", err)
	}
	
	// Clean column names
	columns := make([]string, len(header))
	for i, col := range header {
		columns[i] = strings.TrimSpace(col)
	}
	
	// Read preview rows
	preview := make([][]string, 0, PreviewRows)
	totalRows := 0
	
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Skip malformed rows but continue
			continue
		}
		
		totalRows++
		
		if len(preview) < PreviewRows {
			// Normalize row to match column count
			normalizedRow := normalizeRow(row, len(columns))
			preview = append(preview, normalizedRow)
		}
	}
	
	return columns, preview, totalRows, nil
}

// processXLSX processes an XLSX file and extracts columns and preview
func (s *UploadService) processXLSX(filePath string) ([]string, [][]string, int, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("erro ao abrir arquivo Excel: %w", err)
	}
	defer f.Close()
	
	// Get first sheet
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, nil, 0, ErrEmptyFile
	}
	
	sheetName := sheets[0]
	
	// Get all rows
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("erro ao ler linhas: %w", err)
	}
	
	if len(rows) == 0 {
		return nil, nil, 0, ErrEmptyFile
	}
	
	// First row is header
	header := rows[0]
	columns := make([]string, len(header))
	for i, col := range header {
		columns[i] = strings.TrimSpace(col)
	}
	
	// Get preview rows (skip header)
	preview := make([][]string, 0, PreviewRows)
	totalRows := len(rows) - 1 // Exclude header
	
	for i := 1; i < len(rows) && len(preview) < PreviewRows; i++ {
		normalizedRow := normalizeRow(rows[i], len(columns))
		preview = append(preview, normalizedRow)
	}
	
	return columns, preview, totalRows, nil
}

// normalizeRow ensures a row has the correct number of columns
func normalizeRow(row []string, columnCount int) []string {
	normalized := make([]string, columnCount)
	for i := 0; i < columnCount; i++ {
		if i < len(row) {
			normalized[i] = strings.TrimSpace(row[i])
		} else {
			normalized[i] = ""
		}
	}
	return normalized
}

// saveTempFile saves the uploaded file to a temporary location
func (s *UploadService) saveTempFile(filename string, reader io.Reader) (string, error) {
	ext := filepath.Ext(filename)
	
	// Create temp file with original extension
	tempFile, err := os.CreateTemp(s.tempDir, "upload_*"+ext)
	if err != nil {
		return "", err
	}
	defer tempFile.Close()
	
	// Copy content with size limit
	limitedReader := io.LimitReader(reader, MaxFileSize+1)
	written, err := io.Copy(tempFile, limitedReader)
	if err != nil {
		os.Remove(tempFile.Name())
		return "", err
	}
	
	if written > MaxFileSize {
		os.Remove(tempFile.Name())
		return "", ErrFileTooLarge
	}
	
	return tempFile.Name(), nil
}

// getContentType returns the content type for a file extension
func (s *UploadService) getContentType(ext string) string {
	switch ext {
	case ".csv":
		return "text/csv"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	default:
		return ""
	}
}

// trackTempFile adds a temp file to the tracking map
func (s *UploadService) trackTempFile(path string) {
	s.tempFilesMu.Lock()
	defer s.tempFilesMu.Unlock()
	s.tempFiles[path] = time.Now()
}

// RemoveTempFile removes a temp file from tracking and deletes it
func (s *UploadService) RemoveTempFile(path string) error {
	s.tempFilesMu.Lock()
	delete(s.tempFiles, path)
	s.tempFilesMu.Unlock()
	
	return os.Remove(path)
}

// cleanupLoop periodically cleans up expired temp files
func (s *UploadService) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		s.cleanupExpiredFiles()
	}
}

// cleanupExpiredFiles removes temp files older than TempFileExpiry
func (s *UploadService) cleanupExpiredFiles() {
	s.tempFilesMu.Lock()
	defer s.tempFilesMu.Unlock()
	
	now := time.Now()
	for path, created := range s.tempFiles {
		if now.Sub(created) > TempFileExpiry {
			os.Remove(path)
			delete(s.tempFiles, path)
		}
	}
}

// GetFileData reads all data from a processed file
func (s *UploadService) GetFileData(tempPath string) ([]string, [][]string, error) {
	ext := strings.ToLower(filepath.Ext(tempPath))
	
	switch ext {
	case ".csv":
		return s.readAllCSV(tempPath)
	case ".xlsx":
		return s.readAllXLSX(tempPath)
	default:
		return nil, nil, ErrUnsupportedType
	}
}

// readAllCSV reads all data from a CSV file
func (s *UploadService) readAllCSV(filePath string) ([]string, [][]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("erro ao abrir arquivo: %w", err)
	}
	defer file.Close()
	
	reader := csv.NewReader(file)
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	
	// Read all records
	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("erro ao ler arquivo: %w", err)
	}
	
	if len(records) == 0 {
		return nil, nil, ErrEmptyFile
	}
	
	// First row is header
	columns := make([]string, len(records[0]))
	for i, col := range records[0] {
		columns[i] = strings.TrimSpace(col)
	}
	
	// Rest is data
	data := make([][]string, 0, len(records)-1)
	for i := 1; i < len(records); i++ {
		normalizedRow := normalizeRow(records[i], len(columns))
		data = append(data, normalizedRow)
	}
	
	return columns, data, nil
}

// readAllXLSX reads all data from an XLSX file
func (s *UploadService) readAllXLSX(filePath string) ([]string, [][]string, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("erro ao abrir arquivo Excel: %w", err)
	}
	defer f.Close()
	
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, nil, ErrEmptyFile
	}
	
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, nil, fmt.Errorf("erro ao ler linhas: %w", err)
	}
	
	if len(rows) == 0 {
		return nil, nil, ErrEmptyFile
	}
	
	// First row is header
	columns := make([]string, len(rows[0]))
	for i, col := range rows[0] {
		columns[i] = strings.TrimSpace(col)
	}
	
	// Rest is data
	data := make([][]string, 0, len(rows)-1)
	for i := 1; i < len(rows); i++ {
		normalizedRow := normalizeRow(rows[i], len(columns))
		data = append(data, normalizedRow)
	}
	
	return columns, data, nil
}

// ValidateFileFormat validates that a file has the correct format
func (s *UploadService) ValidateFileFormat(filename string) error {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".csv" && ext != ".xlsx" {
		return ErrUnsupportedType
	}
	return nil
}
