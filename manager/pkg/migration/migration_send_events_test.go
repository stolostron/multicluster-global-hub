package migration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cetypes "github.com/cloudevents/sdk-go/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
)

func TestSendEventToSourceHub_usesMigrationEventExtensions(t *testing.T) {
	mockProducer := &MockProducer{}
	controller := &ClusterMigrationController{Producer: mockProducer}

	migration := &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-migration",
			Namespace: utils.GetDefaultNamespace(),
			UID:       types.UID("uid-source-hub"),
		},
		Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
			From: "source-hub",
			To:   "target-hub",
		},
	}

	require.NoError(t, controller.sendEventToSourceHub(
		context.Background(),
		"source-hub",
		migration,
		migrationv1alpha1.PhaseDeploying,
		[]string{"cluster1"},
		nil,
		"",
	))

	require.Len(t, mockProducer.SentEvents, 1)
	evt := mockProducer.SentEvents[0]
	assert.Equal(t, constants.MigrationSourceMsgKey, evt.Type())

	clusterName, err := cetypes.ToString(evt.Extensions()[constants.CloudEventExtensionKeyClusterName])
	require.NoError(t, err)
	assert.Equal(t, "source-hub", clusterName)

	migrationID, err := cetypes.ToString(evt.Extensions()[constants.CloudEventExtensionKeyMigrationId])
	require.NoError(t, err)
	assert.Equal(t, "uid-source-hub", migrationID)

	stage, err := cetypes.ToString(evt.Extensions()[constants.CloudEventExtensionKeyMigrationStage])
	require.NoError(t, err)
	assert.Equal(t, migrationv1alpha1.PhaseDeploying, stage)

	var payload migrationbundle.MigrationSourceBundle
	require.NoError(t, json.Unmarshal(evt.Data(), &payload))
	assert.Equal(t, "source-hub", payload.ToHub)
	assert.Equal(t, []string{"cluster1"}, payload.ManagedClusters)
}

func TestSendEventToTargetHub_usesMigrationEventExtensions(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clusterv1.Install(scheme))
	require.NoError(t, migrationv1alpha1.AddToScheme(scheme))

	targetHub := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "target-hub"}}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(targetHub).Build()

	mockProducer := &MockProducer{}
	controller := &ClusterMigrationController{
		Producer: mockProducer,
		Client:   fakeClient,
	}

	migration := &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-migration",
			Namespace: utils.GetDefaultNamespace(),
			UID:       types.UID("uid-target-hub"),
			Annotations: map[string]string{
				"global-hub.open-cluster-management.io/managed-serviceaccount-install-namespace": "addon-ns",
			},
		},
		Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
			From: "source-hub",
			To:   "target-hub",
		},
	}

	require.NoError(t, controller.sendEventToTargetHub(
		context.Background(),
		migration,
		migrationv1alpha1.PhaseInitializing,
		[]string{"cluster1"},
		"",
	))

	require.Len(t, mockProducer.SentEvents, 1)
	evt := mockProducer.SentEvents[0]
	assert.Equal(t, constants.MigrationTargetMsgKey, evt.Type())

	clusterName, err := cetypes.ToString(evt.Extensions()[constants.CloudEventExtensionKeyClusterName])
	require.NoError(t, err)
	assert.Equal(t, "target-hub", clusterName)

	expireStr, err := cetypes.ToString(evt.Extensions()[constants.CloudEventExtensionKeyExpireTime])
	require.NoError(t, err)
	expireTime, err := time.Parse(time.RFC3339, expireStr)
	require.NoError(t, err)
	assert.True(t, expireTime.After(time.Now()))

	var payload migrationbundle.MigrationTargetBundle
	require.NoError(t, json.Unmarshal(evt.Data(), &payload))
	assert.Equal(t, "addon-ns", payload.ManagedServiceAccountInstallNamespace)
}

func TestSendEventToSourceHub_propagatesProducerError(t *testing.T) {
	mockProducer := &MockProducer{SendError: errors.New("send failed")}
	controller := &ClusterMigrationController{Producer: mockProducer}
	migration := &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{UID: types.UID("uid-err")},
		Spec:       migrationv1alpha1.ManagedClusterMigrationSpec{From: "hub1", To: "hub2"},
	}

	err := controller.sendEventToSourceHub(context.Background(), "hub1", migration,
		migrationv1alpha1.PhaseDeploying, nil, nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to sync managedclustermigration event")
}

func TestSendEventToTargetHub_usesAddonStatusNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clusterv1.Install(scheme))
	require.NoError(t, migrationv1alpha1.AddToScheme(scheme))
	require.NoError(t, addonapiv1alpha1.AddToScheme(scheme))

	targetHub := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "target-hub"}}
	addon := &addonapiv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{Name: "managed-serviceaccount", Namespace: "target-hub"},
		Status:     addonapiv1alpha1.ManagedClusterAddOnStatus{Namespace: "addon-from-status"},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(targetHub, addon).Build()

	mockProducer := &MockProducer{}
	controller := &ClusterMigrationController{
		Producer: mockProducer,
		Client:   fakeClient,
	}

	migration := &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-migration",
			Namespace: utils.GetDefaultNamespace(),
			UID:       types.UID("uid-addon-ns"),
		},
		Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
			From: "source-hub",
			To:   "target-hub",
		},
	}

	require.NoError(t, controller.sendEventToTargetHub(
		context.Background(),
		migration,
		migrationv1alpha1.PhaseRegistering,
		[]string{"cluster1"},
		"",
	))

	var payload migrationbundle.MigrationTargetBundle
	require.NoError(t, json.Unmarshal(mockProducer.SentEvents[0].Data(), &payload))
	assert.Equal(t, "addon-from-status", payload.ManagedServiceAccountInstallNamespace)
}
