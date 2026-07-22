// Copyright Contributors to the Open Cluster Management project.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package hubha

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func configMapUnstructured(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": "default",
			},
		},
	}
}

func hubHAResourceEvent(t *testing.T, bundle *generic.GenericBundle[*unstructured.Unstructured]) *cloudevents.Event {
	t.Helper()

	evt := cloudevents.NewEvent()
	evt.SetType(constants.HubHAResourcesMsgKey)
	evt.SetSource("hub1")
	if err := evt.SetData(cloudevents.ApplicationJSON, bundle); err != nil {
		t.Fatalf("SetData() error = %v", err)
	}
	return &evt
}

func assertSyncAggregateErrors(t *testing.T, err error, wantCount int, wantMessages ...string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected aggregate sync error when bundle apply fails")
	}

	joined := stderrors.Unwrap(err)
	if joined == nil {
		t.Fatalf("expected wrapped aggregate error, got: %v", err)
	}

	unwrapMulti, ok := joined.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("expected errors.Join aggregate, got: %v", joined)
	}

	syncErrs := unwrapMulti.Unwrap()
	if len(syncErrs) != wantCount {
		t.Fatalf("expected %d aggregated errors, got %d: %v", wantCount, len(syncErrs), syncErrs)
	}

	for _, want := range wantMessages {
		found := false
		for _, syncErr := range syncErrs {
			if strings.Contains(syncErr.Error(), want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected %q in aggregate errors, got: %v", want, syncErrs)
		}
	}
}

func TestHubHAStandbySyncer_Sync_ReturnsAggregateErrorsOnCreateUpdateResync(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}

	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: existingCMName, Namespace: "default"},
	}
	resyncCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "resync-cm", Namespace: "default"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing, resyncCM).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				return fmt.Errorf("create failure")
			},
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				return fmt.Errorf("update failure")
			},
		}).
		Build()

	syncer := NewHubHAStandbySyncer(fakeClient)

	bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
	bundle.Create = []*unstructured.Unstructured{configMapUnstructured("new-cm")}
	bundle.Update = []*unstructured.Unstructured{configMapUnstructured(existingCMName)}
	bundle.Resync = []*unstructured.Unstructured{configMapUnstructured("resync-cm")}

	err := syncer.Sync(context.Background(), hubHAResourceEvent(t, bundle))
	assertSyncAggregateErrors(t, err, 3, "create failure", "update failure")
}
