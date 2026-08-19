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

func TestHubHASpecWriteACLKey(t *testing.T) {
	t.Parallel()

	key := hubHASpecWriteACLKey("gh-spec")
	want := utils.GenerateACLKey(utils.WriteTopicACL("gh-spec"))
	if key != want {
		t.Fatalf("hubHASpecWriteACLKey() = %q, want %q", key, want)
	}
}

func TestHasHubHASpecWriteACL(t *testing.T) {
	restoreTransportConfigAfterTest(t)
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
	restoreTransportConfigAfterTest(t)
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

func TestSyncHubHASpecWriteACLSkipsEmptyHub(t *testing.T) {
	t.Parallel()

	transporter := &strimziTransporter{}
	if err := transporter.SyncHubHASpecWriteACL("", true); err != nil {
		t.Fatalf("SyncHubHASpecWriteACL with empty hub should succeed, got %v", err)
	}
}

func TestSyncHubHASpecWriteACLNotFoundWhenGranting(t *testing.T) {
	restoreTransportConfigAfterTest(t)
	operatorconfig.SetSpecTopic("gh-spec")

	scheme := runtime.NewScheme()
	if err := kafkav1beta2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	transporter := &strimziTransporter{
		ctx:                   context.Background(),
		kafkaClusterNamespace: "test-ns",
		manager:               &migrationACLMockManager{client: fake.NewClientBuilder().WithScheme(scheme).Build()},
	}

	if err := transporter.SyncHubHASpecWriteACL("hub1", true); err == nil {
		t.Fatal("expected error when kafka user is missing during grant")
	}
}

func TestSyncHubHASpecWriteACLNotFoundWhenRevoking(t *testing.T) {
	restoreTransportConfigAfterTest(t)
	operatorconfig.SetSpecTopic("gh-spec")

	scheme := runtime.NewScheme()
	if err := kafkav1beta2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	transporter := &strimziTransporter{
		ctx:                   context.Background(),
		kafkaClusterNamespace: "test-ns",
		manager:               &migrationACLMockManager{client: fake.NewClientBuilder().WithScheme(scheme).Build()},
	}

	if err := transporter.SyncHubHASpecWriteACL("hub1", false); err != nil {
		t.Fatalf("revoke with missing kafka user should succeed, got %v", err)
	}
}

func TestSyncHubHASpecWriteACLGrantWithNilAuthorization(t *testing.T) {
	restoreTransportConfigAfterTest(t)
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

	if err := transporter.SyncHubHASpecWriteACL("hub1", true); err != nil {
		t.Fatalf("grant Hub HA spec write ACL with nil authorization: %v", err)
	}

	updated := &kafkav1beta2.KafkaUser{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(kafkaUser), updated); err != nil {
		t.Fatalf("get updated kafka user: %v", err)
	}
	if !transporter.hasHubHASpecWriteACL(updated.Spec.Authorization.Acls, "gh-spec") {
		t.Fatal("expected Hub HA spec write ACL to be granted on nil authorization")
	}
}

func TestSyncHubHASpecWriteACLWrapper(t *testing.T) {
	restoreTransportConfigAfterTest(t)
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
	if err := SyncHubHASpecWriteACL(mgr, mgh, "hub1", true, WithContext(context.Background())); err != nil {
		t.Fatalf("SyncHubHASpecWriteACL() grant error = %v", err)
	}

	updated := &kafkav1beta2.KafkaUser{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(kafkaUser), updated); err != nil {
		t.Fatalf("get updated kafka user: %v", err)
	}
	wantKey := utils.GenerateACLKey(utils.WriteTopicACL("gh-spec"))
	foundWrite := false
	for _, acl := range updated.Spec.Authorization.Acls {
		if utils.GenerateACLKey(acl) == wantKey {
			foundWrite = true
			break
		}
	}
	if !foundWrite {
		t.Fatal("expected Hub HA spec topic write ACL")
	}
}

func TestIsObsoleteManagedHubACL(t *testing.T) {
	t.Parallel()

	specTopic := "gh-spec"
	wildcard := "*"
	otherTopic := "gh-other"
	namePtr := func(name string) *string { return &name }

	tests := []struct {
		name string
		acl  kafkav1beta2.KafkaUserSpecAuthorizationAclsElem
		want bool
	}{
		{
			name: "nil resource name",
			acl: kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
				Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
					Type: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
				},
			},
			want: false,
		},
		{
			name: "wildcard consumer group",
			acl: kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
				Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
					Type: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeGroup,
					Name: namePtr(wildcard),
				},
			},
			want: true,
		},
		{
			name: "literal consumer group",
			acl: kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
				Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
					Type: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeGroup,
					Name: namePtr("hub1"),
				},
			},
			want: false,
		},
		{
			name: "non spec topic",
			acl: kafkav1beta2.KafkaUserSpecAuthorizationAclsElem{
				Resource: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResource{
					Type: kafkav1beta2.KafkaUserSpecAuthorizationAclsElemResourceTypeTopic,
					Name: namePtr(otherTopic),
				},
				Operations: []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
					kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemWrite,
					kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
				},
			},
			want: false,
		},
		{
			name: "legacy combined spec topic write",
			acl: utils.GetTopicACL(specTopic, []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
				kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
				kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
				kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemWrite,
			}),
			want: true,
		},
		{
			name: "hub ha write only",
			acl:  utils.WriteTopicACL(specTopic),
			want: false,
		},
		{
			name: "spec topic read only",
			acl: utils.GetTopicACL(specTopic, []kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElem{
				kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemDescribe,
				kafkav1beta2.KafkaUserSpecAuthorizationAclsElemOperationsElemRead,
			}),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isObsoleteManagedHubACL(tt.acl, specTopic); got != tt.want {
				t.Fatalf("isObsoleteManagedHubACL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterObsoleteManagedHubACLsEmptyInput(t *testing.T) {
	t.Parallel()

	if got := filterObsoleteManagedHubACLs(nil, "gh-spec"); got != nil {
		t.Fatalf("filterObsoleteManagedHubACLs(nil) = %#v, want nil", got)
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
