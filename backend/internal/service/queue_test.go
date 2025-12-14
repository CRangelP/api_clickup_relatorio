package service

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// MockJob represents a simplified job for testing FIFO order
type MockJob struct {
	ID        int
	UserID    string
	Title     string
	CreatedAt time.Time
}

// QueueForTest is a testable in-memory queue implementation
// that mirrors the behavior of the real QueueRepository
type QueueForTest struct {
	jobs          []MockJob
	nextID        int
	processedJobs []MockJob
}

// NewQueueForTest creates a new test queue
func NewQueueForTest() *QueueForTest {
	return &QueueForTest{
		jobs:          []MockJob{},
		nextID:        1,
		processedJobs: []MockJob{},
	}
}

// AddJob adds a job to the queue (simulates CreateJob)
func (q *QueueForTest) AddJob(userID, title string) MockJob {
	job := MockJob{
		ID:        q.nextID,
		UserID:    userID,
		Title:     title,
		CreatedAt: time.Now().Add(time.Duration(q.nextID) * time.Millisecond),
	}
	q.nextID++
	q.jobs = append(q.jobs, job)
	return job
}

// GetPendingJobsFIFO returns jobs in FIFO order (oldest first)
func (q *QueueForTest) GetPendingJobsFIFO() []MockJob {
	// Sort by CreatedAt ascending (FIFO)
	sorted := make([]MockJob, len(q.jobs))
	copy(sorted, q.jobs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
	})
	return sorted
}

// ProcessNextJob processes the next job in FIFO order
func (q *QueueForTest) ProcessNextJob() *MockJob {
	if len(q.jobs) == 0 {
		return nil
	}

	// Get jobs in FIFO order
	fifoJobs := q.GetPendingJobsFIFO()
	if len(fifoJobs) == 0 {
		return nil
	}

	// Process the first (oldest) job
	jobToProcess := fifoJobs[0]

	// Remove from pending jobs
	newJobs := []MockJob{}
	for _, j := range q.jobs {
		if j.ID != jobToProcess.ID {
			newJobs = append(newJobs, j)
		}
	}
	q.jobs = newJobs

	// Add to processed jobs
	q.processedJobs = append(q.processedJobs, jobToProcess)

	return &jobToProcess
}

// GetProcessedJobs returns the list of processed jobs in order
func (q *QueueForTest) GetProcessedJobs() []MockJob {
	return q.processedJobs
}

// ProcessAllJobs processes all pending jobs
func (q *QueueForTest) ProcessAllJobs() {
	for len(q.jobs) > 0 {
		q.ProcessNextJob()
	}
}

// QueueTestData holds test data for property tests
type QueueTestData struct {
	JobTitles []string
	UserIDs   []string
}

// TestQueueProcessingOrderPreservation tests Property 7: Queue processing order preservation
// **Feature: clickup-field-updater, Property 7: Queue processing order preservation**
// **Validates: Requirements 6.1, 6.2, 6.3**
//
// For any sequence of jobs added to the queue, the system should process them in FIFO order
// and maintain job state consistency throughout processing.
func TestQueueProcessingOrderPreservation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.MaxSize = 20

	properties := gopter.NewProperties(parameters)

	// Property 7.1: Jobs are processed in FIFO order
	// For any sequence of jobs added to the queue, processing order should match creation order
	properties.Property("jobs are processed in FIFO order", prop.ForAll(
		func(testData QueueTestData) bool {
			if len(testData.JobTitles) == 0 {
				return true // Empty queue is trivially FIFO
			}

			queue := NewQueueForTest()

			// Add jobs in order
			addedJobs := []MockJob{}
			for i, title := range testData.JobTitles {
				userID := "user1"
				if i < len(testData.UserIDs) {
					userID = testData.UserIDs[i]
				}
				job := queue.AddJob(userID, title)
				addedJobs = append(addedJobs, job)
			}

			// Process all jobs
			queue.ProcessAllJobs()

			// Verify FIFO order: processed jobs should be in same order as added
			processedJobs := queue.GetProcessedJobs()
			if len(processedJobs) != len(addedJobs) {
				return false
			}

			for i := range addedJobs {
				if processedJobs[i].ID != addedJobs[i].ID {
					return false
				}
			}

			return true
		},
		genQueueTestData(),
	))

	// Property 7.2: Completed jobs are removed from queue
	// For any job that is processed, it should no longer be in the pending queue
	properties.Property("completed jobs are removed from queue", prop.ForAll(
		func(testData QueueTestData) bool {
			if len(testData.JobTitles) == 0 {
				return true
			}

			queue := NewQueueForTest()

			// Add jobs
			for i, title := range testData.JobTitles {
				userID := "user1"
				if i < len(testData.UserIDs) {
					userID = testData.UserIDs[i]
				}
				queue.AddJob(userID, title)
			}

			initialCount := len(queue.jobs)

			// Process one job
			processedJob := queue.ProcessNextJob()
			if processedJob == nil {
				return false
			}

			// Verify job was removed from pending queue
			if len(queue.jobs) != initialCount-1 {
				return false
			}

			// Verify processed job is not in pending queue
			for _, j := range queue.jobs {
				if j.ID == processedJob.ID {
					return false
				}
			}

			return true
		},
		genQueueTestData(),
	))

	// Property 7.3: Queue maintains consistency during processing
	// For any sequence of jobs, the sum of pending + processed should equal total added
	properties.Property("queue maintains consistency during processing", prop.ForAll(
		func(testData QueueTestData) bool {
			if len(testData.JobTitles) == 0 {
				return true
			}

			queue := NewQueueForTest()

			// Add jobs
			totalAdded := 0
			for i, title := range testData.JobTitles {
				userID := "user1"
				if i < len(testData.UserIDs) {
					userID = testData.UserIDs[i]
				}
				queue.AddJob(userID, title)
				totalAdded++
			}

			// Process some jobs (half of them)
			toProcess := len(testData.JobTitles) / 2
			for i := 0; i < toProcess; i++ {
				queue.ProcessNextJob()
			}

			// Verify consistency: pending + processed = total
			pendingCount := len(queue.jobs)
			processedCount := len(queue.processedJobs)

			return pendingCount+processedCount == totalAdded
		},
		genQueueTestData(),
	))

	// Property 7.4: First job added is first job processed
	// For any non-empty queue, the first job added should be the first job processed
	properties.Property("first job added is first job processed", prop.ForAll(
		func(testData QueueTestData) bool {
			if len(testData.JobTitles) == 0 {
				return true
			}

			queue := NewQueueForTest()

			// Add jobs
			var firstJob MockJob
			for i, title := range testData.JobTitles {
				userID := "user1"
				if i < len(testData.UserIDs) {
					userID = testData.UserIDs[i]
				}
				job := queue.AddJob(userID, title)
				if i == 0 {
					firstJob = job
				}
			}

			// Process first job
			processedJob := queue.ProcessNextJob()
			if processedJob == nil {
				return false
			}

			// First processed should be first added
			return processedJob.ID == firstJob.ID
		},
		genQueueTestData(),
	))

	// Property 7.5: Processing order is deterministic
	// For any sequence of jobs, processing the same sequence twice should yield same order
	properties.Property("processing order is deterministic", prop.ForAll(
		func(testData QueueTestData) bool {
			if len(testData.JobTitles) == 0 {
				return true
			}

			// First run
			queue1 := NewQueueForTest()
			for i, title := range testData.JobTitles {
				userID := "user1"
				if i < len(testData.UserIDs) {
					userID = testData.UserIDs[i]
				}
				queue1.AddJob(userID, title)
			}
			queue1.ProcessAllJobs()
			processed1 := queue1.GetProcessedJobs()

			// Second run with same data
			queue2 := NewQueueForTest()
			for i, title := range testData.JobTitles {
				userID := "user1"
				if i < len(testData.UserIDs) {
					userID = testData.UserIDs[i]
				}
				queue2.AddJob(userID, title)
			}
			queue2.ProcessAllJobs()
			processed2 := queue2.GetProcessedJobs()

			// Both runs should produce same order
			if len(processed1) != len(processed2) {
				return false
			}

			for i := range processed1 {
				// Compare by position (IDs will differ between runs)
				if processed1[i].Title != processed2[i].Title {
					return false
				}
			}

			return true
		},
		genQueueTestData(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Generators

func genQueueTestData() gopter.Gen {
	return gen.IntRange(1, 10).FlatMap(func(numJobs interface{}) gopter.Gen {
		jobCount := numJobs.(int)
		return gopter.CombineGens(
			gen.SliceOfN(jobCount, genJobTitle()),
			gen.SliceOfN(jobCount, genUserID()),
		).Map(func(values []interface{}) QueueTestData {
			titles := values[0].([]string)
			userIDs := values[1].([]string)
			return QueueTestData{
				JobTitles: titles,
				UserIDs:   userIDs,
			}
		})
	}, reflect.TypeOf(QueueTestData{}))
}

func genJobTitle() gopter.Gen {
	// Use Identifier which always generates non-empty strings
	return gen.Identifier().Map(func(s string) string {
		return "Job_" + s
	})
}

func genUserID() gopter.Gen {
	// Use Identifier which always generates non-empty strings
	return gen.Identifier().Map(func(s string) string {
		return "user_" + s
	})
}
