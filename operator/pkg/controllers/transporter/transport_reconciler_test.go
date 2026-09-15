package transporter

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var errMigrationACLSetup = errors.New("migration ACL controller registration failed")

// Predicate Tests - These test critical watch logic for Kafka-related resources

const (
	testSecretName      = "some-secret"
	strimziClusterLabel = "strimzi.io/cluster"
	strimziKindLabel    = "strimzi.io/kind"
	strimziKindUser     = "KafkaUser"
)

// assertPredicateAllEvents asserts Create, Update, and Delete predicate results for the same expected value.
func assertPredicateAllEvents(t *testing.T, pred predicate.Funcs, obj client.Object, want bool) {
	t.Helper()
	assert.Equal(t, want, pred.Create(event.CreateEvent{Object: obj}),
		"CreateFunc: transport secret %q must match for reconciliation: %v", obj.GetName(), want)
	assert.Equal(t, want, pred.Update(event.UpdateEvent{ObjectNew: obj}),
		"UpdateFunc: transport secret %q must match for reconciliation: %v", obj.GetName(), want)
	assert.Equal(t, want, pred.Delete(event.DeleteEvent{Object: obj}),
		"DeleteFunc: transport secret %q must match for reconciliation: %v", obj.GetName(), want)
}

// TestSecretPredicate covers create/update/delete filters for transport secrets.
func TestSecretPredicate(t *testing.T) {
	tests := []struct {
		name     string
		obj      *corev1.Secret
		wantBool bool
	}{
		{
			name: "transport secret should match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.GHTransportSecretName,
					Namespace: utils.GetDefaultNamespace(),
				},
			},
			wantBool: true,
		},
		{
			name: "per-hub BYO transport secret should match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.GHTransportSecretNameForCluster("hub1"),
					Namespace: utils.GetDefaultNamespace(),
				},
			},
			wantBool: true,
		},
		{
			name: "kafka user secret with strimzi labels should match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka-user-secret",
					Namespace: utils.GetDefaultNamespace(),
					Labels: map[string]string{
						strimziClusterLabel: protocol.KafkaClusterName,
						strimziKindLabel:    strimziKindUser,
					},
				},
			},
			wantBool: true,
		},
		{
			name: "kafka secret with wrong cluster name should not match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka-user-secret",
					Namespace: utils.GetDefaultNamespace(),
					Labels: map[string]string{
						strimziClusterLabel: "wrong-cluster",
						strimziKindLabel:    strimziKindUser,
					},
				},
			},
			wantBool: false,
		},
		{
			name: "kafka secret with wrong kind should not match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka-secret",
					Namespace: utils.GetDefaultNamespace(),
					Labels: map[string]string{
						strimziClusterLabel: protocol.KafkaClusterName,
						strimziKindLabel:    "Kafka",
					},
				},
			},
			wantBool: false,
		},
		{
			name: "other secret should not match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-secret",
					Namespace: utils.GetDefaultNamespace(),
				},
			},
			wantBool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertPredicateAllEvents(t, secretPred, tt.obj, tt.wantBool)
		})
	}
}

// TestSecretCond matches shared and per-hub BYO transport secret names.
func TestSecretCond(t *testing.T) {
	tests := []struct {
		name     string
		obj      *corev1.Secret
		expected bool
	}{
		{
			name: "transport secret name matches",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.GHTransportSecretName,
				},
			},
			expected: true,
		},
		{
			name: "per-hub BYO transport secret name matches",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.GHTransportSecretNameForCluster("hub1"),
				},
			},
			expected: true,
		},
		{
			name: "kafka user secret with both labels matches",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: testSecretName,
					Labels: map[string]string{
						strimziClusterLabel: protocol.KafkaClusterName,
						strimziKindLabel:    strimziKindUser,
					},
				},
			},
			expected: true,
		},
		{
			name: "kafka secret with only cluster label does not match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: testSecretName,
					Labels: map[string]string{
						strimziClusterLabel: protocol.KafkaClusterName,
					},
				},
			},
			expected: false,
		},
		{
			name: "kafka secret with only kind label does not match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: testSecretName,
					Labels: map[string]string{
						strimziKindLabel: strimziKindUser,
					},
				},
			},
			expected: false,
		},
		{
			name: "other secret does not match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "other-secret",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := secretCond(tt.obj)
			assert.Equal(t, tt.expected, result,
				"transport secret %q must match for reconciliation: %v", tt.obj.Name, tt.expected)
		})
	}
}

// State Management Tests

func TestTransportReconcilerIsResourceRemoved(t *testing.T) {
	r := &TransportReconciler{}

	// Test with default state (should be true initially)
	originalState := isResourceRemoved
	defer func() {
		isResourceRemoved = originalState
	}()

	// Test when isResourceRemoved is true
	isResourceRemoved = true
	result := r.IsResourceRemoved()
	assert.True(t, result)

	// Test when isResourceRemoved is false
	isResourceRemoved = false
	result = r.IsResourceRemoved()
	assert.False(t, result)
}

func TestStartControllerSingleton(t *testing.T) {
	// Save and restore the singleton so we don't affect other tests.
	original := transportReconciler
	originalMigrationACL := migrationACLControllerStarted
	t.Cleanup(func() {
		transportReconciler = original
		migrationACLControllerStarted = originalMigrationACL
	})

	existing := &TransportReconciler{}
	transportReconciler = existing
	migrationACLControllerStarted = true

	// Second call must return the pre-existing instance without error.
	controller, err := StartController(config.ControllerOption{})
	assert.NoError(t, err, "StartController should succeed when transport singleton is already initialized")
	assert.Equal(t, existing, controller, "StartController should reuse the existing transport controller instance")
}

func TestStartControllerRetriesMigrationACLSetup(t *testing.T) {
	originalTransport := transportReconciler
	originalMigrationACL := migrationACLControllerStarted
	originalSetup := migrationACLReconcilerSetup
	t.Cleanup(func() {
		transportReconciler = originalTransport
		migrationACLControllerStarted = originalMigrationACL
		migrationACLReconcilerSetup = originalSetup
	})

	scheme := runtime.NewScheme()
	if err := migrationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(ManagedClusterMigration) error = %v", err)
	}
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{migrationv1alpha1.GroupVersion})
	mapper.Add(migrationv1alpha1.GroupVersion.WithKind("ManagedClusterMigration"), meta.RESTScopeNamespace)

	transportReconciler = &TransportReconciler{}
	migrationACLControllerStarted = false
	migrationACLReconcilerSetup = func(ctrl.Manager) error { return errMigrationACLSetup }

	mgr := &migrationACLSetupFailManager{
		client:     fake.NewClientBuilder().WithScheme(scheme).Build(),
		scheme:     scheme,
		restMapper: mapper,
	}

	controller, err := StartController(config.ControllerOption{Manager: mgr})
	assert.ErrorIs(t, err, errMigrationACLSetup, "StartController should return migration ACL setup error")
	assert.Nil(t, controller, "StartController should not return a controller when migration ACL setup fails")
	assert.False(
		t,
		migrationACLControllerStarted,
		"migration ACL controller must not be marked started after setup failure",
	)
}

type migrationACLSetupFailManager struct {
	client     client.Client
	scheme     *runtime.Scheme
	restMapper meta.RESTMapper
	addErr     error
}

func (m *migrationACLSetupFailManager) Add(manager.Runnable) error { return m.addErr }
func (m *migrationACLSetupFailManager) GetClient() client.Client   { return m.client }
func (m *migrationACLSetupFailManager) GetScheme() *runtime.Scheme { return m.scheme }
func (m *migrationACLSetupFailManager) GetFieldIndexer() client.FieldIndexer {
	return nil
}
func (m *migrationACLSetupFailManager) GetCache() cache.Cache { return nil }
func (m *migrationACLSetupFailManager) GetEventRecorderFor(string) record.EventRecorder {
	return nil
}
func (m *migrationACLSetupFailManager) GetRESTMapper() meta.RESTMapper { return m.restMapper }
func (m *migrationACLSetupFailManager) GetAPIReader() client.Reader    { return m.client }
func (m *migrationACLSetupFailManager) Start(context.Context) error    { return nil }
func (m *migrationACLSetupFailManager) GetWebhookServer() webhook.Server {
	return nil
}
func (m *migrationACLSetupFailManager) GetLogger() logr.Logger { return logr.Discard() }
func (m *migrationACLSetupFailManager) GetControllerOptions() crconfig.Controller {
	return crconfig.Controller{}
}
func (m *migrationACLSetupFailManager) Elected() <-chan struct{} { return nil }
func (m *migrationACLSetupFailManager) AddHealthzCheck(string, healthz.Checker) error {
	return nil
}

func (m *migrationACLSetupFailManager) AddReadyzCheck(string, healthz.Checker) error {
	return nil
}
func (m *migrationACLSetupFailManager) GetHTTPClient() *http.Client { return nil }
func (m *migrationACLSetupFailManager) AddMetricsServerExtraHandler(string, http.Handler) error {
	return nil
}
func (m *migrationACLSetupFailManager) GetConfig() *rest.Config { return nil }

var _ ctrl.Manager = (*migrationACLSetupFailManager)(nil)

func TestAMigrationACLReconcilerSetupSuccess(t *testing.T) {
	original := migrationACLControllerStarted
	migrationACLControllerStarted = false
	t.Cleanup(func() { migrationACLControllerStarted = original })

	scheme := runtime.NewScheme()
	if err := migrationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(ManagedClusterMigration) error = %v", err)
	}
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{migrationv1alpha1.GroupVersion})
	mapper.Add(migrationv1alpha1.GroupVersion.WithKind("ManagedClusterMigration"), meta.RESTScopeNamespace)

	mgr := &migrationACLSetupFailManager{
		client:     fake.NewClientBuilder().WithScheme(scheme).Build(),
		scheme:     scheme,
		restMapper: mapper,
	}

	if err := setupMigrationACLReconciler(mgr); err != nil {
		t.Fatalf("setupMigrationACLReconciler() error = %v", err)
	}
	if !migrationACLControllerStarted {
		t.Fatal("expected migration ACL controller to be marked started")
	}
}

func TestSyncManagerTransportConnAndPersist(t *testing.T) {
	ctx := context.Background()
	ns := utils.GetDefaultNamespace()
	scheme := runtime.NewScheme()
	if err := v1alpha4.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(MulticlusterGlobalHub) error = %v", err)
	}

	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "globalhub", Namespace: ns},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mgh).
		WithStatusSubresource(&v1alpha4.MulticlusterGlobalHub{}).
		Build()
	config.SetMGHNamespacedName(types.NamespacedName{Name: mgh.Name, Namespace: mgh.Namespace})
	t.Cleanup(func() {
		config.SetMGHNamespacedName(types.NamespacedName{})
		config.SetTransporterConn(nil)
	})

	mgr := &migrationACLSetupFailManager{
		client: fakeClient,
		scheme: scheme,
	}
	reconciler := &TransportReconciler{
		Manager:     mgr,
		transporter: &noopTransporter{},
	}

	result, err := reconciler.syncManagerTransportConnAndPersist(ctx)
	assert.NoError(t, err)
	assert.Equal(t, time.Duration(0), result.RequeueAfter)

	updatedMGH := &v1alpha4.MulticlusterGlobalHub{}
	err = fakeClient.Get(ctx, types.NamespacedName{Name: mgh.Name, Namespace: mgh.Namespace}, updatedMGH)
	assert.NoError(t, err)
	assert.Contains(t, updatedMGH.Status.Components, config.COMPONENTS_KAFKA_NAME)
}

func TestSyncManagerTransportConnAndPersistRequeue(t *testing.T) {
	ctx := context.Background()
	ns := utils.GetDefaultNamespace()
	scheme := runtime.NewScheme()
	if err := v1alpha4.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(MulticlusterGlobalHub) error = %v", err)
	}
	if err := kafkav1beta2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(Kafka) error = %v", err)
	}

	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "globalhub", Namespace: ns},
	}
	notReadyType := "Ready"
	notReadyStatus := "False"
	notReadyReason := "NotReady"
	notReadyMessage := "Kafka cluster not ready"
	notReadyKafka := &kafkav1beta2.Kafka{
		ObjectMeta: metav1.ObjectMeta{Name: protocol.KafkaClusterName, Namespace: ns},
		Spec: &kafkav1beta2.KafkaSpec{
			Kafka: kafkav1beta2.KafkaSpecKafka{
				Listeners: []kafkav1beta2.KafkaSpecKafkaListenersElem{
					{Name: "tls", Port: 9093, Type: "nodeport", Tls: true},
				},
			},
		},
		Status: &kafkav1beta2.KafkaStatus{
			Conditions: []kafkav1beta2.KafkaStatusConditionsElem{
				{
					Type:    &notReadyType,
					Status:  &notReadyStatus,
					Reason:  &notReadyReason,
					Message: &notReadyMessage,
				},
			},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mgh, notReadyKafka).Build()
	config.SetMGHNamespacedName(types.NamespacedName{Name: mgh.Name, Namespace: mgh.Namespace})
	t.Cleanup(func() {
		config.SetMGHNamespacedName(types.NamespacedName{})
		config.SetTransporter(nil)
		config.SetTransporterConn(nil)
	})

	mgr := &migrationACLSetupFailManager{client: fakeClient, scheme: scheme}
	reconciler := &TransportReconciler{
		Manager:     mgr,
		transporter: protocol.NewStrimziTransporter(mgr, mgh, protocol.WithContext(ctx)),
	}

	result, err := reconciler.syncManagerTransportConnAndPersist(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 5*time.Second, result.RequeueAfter)
}

func TestSyncManagerTransportConnAndPersistError(t *testing.T) {
	ctx := context.Background()
	ns := utils.GetDefaultNamespace()
	scheme := runtime.NewScheme()
	if err := v1alpha4.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(MulticlusterGlobalHub) error = %v", err)
	}
	if err := kafkav1beta2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(Kafka) error = %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(corev1) error = %v", err)
	}

	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "globalhub", Namespace: ns},
	}
	readyType := "Ready"
	readyStatus := "True"
	bootServer := "kafka-kafka-bootstrap.example.svc:9092"
	clusterID := "test-cluster-id"
	readyKafka := &kafkav1beta2.Kafka{
		ObjectMeta: metav1.ObjectMeta{Name: protocol.KafkaClusterName, Namespace: ns},
		Spec: &kafkav1beta2.KafkaSpec{
			Kafka: kafkav1beta2.KafkaSpecKafka{
				Listeners: []kafkav1beta2.KafkaSpecKafkaListenersElem{
					{Name: "tls", Port: 9093, Type: "nodeport", Tls: true},
				},
			},
		},
		Status: &kafkav1beta2.KafkaStatus{
			ClusterId: &clusterID,
			Listeners: []kafkav1beta2.KafkaStatusListenersElem{
				{BootstrapServers: &bootServer, Certificates: []string{"cert"}},
			},
			Conditions: []kafkav1beta2.KafkaStatusConditionsElem{
				{Type: &readyType, Status: &readyStatus},
			},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mgh, readyKafka).Build()
	config.SetMGHNamespacedName(types.NamespacedName{Name: mgh.Name, Namespace: mgh.Namespace})
	t.Cleanup(func() {
		config.SetMGHNamespacedName(types.NamespacedName{})
		config.SetTransporter(nil)
		config.SetTransporterConn(nil)
	})

	mgr := &migrationACLSetupFailManager{client: fakeClient, scheme: scheme}
	reconciler := &TransportReconciler{
		Manager:     mgr,
		transporter: protocol.NewStrimziTransporter(mgr, mgh, protocol.WithContext(ctx)),
	}

	_, err := reconciler.syncManagerTransportConnAndPersist(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get manager transport connection")
}

type noopTransporter struct{}

func (n *noopTransporter) EnsureUser(clusterName string) (string, error) { return "", nil }
func (n *noopTransporter) EnsureTopic(clusterName string) (*transport.ClusterTopic, error) {
	return nil, nil
}

func (n *noopTransporter) GetConnCredential(clusterName string) (*transport.KafkaConfig, error) {
	return nil, nil
}
func (n *noopTransporter) EnsureKafka() (bool, error)     { return false, nil }
func (n *noopTransporter) Prune(clusterName string) error { return nil }
