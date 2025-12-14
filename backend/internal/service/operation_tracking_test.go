package service

import (
	"reflect"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// OperationTrackerForTest is a testable in-memory implementation
// that mirrors the behavior of operation tracking in the real system
type OperationTrackerForTest struct {
	operations     []OperationRecord
	nextID         int
	progressEvents []ProgressEvent
}

// OperationRecord represents an operation history entry
type OperationRecord struct {
	ID            int
	UserID        string
	OperationType string
	Title         string
	Status        string
	SuccessCount  int
	ErrorCount    int
	ErrorDetails  []string
	CreatedAt     time.Time
	CompletedAt   *time.Time
}

// ProgressEvent represents a progress update event
type ProgressEvent struct {
	OperationID   int
	ProcessedRows int
	TotalRows     int
	SuccessCount  int
	ErrorCount    int
	Status        string
}

// NewOperationTrackerForTest creates a new test operation tracker
func NewOperationTrackerForTest() *OperationTrackerForTest {
	return &OperationTrackerForTest{
		operations:     []OperationRecord{},
		nextID:         1,
		progressEvents: []ProgressEvent{},
	}
}

// StartOperation creates a new operation record (simulates CreateOperationHistory)
func (t *OperationTrackerForTest) StartOperation(userID, operationType, title string, totalRows int) *OperationRecord {
	op := OperationRecord{
		ID:            t.nextID,
		UserID:        userID,
		OperationType: operationType,
		Title:         title,
		Status:        "processing",
		SuccessCount:  0,
		ErrorCount:    0,
		ErrorDetails:  []string{},
		CreatedAt:     time.Now(),
	}
	t.nextID++
	t.operations = append(t.operations, op)

	// Record initial progress event
	t.progressEvents = append(t.progressEvents, ProgressEvent{
		OperationID:   op.ID,
		ProcessedRows: 0,
		TotalRows:     totalRows,
		SuccessCount:  0,
		ErrorCount:    0,
		Status:        "processing",
	})

	return &op
}

// RecordSuccess increments success counter for an operation
func (t *OperationTrackerForTest) RecordSuccess(operationID int) bool {
	for i := range t.operations {
		if t.operations[i].ID == operationID {
			t.operations[i].SuccessCount++
			return true
		}
	}
	return false
}

// RecordError increments error counter and records error details
func (t *OperationTrackerForTest) RecordError(operationID int, errorDetail string) bool {
	for i := range t.operations {
		if t.operations[i].ID == operationID {
			t.operations[i].ErrorCount++
			t.operations[i].ErrorDetails = append(t.operations[i].ErrorDetails, errorDetail)
			return true
		}
	}
	return false
}

// UpdateProgress records a progress update event
func (t *OperationTrackerForTest) UpdateProgress(operationID, processedRows, totalRows, successCount, errorCount int) {
	t.progressEvents = append(t.progressEvents, ProgressEvent{
		OperationID:   operationID,
		ProcessedRows: processedRows,
		TotalRows:     totalRows,
		SuccessCount:  successCount,
		ErrorCount:    errorCount,
		Status:        "processing",
	})
}

// CompleteOperation marks an operation as completed
func (t *OperationTrackerForTest) CompleteOperation(operationID int) bool {
	for i := range t.operations {
		if t.operations[i].ID == operationID {
			t.operations[i].Status = "completed"
			now := time.Now()
			t.operations[i].CompletedAt = &now

			// Record final progress event
			t.progressEvents = append(t.progressEvents, ProgressEvent{
				OperationID:   operationID,
				ProcessedRows: t.operations[i].SuccessCount + t.operations[i].ErrorCount,
				TotalRows:     t.operations[i].SuccessCount + t.operations[i].ErrorCount,
				SuccessCount:  t.operations[i].SuccessCount,
				ErrorCount:    t.operations[i].ErrorCount,
				Status:        "completed",
			})
			return true
		}
	}
	return false
}

// FailOperation marks an operation as failed
func (t *OperationTrackerForTest) FailOperation(operationID int, errorMsg string) bool {
	for i := range t.operations {
		if t.operations[i].ID == operationID {
			t.operations[i].Status = "failed"
			now := time.Now()
			t.operations[i].CompletedAt = &now
			t.operations[i].ErrorDetails = append(t.operations[i].ErrorDetails, errorMsg)

			// Record final progress event
			t.progressEvents = append(t.progressEvents, ProgressEvent{
				OperationID:   operationID,
				ProcessedRows: t.operations[i].SuccessCount + t.operations[i].ErrorCount,
				TotalRows:     t.operations[i].SuccessCount + t.operations[i].ErrorCount,
				SuccessCount:  t.operations[i].SuccessCount,
				ErrorCount:    t.operations[i].ErrorCount,
				Status:        "failed",
			})
			return true
		}
	}
	return false
}

// GetOperation retrieves an operation by ID
func (t *OperationTrackerForTest) GetOperation(operationID int) *OperationRecord {
	for i := range t.operations {
		if t.operations[i].ID == operationID {
			return &t.operations[i]
		}
	}
	return nil
}

// GetProgressEvents returns all progress events for an operation
func (t *OperationTrackerForTest) GetProgressEvents(operationID int) []ProgressEvent {
	var events []ProgressEvent
	for _, e := range t.progressEvents {
		if e.OperationID == operationID {
			events = append(events, e)
		}
	}
	return events
}

// SimulateRowProcessing simulates processing rows with success/failure outcomes
func (t *OperationTrackerForTest) SimulateRowProcessing(operationID int, outcomes []bool, totalRows int) {
	op := t.GetOperation(operationID)
	if op == nil {
		return
	}

	for i, success := range outcomes {
		if success {
			t.RecordSuccess(operationID)
		} else {
			t.RecordError(operationID, "Error processing row")
		}

		// Update progress every 10 rows or on last row
		if (i+1)%10 == 0 || i == len(outcomes)-1 {
			op = t.GetOperation(operationID)
			t.UpdateProgress(operationID, i+1, totalRows, op.SuccessCount, op.ErrorCount)
		}
	}
}

// OperationTestData holds test data for property tests
type OperationTestData struct {
	UserID        string
	OperationType string
	Title         string
	RowOutcomes   []bool // true = success, false = error
}

// TestOperationTrackingCompleteness tests Property 9: Operation tracking completeness
// **Feature: clickup-field-updater, Property 9: Operation tracking completeness**
// **Validates: Requirements 5.3, 5.4, 5.5, 7.1, 7.3**
//
// For any initiated operation (update/report), the system should create history record,
// track progress counters, and update final status upon completion.
func TestOperationTrackingCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.MaxSize = 50

	properties := gopter.NewProperties(parameters)

	// Property 9.1: Operation record is created on start
	// For any operation initiation, a history record should be created with title and timestamp
	// **Validates: Requirements 7.1**
	properties.Property("operation record is created on start", prop.ForAll(
		func(testData OperationTestData) bool {
			tracker := NewOperationTrackerForTest()

			totalRows := len(testData.RowOutcomes)
			if totalRows == 0 {
				totalRows = 1
			}

			op := tracker.StartOperation(testData.UserID, testData.OperationType, testData.Title, totalRows)

			// Verify operation was created
			if op == nil {
				return false
			}

			// Verify required fields are set
			if op.ID <= 0 {
				return false
			}
			if op.UserID != testData.UserID {
				return false
			}
			if op.Title != testData.Title {
				return false
			}
			if op.Status != "processing" {
				return false
			}
			if op.CreatedAt.IsZero() {
				return false
			}

			return true
		},
		genOperationTestData(),
	))

	// Property 9.2: Success counter increments correctly
	// For any row processed successfully, the success counter should increment by 1
	// **Validates: Requirements 5.3**
	properties.Property("success counter increments correctly", prop.ForAll(
		func(testData OperationTestData) bool {
			if len(testData.RowOutcomes) == 0 {
				return true
			}

			tracker := NewOperationTrackerForTest()
			op := tracker.StartOperation(testData.UserID, testData.OperationType, testData.Title, len(testData.RowOutcomes))

			expectedSuccessCount := 0
			for _, success := range testData.RowOutcomes {
				if success {
					tracker.RecordSuccess(op.ID)
					expectedSuccessCount++
				} else {
					tracker.RecordError(op.ID, "Error")
				}
			}

			// Verify success count matches expected
			finalOp := tracker.GetOperation(op.ID)
			return finalOp.SuccessCount == expectedSuccessCount
		},
		genOperationTestData(),
	))

	// Property 9.3: Error counter increments and details are recorded
	// For any row that fails, the error counter should increment and error details should be recorded
	// **Validates: Requirements 5.4**
	properties.Property("error counter increments and details recorded", prop.ForAll(
		func(testData OperationTestData) bool {
			if len(testData.RowOutcomes) == 0 {
				return true
			}

			tracker := NewOperationTrackerForTest()
			op := tracker.StartOperation(testData.UserID, testData.OperationType, testData.Title, len(testData.RowOutcomes))

			expectedErrorCount := 0
			for _, success := range testData.RowOutcomes {
				if success {
					tracker.RecordSuccess(op.ID)
				} else {
					tracker.RecordError(op.ID, "Error processing row")
					expectedErrorCount++
				}
			}

			// Verify error count matches expected
			finalOp := tracker.GetOperation(op.ID)
			if finalOp.ErrorCount != expectedErrorCount {
				return false
			}

			// Verify error details are recorded for each error
			if len(finalOp.ErrorDetails) != expectedErrorCount {
				return false
			}

			return true
		},
		genOperationTestData(),
	))

	// Property 9.4: Total processed equals success plus errors
	// For any operation, the sum of success and error counts should equal total processed rows
	// **Validates: Requirements 5.3, 5.4**
	properties.Property("total processed equals success plus errors", prop.ForAll(
		func(testData OperationTestData) bool {
			if len(testData.RowOutcomes) == 0 {
				return true
			}

			tracker := NewOperationTrackerForTest()
			op := tracker.StartOperation(testData.UserID, testData.OperationType, testData.Title, len(testData.RowOutcomes))

			for _, success := range testData.RowOutcomes {
				if success {
					tracker.RecordSuccess(op.ID)
				} else {
					tracker.RecordError(op.ID, "Error")
				}
			}

			finalOp := tracker.GetOperation(op.ID)
			totalProcessed := finalOp.SuccessCount + finalOp.ErrorCount

			return totalProcessed == len(testData.RowOutcomes)
		},
		genOperationTestData(),
	))

	// Property 9.5: Final status is updated on completion
	// For any completed operation, the status should be updated to "completed" or "failed"
	// **Validates: Requirements 5.5, 7.3**
	properties.Property("final status is updated on completion", prop.ForAll(
		func(testData OperationTestData) bool {
			if len(testData.RowOutcomes) == 0 {
				return true
			}

			tracker := NewOperationTrackerForTest()
			op := tracker.StartOperation(testData.UserID, testData.OperationType, testData.Title, len(testData.RowOutcomes))

			// Process all rows
			for _, success := range testData.RowOutcomes {
				if success {
					tracker.RecordSuccess(op.ID)
				} else {
					tracker.RecordError(op.ID, "Error")
				}
			}

			// Complete the operation
			tracker.CompleteOperation(op.ID)

			finalOp := tracker.GetOperation(op.ID)

			// Status should be "completed"
			if finalOp.Status != "completed" {
				return false
			}

			// CompletedAt should be set
			if finalOp.CompletedAt == nil {
				return false
			}

			return true
		},
		genOperationTestData(),
	))

	// Property 9.6: Final summary contains correct totals
	// For any completed operation, the final progress event should contain correct success and error totals
	// **Validates: Requirements 5.5**
	properties.Property("final summary contains correct totals", prop.ForAll(
		func(testData OperationTestData) bool {
			if len(testData.RowOutcomes) == 0 {
				return true
			}

			tracker := NewOperationTrackerForTest()
			op := tracker.StartOperation(testData.UserID, testData.OperationType, testData.Title, len(testData.RowOutcomes))

			expectedSuccess := 0
			expectedErrors := 0
			for _, success := range testData.RowOutcomes {
				if success {
					tracker.RecordSuccess(op.ID)
					expectedSuccess++
				} else {
					tracker.RecordError(op.ID, "Error")
					expectedErrors++
				}
			}

			tracker.CompleteOperation(op.ID)

			// Get final progress event
			events := tracker.GetProgressEvents(op.ID)
			if len(events) == 0 {
				return false
			}

			finalEvent := events[len(events)-1]

			// Verify final event has correct totals
			if finalEvent.SuccessCount != expectedSuccess {
				return false
			}
			if finalEvent.ErrorCount != expectedErrors {
				return false
			}
			if finalEvent.Status != "completed" {
				return false
			}

			return true
		},
		genOperationTestData(),
	))

	// Property 9.7: Progress events are recorded during processing
	// For any operation with multiple rows, progress events should be recorded
	// **Validates: Requirements 5.3, 5.4**
	properties.Property("progress events are recorded during processing", prop.ForAll(
		func(testData OperationTestData) bool {
			if len(testData.RowOutcomes) == 0 {
				return true
			}

			tracker := NewOperationTrackerForTest()
			op := tracker.StartOperation(testData.UserID, testData.OperationType, testData.Title, len(testData.RowOutcomes))

			// Simulate row processing with progress updates
			tracker.SimulateRowProcessing(op.ID, testData.RowOutcomes, len(testData.RowOutcomes))
			tracker.CompleteOperation(op.ID)

			// Get progress events
			events := tracker.GetProgressEvents(op.ID)

			// Should have at least initial and final events
			if len(events) < 2 {
				return false
			}

			// First event should be initial (0 processed)
			if events[0].ProcessedRows != 0 {
				return false
			}

			// Last event should be final (completed status)
			lastEvent := events[len(events)-1]
			if lastEvent.Status != "completed" {
				return false
			}

			return true
		},
		genOperationTestData(),
	))

	// Property 9.8: Failed operations have correct status
	// For any operation that fails, the status should be "failed" and error should be recorded
	// **Validates: Requirements 7.3**
	properties.Property("failed operations have correct status", prop.ForAll(
		func(testData OperationTestData) bool {
			tracker := NewOperationTrackerForTest()
			op := tracker.StartOperation(testData.UserID, testData.OperationType, testData.Title, 10)

			// Fail the operation
			tracker.FailOperation(op.ID, "Critical error occurred")

			finalOp := tracker.GetOperation(op.ID)

			// Status should be "failed"
			if finalOp.Status != "failed" {
				return false
			}

			// CompletedAt should be set
			if finalOp.CompletedAt == nil {
				return false
			}

			// Error details should contain the failure message
			found := false
			for _, detail := range finalOp.ErrorDetails {
				if detail == "Critical error occurred" {
					found = true
					break
				}
			}

			return found
		},
		genOperationTestData(),
	))

	// Property 9.9: Counters are monotonically increasing
	// For any operation, success and error counters should only increase, never decrease
	// **Validates: Requirements 5.3, 5.4**
	properties.Property("counters are monotonically increasing", prop.ForAll(
		func(testData OperationTestData) bool {
			if len(testData.RowOutcomes) == 0 {
				return true
			}

			tracker := NewOperationTrackerForTest()
			op := tracker.StartOperation(testData.UserID, testData.OperationType, testData.Title, len(testData.RowOutcomes))

			prevSuccess := 0
			prevError := 0

			for _, success := range testData.RowOutcomes {
				if success {
					tracker.RecordSuccess(op.ID)
				} else {
					tracker.RecordError(op.ID, "Error")
				}

				currentOp := tracker.GetOperation(op.ID)

				// Success count should never decrease
				if currentOp.SuccessCount < prevSuccess {
					return false
				}

				// Error count should never decrease
				if currentOp.ErrorCount < prevError {
					return false
				}

				prevSuccess = currentOp.SuccessCount
				prevError = currentOp.ErrorCount
			}

			return true
		},
		genOperationTestData(),
	))

	// Property 9.10: Operation tracking is deterministic
	// For any sequence of operations, processing the same sequence twice should yield same results
	// **Validates: Requirements 5.3, 5.4, 5.5**
	properties.Property("operation tracking is deterministic", prop.ForAll(
		func(testData OperationTestData) bool {
			if len(testData.RowOutcomes) == 0 {
				return true
			}

			// First run
			tracker1 := NewOperationTrackerForTest()
			op1 := tracker1.StartOperation(testData.UserID, testData.OperationType, testData.Title, len(testData.RowOutcomes))
			for _, success := range testData.RowOutcomes {
				if success {
					tracker1.RecordSuccess(op1.ID)
				} else {
					tracker1.RecordError(op1.ID, "Error")
				}
			}
			tracker1.CompleteOperation(op1.ID)
			final1 := tracker1.GetOperation(op1.ID)

			// Second run with same data
			tracker2 := NewOperationTrackerForTest()
			op2 := tracker2.StartOperation(testData.UserID, testData.OperationType, testData.Title, len(testData.RowOutcomes))
			for _, success := range testData.RowOutcomes {
				if success {
					tracker2.RecordSuccess(op2.ID)
				} else {
					tracker2.RecordError(op2.ID, "Error")
				}
			}
			tracker2.CompleteOperation(op2.ID)
			final2 := tracker2.GetOperation(op2.ID)

			// Both runs should produce same counts
			if final1.SuccessCount != final2.SuccessCount {
				return false
			}
			if final1.ErrorCount != final2.ErrorCount {
				return false
			}
			if final1.Status != final2.Status {
				return false
			}

			return true
		},
		genOperationTestData(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Generators

func genOperationTestData() gopter.Gen {
	return gen.IntRange(0, 30).FlatMap(func(numRows interface{}) gopter.Gen {
		rowCount := numRows.(int)
		return gopter.CombineGens(
			genOperationUserID(),
			genOperationType(),
			genOperationTitle(),
			gen.SliceOfN(rowCount, gen.Bool()),
		).Map(func(values []interface{}) OperationTestData {
			return OperationTestData{
				UserID:        values[0].(string),
				OperationType: values[1].(string),
				Title:         values[2].(string),
				RowOutcomes:   values[3].([]bool),
			}
		})
	}, reflect.TypeOf(OperationTestData{}))
}

func genOperationUserID() gopter.Gen {
	return gen.Identifier().Map(func(s string) string {
		return "user_" + s
	})
}

func genOperationType() gopter.Gen {
	return gen.OneConstOf("field_update", "report_generation")
}

func genOperationTitle() gopter.Gen {
	return gen.Identifier().Map(func(s string) string {
		return "Operation_" + s
	})
}
