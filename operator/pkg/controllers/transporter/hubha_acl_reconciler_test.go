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

func TestHubHAACLReconcilerReconcileActiveRoleGrantsACL(t *testing.T) {
	originalBYO := operatorconfig.IsBYOKafka()
	t.Cleanup(func() { operatorconfig.SetBYOKafka(originalBYO) })
	operatorconfig.SetBYOKafka(false)
	operatorconfig.SetSpecTopic("gh-spec")

	scheme := runtime.NewScheme()
	if err := kafkav1beta2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	if err := operatorv1alpha4.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	if err := clusterv1.Install(scheme); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	mgh := &operatorv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "multiclusterglobalhub", Namespace: "test-ns"},
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
			Authorization: &kafkav1beta2.KafkaUserSpecAuthorization{
				Type: kafkav1beta2.KafkaUserSpecAuthorizationTypeSimple,
				Acls: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
					utils.GetTopicACL("gh-spec", []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
						kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
						kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
					}),
				},
			},
		},
	}
	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hub1",
			Labels: map[string]string{
				constants.GHHubRoleLabelKey: constants.GHHubRoleActive,
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(mgh, kafkaUser, cluster).
		Build()

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
