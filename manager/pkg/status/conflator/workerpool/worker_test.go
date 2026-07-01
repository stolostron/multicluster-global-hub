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

		worker.RunAsync(job)

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

		worker.RunAsync(job1)

		go worker.RunAsync(job2)

		select {
		case receivedJob := <-worker.jobsQueue:
			assert.Equal(t, job1, receivedJob)
		case <-time.After(100 * time.Millisecond):
			t.Error("First job was not added to queue")
		}

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

		go worker.start(ctx)

		select {
		case availableWorker := <-dbWorkersPool:
			assert.Equal(t, worker, availableWorker)
		case <-time.After(1 * time.Second):
			t.Error("Worker did not become available")
		}

		cancel()
		time.Sleep(100 * time.Millisecond)
	})

	t.Run("start handles context cancellation", func(t *testing.T) {
		dbWorkersPool := make(chan *Worker, 2)
		worker := NewWorker(1, dbWorkersPool, &statistics.Statistics{})

		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan bool)
		go func() {
			worker.start(ctx)
			done <- true
		}()

		select {
		case <-dbWorkersPool:
		case <-time.After(1 * time.Second):
			t.Error("Worker did not become available")
		}

		cancel()

		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Error("Worker did not stop after context cancellation")
		}
	})

	t.Run("start stops when context already cancelled", func(t *testing.T) {
		dbWorkersPool := make(chan *Worker, 2)
		worker := NewWorker(1, dbWorkersPool, &statistics.Statistics{})

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		done := make(chan struct{})
		go func() {
			worker.start(ctx)
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Error("Worker did not stop when context was already cancelled")
		}

		_, ok := <-worker.jobsQueue
		assert.False(t, ok, "jobs queue should be closed on shutdown")
	})

	t.Run("start stops during blocked pool registration", func(t *testing.T) {
		dbWorkersPool := make(chan *Worker, 1)
		occupant := NewWorker(99, dbWorkersPool, &statistics.Statistics{})
		dbWorkersPool <- occupant

		worker := NewWorker(1, dbWorkersPool, &statistics.Statistics{})

		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan struct{})
		go func() {
			worker.start(ctx)
			close(done)
		}()

		time.Sleep(50 * time.Millisecond)
		cancel()

		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Error("Worker did not stop during blocked pool registration")
		}

		_, ok := <-worker.jobsQueue
		assert.False(t, ok, "jobs queue should be closed on shutdown")
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

		assert.Panics(t, func() {
			worker.start(ctx)
		})
	})

	t.Run("worker queue capacity", func(t *testing.T) {
		worker := NewWorker(1, make(chan *Worker, 10), &statistics.Statistics{})

		assert.Equal(t, 1, cap(worker.jobsQueue))

		event := cloudevents.NewEvent()
		event.SetType("test.event")
		event.SetSource("test-source")

		job1 := &conflator.ConflationJob{Event: &event}
		job2 := &conflator.ConflationJob{Event: &event}

		worker.RunAsync(job1)

		jobAdded := make(chan bool)
		go func() {
			worker.RunAsync(job2)
			jobAdded <- true
		}()

		<-worker.jobsQueue

		select {
		case <-jobAdded:
		case <-time.After(100 * time.Millisecond):
			t.Error("Second job was not added")
		}

		<-worker.jobsQueue
	})
}

func TestWorker_WorkflowIntegration(t *testing.T) {
	t.Run("complete worker workflow", func(t *testing.T) {
		dbWorkersPool := make(chan *Worker, 2)
		worker := NewWorker(1, dbWorkersPool, &statistics.Statistics{})

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		go worker.start(ctx)

		select {
		case availableWorker := <-dbWorkersPool:
			assert.Equal(t, worker, availableWorker)
		case <-time.After(500 * time.Millisecond):
			t.Error("Worker did not become available")
		}

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

		worker.RunAsync(job)

		time.Sleep(100 * time.Millisecond)

		assert.True(t, metadata.Processed())

		results := reporter.GetResults()
		assert.Len(t, results, 1)
		assert.Equal(t, metadata, results[0].metadata)
		assert.Nil(t, results[0].err)
	})
}
