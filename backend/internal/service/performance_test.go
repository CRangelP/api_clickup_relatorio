package service

import (
	"bytes"
	"encoding/csv"
	"os"
	"runtime"
	"testing"
)

// TestMemoryUsageDuringLargeFileProcessing tests that memory usage stays within acceptable limits
// when processing large files (Requirements 15.4, 15.5)
func TestMemoryUsageDuringLargeFileProcessing(t *testing.T) {
	// Create temp directory for tests
	tempDir, err := os.MkdirTemp("", "perf_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	uploadService := NewUploadService(tempDir)

	// Force GC before measuring baseline
	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	// Create a large CSV file with 1000+ rows (as per requirement 15.4)
	numRows := 1500
	numCols := 10
	csvContent := createLargeCSVContent(numCols, numRows)

	// Process the file
	reader := bytes.NewReader(csvContent)
	result, err := uploadService.ProcessFile("large_test.csv", reader, int64(len(csvContent)))
	if err != nil {
		t.Fatalf("Failed to process large file: %v", err)
	}
	defer uploadService.RemoveTempFile(result.TempPath)

	// Force GC and measure memory after processing
	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// Calculate memory used (in MB) - use TotalAlloc for cumulative allocations
	// or HeapAlloc for current heap usage
	memUsedMB := float64(memAfter.HeapAlloc) / (1024 * 1024)

	// Requirement 15.4: Memory usage should stay below 512MB for 1000+ records
	// We use a more conservative limit for this unit test (100MB)
	maxMemoryMB := 100.0
	if memUsedMB > maxMemoryMB {
		t.Errorf("Memory usage exceeded limit: %.2f MB (limit: %.2f MB)", memUsedMB, maxMemoryMB)
	}

	// Verify file was processed correctly
	if result.TotalRows != numRows {
		t.Errorf("Expected %d rows, got %d", numRows, result.TotalRows)
	}

	if len(result.Columns) != numCols {
		t.Errorf("Expected %d columns, got %d", numCols, len(result.Columns))
	}

	// Preview should have at most PreviewRows
	if len(result.Preview) > PreviewRows {
		t.Errorf("Preview should have at most %d rows, got %d", PreviewRows, len(result.Preview))
	}

	t.Logf("Processed %d rows with %d columns, heap memory: %.2f MB", numRows, numCols, memUsedMB)
}

// TestMemoryUsageWithMultipleFiles tests memory behavior when processing multiple files
func TestMemoryUsageWithMultipleFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "perf_test_multi_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	uploadService := NewUploadService(tempDir)

	// Process multiple files sequentially
	numFiles := 5
	rowsPerFile := 500
	numCols := 8
	tempPaths := make([]string, 0, numFiles)

	for i := 0; i < numFiles; i++ {
		csvContent := createLargeCSVContent(numCols, rowsPerFile)
		reader := bytes.NewReader(csvContent)
		result, err := uploadService.ProcessFile("test_file.csv", reader, int64(len(csvContent)))
		if err != nil {
			t.Fatalf("Failed to process file %d: %v", i, err)
		}
		tempPaths = append(tempPaths, result.TempPath)
	}

	// Cleanup temp files
	for _, path := range tempPaths {
		uploadService.RemoveTempFile(path)
	}

	// Force GC and measure memory after cleanup
	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	memUsedMB := float64(memAfter.HeapAlloc) / (1024 * 1024)

	// Memory should not grow linearly with number of files processed
	// After cleanup, memory should be relatively low
	maxMemoryMB := 100.0
	if memUsedMB > maxMemoryMB {
		t.Errorf("Memory usage after cleanup exceeded limit: %.2f MB (limit: %.2f MB)", memUsedMB, maxMemoryMB)
	}

	t.Logf("Processed %d files with %d rows each, heap memory after cleanup: %.2f MB", numFiles, rowsPerFile, memUsedMB)
}

// createLargeCSVContent creates CSV content with specified dimensions
func createLargeCSVContent(numCols, numRows int) []byte {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write header
	header := make([]string, numCols)
	for i := 0; i < numCols; i++ {
		header[i] = "Column" + string(rune('A'+i))
	}
	writer.Write(header)

	// Write data rows
	for r := 0; r < numRows; r++ {
		row := make([]string, numCols)
		for c := 0; c < numCols; c++ {
			row[c] = "DataValue"
		}
		writer.Write(row)
	}

	writer.Flush()
	return buf.Bytes()
}
