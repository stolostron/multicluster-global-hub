package transporter

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var errMigrationACLSetup = errors.New("migration ACL controller registration failed")

// Predicate Tests - These test critical watch logic for Kafka-related resources

const testSecretName = "some-secret"

// assertPredicateAllEvents asserts Create, Update, and Delete predicate results for the same expected value.
func assertPredicateAllEvents(t *testing.T, pred predicate.Funcs, obj client.Object, want bool) {
	t.Helper()
	assert.Equal(t, want, pred.Create(event.CreateEvent{Object: obj}), "CreateFunc")
	assert.Equal(t, want, pred.Update(event.UpdateEvent{ObjectNew: obj}), "UpdateFunc")
	assert.Equal(t, want, pred.Delete(event.DeleteEvent{Object: obj}), "DeleteFunc")
}

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
			name: "kafka user secret with strimzi labels should match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka-user-secret",
					Namespace: utils.GetDefaultNamespace(),
					Labels: map[string]string{
						StrimziClusterLabel: protocol.KafkaClusterName,
						StrimziKindLabel:    StrimziKindUser,
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
						StrimziClusterLabel: "wrong-cluster",
						StrimziKindLabel:    StrimziKindUser,
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
						StrimziClusterLabel: protocol.KafkaClusterName,
						StrimziKindLabel:    "Kafka",
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

func TestNetworkPolicyPredicate_Transport(t *testing.T) {
	tests := []struct {
		name     string
		obj      *networkingv1.NetworkPolicy
		wantBool bool
	}{
		{
			name: "kafka network policy should match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      protocol.KafkaClusterName,
					Namespace: utils.GetDefaultNamespace(),
				},
			},
			wantBool: true,
		},
		{
			name: "kafka network policy in wrong namespace should not match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      protocol.KafkaClusterName,
					Namespace: "other-namespace",
				},
			},
			wantBool: false,
		},
		{
			name: "other network policy should not match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-policy",
					Namespace: utils.GetDefaultNamespace(),
				},
			},
			wantBool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertPredicateAllEvents(t, networkPolicyPred, tt.obj, tt.wantBool)
		})
	}
}

// Helper Function Tests - These test the predicate condition logic

func TestNetworkPolicyCond(t *testing.T) {
	tests := []struct {
		name     string
		objName  string
		expected bool
	}{
		{
			name:     "kafka cluster name matches",
			objName:  protocol.KafkaClusterName,
			expected: true,
		},
		{
			name:     "other name does not match",
			objName:  "other-name",
			expected: false,
		},
		{
			name:     "empty name does not match",
			objName:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.objName,
					Namespace: utils.GetDefaultNamespace(),
				},
			}
			result := networkPolicyCond(obj)
			assert.Equal(t, tt.expected, result)
		})
	}
}

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
			name: "kafka user secret with both labels matches",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: testSecretName,
					Labels: map[string]string{
						StrimziClusterLabel: protocol.KafkaClusterName,
						StrimziKindLabel:    StrimziKindUser,
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
						StrimziClusterLabel: protocol.KafkaClusterName,
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
						StrimziKindLabel: StrimziKindUser,
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
			assert.Equal(t, tt.expected, result)
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

func TestNetworkPolicyCondCustomNamespace(t *testing.T) {
	// networkPolicyCond uses kafkaNetworkPolicyWatchNamespace when set;
	// verify it does NOT fall back to the default namespace.
	originalNS := kafkaNetworkPolicyWatchNamespace
	t.Cleanup(func() { kafkaNetworkPolicyWatchNamespace = originalNS })

	customNamespace := "custom-kafka-namespace"
	kafkaNetworkPolicyWatchNamespace = customNamespace

	tests := []struct {
		name     string
		obj      *networkingv1.NetworkPolicy
		expected bool
	}{
		{
			name: "kafka NP in custom namespace should match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      protocol.KafkaClusterName,
					Namespace: customNamespace,
				},
			},
			expected: true,
		},
		{
			name: "kafka NP in default namespace should NOT match when custom namespace is set",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      protocol.KafkaClusterName,
					Namespace: utils.GetDefaultNamespace(),
				},
			},
			expected: false,
		},
		{
			name: "unknown NP in custom namespace should not match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-policy",
					Namespace: customNamespace,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, networkPolicyCond(tt.obj))
		})
	}
}
