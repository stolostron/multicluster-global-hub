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
	"testing"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	operatorconfig "github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func restoreTransportConfigAfterTest(t *testing.T) {
	t.Helper()
	snapshot := operatorconfig.SnapshotTransportConfigForTest()
	t.Cleanup(func() {
		operatorconfig.RestoreTransportConfigForTest(snapshot)
	})
}

func TestHubHAACLReconcilerReconcileActiveRoleGrantsACL(t *testing.T) {
	originalBYO := operatorconfig.IsBYOKafka()
	t.Cleanup(func() { operatorconfig.SetBYOKafka(originalBYO) })
	operatorconfig.SetBYOKafka(false)
	restoreTransportConfigAfterTest(t)
	operatorconfig.SetSpecTopic("gh-spec")

	_, fakeClient, _, kafkaUser := newHubHAACLReconcilerFixtures(t)
	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hub1",
			Labels: map[string]string{
				constants.GHHubRoleLabelKey: constants.GHHubRoleActive,
			},
		},
	}
	if err := fakeClient.Create(context.Background(), cluster); err != nil {
		t.Fatalf("create active cluster: %v", err)
	}

	reconciler := &HubHAACLReconciler{
		mgr:    &migrationACLReconcilerMockManager{client: fakeClient},
		Client: fakeClient,
	}
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cluster)})
	if err != nil {
		t.Fatalf("Reconcile() active hub error = %v", err)
	}

	updated := &kafkav1beta2.KafkaUser{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(kafkaUser), updated); err != nil {
		t.Fatalf("get updated kafka user: %v", err)
	}
	wantKey := utils.GenerateACLKey(utils.WriteTopicACL("gh-spec"))
	found := false
	for _, acl := range updated.Spec.Authorization.Acls {
		if utils.GenerateACLKey(acl) == wantKey {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected Hub HA spec write ACL to be granted for active hub role")
	}
}

func TestHubHAACLReconcilerReconcileSkipsBYO(t *testing.T) {
	originalBYO := operatorconfig.IsBYOKafka()
	t.Cleanup(func() { operatorconfig.SetBYOKafka(originalBYO) })
	operatorconfig.SetBYOKafka(true)

	reconciler := &HubHAACLReconciler{}
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: clientObjectKey("", "hub1"),
	})
	if err != nil {
		t.Fatalf("Reconcile() BYO skip error = %v", err)
	}
}

func TestHubHAACLReconcilerReconcileNotFound(t *testing.T) {
	originalBYO := operatorconfig.IsBYOKafka()
	t.Cleanup(func() { operatorconfig.SetBYOKafka(originalBYO) })
	operatorconfig.SetBYOKafka(false)
	restoreTransportConfigAfterTest(t)
	operatorconfig.SetSpecTopic("gh-spec")

	_, fakeClient, _, kafkaUser := newHubHAACLReconcilerFixtures(t)
	kafkaUser.Spec.Authorization.Acls = []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
		utils.WriteTopicACL("gh-spec"),
	}
	if err := fakeClient.Update(context.Background(), kafkaUser); err != nil {
		t.Fatalf("update kafka user with Hub HA ACL: %v", err)
	}

	reconciler := &HubHAACLReconciler{
		mgr:    &migrationACLReconcilerMockManager{client: fakeClient},
		Client: fakeClient,
	}
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: clientObjectKey("", "hub1"),
	})
	if err != nil {
		t.Fatalf("Reconcile() deleted cluster should succeed, got %v", err)
	}

	updated := &kafkav1beta2.KafkaUser{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(kafkaUser), updated); err != nil {
		t.Fatalf("get updated kafka user: %v", err)
	}
	if updated.Spec.Authorization != nil {
		for _, acl := range updated.Spec.Authorization.Acls {
			if utils.GenerateACLKey(acl) == utils.GenerateACLKey(utils.WriteTopicACL("gh-spec")) {
				t.Fatal("expected Hub HA spec write ACL to be revoked after cluster deletion")
			}
		}
	}
}

func TestHubHAACLReconcilerReconcileStandbyRevokesACL(t *testing.T) {
	originalBYO := operatorconfig.IsBYOKafka()
	t.Cleanup(func() { operatorconfig.SetBYOKafka(originalBYO) })
	operatorconfig.SetBYOKafka(false)
	restoreTransportConfigAfterTest(t)
	operatorconfig.SetSpecTopic("gh-spec")

	_, fakeClient, _, kafkaUser := newHubHAACLReconcilerFixtures(t)
	kafkaUser.Spec.Authorization.Acls = []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
		utils.WriteTopicACL("gh-spec"),
	}
	if err := fakeClient.Update(context.Background(), kafkaUser); err != nil {
		t.Fatalf("update kafka user with Hub HA ACL: %v", err)
	}

	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hub1",
			Labels: map[string]string{
				constants.GHHubRoleLabelKey: constants.GHHubRoleStandby,
			},
		},
	}
	if err := fakeClient.Create(context.Background(), cluster); err != nil {
		t.Fatalf("create standby cluster: %v", err)
	}

	reconciler := &HubHAACLReconciler{
		mgr:    &migrationACLReconcilerMockManager{client: fakeClient},
		Client: fakeClient,
	}
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cluster)})
	if err != nil {
		t.Fatalf("Reconcile() standby hub error = %v", err)
	}

	updated := &kafkav1beta2.KafkaUser{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(kafkaUser), updated); err != nil {
		t.Fatalf("get updated kafka user: %v", err)
	}
	if updated.Spec.Authorization != nil {
		for _, acl := range updated.Spec.Authorization.Acls {
			if utils.GenerateACLKey(acl) == utils.GenerateACLKey(utils.WriteTopicACL("gh-spec")) {
				t.Fatal("expected Hub HA spec write ACL to be revoked for standby hub role")
			}
		}
	}
}

func TestHubHAACLReconcilerReconcileSkipsPausedMGH(t *testing.T) {
	originalBYO := operatorconfig.IsBYOKafka()
	t.Cleanup(func() { operatorconfig.SetBYOKafka(originalBYO) })
	operatorconfig.SetBYOKafka(false)

	_, fakeClient, mgh, _ := newHubHAACLReconcilerFixtures(t)
	mgh.Annotations = map[string]string{"mgh-pause": "true"}
	if err := fakeClient.Update(context.Background(), mgh); err != nil {
		t.Fatalf("update paused mgh: %v", err)
	}

	reconciler := &HubHAACLReconciler{
		mgr:    &migrationACLReconcilerMockManager{client: fakeClient},
		Client: fakeClient,
	}
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: clientObjectKey("", "hub1"),
	})
	if err != nil {
		t.Fatalf("Reconcile() paused mgh should succeed, got %v", err)
	}
}

func newHubHAACLReconcilerFixtures(
	t *testing.T,
) (*runtime.Scheme, client.Client, *operatorv1alpha4.MulticlusterGlobalHub, *kafkav1beta2.KafkaUser) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := kafkav1beta2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(KafkaUser) error = %v", err)
	}
	if err := operatorv1alpha4.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(MulticlusterGlobalHub) error = %v", err)
	}
	if err := clusterv1.Install(scheme); err != nil {
		t.Fatalf("Install(ManagedCluster) error = %v", err)
	}

	mgh := &operatorv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "mgh", Namespace: "test-ns"},
		Spec: operatorv1alpha4.MulticlusterGlobalHubSpec{
			DataLayerSpec: operatorv1alpha4.DataLayerSpec{
				Kafka: operatorv1alpha4.KafkaSpec{
					KafkaTopics: operatorv1alpha4.KafkaTopics{
						SpecTopic: "gh-spec",
					},
				},
			},
		},
	}
	kafkaUser := &kafkav1beta2.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{Name: "hub1-kafka-user", Namespace: "test-ns"},
		Spec: &kafkav1beta2.KafkaUserSpec{
			Authorization: simpleKafkaUserAuthorization(
				utils.GetTopicACL("gh-spec", []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
					kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
					kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
				}),
			),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mgh, kafkaUser).
		Build()

	return scheme, fakeClient, mgh, kafkaUser
}
