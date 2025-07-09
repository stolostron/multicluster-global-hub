package conflator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
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

func TestElementState(t *testing.T) {
	t.Run("ElementState initialization", func(t *testing.T) {
		state := &ElementState{
			LastProcessedVersion:  version.NewVersion(),
			LastProcessedMetadata: &mockConflationMetadata{},
			HandlerLock:           sync.Mutex{},
		}

		assert.NotNil(t, state.LastProcessedVersion)
		assert.NotNil(t, state.LastProcessedMetadata)
		assert.NotNil(t, state.HandlerLock)
	})

	t.Run("ElementState thread safety", func(t *testing.T) {
		state := &ElementState{
			LastProcessedVersion:  version.NewVersion(),
			LastProcessedMetadata: &mockConflationMetadata{},
			HandlerLock:           sync.Mutex{},
		}

		// Test concurrent access to HandlerLock
		var wg sync.WaitGroup
		counter := 0

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				state.HandlerLock.Lock()
				counter++
				state.HandlerLock.Unlock()
			}()
		}

		wg.Wait()
		assert.Equal(t, 10, counter)
	})
}

func TestNewHybridElement(t *testing.T) {
	t.Run("NewHybridElement creates element correctly", func(t *testing.T) {
		mockHandlerFunc := func(ctx context.Context, event *cloudevents.Event) error {
			return nil
		}

		registration := &ConflationRegistration{
			eventType:  "test.event.type",
			syncMode:   enum.HybridStateMode,
			handleFunc: mockHandlerFunc,
		}

		// Note: The current implementation returns *deltaElement, which seems to be a bug
		// This test documents the current behavior
		element := NewHybridElement(registration)

		assert.NotNil(t, element)
		// The function should return *hybridElement but currently returns *deltaElement
		// This is a bug in the original implementation
	})
}

func TestHybridElement_Name(t *testing.T) {
	t.Run("Name returns correct event type", func(t *testing.T) {
		element := &hybridElement{
			eventType: "test.event.type",
		}

		name := element.Name()
		assert.Equal(t, "test.event.type", name)
	})

	t.Run("Name with empty event type", func(t *testing.T) {
		element := &hybridElement{
			eventType: "",
		}

		name := element.Name()
		assert.Equal(t, "", name)
	})
}

func TestHybridElement_Metadata(t *testing.T) {
	t.Run("Metadata returns correct metadata", func(t *testing.T) {
		mockMetadata := &mockConflationMetadata{}
		state := &ElementState{
			LastProcessedMetadata: mockMetadata,
		}
		element := &hybridElement{
			elementState: state,
		}

		metadata := element.Metadata()
		assert.Equal(t, mockMetadata, metadata)
	})

	t.Run("Metadata with nil state", func(t *testing.T) {
		element := &hybridElement{
			elementState: nil,
		}

		// This should panic due to nil pointer access
		assert.Panics(t, func() {
			element.Metadata()
		})
	})
}

func TestHybridElement_SyncMode(t *testing.T) {
	t.Run("SyncMode returns correct mode", func(t *testing.T) {
		element := &hybridElement{
			syncMode: enum.HybridStateMode,
		}

		mode := element.SyncMode()
		assert.Equal(t, enum.HybridStateMode, mode)
	})

	t.Run("SyncMode with different modes", func(t *testing.T) {
		testCases := []enum.EventSyncMode{
			enum.CompleteStateMode,
			enum.DeltaStateMode,
			enum.HybridStateMode,
		}

		for _, mode := range testCases {
			element := &hybridElement{
				syncMode: mode,
			}

			result := element.SyncMode()
			assert.Equal(t, mode, result)
		}
	})
}

func TestHybridElement_Predicate(t *testing.T) {
	t.Run("Predicate with newer generation", func(t *testing.T) {
		state := &ElementState{
			LastProcessedVersion: version.NewVersion(),
		}
		element := &hybridElement{
			eventType:    "test.event.type",
			elementState: state,
		}

		// Create a newer version
		newVersion := version.NewVersion()
		newVersion.Next() // This increments generation

		result := element.Predicate(newVersion)
		assert.True(t, result)
	})

	t.Run("Predicate with older generation", func(t *testing.T) {
		// Create an initial version and increment it
		lastProcessedVersion := version.NewVersion()
		lastProcessedVersion.Next() // This increments generation

		state := &ElementState{
			LastProcessedVersion: lastProcessedVersion,
		}
		element := &hybridElement{
			eventType:    "test.event.type",
			elementState: state,
		}

		// Create an older version (Generation = 0)
		oldVersion := version.NewVersion()

		result := element.Predicate(oldVersion)
		// Since oldVersion.InitGen() is true, it will reset LastProcessedVersion
		// and then compare with NewerGenerationThan, which will be true (0 >= 0)
		assert.True(t, result)
	})

	t.Run("Predicate with init generation", func(t *testing.T) {
		// Create a version with higher generation
		lastProcessedVersion := version.NewVersion()
		lastProcessedVersion.Next() // This increments generation

		state := &ElementState{
			LastProcessedVersion: lastProcessedVersion,
		}
		element := &hybridElement{
			eventType:    "test.event.type",
			elementState: state,
		}

		// Create a version with init generation (Generation = 0)
		initVersion := version.NewVersion() // This creates version with Generation = 0

		result := element.Predicate(initVersion)
		assert.True(t, result)
		// After init generation, the last processed version should be reset
		assert.Equal(t, uint64(0), state.LastProcessedVersion.Generation)
	})

	t.Run("Predicate with same generation", func(t *testing.T) {
		state := &ElementState{
			LastProcessedVersion: version.NewVersion(),
		}
		element := &hybridElement{
			eventType:    "test.event.type",
			elementState: state,
		}

		// Create the same version
		sameVersion := version.NewVersion()

		result := element.Predicate(sameVersion)
		// Since sameVersion.InitGen() is true, it will reset LastProcessedVersion
		// and then compare with NewerGenerationThan, which will be true (0 >= 0)
		assert.True(t, result)
	})

	t.Run("Predicate with non-initial generation false case", func(t *testing.T) {
		// Set up a version with generation > 0
		lastProcessedVersion := version.NewVersion()
		lastProcessedVersion.Next() // Generation = 1
		lastProcessedVersion.Next() // Generation = 2

		state := &ElementState{
			LastProcessedVersion: lastProcessedVersion,
		}
		element := &hybridElement{
			eventType:    "test.event.type",
			elementState: state,
		}

		// Create a version with lower generation
		lowerVersion := version.NewVersion()
		lowerVersion.Next() // Generation = 1

		result := element.Predicate(lowerVersion)
		// Since lowerVersion.InitGen() is false and generation 1 < 2
		assert.False(t, result)
	})
}

func TestHybridElement_AddToReadyQueue(t *testing.T) {
	t.Run("AddToReadyQueue adds job to channel", func(t *testing.T) {
		mockHandlerFunc := func(ctx context.Context, event *cloudevents.Event) error {
			return nil
		}

		state := &ElementState{
			LastProcessedVersion:  version.NewVersion(),
			LastProcessedMetadata: &mockConflationMetadata{},
			HandlerLock:           sync.Mutex{},
		}

		element := &hybridElement{
			eventType:       "test.event.type",
			handlerFunction: mockHandlerFunc,
			elementState:    state,
		}

		// Create a conflation unit with ready queue
		readyQueue := NewConflationReadyQueue(&statistics.Statistics{})
		cu := &ConflationUnit{
			readyQueue: readyQueue,
		}

		// Create a cloud event
		event := cloudevents.NewEvent()
		event.SetType("test.event.type")
		event.SetSource("test-source")

		// Create mock metadata
		mockMetadata := &mockConflationMetadata{
			version: version.NewVersion(),
		}

		// Add to ready queue
		element.AddToReadyQueue(&event, mockMetadata, cu)

		// Verify a job was added to the channel
		select {
		case job := <-readyQueue.DeltaEventJobChan:
			assert.NotNil(t, job)
			assert.Equal(t, &event, job.Event)
			assert.Equal(t, mockMetadata, job.Metadata)
			assert.NotNil(t, job.Handle) // Cannot compare functions directly
			assert.Equal(t, cu, job.Reporter)
			assert.Equal(t, state, job.ElementState)
		case <-time.After(100 * time.Millisecond):
			t.Error("No job was added to the channel")
		}
	})

	t.Run("AddToReadyQueue with nil event", func(t *testing.T) {
		mockHandlerFunc := func(ctx context.Context, event *cloudevents.Event) error {
			return nil
		}

		state := &ElementState{
			LastProcessedVersion:  version.NewVersion(),
			LastProcessedMetadata: &mockConflationMetadata{},
			HandlerLock:           sync.Mutex{},
		}

		element := &hybridElement{
			eventType:       "test.event.type",
			handlerFunction: mockHandlerFunc,
			elementState:    state,
		}

		readyQueue := NewConflationReadyQueue(&statistics.Statistics{})
		cu := &ConflationUnit{
			readyQueue: readyQueue,
		}

		mockMetadata := &mockConflationMetadata{}

		// This should not panic
		assert.NotPanics(t, func() {
			element.AddToReadyQueue(nil, mockMetadata, cu)
		})
	})
}

func TestHybridElement_PostProcess(t *testing.T) {
	t.Run("PostProcess with no error", func(t *testing.T) {
		state := &ElementState{
			LastProcessedVersion:  version.NewVersion(),
			LastProcessedMetadata: &mockConflationMetadata{},
			HandlerLock:           sync.Mutex{},
		}

		element := &hybridElement{
			eventType:    "test.event.type",
			elementState: state,
		}

		mockMetadata := &mockConflationMetadata{
			version: version.NewVersion(),
		}

		// PostProcess should not panic and should not log error
		assert.NotPanics(t, func() {
			element.PostProcess(mockMetadata, nil)
		})
	})

	t.Run("PostProcess with error", func(t *testing.T) {
		state := &ElementState{
			LastProcessedVersion:  version.NewVersion(),
			LastProcessedMetadata: &mockConflationMetadata{},
			HandlerLock:           sync.Mutex{},
		}

		element := &hybridElement{
			eventType:    "test.event.type",
			elementState: state,
		}

		mockMetadata := &mockConflationMetadata{
			version: version.NewVersion(),
		}

		testError := errors.New("test error")

		// PostProcess should not panic even with error
		assert.NotPanics(t, func() {
			element.PostProcess(mockMetadata, testError)
		})
	})

	t.Run("PostProcess with nil metadata", func(t *testing.T) {
		state := &ElementState{
			LastProcessedVersion:  version.NewVersion(),
			LastProcessedMetadata: &mockConflationMetadata{},
			HandlerLock:           sync.Mutex{},
		}

		element := &hybridElement{
			eventType:    "test.event.type",
			elementState: state,
		}

		// This should not panic even with nil metadata
		assert.NotPanics(t, func() {
			element.PostProcess(nil, nil)
		})
	})
}

func TestHybridElement_Integration(t *testing.T) {
	t.Run("Full workflow integration test", func(t *testing.T) {
		mockHandlerFunc := func(ctx context.Context, event *cloudevents.Event) error {
			return nil
		}

		state := &ElementState{
			LastProcessedVersion:  version.NewVersion(),
			LastProcessedMetadata: &mockConflationMetadata{},
			HandlerLock:           sync.Mutex{},
		}

		element := &hybridElement{
			eventType:       "test.event.type",
			syncMode:        enum.HybridStateMode,
			handlerFunction: mockHandlerFunc,
			elementState:    state,
		}

		// Test Name
		assert.Equal(t, "test.event.type", element.Name())

		// Test SyncMode
		assert.Equal(t, enum.HybridStateMode, element.SyncMode())

		// Test Predicate with newer version
		newVersion := version.NewVersion()
		newVersion.Next() // This increments generation
		assert.True(t, element.Predicate(newVersion))

		// Test AddToReadyQueue
		readyQueue := NewConflationReadyQueue(&statistics.Statistics{})
		cu := &ConflationUnit{
			readyQueue: readyQueue,
		}

		event := cloudevents.NewEvent()
		event.SetType("test.event.type")
		event.SetSource("test-source")

		mockMetadata := &mockConflationMetadata{
			version: version.NewVersion(),
		}

		element.AddToReadyQueue(&event, mockMetadata, cu)

		// Verify job was added
		select {
		case job := <-readyQueue.DeltaEventJobChan:
			assert.NotNil(t, job)
			assert.Equal(t, &event, job.Event)
		case <-time.After(100 * time.Millisecond):
			t.Error("No job was added to the channel")
		}

		// Test PostProcess
		assert.NotPanics(t, func() {
			element.PostProcess(mockMetadata, nil)
		})
	})
}

func TestHybridElement_EdgeCases(t *testing.T) {
	t.Run("Element with nil state", func(t *testing.T) {
		element := &hybridElement{
			eventType:    "test.event.type",
			elementState: nil,
		}

		// These should not panic
		assert.NotPanics(t, func() {
			element.Name()
		})

		assert.NotPanics(t, func() {
			element.SyncMode()
		})

		// This might panic due to nil pointer access
		assert.Panics(t, func() {
			element.Predicate(version.NewVersion())
		})
	})

	t.Run("Element with empty event type", func(t *testing.T) {
		state := &ElementState{
			LastProcessedVersion:  version.NewVersion(),
			LastProcessedMetadata: &mockConflationMetadata{},
			HandlerLock:           sync.Mutex{},
		}

		element := &hybridElement{
			eventType:    "",
			elementState: state,
		}

		assert.Equal(t, "", element.Name())
	})
}

// Helper function to create a test metadata implementation
func createTestMetadata(eventType string, version *version.Version) ConflationMetadata {
	return &mockConflationMetadata{
		processed:         false,
		version:           version,
		dependencyVersion: version,
		eventType:         eventType,
		position: &transport.EventPosition{
			Topic:     "test-topic",
			Partition: 0,
			Offset:    1,
		},
	}
}
