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

package protocol

import (
	"context"
	"net/http"
	"testing"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	operatorconfig "github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

type migrationACLMockManager struct {
	client client.Client
}

func (m *migrationACLMockManager) Add(manager.Runnable) error { return nil }
func (m *migrationACLMockManager) GetClient() client.Client   { return m.client }
func (m *migrationACLMockManager) GetScheme() *runtime.Scheme {
	return nil
}
func (m *migrationACLMockManager) GetFieldIndexer() client.FieldIndexer { return nil }
func (m *migrationACLMockManager) GetCache() cache.Cache                { return nil }
func (m *migrationACLMockManager) GetEventRecorderFor(string) record.EventRecorder {
	return nil
}
func (m *migrationACLMockManager) GetRESTMapper() meta.RESTMapper { return nil }
func (m *migrationACLMockManager) GetAPIReader() client.Reader    { return nil }
func (m *migrationACLMockManager) Start(context.Context) error    { return nil }
func (m *migrationACLMockManager) GetWebhookServer() webhook.Server {
	return nil
}
func (m *migrationACLMockManager) GetLogger() logr.Logger { return logr.Discard() }
func (m *migrationACLMockManager) GetControllerOptions() config.Controller {
	return config.Controller{}
}
func (m *migrationACLMockManager) Elected() <-chan struct{} { return nil }
func (m *migrationACLMockManager) AddHealthzCheck(string, healthz.Checker) error {
	return nil
}

func (m *migrationACLMockManager) AddReadyzCheck(string, healthz.Checker) error {
	return nil
}
func (m *migrationACLMockManager) GetHTTPClient() *http.Client { return nil }
func (m *migrationACLMockManager) AddMetricsServerExtraHandler(string, http.Handler) error {
	return nil
}
func (m *migrationACLMockManager) GetConfig() *rest.Config { return nil }

func TestMigrationWriteACLKey(t *testing.T) {
	t.Parallel()

	key := migrationWriteACLKey("gh-migration")
	want := utils.GenerateACLKey(utils.WriteTopicACL("gh-migration"))
	if key != want {
		t.Fatalf("migrationWriteACLKey() = %q, want %q", key, want)
	}
}

func TestHasMigrationWriteACL(t *testing.T) {
	t.Parallel()

	operatorconfig.SetMigrationTopic("gh-migration")
	transporter := &strimziTransporter{}
	withACL := []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
		utils.WriteTopicACL("gh-migration"),
	}
	if !transporter.hasMigrationWriteACL(withACL) {
		t.Fatal("expected migration write ACL to be present")
	}
	if transporter.hasMigrationWriteACL(nil) {
		t.Fatal("expected nil ACL list to return false")
	}
}

func TestCurrentKafkaUserACLs(t *testing.T) {
	t.Parallel()

	if got := currentKafkaUserACLs(&kafkav1beta2.KafkaUser{}); got != nil {
		t.Fatalf("expected nil ACLs for missing spec, got %#v", got)
	}
	if got := currentKafkaUserACLs(nil); got != nil {
		t.Fatalf("expected nil ACLs for nil user, got %#v", got)
	}

	acl := utils.WriteTopicACL("gh-migration")
	user := &kafkav1beta2.KafkaUser{
		Spec: &kafkav1beta2.KafkaUserSpec{
			Authorization: &kafkav1beta2.KafkaUserSpecAuthorization{
				Acls: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{acl},
			},
		},
	}
	got := currentKafkaUserACLs(user)
	if len(got) != 1 {
		t.Fatalf("expected one ACL, got %d", len(got))
	}
}

func TestSyncMigrationWriteACLSkipsEmptyHub(t *testing.T) {
	t.Parallel()

	transporter := &strimziTransporter{}
	if err := transporter.SyncMigrationWriteACL("", true); err != nil {
		t.Fatalf("SyncMigrationWriteACL with empty hub should succeed, got %v", err)
	}
}

func TestSyncMigrationWriteACLGrantAndRevoke(t *testing.T) {
	operatorconfig.SetMigrationTopic("gh-migration")

	scheme := runtime.NewScheme()
	if err := kafkav1beta2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	kafkaUser := &kafkav1beta2.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hub1-kafka-user",
			Namespace: "test-ns",
		},
		Spec: &kafkav1beta2.KafkaUserSpec{
			Authorization: &kafkav1beta2.KafkaUserSpecAuthorization{
				Type: kafkav1beta2.KafkaUserSpecAuthorizationTypeSimple,
				Acls: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
					utils.ReadTopicACL("gh-spec", false),
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(kafkaUser).
		Build()

	transporter := &strimziTransporter{
		ctx:                   context.Background(),
		kafkaClusterNamespace: "test-ns",
		manager:               &migrationACLMockManager{client: fakeClient},
	}

	if err := transporter.SyncMigrationWriteACL("hub1", true); err != nil {
		t.Fatalf("grant migration write ACL: %v", err)
	}

	updated := &kafkav1beta2.KafkaUser{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(kafkaUser), updated); err != nil {
		t.Fatalf("get updated kafka user: %v", err)
	}
	if !transporter.hasMigrationWriteACL(updated.Spec.Authorization.Acls) {
		t.Fatal("expected migration write ACL to be granted")
	}

	if err := transporter.SyncMigrationWriteACL("hub1", false); err != nil {
		t.Fatalf("revoke migration write ACL: %v", err)
	}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(kafkaUser), updated); err != nil {
		t.Fatalf("get updated kafka user after revoke: %v", err)
	}
	if transporter.hasMigrationWriteACL(updated.Spec.Authorization.Acls) {
		t.Fatal("expected migration write ACL to be revoked")
	}
}

func TestSyncMigrationWriteACLNotFoundWhenGranting(t *testing.T) {
	operatorconfig.SetMigrationTopic("gh-migration")

	scheme := runtime.NewScheme()
	if err := kafkav1beta2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	transporter := &strimziTransporter{
		ctx:                   context.Background(),
		kafkaClusterNamespace: "test-ns",
		manager:               &migrationACLMockManager{client: fake.NewClientBuilder().WithScheme(scheme).Build()},
	}

	if err := transporter.SyncMigrationWriteACL("hub1", true); err == nil {
		t.Fatal("expected error when kafka user is missing during grant")
	}
}

func TestSyncMigrationWriteACLNotFoundWhenRevoking(t *testing.T) {
	operatorconfig.SetMigrationTopic("gh-migration")

	scheme := runtime.NewScheme()
	if err := kafkav1beta2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	transporter := &strimziTransporter{
		ctx:                   context.Background(),
		kafkaClusterNamespace: "test-ns",
		manager:               &migrationACLMockManager{client: fake.NewClientBuilder().WithScheme(scheme).Build()},
	}

	if err := transporter.SyncMigrationWriteACL("hub1", false); err != nil {
		t.Fatalf("revoke on missing kafka user should succeed, got %v", err)
	}
}

func TestSyncMigrationWriteACLInitializesNilAuthorization(t *testing.T) {
	operatorconfig.SetMigrationTopic("gh-migration")

	scheme := runtime.NewScheme()
	if err := kafkav1beta2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	kafkaUser := &kafkav1beta2.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hub1-kafka-user",
			Namespace: "test-ns",
		},
		Spec: &kafkav1beta2.KafkaUserSpec{},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(kafkaUser).
		Build()

	transporter := &strimziTransporter{
		ctx:                   context.Background(),
		kafkaClusterNamespace: "test-ns",
		manager:               &migrationACLMockManager{client: fakeClient},
	}

	if err := transporter.SyncMigrationWriteACL("hub1", true); err != nil {
		t.Fatalf("grant migration write ACL with nil authorization: %v", err)
	}

	updated := &kafkav1beta2.KafkaUser{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(kafkaUser), updated); err != nil {
		t.Fatalf("get updated kafka user: %v", err)
	}
	if updated.Spec.Authorization == nil || len(updated.Spec.Authorization.Acls) != 1 {
		t.Fatal("expected authorization block with migration write ACL")
	}
}

var _ ctrl.Manager = (*migrationACLMockManager)(nil)
