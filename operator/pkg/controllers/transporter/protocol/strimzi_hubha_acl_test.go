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

	operatorconfig "github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestHubHASpecWriteACLKey(t *testing.T) {
	t.Parallel()

	key := hubHASpecWriteACLKey("gh-spec")
	want := utils.GenerateACLKey(utils.WriteTopicACL("gh-spec"))
	if key != want {
		t.Fatalf("hubHASpecWriteACLKey() = %q, want %q", key, want)
	}
}

func TestHasHubHASpecWriteACL(t *testing.T) {
	t.Parallel()

	operatorconfig.SetSpecTopic("gh-spec")
	transporter := &strimziTransporter{}
	withACL := []kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
		utils.WriteTopicACL("gh-spec"),
	}
	if !transporter.hasHubHASpecWriteACL(withACL, "gh-spec") {
		t.Fatal("expected Hub HA spec write ACL to be present")
	}
	if transporter.hasHubHASpecWriteACL(nil, "gh-spec") {
		t.Fatal("expected nil ACL list to return false")
	}
}

func TestSyncHubHASpecWriteACLGrantAndRevoke(t *testing.T) {
	operatorconfig.SetSpecTopic("gh-spec")

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
					utils.GetTopicACL("gh-spec", []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
						kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
						kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
					}),
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

	if err := transporter.SyncHubHASpecWriteACL("hub1", true); err != nil {
		t.Fatalf("grant Hub HA spec write ACL: %v", err)
	}

	updated := &kafkav1beta2.KafkaUser{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(kafkaUser), updated); err != nil {
		t.Fatalf("get updated kafka user: %v", err)
	}
	if !transporter.hasHubHASpecWriteACL(updated.Spec.Authorization.Acls, "gh-spec") {
		t.Fatal("expected Hub HA spec write ACL to be granted")
	}

	if err := transporter.SyncHubHASpecWriteACL("hub1", false); err != nil {
		t.Fatalf("revoke Hub HA spec write ACL: %v", err)
	}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(kafkaUser), updated); err != nil {
		t.Fatalf("get updated kafka user after revoke: %v", err)
	}
	if transporter.hasHubHASpecWriteACL(updated.Spec.Authorization.Acls, "gh-spec") {
		t.Fatal("expected Hub HA spec write ACL to be revoked")
	}
}

func TestFilterObsoleteManagedHubACLPreservesHubHAWrite(t *testing.T) {
	t.Parallel()

	specTopic := "gh-spec"
	legacyACL := utils.GetTopicACL(specTopic, []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
		kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemWrite,
	})
	hubHAWriteACL := utils.WriteTopicACL(specTopic)

	filtered := filterObsoleteManagedHubACLs([]kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
		legacyACL,
		hubHAWriteACL,
	}, specTopic)

	if len(filtered) != 1 {
		t.Fatalf("expected one ACL after filtering legacy combined gh-spec Write, got %d", len(filtered))
	}
	if utils.GenerateACLKey(filtered[0]) != utils.GenerateACLKey(hubHAWriteACL) {
		t.Fatal("expected Hub HA Write-only ACL to survive obsolete ACL filtering")
	}
}
