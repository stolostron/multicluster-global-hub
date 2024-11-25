package hubmanagement

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

// mockHubManagement implements a mock hub management process for testing.
type mockHubManagement struct{}

func (m *mockHubManagement) inactive(ctx context.Context, hubs []models.LeafHubHeartbeat) error {
	return nil
}

func (m *mockHubManagement) reactive(ctx context.Context, hubs []models.LeafHubHeartbeat) error {
	return nil
}

func TestManagerClusterAddonController_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	err := addonv1alpha1.AddToScheme(scheme)
	assert.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	ctx := context.Background()
	controller := &managerClusterAddonController{
		client: fakeClient,
	}

	t.Run("addon not found", func(t *testing.T) {
		request := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "test-namespace",
				Name:      "test-addon",
			},
		}

		res, err := controller.Reconcile(ctx, request)
		assert.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, res)
	})

	t.Run("addon with deletion timestamp", func(t *testing.T) {
		now := metav1.Now()
		addon := &addonv1alpha1.ManagedClusterAddOn{
			ObjectMeta: metav1.ObjectMeta{
				Name:              constants.GHManagedClusterAddonName,
				Namespace:         "test-namespace",
				DeletionTimestamp: &now,
				Finalizers:        []string{"deleting"},
			},
		}

		err := fakeClient.Create(ctx, addon)
		assert.NoError(t, err)

		request := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "test-namespace",
				Name:      constants.GHManagedClusterAddonName,
			},
		}

		err = fakeClient.Delete(ctx, addon)
		assert.NoError(t, err)

		res, err := controller.Reconcile(ctx, request)
		assert.NoError(t, err)
		assert.Equal(t, ctrl.Result{Requeue: true, RequeueAfter: 5 * time.Second}, res)

		hubStatusManager = &mockHubManagement{}
		res, err = controller.Reconcile(ctx, request)
		assert.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, res)
	})
}
