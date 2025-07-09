package workerpool

import (
	"context"
	"sync"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// Mock implementations for testing
type mockConflationMetadata struct {
	processed         bool
	version           *version.Version
	dependencyVersion *version.Version
	eventType         string
	position          *transport.EventPosition
}

func (m *mockConflationMetadata) MarkAsProcessed() {
	m.processed = true
}

func (m *mockConflationMetadata) Processed() bool {
	return m.processed
}

func (m *mockConflationMetadata) MarkAsUnprocessed() {
	m.processed = false
}

func (m *mockConflationMetadata) Version() *version.Version {
	return m.version
}

func (m *mockConflationMetadata) DependencyVersion() *version.Version {
	return m.dependencyVersion
}

func (m *mockConflationMetadata) EventType() string {
	return m.eventType
}

func (m *mockConflationMetadata) TransportPosition() *transport.EventPosition {
	return m.position
}

type mockResultReporter struct {
	results []reportResult
	mu      sync.Mutex
}

type reportResult struct {
	metadata conflator.ConflationMetadata
	err      error
}

func (m *mockResultReporter) ReportResult(metadata conflator.ConflationMetadata, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results = append(m.results, reportResult{metadata: metadata, err: err})
}

func (m *mockResultReporter) GetResults() []reportResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]reportResult{}, m.results...)
}

func TestNewWorker(t *testing.T) {
	t.Run("NewWorker creates worker correctly", func(t *testing.T) {
		workerID := int32(1)
		dbWorkersPool := make(chan *Worker, 10)
		stats := &statistics.Statistics{}

		worker := NewWorker(workerID, dbWorkersPool, stats)

		assert.NotNil(t, worker)
		assert.Equal(t, workerID, worker.workerID)
		assert.Equal(t, dbWorkersPool, worker.workers)
		assert.Equal(t, stats, worker.statistics)
		assert.NotNil(t, worker.jobsQueue)
		assert.Equal(t, 1, cap(worker.jobsQueue))
	})

	t.Run("NewWorker with different worker ID", func(t *testing.T) {
		workerID := int32(42)
		dbWorkersPool := make(chan *Worker, 5)
		stats := &statistics.Statistics{}

		worker := NewWorker(workerID, dbWorkersPool, stats)

		assert.Equal(t, workerID, worker.workerID)
		assert.Equal(t, dbWorkersPool, worker.workers)
	})

	t.Run("NewWorker with nil statistics", func(t *testing.T) {
		workerID := int32(1)
		dbWorkersPool := make(chan *Worker, 10)

		worker := NewWorker(workerID, dbWorkersPool, nil)

		assert.NotNil(t, worker)
		assert.Nil(t, worker.statistics)
	})
}

func TestWorker_RunAsync(t *testing.T) {
	t.Run("RunAsync adds job to queue", func(t *testing.T) {
		worker := NewWorker(1, make(chan *Worker, 10), &statistics.Statistics{})

		event := cloudevents.NewEvent()
		event.SetType("test.event")
		event.SetSource("test-source")

		job := &conflator.ConflationJob{
			Event: &event,
		}

		// Run async should not block
		worker.RunAsync(job)

		// Verify job was added to queue
		select {
		case receivedJob := <-worker.jobsQueue:
			assert.Equal(t, job, receivedJob)
		case <-time.After(100 * time.Millisecond):
			t.Error("Job was not added to queue")
		}
	})

	t.Run("RunAsync with multiple jobs", func(t *testing.T) {
		worker := NewWorker(1, make(chan *Worker, 10), &statistics.Statistics{})

		event1 := cloudevents.NewEvent()
		event1.SetType("test.event1")
		event1.SetSource("test-source1")

		event2 := cloudevents.NewEvent()
		event2.SetType("test.event2")
		event2.SetSource("test-source2")

		job1 := &conflator.ConflationJob{Event: &event1}
		job2 := &conflator.ConflationJob{Event: &event2}

		// First job should be added immediately
		worker.RunAsync(job1)

		// Second job should be added but may block due to queue capacity
		go worker.RunAsync(job2)

		// Verify first job
		select {
		case receivedJob := <-worker.jobsQueue:
			assert.Equal(t, job1, receivedJob)
		case <-time.After(100 * time.Millisecond):
			t.Error("First job was not added to queue")
		}

		// Verify second job
		select {
		case receivedJob := <-worker.jobsQueue:
			assert.Equal(t, job2, receivedJob)
		case <-time.After(100 * time.Millisecond):
			t.Error("Second job was not added to queue")
		}
	})
}

func TestWorker_start(t *testing.T) {
	t.Run("start makes worker available in pool", func(t *testing.T) {
		dbWorkersPool := make(chan *Worker, 2)
		worker := NewWorker(1, dbWorkersPool, &statistics.Statistics{})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Start worker in background
		go worker.start(ctx)

		// Wait for worker to be available
		select {
		case availableWorker := <-dbWorkersPool:
			assert.Equal(t, worker, availableWorker)
		case <-time.After(1 * time.Second):
			t.Error("Worker did not become available")
		}

		// Cancel context to stop worker
		cancel()

		// Worker should stop gracefully
		time.Sleep(100 * time.Millisecond)
	})

	t.Run("start handles context cancellation", func(t *testing.T) {
		dbWorkersPool := make(chan *Worker, 2)
		worker := NewWorker(1, dbWorkersPool, &statistics.Statistics{})

		ctx, cancel := context.WithCancel(context.Background())

		// Start worker in background
		done := make(chan bool)
		go func() {
			worker.start(ctx)
			done <- true
		}()

		// Wait for worker to be available
		select {
		case <-dbWorkersPool:
			// Worker is available
		case <-time.After(1 * time.Second):
			t.Error("Worker did not become available")
		}

		// Cancel context immediately
		cancel()

		// Worker should stop
		select {
		case <-done:
			// Worker stopped
		case <-time.After(1 * time.Second):
			t.Error("Worker did not stop after context cancellation")
		}
	})
}

func TestWorker_EdgeCases(t *testing.T) {
	t.Run("worker with nil statistics", func(t *testing.T) {
		worker := NewWorker(1, make(chan *Worker, 10), nil)

		event := cloudevents.NewEvent()
		event.SetType("test.event")
		event.SetSource("test-source")

		job := &conflator.ConflationJob{
			Event: &event,
		}

		// This should not panic
		assert.NotPanics(t, func() {
			worker.RunAsync(job)
		})
	})

	t.Run("worker with closed workers channel", func(t *testing.T) {
		dbWorkersPool := make(chan *Worker, 1)
		close(dbWorkersPool)

		worker := NewWorker(1, dbWorkersPool, &statistics.Statistics{})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Worker should panic when trying to send to closed channel - this is expected behavior
		assert.Panics(t, func() {
			worker.start(ctx)
		})
	})

	t.Run("worker queue capacity", func(t *testing.T) {
		worker := NewWorker(1, make(chan *Worker, 10), &statistics.Statistics{})

		// Test that job queue has capacity of 1
		assert.Equal(t, 1, cap(worker.jobsQueue))

		event := cloudevents.NewEvent()
		event.SetType("test.event")
		event.SetSource("test-source")

		job1 := &conflator.ConflationJob{Event: &event}
		job2 := &conflator.ConflationJob{Event: &event}

		// First job should be added immediately
		worker.RunAsync(job1)

		// Second job will block due to capacity
		jobAdded := make(chan bool)
		go func() {
			worker.RunAsync(job2)
			jobAdded <- true
		}()

		// Consume first job
		<-worker.jobsQueue

		// Now second job should be added
		select {
		case <-jobAdded:
			// Second job was added
		case <-time.After(100 * time.Millisecond):
			t.Error("Second job was not added")
		}

		// Consume second job
		<-worker.jobsQueue
	})
}

func TestWorker_WorkflowIntegration(t *testing.T) {
	t.Run("complete worker workflow", func(t *testing.T) {
		dbWorkersPool := make(chan *Worker, 2)
		worker := NewWorker(1, dbWorkersPool, &statistics.Statistics{})

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// Start worker
		go worker.start(ctx)

		// Verify worker becomes available
		select {
		case availableWorker := <-dbWorkersPool:
			assert.Equal(t, worker, availableWorker)
		case <-time.After(500 * time.Millisecond):
			t.Error("Worker did not become available")
		}

		// Create and submit job with all required fields
		event := cloudevents.NewEvent()
		event.SetType("test.event")
		event.SetSource("test-source")

		metadata := &mockConflationMetadata{
			version:   version.NewVersion(),
			eventType: "test.event",
		}

		reporter := &mockResultReporter{}

		handler := func(ctx context.Context, event *cloudevents.Event) error {
			return nil
		}

		job := &conflator.ConflationJob{
			Event:    &event,
			Handle:   handler,
			Metadata: metadata,
			Reporter: reporter,
		}

		// Submit job
		worker.RunAsync(job)

		// Wait for processing
		time.Sleep(100 * time.Millisecond)

		// Verify job was processed successfully
		assert.True(t, metadata.Processed())

		// Verify reporter was called
		results := reporter.GetResults()
		assert.Len(t, results, 1)
		assert.Equal(t, metadata, results[0].metadata)
		assert.Nil(t, results[0].err)
	})
}
