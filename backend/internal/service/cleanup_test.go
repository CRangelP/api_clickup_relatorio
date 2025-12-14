package service

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: clickup-field-updater, Property 11: Resource cleanup consistency**
// For any completed or failed job, the system should clean up temporary resources
// (files, queue entries) according to defined retention policies
// **Validates: Requirements 6.3, 6.4, 15.1, 15.2, 15.3**

// CleanupTestJob represents a job for cleanup testing
type CleanupTestJob struct {
	ID          int
	UserID      string
	Title       string
	Status      string // pending, processing, completed, failed
	FilePath    string
	CreatedAt   time.Time
	CompletedAt *time.Time
}

// CleanupTestFile represents a temporary file for cleanup testing
type CleanupTestFile struct {
	Path      string
	CreatedAt time.Time
}

// ResourceCleanupQueue simulates the queue cleanup behavior
type ResourceCleanupQueue struct {
	jobs      []CleanupTestJob
	nextID    int
	tempFiles map[string]time.Time
	
	// Cleanup policies
	failedJobRetention time.Duration
	tempFileExpiry     time.Duration
}

// NewResourceCleanupQueue creates a new cleanup test queue
func NewResourceCleanupQueue() *ResourceCleanupQueue {
	return &ResourceCleanupQueue{
		jobs:               []CleanupTestJob{},
		nextID:             1,
		tempFiles:          make(map[string]time.Time),
		failedJobRetention: 24 * time.Hour,
		tempFileExpiry:     1 * time.Hour,
	}
}

// AddJob adds a job to the queue
func (q *ResourceCleanupQueue) AddJob(userID, title, filePath string) CleanupTestJob {
	job := CleanupTestJob{
		ID:        q.nextID,
		UserID:    userID,
		Title:     title,
		Status:    "pending",
		FilePath:  filePath,
		CreatedAt: time.Now(),
	}
	q.nextID++
	q.jobs = append(q.jobs, job)
	return job
}

// CompleteJob marks a job as completed
func (q *ResourceCleanupQueue) CompleteJob(jobID int) bool {
	for i := range q.jobs {
		if q.jobs[i].ID == jobID {
			now := time.Now()
			q.jobs[i].Status = "completed"
			q.jobs[i].CompletedAt = &now
			return true
		}
	}
	return false
}

// FailJob marks a job as failed
func (q *ResourceCleanupQueue) FailJob(jobID int) bool {
	for i := range q.jobs {
		if q.jobs[i].ID == jobID {
			now := time.Now()
			q.jobs[i].Status = "failed"
			q.jobs[i].CompletedAt = &now
			return true
		}
	}
	return false
}

// FailJobWithTime marks a job as failed with a specific time (for testing old jobs)
func (q *ResourceCleanupQueue) FailJobWithTime(jobID int, failedAt time.Time) bool {
	for i := range q.jobs {
		if q.jobs[i].ID == jobID {
			q.jobs[i].Status = "failed"
			q.jobs[i].CompletedAt = &failedAt
			return true
		}
	}
	return false
}

// TrackTempFile adds a temp file to tracking
func (q *ResourceCleanupQueue) TrackTempFile(path string, createdAt time.Time) {
	q.tempFiles[path] = createdAt
}

// CleanupCompletedJobs removes all completed jobs (simulates DeleteCompletedJobs)
func (q *ResourceCleanupQueue) CleanupCompletedJobs() int {
	newJobs := []CleanupTestJob{}
	removedCount := 0
	
	for _, job := range q.jobs {
		if job.Status != "completed" {
			newJobs = append(newJobs, job)
		} else {
			removedCount++
		}
	}
	
	q.jobs = newJobs
	return removedCount
}

// CleanupOldFailedJobs removes failed jobs older than retention period
func (q *ResourceCleanupQueue) CleanupOldFailedJobs(currentTime time.Time) int {
	newJobs := []CleanupTestJob{}
	removedCount := 0
	
	for _, job := range q.jobs {
		if job.Status == "failed" && job.CompletedAt != nil {
			if currentTime.Sub(*job.CompletedAt) > q.failedJobRetention {
				removedCount++
				continue
			}
		}
		newJobs = append(newJobs, job)
	}
	
	q.jobs = newJobs
	return removedCount
}

// CleanupExpiredTempFiles removes temp files older than expiry
func (q *ResourceCleanupQueue) CleanupExpiredTempFiles(currentTime time.Time) []string {
	removed := []string{}
	
	for path, createdAt := range q.tempFiles {
		if currentTime.Sub(createdAt) > q.tempFileExpiry {
			removed = append(removed, path)
			delete(q.tempFiles, path)
		}
	}
	
	return removed
}

// RemoveTempFile explicitly removes a temp file
func (q *ResourceCleanupQueue) RemoveTempFile(path string) bool {
	if _, exists := q.tempFiles[path]; exists {
		delete(q.tempFiles, path)
		return true
	}
	return false
}

// GetJobsByStatus returns jobs with a specific status
func (q *ResourceCleanupQueue) GetJobsByStatus(status string) []CleanupTestJob {
	result := []CleanupTestJob{}
	for _, job := range q.jobs {
		if job.Status == status {
			result = append(result, job)
		}
	}
	return result
}

// GetAllJobs returns all jobs
func (q *ResourceCleanupQueue) GetAllJobs() []CleanupTestJob {
	return q.jobs
}

// GetTrackedTempFiles returns all tracked temp files
func (q *ResourceCleanupQueue) GetTrackedTempFiles() map[string]time.Time {
	return q.tempFiles
}

// CleanupTestData holds test data for property tests
type CleanupTestData struct {
	JobTitles   []string
	JobStatuses []string // "completed" or "failed"
	TempFiles   []string
}

// TestResourceCleanupConsistency tests Property 11: Resource cleanup consistency
func TestResourceCleanupConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.MaxSize = 15

	properties := gopter.NewProperties(parameters)

	// Property 11.1: Completed jobs are removed after cleanup
	// For any completed job, running cleanup should remove it from the queue
	// **Validates: Requirements 6.3, 15.1**
	properties.Property("completed jobs are removed after cleanup", prop.ForAll(
		func(testData CleanupTestData) bool {
			if len(testData.JobTitles) == 0 {
				return true
			}

			queue := NewResourceCleanupQueue()

			// Add jobs and mark some as completed
			completedCount := 0
			for i, title := range testData.JobTitles {
				job := queue.AddJob("user1", title, "/tmp/file_"+title)
				
				status := "pending"
				if i < len(testData.JobStatuses) {
					status = testData.JobStatuses[i]
				}
				
				if status == "completed" {
					queue.CompleteJob(job.ID)
					completedCount++
				} else if status == "failed" {
					queue.FailJob(job.ID)
				}
			}

			initialTotal := len(queue.GetAllJobs())
			
			// Run cleanup
			removed := queue.CleanupCompletedJobs()

			// Verify completed jobs were removed
			if removed != completedCount {
				t.Logf("Removed count mismatch: expected %d, got %d", completedCount, removed)
				return false
			}

			// Verify no completed jobs remain
			remainingCompleted := queue.GetJobsByStatus("completed")
			if len(remainingCompleted) != 0 {
				t.Logf("Completed jobs still remain: %d", len(remainingCompleted))
				return false
			}

			// Verify total count is correct
			expectedRemaining := initialTotal - completedCount
			if len(queue.GetAllJobs()) != expectedRemaining {
				t.Logf("Total jobs mismatch: expected %d, got %d", expectedRemaining, len(queue.GetAllJobs()))
				return false
			}

			return true
		},
		genCleanupTestData(),
	))

	// Property 11.2: Failed jobs are retained for 24 hours
	// For any failed job less than 24 hours old, cleanup should NOT remove it
	// **Validates: Requirements 6.4, 15.2**
	properties.Property("recent failed jobs are retained during cleanup", prop.ForAll(
		func(testData CleanupTestData) bool {
			if len(testData.JobTitles) == 0 {
				return true
			}

			queue := NewResourceCleanupQueue()

			// Add jobs and mark all as failed (recently)
			failedCount := 0
			for _, title := range testData.JobTitles {
				job := queue.AddJob("user1", title, "/tmp/file_"+title)
				queue.FailJob(job.ID)
				failedCount++
			}

			// Run cleanup with current time (all jobs are recent)
			currentTime := time.Now()
			removed := queue.CleanupOldFailedJobs(currentTime)

			// No jobs should be removed (all are recent)
			if removed != 0 {
				t.Logf("Recent failed jobs were incorrectly removed: %d", removed)
				return false
			}

			// All failed jobs should remain
			remainingFailed := queue.GetJobsByStatus("failed")
			if len(remainingFailed) != failedCount {
				t.Logf("Failed jobs count mismatch: expected %d, got %d", failedCount, len(remainingFailed))
				return false
			}

			return true
		},
		genCleanupTestData(),
	))

	// Property 11.3: Old failed jobs are removed after 24 hours
	// For any failed job older than 24 hours, cleanup should remove it
	// **Validates: Requirements 6.4, 15.2**
	properties.Property("old failed jobs are removed after 24 hours", prop.ForAll(
		func(testData CleanupTestData) bool {
			if len(testData.JobTitles) == 0 {
				return true
			}

			queue := NewResourceCleanupQueue()

			// Add jobs and mark all as failed with old timestamp
			oldTime := time.Now().Add(-25 * time.Hour) // 25 hours ago
			
			for _, title := range testData.JobTitles {
				job := queue.AddJob("user1", title, "/tmp/file_"+title)
				queue.FailJobWithTime(job.ID, oldTime)
			}

			initialCount := len(queue.GetAllJobs())

			// Run cleanup with current time
			currentTime := time.Now()
			removed := queue.CleanupOldFailedJobs(currentTime)

			// All old failed jobs should be removed
			if removed != initialCount {
				t.Logf("Old failed jobs removal mismatch: expected %d, got %d", initialCount, removed)
				return false
			}

			// No jobs should remain
			if len(queue.GetAllJobs()) != 0 {
				t.Logf("Jobs still remain after cleanup: %d", len(queue.GetAllJobs()))
				return false
			}

			return true
		},
		genCleanupTestData(),
	))

	// Property 11.4: Pending and processing jobs are never removed by cleanup
	// For any pending or processing job, cleanup should NOT remove it
	// **Validates: Requirements 6.3, 6.4**
	properties.Property("pending and processing jobs are never removed by cleanup", prop.ForAll(
		func(testData CleanupTestData) bool {
			if len(testData.JobTitles) == 0 {
				return true
			}

			queue := NewResourceCleanupQueue()

			// Add jobs (all remain pending)
			for _, title := range testData.JobTitles {
				queue.AddJob("user1", title, "/tmp/file_"+title)
			}

			initialCount := len(queue.GetAllJobs())

			// Run both cleanup operations
			queue.CleanupCompletedJobs()
			queue.CleanupOldFailedJobs(time.Now().Add(48 * time.Hour)) // Even with future time

			// All pending jobs should remain
			if len(queue.GetAllJobs()) != initialCount {
				t.Logf("Pending jobs were incorrectly removed: expected %d, got %d", initialCount, len(queue.GetAllJobs()))
				return false
			}

			return true
		},
		genCleanupTestData(),
	))

	// Property 11.5: Temp files are removed after expiry
	// For any temp file older than 1 hour, cleanup should remove it
	// **Validates: Requirements 15.3**
	properties.Property("expired temp files are removed after 1 hour", prop.ForAll(
		func(testData CleanupTestData) bool {
			if len(testData.TempFiles) == 0 {
				return true
			}

			queue := NewResourceCleanupQueue()

			// Track temp files with old timestamps
			oldTime := time.Now().Add(-2 * time.Hour) // 2 hours ago
			for _, path := range testData.TempFiles {
				queue.TrackTempFile(path, oldTime)
			}

			initialCount := len(queue.GetTrackedTempFiles())

			// Run cleanup with current time
			removed := queue.CleanupExpiredTempFiles(time.Now())

			// All old temp files should be removed
			if len(removed) != initialCount {
				t.Logf("Temp file removal mismatch: expected %d, got %d", initialCount, len(removed))
				return false
			}

			// No temp files should remain
			if len(queue.GetTrackedTempFiles()) != 0 {
				t.Logf("Temp files still remain: %d", len(queue.GetTrackedTempFiles()))
				return false
			}

			return true
		},
		genCleanupTestData(),
	))

	// Property 11.6: Recent temp files are retained
	// For any temp file less than 1 hour old, cleanup should NOT remove it
	// **Validates: Requirements 15.3**
	properties.Property("recent temp files are retained during cleanup", prop.ForAll(
		func(testData CleanupTestData) bool {
			if len(testData.TempFiles) == 0 {
				return true
			}

			queue := NewResourceCleanupQueue()

			// Track temp files with recent timestamps
			recentTime := time.Now().Add(-30 * time.Minute) // 30 minutes ago
			for _, path := range testData.TempFiles {
				queue.TrackTempFile(path, recentTime)
			}

			initialCount := len(queue.GetTrackedTempFiles())

			// Run cleanup with current time
			removed := queue.CleanupExpiredTempFiles(time.Now())

			// No temp files should be removed
			if len(removed) != 0 {
				t.Logf("Recent temp files were incorrectly removed: %d", len(removed))
				return false
			}

			// All temp files should remain
			if len(queue.GetTrackedTempFiles()) != initialCount {
				t.Logf("Temp files count mismatch: expected %d, got %d", initialCount, len(queue.GetTrackedTempFiles()))
				return false
			}

			return true
		},
		genCleanupTestData(),
	))

	// Property 11.7: Explicit temp file removal works correctly
	// For any temp file that is explicitly removed, it should no longer be tracked
	// **Validates: Requirements 15.3**
	properties.Property("explicit temp file removal works correctly", prop.ForAll(
		func(testData CleanupTestData) bool {
			if len(testData.TempFiles) == 0 {
				return true
			}

			queue := NewResourceCleanupQueue()

			// Deduplicate temp files (same path can only be tracked once)
			uniquePaths := make(map[string]bool)
			for _, path := range testData.TempFiles {
				uniquePaths[path] = true
			}

			// Track unique temp files
			for path := range uniquePaths {
				queue.TrackTempFile(path, time.Now())
			}

			initialCount := len(queue.GetTrackedTempFiles())

			// Remove each unique file explicitly
			removedCount := 0
			for path := range uniquePaths {
				if queue.RemoveTempFile(path) {
					removedCount++
				}
			}

			// All tracked files should be removed
			if removedCount != initialCount {
				t.Logf("Removed count mismatch: expected %d, got %d", initialCount, removedCount)
				return false
			}

			// No temp files should remain
			if len(queue.GetTrackedTempFiles()) != 0 {
				t.Logf("Temp files still remain after explicit removal: %d", len(queue.GetTrackedTempFiles()))
				return false
			}

			return true
		},
		genCleanupTestData(),
	))

	// Property 11.8: Mixed job statuses are handled correctly
	// For any mix of completed, failed, and pending jobs, cleanup should only remove appropriate ones
	// **Validates: Requirements 6.3, 6.4, 15.1, 15.2**
	properties.Property("mixed job statuses are handled correctly during cleanup", prop.ForAll(
		func(testData CleanupTestData) bool {
			if len(testData.JobTitles) == 0 {
				return true
			}

			queue := NewResourceCleanupQueue()

			pendingCount := 0
			completedCount := 0
			recentFailedCount := 0
			oldFailedCount := 0

			oldTime := time.Now().Add(-25 * time.Hour)

			for i, title := range testData.JobTitles {
				job := queue.AddJob("user1", title, "/tmp/file_"+title)
				
				// Cycle through different statuses
				switch i % 4 {
				case 0:
					// Keep pending
					pendingCount++
				case 1:
					queue.CompleteJob(job.ID)
					completedCount++
				case 2:
					queue.FailJob(job.ID) // Recent failure
					recentFailedCount++
				case 3:
					queue.FailJobWithTime(job.ID, oldTime) // Old failure
					oldFailedCount++
				}
			}

			// Run cleanup
			queue.CleanupCompletedJobs()
			queue.CleanupOldFailedJobs(time.Now())

			// Verify correct jobs remain
			expectedRemaining := pendingCount + recentFailedCount
			actualRemaining := len(queue.GetAllJobs())

			if actualRemaining != expectedRemaining {
				t.Logf("Remaining jobs mismatch: expected %d (pending=%d, recentFailed=%d), got %d",
					expectedRemaining, pendingCount, recentFailedCount, actualRemaining)
				return false
			}

			// Verify no completed jobs remain
			if len(queue.GetJobsByStatus("completed")) != 0 {
				t.Logf("Completed jobs still remain")
				return false
			}

			return true
		},
		genCleanupTestData(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Generators for cleanup test data

func genCleanupTestData() gopter.Gen {
	return gen.IntRange(1, 10).FlatMap(func(numJobs interface{}) gopter.Gen {
		jobCount := numJobs.(int)
		return gopter.CombineGens(
			gen.SliceOfN(jobCount, genCleanupJobTitle()),
			gen.SliceOfN(jobCount, genJobStatus()),
			gen.SliceOfN(jobCount, genTempFilePath()),
		).Map(func(values []interface{}) CleanupTestData {
			titles := values[0].([]string)
			statuses := values[1].([]string)
			tempFiles := values[2].([]string)
			return CleanupTestData{
				JobTitles:   titles,
				JobStatuses: statuses,
				TempFiles:   tempFiles,
			}
		})
	}, reflect.TypeOf(CleanupTestData{}))
}

func genCleanupJobTitle() gopter.Gen {
	return gen.Identifier().Map(func(s string) string {
		return "CleanupJob_" + s
	})
}

func genJobStatus() gopter.Gen {
	return gen.OneConstOf("completed", "failed", "pending")
}

func genTempFilePath() gopter.Gen {
	return gen.Identifier().Map(func(s string) string {
		return "/tmp/upload_" + s + ".csv"
	})
}

// TestUploadServiceTempFileCleanup tests the actual UploadService temp file cleanup
func TestUploadServiceTempFileCleanup(t *testing.T) {
	// Create temp directory for tests
	tempDir, err := os.MkdirTemp("", "cleanup_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50
	parameters.MaxSize = 5

	properties := gopter.NewProperties(parameters)

	// Property: Temp files are tracked and can be removed
	properties.Property("temp files can be explicitly removed after processing", prop.ForAll(
		func(testData FileTestData) bool {
			if len(testData.Columns) == 0 {
				return true
			}

			uploadService := NewUploadService(tempDir)

			// Create and process a file
			csvContent := createCSVContent(testData.Columns, testData.Rows)
			reader := strings.NewReader(csvContent)

			result, err := uploadService.ProcessFile("test.csv", reader, int64(len(csvContent)))
			if err != nil {
				t.Logf("Error processing file: %v", err)
				return false
			}

			// Verify temp file exists
			if _, err := os.Stat(result.TempPath); os.IsNotExist(err) {
				t.Logf("Temp file does not exist: %s", result.TempPath)
				return false
			}

			// Remove temp file
			err = uploadService.RemoveTempFile(result.TempPath)
			if err != nil {
				t.Logf("Error removing temp file: %v", err)
				return false
			}

			// Verify temp file is removed
			if _, err := os.Stat(result.TempPath); !os.IsNotExist(err) {
				t.Logf("Temp file still exists after removal: %s", result.TempPath)
				return false
			}

			return true
		},
		genFileTestData(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Helper to create unique temp file paths
func createTempFilePath(tempDir, prefix string) string {
	return filepath.Join(tempDir, prefix+"_"+time.Now().Format("20060102150405")+".tmp")
}
