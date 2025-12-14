package service

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/xuri/excelize/v2"
)

// **Feature: clickup-field-updater, Property 4: File processing consistency**
// For any valid uploaded file (CSV/XLSX), the system should extract all columns,
// display preview, and maintain file integrity throughout the mapping process
// **Validates: Requirements 3.2, 3.4, 4.1**

func TestFileProcessingConsistency(t *testing.T) {
	// Create temp directory for tests
	tempDir, err := os.MkdirTemp("", "upload_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	uploadService := NewUploadService(tempDir)

	// Configure properties with reasonable test parameters
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.MaxSize = 10
	properties := gopter.NewProperties(parameters)

	// Property 4: File processing consistency
	// For any valid file data, processing should:
	// 1. Extract all columns correctly
	// 2. Generate preview with first 5 rows (or less if fewer rows)
	// 3. Maintain data integrity (columns match, preview matches original data)
	properties.Property("CSV file processing extracts all columns and generates correct preview", prop.ForAll(
		func(testData FileTestData) bool {
			// Skip invalid test data
			if len(testData.Columns) == 0 {
				return true
			}

			// Create CSV file
			csvContent := createCSVContent(testData.Columns, testData.Rows)
			reader := strings.NewReader(csvContent)

			// Process file
			result, err := uploadService.ProcessFile("test.csv", reader, int64(len(csvContent)))
			if err != nil {
				t.Logf("Error processing CSV: %v", err)
				return false
			}
			defer uploadService.RemoveTempFile(result.TempPath)

			// Verify columns match
			if len(result.Columns) != len(testData.Columns) {
				t.Logf("Column count mismatch: expected %d, got %d", len(testData.Columns), len(result.Columns))
				return false
			}

			for i, col := range testData.Columns {
				if result.Columns[i] != col {
					t.Logf("Column mismatch at %d: expected %q, got %q", i, col, result.Columns[i])
					return false
				}
			}

			// Verify preview row count
			expectedPreviewRows := len(testData.Rows)
			if expectedPreviewRows > PreviewRows {
				expectedPreviewRows = PreviewRows
			}

			if len(result.Preview) != expectedPreviewRows {
				t.Logf("Preview row count mismatch: expected %d, got %d", expectedPreviewRows, len(result.Preview))
				return false
			}

			// Verify preview data matches original
			for i, row := range result.Preview {
				if i >= len(testData.Rows) {
					break
				}
				for j, cell := range row {
					if j < len(testData.Rows[i]) {
						expected := testData.Rows[i][j]
						if cell != expected {
							t.Logf("Preview data mismatch at [%d][%d]: expected %q, got %q", i, j, expected, cell)
							return false
						}
					}
				}
			}

			// Verify total rows count
			if result.TotalRows != len(testData.Rows) {
				t.Logf("Total rows mismatch: expected %d, got %d", len(testData.Rows), result.TotalRows)
				return false
			}

			return true
		},
		genFileTestData(),
	))

	properties.Property("XLSX file processing extracts all columns and generates correct preview", prop.ForAll(
		func(testData FileTestData) bool {
			// Skip invalid test data
			if len(testData.Columns) == 0 {
				return true
			}

			// Create XLSX file
			xlsxPath, err := createXLSXFile(tempDir, testData.Columns, testData.Rows)
			if err != nil {
				t.Logf("Error creating XLSX: %v", err)
				return false
			}
			defer os.Remove(xlsxPath)

			// Open file for processing
			file, err := os.Open(xlsxPath)
			if err != nil {
				t.Logf("Error opening XLSX: %v", err)
				return false
			}

			stat, _ := file.Stat()
			
			// Process file
			result, err := uploadService.ProcessFile("test.xlsx", file, stat.Size())
			file.Close()
			
			if err != nil {
				t.Logf("Error processing XLSX: %v", err)
				return false
			}
			defer uploadService.RemoveTempFile(result.TempPath)

			// Verify columns match
			if len(result.Columns) != len(testData.Columns) {
				t.Logf("Column count mismatch: expected %d, got %d", len(testData.Columns), len(result.Columns))
				return false
			}

			for i, col := range testData.Columns {
				if result.Columns[i] != col {
					t.Logf("Column mismatch at %d: expected %q, got %q", i, col, result.Columns[i])
					return false
				}
			}

			// Verify preview row count
			expectedPreviewRows := len(testData.Rows)
			if expectedPreviewRows > PreviewRows {
				expectedPreviewRows = PreviewRows
			}

			if len(result.Preview) != expectedPreviewRows {
				t.Logf("Preview row count mismatch: expected %d, got %d", expectedPreviewRows, len(result.Preview))
				return false
			}

			// Verify total rows count
			if result.TotalRows != len(testData.Rows) {
				t.Logf("Total rows mismatch: expected %d, got %d", len(testData.Rows), result.TotalRows)
				return false
			}

			return true
		},
		genFileTestData(),
	))

	// Property: File data can be fully retrieved after processing
	properties.Property("processed file data can be fully retrieved", prop.ForAll(
		func(testData FileTestData) bool {
			// Skip invalid test data
			if len(testData.Columns) == 0 {
				return true
			}

			// Create CSV file
			csvContent := createCSVContent(testData.Columns, testData.Rows)
			reader := strings.NewReader(csvContent)

			// Process file
			result, err := uploadService.ProcessFile("test.csv", reader, int64(len(csvContent)))
			if err != nil {
				t.Logf("Error processing CSV: %v", err)
				return false
			}
			defer uploadService.RemoveTempFile(result.TempPath)

			// Get all file data
			columns, data, err := uploadService.GetFileData(result.TempPath)
			if err != nil {
				t.Logf("Error getting file data: %v", err)
				return false
			}

			// Verify all columns are retrieved
			if len(columns) != len(testData.Columns) {
				t.Logf("Retrieved column count mismatch: expected %d, got %d", len(testData.Columns), len(columns))
				return false
			}

			// Verify all rows are retrieved
			if len(data) != len(testData.Rows) {
				t.Logf("Retrieved row count mismatch: expected %d, got %d", len(testData.Rows), len(data))
				return false
			}

			// Verify data integrity
			for i, row := range data {
				if i >= len(testData.Rows) {
					break
				}
				for j, cell := range row {
					if j < len(testData.Rows[i]) {
						expected := testData.Rows[i][j]
						if cell != expected {
							t.Logf("Data mismatch at [%d][%d]: expected %q, got %q", i, j, expected, cell)
							return false
						}
					}
				}
			}

			return true
		},
		genFileTestData(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}


// Test data structures
type FileTestData struct {
	Columns []string
	Rows    [][]string
}

// Generators for property-based testing
func genFileTestData() gopter.Gen {
	return gen.IntRange(1, 8).FlatMap(func(numCols interface{}) gopter.Gen {
		colCount := numCols.(int)
		return gen.IntRange(0, 15).FlatMap(func(numRows interface{}) gopter.Gen {
			rowCount := numRows.(int)
			return gopter.CombineGens(
				genColumns(colCount),
				genRows(colCount, rowCount),
			).Map(func(values []interface{}) FileTestData {
				return FileTestData{
					Columns: values[0].([]string),
					Rows:    values[1].([][]string),
				}
			})
		}, reflect.TypeOf(FileTestData{}))
	}, reflect.TypeOf(FileTestData{}))
}

func genColumns(count int) gopter.Gen {
	// Generate unique column names
	return gen.SliceOfN(count, genColumnName()).SuchThat(func(cols []string) bool {
		// Ensure unique column names
		seen := make(map[string]bool)
		for _, col := range cols {
			if seen[col] || col == "" {
				return false
			}
			seen[col] = true
		}
		return len(cols) == count
	})
}

func genColumnName() gopter.Gen {
	// Generate simple alphanumeric column names without special characters
	return gen.RegexMatch(`^[A-Za-z][A-Za-z0-9]{0,9}$`).SuchThat(func(s string) bool {
		return len(s) > 0 && len(s) <= 10
	})
}

func genRows(columnCount, rowCount int) gopter.Gen {
	if columnCount == 0 || rowCount == 0 {
		return gen.Const([][]string{})
	}

	return gen.SliceOfN(rowCount, genRow(columnCount))
}

func genRow(columnCount int) gopter.Gen {
	return gen.SliceOfN(columnCount, genCellValue())
}

func genCellValue() gopter.Gen {
	// Generate simple cell values - at least one character to avoid empty rows
	return gen.RegexMatch(`^[A-Za-z0-9]{1,20}$`)
}

// Helper functions
func createCSVContent(columns []string, rows [][]string) string {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write header
	writer.Write(columns)

	// Write rows
	for _, row := range rows {
		// Ensure row has correct number of columns
		normalizedRow := make([]string, len(columns))
		for i := 0; i < len(columns); i++ {
			if i < len(row) {
				normalizedRow[i] = row[i]
			} else {
				normalizedRow[i] = ""
			}
		}
		writer.Write(normalizedRow)
	}

	writer.Flush()
	return buf.String()
}

func createXLSXFile(tempDir string, columns []string, rows [][]string) (string, error) {
	f := excelize.NewFile()
	defer f.Close()

	sheetName := "Sheet1"

	// Write header
	for i, col := range columns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, col)
	}

	// Write rows
	for rowIdx, row := range rows {
		for colIdx := 0; colIdx < len(columns); colIdx++ {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			if colIdx < len(row) {
				f.SetCellValue(sheetName, cell, row[colIdx])
			} else {
				f.SetCellValue(sheetName, cell, "")
			}
		}
	}

	// Save to temp file
	tempFile := filepath.Join(tempDir, "test_xlsx_*.xlsx")
	file, err := os.CreateTemp(tempDir, "test_xlsx_*.xlsx")
	if err != nil {
		return "", err
	}
	tempFile = file.Name()
	file.Close()

	if err := f.SaveAs(tempFile); err != nil {
		return "", err
	}

	return tempFile, nil
}

// Unit tests for edge cases
func TestUploadService_FileSizeValidation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "upload_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	uploadService := NewUploadService(tempDir)

	// Test file too large
	largeContent := strings.Repeat("a,b,c\n", MaxFileSize/6+1)
	reader := strings.NewReader(largeContent)

	_, err = uploadService.ProcessFile("large.csv", reader, int64(len(largeContent)))
	if err != ErrFileTooLarge {
		t.Errorf("Expected ErrFileTooLarge, got %v", err)
	}
}

func TestUploadService_EmptyFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "upload_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	uploadService := NewUploadService(tempDir)

	// Test empty file
	reader := strings.NewReader("")

	_, err = uploadService.ProcessFile("empty.csv", reader, 0)
	if err != ErrEmptyFile {
		t.Errorf("Expected ErrEmptyFile, got %v", err)
	}
}

func TestUploadService_UnsupportedFormat(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "upload_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	uploadService := NewUploadService(tempDir)

	// Test unsupported format
	reader := strings.NewReader("some content")

	_, err = uploadService.ProcessFile("test.txt", reader, 12)
	if err != ErrUnsupportedType {
		t.Errorf("Expected ErrUnsupportedType, got %v", err)
	}
}

func TestUploadService_ValidateFileFormat(t *testing.T) {
	uploadService := NewUploadService("")

	tests := []struct {
		filename string
		wantErr  bool
	}{
		{"test.csv", false},
		{"test.CSV", false},
		{"test.xlsx", false},
		{"test.XLSX", false},
		{"test.txt", true},
		{"test.pdf", true},
		{"test", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			err := uploadService.ValidateFileFormat(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFileFormat(%q) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
			}
		})
	}
}


