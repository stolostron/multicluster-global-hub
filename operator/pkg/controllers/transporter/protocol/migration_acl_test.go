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
	"testing"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	operatorconfig "github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestSyncMigrationWriteACLWrapper(t *testing.T) {
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

	mgh := &operatorv1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "mgh", Namespace: "test-ns"},
	}

	mgr := &migrationACLMockManager{client: fakeClient}
	if err := SyncMigrationWriteACL(mgr, mgh, "hub1", true, WithContext(context.Background())); err != nil {
		t.Fatalf("SyncMigrationWriteACL() grant error = %v", err)
	}

	updated := &kafkav1beta2.KafkaUser{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(kafkaUser), updated); err != nil {
		t.Fatalf("get updated kafka user: %v", err)
	}
	if len(updated.Spec.Authorization.Acls) != 2 {
		t.Fatalf("expected read and write ACLs after grant, got %d", len(updated.Spec.Authorization.Acls))
	}
	wantKey := utils.GenerateACLKey(utils.WriteTopicACL("gh-migration"))
	foundWrite := false
	for _, acl := range updated.Spec.Authorization.Acls {
		if utils.GenerateACLKey(acl) == wantKey {
			foundWrite = true
			break
		}
	}
	if !foundWrite {
		t.Fatal("expected migration topic write ACL")
	}
}
