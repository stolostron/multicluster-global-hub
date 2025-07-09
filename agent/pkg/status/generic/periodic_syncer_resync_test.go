package generic

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ResyncMockEmitter implements the emitters.Emitter interface for testing the Resync function
type ResyncMockEmitter struct {
	eventType     string
	resyncObjects []client.Object
	resyncError   error
	resyncCalled  bool
}

func (m *ResyncMockEmitter) EventType() string {
	return m.eventType
}

func (m *ResyncMockEmitter) EventFilter() predicate.Predicate {
	return predicate.Funcs{}
}

func (m *ResyncMockEmitter) Update(obj client.Object) error {
	return nil
}

func (m *ResyncMockEmitter) Delete(obj client.Object) error {
	return nil
}

func (m *ResyncMockEmitter) Resync(objects []client.Object) error {
	m.resyncCalled = true
	m.resyncObjects = objects
	return m.resyncError
}

func (m *ResyncMockEmitter) Send() error {
	return nil
}

func TestPeriodicSyncer_Resync(t *testing.T) {
	// Create test objects
	configMap1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm-1",
			Namespace: "default",
		},
		Data: map[string]string{
			"key1": "value1",
		},
	}

	configMap2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm-2",
			Namespace: "default",
		},
		Data: map[string]string{
			"key2": "value2",
		},
	}

	secret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret-1",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"password": []byte("secret123"),
		},
	}

	tests := []struct {
		name          string
		eventType     string
		setupObjects  []client.Object
		setupStates   func(fakeClient client.Client) []*SyncState
		expectError   bool
		expectResync  bool
		expectedCount int
		errorContains string
	}{
		{
			name:         "successful resync with configmaps",
			eventType:    "configmap-event",
			setupObjects: []client.Object{configMap1, configMap2},
			setupStates: func(fakeClient client.Client) []*SyncState {
				emitter := &ResyncMockEmitter{eventType: "configmap-event"}
				return []*SyncState{
					{
						Registration: &EmitterRegistration{
							ListFunc: func() ([]client.Object, error) {
								configMapList := &corev1.ConfigMapList{}
								err := fakeClient.List(context.Background(), configMapList)
								if err != nil {
									return nil, err
								}
								objects := make([]client.Object, len(configMapList.Items))
								for i := range configMapList.Items {
									objects[i] = &configMapList.Items[i]
								}
								return objects, nil
							},
							Emitter: emitter,
						},
						NextResyncAt: time.Now().Add(-1 * time.Hour), // Past time
					},
				}
			},
			expectError:   false,
			expectResync:  true,
			expectedCount: 2,
		},
		{
			name:         "successful resync with secrets",
			eventType:    "secret-event",
			setupObjects: []client.Object{secret1},
			setupStates: func(fakeClient client.Client) []*SyncState {
				emitter := &ResyncMockEmitter{eventType: "secret-event"}
				return []*SyncState{
					{
						Registration: &EmitterRegistration{
							ListFunc: func() ([]client.Object, error) {
								secretList := &corev1.SecretList{}
								err := fakeClient.List(context.Background(), secretList)
								if err != nil {
									return nil, err
								}
								objects := make([]client.Object, len(secretList.Items))
								for i := range secretList.Items {
									objects[i] = &secretList.Items[i]
								}
								return objects, nil
							},
							Emitter: emitter,
						},
						NextResyncAt: time.Now().Add(-1 * time.Hour),
					},
				}
			},
			expectError:   false,
			expectResync:  true,
			expectedCount: 1,
		},
		{
			name:         "no objects found - should return early",
			eventType:    "empty-event",
			setupObjects: []client.Object{}, // No objects
			setupStates: func(fakeClient client.Client) []*SyncState {
				emitter := &ResyncMockEmitter{eventType: "empty-event"}
				return []*SyncState{
					{
						Registration: &EmitterRegistration{
							ListFunc: func() ([]client.Object, error) {
								configMapList := &corev1.ConfigMapList{}
								err := fakeClient.List(context.Background(), configMapList)
								if err != nil {
									return nil, err
								}
								return []client.Object{}, nil // Empty list
							},
							Emitter: emitter,
						},
						NextResyncAt: time.Now().Add(-1 * time.Hour),
					},
				}
			},
			expectError:   false,
			expectResync:  false, // Should not call resync for empty list
			expectedCount: 0,
		},
		{
			name:         "list function returns error",
			eventType:    "error-event",
			setupObjects: []client.Object{},
			setupStates: func(fakeClient client.Client) []*SyncState {
				emitter := &ResyncMockEmitter{eventType: "error-event"}
				return []*SyncState{
					{
						Registration: &EmitterRegistration{
							ListFunc: func() ([]client.Object, error) {
								return nil, errors.New("list operation failed")
							},
							Emitter: emitter,
						},
						NextResyncAt: time.Now().Add(-1 * time.Hour),
					},
				}
			},
			expectError:   true,
			expectResync:  false,
			expectedCount: 0,
			errorContains: "failed to list objects for event type error-event",
		},
		{
			name:         "emitter resync returns error",
			eventType:    "resync-error-event",
			setupObjects: []client.Object{configMap1},
			setupStates: func(fakeClient client.Client) []*SyncState {
				emitter := &ResyncMockEmitter{
					eventType:   "resync-error-event",
					resyncError: errors.New("resync operation failed"),
				}
				return []*SyncState{
					{
						Registration: &EmitterRegistration{
							ListFunc: func() ([]client.Object, error) {
								configMapList := &corev1.ConfigMapList{}
								err := fakeClient.List(context.Background(), configMapList)
								if err != nil {
									return nil, err
								}
								objects := make([]client.Object, len(configMapList.Items))
								for i := range configMapList.Items {
									objects[i] = &configMapList.Items[i]
								}
								return objects, nil
							},
							Emitter: emitter,
						},
						NextResyncAt: time.Now().Add(-1 * time.Hour),
					},
				}
			},
			expectError:   true,
			expectResync:  true, // Should call resync but fail
			expectedCount: 1,
			errorContains: "failed to resync objects for event type resync-error-event",
		},
		{
			name:         "no emitter registered for event type",
			eventType:    "unregistered-event",
			setupObjects: []client.Object{configMap1},
			setupStates: func(fakeClient client.Client) []*SyncState {
				emitter := &ResyncMockEmitter{eventType: "different-event"}
				return []*SyncState{
					{
						Registration: &EmitterRegistration{
							ListFunc: func() ([]client.Object, error) {
								return []client.Object{configMap1}, nil
							},
							Emitter: emitter,
						},
						NextResyncAt: time.Now().Add(-1 * time.Hour),
					},
				}
			},
			expectError:   false,
			expectResync:  false,
			expectedCount: 0,
		},
		{
			name:         "multiple emitters - only matching one should resync",
			eventType:    "target-event",
			setupObjects: []client.Object{configMap1, secret1},
			setupStates: func(fakeClient client.Client) []*SyncState {
				targetEmitter := &ResyncMockEmitter{eventType: "target-event"}
				otherEmitter := &ResyncMockEmitter{eventType: "other-event"}
				return []*SyncState{
					{
						Registration: &EmitterRegistration{
							ListFunc: func() ([]client.Object, error) {
								return []client.Object{secret1}, nil
							},
							Emitter: otherEmitter,
						},
						NextResyncAt: time.Now().Add(-1 * time.Hour),
					},
					{
						Registration: &EmitterRegistration{
							ListFunc: func() ([]client.Object, error) {
								configMapList := &corev1.ConfigMapList{}
								err := fakeClient.List(context.Background(), configMapList)
								if err != nil {
									return nil, err
								}
								objects := make([]client.Object, len(configMapList.Items))
								for i := range configMapList.Items {
									objects[i] = &configMapList.Items[i]
								}
								return objects, nil
							},
							Emitter: targetEmitter,
						},
						NextResyncAt: time.Now().Add(-1 * time.Hour),
					},
				}
			},
			expectError:   false,
			expectResync:  true,
			expectedCount: 1, // Only configmap should be resynced
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with test objects
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.setupObjects...).
				Build()

			// Setup the PeriodicSyncer with test states
			syncer := &PeriodicSyncer{
				syncStates: tt.setupStates(fakeClient),
			}

			// Store original NextResyncAt times to verify updates
			originalResyncTimes := make([]time.Time, len(syncer.syncStates))
			for i, state := range syncer.syncStates {
				originalResyncTimes[i] = state.NextResyncAt
			}

			// Execute the Resync function
			err := syncer.Resync(context.Background(), tt.eventType)

			// Verify error expectations
			if tt.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errorContains != "" {
					if !contains(err.Error(), tt.errorContains) {
						t.Errorf("expected error to contain %q, got %q", tt.errorContains, err.Error())
					}
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}

			// Verify resync behavior
			var targetEmitter *ResyncMockEmitter
			for i, state := range syncer.syncStates {
				if state.Registration.Emitter.EventType() == tt.eventType {
					if mockEmitter, ok := state.Registration.Emitter.(*ResyncMockEmitter); ok {
						targetEmitter = mockEmitter

						// Verify NextResyncAt was updated if resync was successful
						if tt.expectResync && !tt.expectError {
							if !state.NextResyncAt.After(originalResyncTimes[i]) {
								t.Error("expected NextResyncAt to be updated after successful resync")
							}
						}
					}
					break
				}
			}

			if tt.expectResync && targetEmitter != nil {
				if !targetEmitter.resyncCalled {
					t.Error("expected Resync to be called on emitter")
				}
				if len(targetEmitter.resyncObjects) != tt.expectedCount {
					t.Errorf("expected %d objects to be resynced, got %d", tt.expectedCount, len(targetEmitter.resyncObjects))
				}
			} else if !tt.expectResync && targetEmitter != nil {
				if targetEmitter.resyncCalled {
					t.Error("expected Resync NOT to be called on emitter")
				}
			}

			// For successful cases with objects, verify the object content
			if !tt.expectError && tt.expectResync && targetEmitter != nil && len(targetEmitter.resyncObjects) > 0 {
				// Verify we got the correct objects
				firstObj := targetEmitter.resyncObjects[0]
				switch obj := firstObj.(type) {
				case *corev1.ConfigMap:
					if obj.Name == "" {
						t.Error("expected ConfigMap to have a name")
					}
				case *corev1.Secret:
					if obj.Name == "" {
						t.Error("expected Secret to have a name")
					}
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		})()
}
