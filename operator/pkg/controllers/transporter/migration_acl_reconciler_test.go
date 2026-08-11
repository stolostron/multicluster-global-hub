// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
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

package transporter

import (
	"context"
	"net/http"
	"testing"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	operatorv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	operatorconfig "github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestDeployingSourceHubs(t *testing.T) {
	t.Parallel()

	list := &migrationv1alpha1.ManagedClusterMigrationList{
		Items: []migrationv1alpha1.ManagedClusterMigration{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "active"},
				Spec:       migrationv1alpha1.ManagedClusterMigrationSpec{From: "hub1"},
				Status:     migrationv1alpha1.ManagedClusterMigrationStatus{Phase: migrationv1alpha1.PhaseDeploying},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "done"},
				Spec:       migrationv1alpha1.ManagedClusterMigrationSpec{From: "hub2"},
				Status:     migrationv1alpha1.ManagedClusterMigrationStatus{Phase: migrationv1alpha1.PhaseCompleted},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "deleted",
					DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
				},
				Spec:   migrationv1alpha1.ManagedClusterMigrationSpec{From: "hub3"},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{Phase: migrationv1alpha1.PhaseDeploying},
			},
		},
	}

	got := deployingSourceHubs(list)
	if len(got) != 1 {
		t.Fatalf("deployingSourceHubs() len = %d, want 1", len(got))
	}
	if _, ok := got["hub1"]; !ok {
		t.Fatalf("deployingSourceHubs() = %#v, want hub1 only", got)
	}
}

func TestHubsToSyncForMigration(t *testing.T) {
	t.Parallel()

	needed := map[string]struct{}{
		"hub1": {},
		"hub2": {},
	}
	got := hubsToSyncForMigration("hub3", needed)
	if len(got) != 3 {
		t.Fatalf("hubsToSyncForMigration() len = %d, want 3", len(got))
	}
}

func TestManagedHubFromKafkaUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		user    *kafkav1beta2.KafkaUser
		wantHub string
		wantOK  bool
	}{
		{
			name:    "managed hub user",
			user:    &kafkav1beta2.KafkaUser{ObjectMeta: metav1.ObjectMeta{Name: "hub1-kafka-user"}},
			wantHub: "hub1",
			wantOK:  true,
		},
		{
			name:   "global hub user skipped",
			user:   &kafkav1beta2.KafkaUser{ObjectMeta: metav1.ObjectMeta{Name: protocol.DefaultGlobalHubKafkaUserName}},
			wantOK: false,
		},
		{
			name:   "non kafka-user skipped",
			user:   &kafkav1beta2.KafkaUser{ObjectMeta: metav1.ObjectMeta{Name: "other-user"}},
			wantOK: false,
		},
		{
			name:   "local cluster skipped",
			user:   &kafkav1beta2.KafkaUser{ObjectMeta: metav1.ObjectMeta{Name: constants.LocalClusterName + "-kafka-user"}},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHub, gotOK := managedHubFromKafkaUser(tt.user)
			if gotHub != tt.wantHub || gotOK != tt.wantOK {
				t.Fatalf("managedHubFromKafkaUser() = (%q, %v), want (%q, %v)", gotHub, gotOK, tt.wantHub, tt.wantOK)
			}
		})
	}
}

func TestMigrationACLReconcilerReconcileSkipsBYO(t *testing.T) {
	originalBYO := operatorconfig.IsBYOKafka()
	t.Cleanup(func() { operatorconfig.SetBYOKafka(originalBYO) })
	operatorconfig.SetBYOKafka(true)

	reconciler := &MigrationACLReconciler{}
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: clientObjectKey("test-ns", "migration-1"),
	})
	if err != nil {
		t.Fatalf("Reconcile() BYO skip error = %v", err)
	}
}

func TestMigrationACLReconcilerReconcileNotFound(t *testing.T) {
	originalBYO := operatorconfig.IsBYOKafka()
	t.Cleanup(func() { operatorconfig.SetBYOKafka(originalBYO) })
	operatorconfig.SetBYOKafka(false)
	operatorconfig.SetMigrationTopic("gh-migration")

	scheme := runtime.NewScheme()
	if err := operatorv1alpha4.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(MulticlusterGlobalHub) error = %v", err)
	}
	if err := migrationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(ManagedClusterMigration) error = %v", err)
	}
	if err := kafkav1beta2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(KafkaUser) error = %v", err)
	}

	mgh := &operatorv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "mgh", Namespace: "test-ns"},
	}
	kafkaUser := &kafkav1beta2.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{Name: "hub1-kafka-user", Namespace: "test-ns"},
		Spec: &kafkav1beta2.KafkaUserSpec{
			Authorization: simpleKafkaUserAuthorization(
				utils.WriteTopicACL("gh-migration"),
			),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mgh, kafkaUser).
		Build()

	reconciler := &MigrationACLReconciler{
		mgr:    &migrationACLReconcilerMockManager{client: fakeClient},
		Client: fakeClient,
	}
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: clientObjectKey("test-ns", "missing"),
	})
	if err != nil {
		t.Fatalf("Reconcile() missing migration should succeed, got %v", err)
	}

	updated := &kafkav1beta2.KafkaUser{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(kafkaUser), updated); err != nil {
		t.Fatalf("get updated kafka user: %v", err)
	}
	if updated.Spec.Authorization != nil {
		for _, acl := range updated.Spec.Authorization.Acls {
			if utils.GenerateACLKey(acl) == utils.GenerateACLKey(utils.WriteTopicACL("gh-migration")) {
				t.Fatal("expected migration write ACL to be revoked after migration deletion")
			}
		}
	}
}

func TestMigrationACLReconcilerReconcileDeployingGrantsACL(t *testing.T) {
	originalBYO := operatorconfig.IsBYOKafka()
	t.Cleanup(func() { operatorconfig.SetBYOKafka(originalBYO) })
	operatorconfig.SetBYOKafka(false)
	operatorconfig.SetMigrationTopic("gh-migration")

	_, fakeClient, _, kafkaUser := newMigrationACLReconcilerFixtures(t)
	migration := &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{Name: "migration-1", Namespace: "test-ns"},
		Spec:       migrationv1alpha1.ManagedClusterMigrationSpec{From: "hub1", To: "hub2"},
		Status:     migrationv1alpha1.ManagedClusterMigrationStatus{Phase: migrationv1alpha1.PhaseDeploying},
	}
	if err := fakeClient.Create(context.Background(), migration); err != nil {
		t.Fatalf("create migration: %v", err)
	}

	reconciler := &MigrationACLReconciler{
		mgr:    &migrationACLReconcilerMockManager{client: fakeClient},
		Client: fakeClient,
	}
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: clientObjectKey("test-ns", "migration-1"),
	})
	if err != nil {
		t.Fatalf("Reconcile() deploying migration error = %v", err)
	}

	updated := &kafkav1beta2.KafkaUser{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(kafkaUser), updated); err != nil {
		t.Fatalf("get updated kafka user: %v", err)
	}
	wantKey := utils.GenerateACLKey(utils.WriteTopicACL("gh-migration"))
	found := false
	for _, acl := range updated.Spec.Authorization.Acls {
		if utils.GenerateACLKey(acl) == wantKey {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected migration write ACL to be granted for deploying migration")
	}
}

func TestMigrationACLReconcilerReconcileCompletedRevokesACL(t *testing.T) {
	originalBYO := operatorconfig.IsBYOKafka()
	t.Cleanup(func() { operatorconfig.SetBYOKafka(originalBYO) })
	operatorconfig.SetBYOKafka(false)
	operatorconfig.SetMigrationTopic("gh-migration")

	scheme, fakeClient, _, kafkaUser := newMigrationACLReconcilerFixtures(t)
	_ = scheme
	kafkaUser.Spec.Authorization.Acls = []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
		utils.WriteTopicACL("gh-migration"),
	}
	if err := fakeClient.Update(context.Background(), kafkaUser); err != nil {
		t.Fatalf("update kafka user with migration ACL: %v", err)
	}

	migration := &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{Name: "migration-1", Namespace: "test-ns"},
		Spec:       migrationv1alpha1.ManagedClusterMigrationSpec{From: "hub1", To: "hub2"},
		Status:     migrationv1alpha1.ManagedClusterMigrationStatus{Phase: migrationv1alpha1.PhaseCompleted},
	}
	if err := fakeClient.Create(context.Background(), migration); err != nil {
		t.Fatalf("create migration: %v", err)
	}

	reconciler := &MigrationACLReconciler{
		mgr:    &migrationACLReconcilerMockManager{client: fakeClient},
		Client: fakeClient,
	}
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: clientObjectKey("test-ns", "migration-1"),
	})
	if err != nil {
		t.Fatalf("Reconcile() completed migration error = %v", err)
	}

	updated := &kafkav1beta2.KafkaUser{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(kafkaUser), updated); err != nil {
		t.Fatalf("get updated kafka user: %v", err)
	}
	if updated.Spec.Authorization != nil {
		for _, acl := range updated.Spec.Authorization.Acls {
			if utils.GenerateACLKey(acl) == utils.GenerateACLKey(utils.WriteTopicACL("gh-migration")) {
				t.Fatal("expected migration write ACL to be revoked after migration completed")
			}
		}
	}
}

func TestMigrationACLReconcilerReconcileSkipsPausedMGH(t *testing.T) {
	originalBYO := operatorconfig.IsBYOKafka()
	t.Cleanup(func() { operatorconfig.SetBYOKafka(originalBYO) })
	operatorconfig.SetBYOKafka(false)

	_, fakeClient, mgh, _ := newMigrationACLReconcilerFixtures(t)
	mgh.Annotations = map[string]string{"mgh-pause": "true"}
	if err := fakeClient.Update(context.Background(), mgh); err != nil {
		t.Fatalf("update paused mgh: %v", err)
	}

	reconciler := &MigrationACLReconciler{
		mgr:    &migrationACLReconcilerMockManager{client: fakeClient},
		Client: fakeClient,
	}
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: clientObjectKey("test-ns", "migration-1"),
	})
	if err != nil {
		t.Fatalf("Reconcile() paused mgh should succeed, got %v", err)
	}
}

func newMigrationACLReconcilerFixtures(
	t *testing.T,
) (*runtime.Scheme, client.Client, *operatorv1alpha4.MulticlusterGlobalHub, *kafkav1beta2.KafkaUser) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := operatorv1alpha4.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(MulticlusterGlobalHub) error = %v", err)
	}
	if err := migrationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(ManagedClusterMigration) error = %v", err)
	}
	if err := kafkav1beta2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(KafkaUser) error = %v", err)
	}

	mgh := &operatorv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "mgh", Namespace: "test-ns"},
	}
	kafkaUser := &kafkav1beta2.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{Name: "hub1-kafka-user", Namespace: "test-ns"},
		Spec: &kafkav1beta2.KafkaUserSpec{
			Authorization: simpleKafkaUserAuthorization(
				utils.ReadTopicACL("gh-spec", false),
			),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mgh, kafkaUser).
		Build()

	return scheme, fakeClient, mgh, kafkaUser
}

type migrationACLReconcilerMockManager struct {
	client client.Client
}

func (m *migrationACLReconcilerMockManager) Add(manager.Runnable) error { return nil }
func (m *migrationACLReconcilerMockManager) GetClient() client.Client   { return m.client }
func (m *migrationACLReconcilerMockManager) GetScheme() *runtime.Scheme { return nil }
func (m *migrationACLReconcilerMockManager) GetFieldIndexer() client.FieldIndexer {
	return nil
}
func (m *migrationACLReconcilerMockManager) GetCache() cache.Cache { return nil }
func (m *migrationACLReconcilerMockManager) GetEventRecorderFor(string) record.EventRecorder {
	return nil
}
func (m *migrationACLReconcilerMockManager) GetRESTMapper() meta.RESTMapper { return nil }
func (m *migrationACLReconcilerMockManager) GetAPIReader() client.Reader    { return nil }
func (m *migrationACLReconcilerMockManager) Start(context.Context) error    { return nil }
func (m *migrationACLReconcilerMockManager) GetWebhookServer() webhook.Server {
	return nil
}
func (m *migrationACLReconcilerMockManager) GetLogger() logr.Logger { return logr.Discard() }
func (m *migrationACLReconcilerMockManager) GetControllerOptions() crconfig.Controller {
	return crconfig.Controller{}
}
func (m *migrationACLReconcilerMockManager) Elected() <-chan struct{} { return nil }
func (m *migrationACLReconcilerMockManager) AddHealthzCheck(string, healthz.Checker) error {
	return nil
}

func (m *migrationACLReconcilerMockManager) AddReadyzCheck(string, healthz.Checker) error {
	return nil
}
func (m *migrationACLReconcilerMockManager) GetHTTPClient() *http.Client { return nil }
func (m *migrationACLReconcilerMockManager) AddMetricsServerExtraHandler(string, http.Handler) error {
	return nil
}
func (m *migrationACLReconcilerMockManager) GetConfig() *rest.Config { return nil }

var _ ctrl.Manager = (*migrationACLReconcilerMockManager)(nil)

func clientObjectKey(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}

func simpleKafkaUserAuthorization(
	acls ...kafkav1beta2.KafkaUserSpecAuthorizationAclsElem,
) *kafkav1beta2.KafkaUserSpecAuthorization {
	return &kafkav1beta2.KafkaUserSpecAuthorization{
		Type: kafkav1beta2.KafkaUserSpecAuthorizationTypeSimple,
		Acls: acls,
	}
}
