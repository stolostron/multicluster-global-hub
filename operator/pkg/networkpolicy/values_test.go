/*
Copyright 2023

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package networkpolicy

import (
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func kubernetesServiceObjects() []runtime.Object {
	return []runtime.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      kubernetesServiceName,
				Namespace: kubernetesDefaultNamespace,
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: "10.96.0.1",
			},
		},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kubernetes-endpoints",
				Namespace: kubernetesDefaultNamespace,
				Labels: map[string]string{
					discoveryv1.LabelServiceName: kubernetesServiceName,
				},
			},
			AddressType: discoveryv1.AddressTypeIPv4,
			Endpoints: []discoveryv1.Endpoint{
				{Addresses: []string{"192.168.1.10"}},
			},
		},
	}
}

func TestBuildBaselineValues(t *testing.T) {
	scheme := newTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(kubernetesServiceObjects()...).Build()

	values := BuildBaselineValues(t.Context(), c, "gh-ns", "postgres")
	assert.Equal(t, "gh-ns", values.Namespace, "namespace")
	assert.Equal(t, "postgres", values.PostgresName, "postgres name")
	assert.ElementsMatch(t, []string{"10.96.0.1/32", "192.168.1.10/32"}, values.APIServerCIDRs, "API server CIDRs")
}

func TestBuildBaselineValues_Fallback(t *testing.T) {
	scheme := newTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	values := BuildBaselineValues(t.Context(), c, "gh-ns", "postgres")
	assert.Empty(t, values.APIServerCIDRs, "expected empty API server CIDR fallback")
}

func TestBuildAgentValues(t *testing.T) {
	scheme := newTestScheme(t)
	objs := append(kubernetesServiceObjects(),
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kafka-kafka-bootstrap",
				Namespace: "gh-ns",
			},
			Spec: corev1.ServiceSpec{ClusterIP: "172.30.10.5"},
		},
		&configv1.Network{
			ObjectMeta: metav1.ObjectMeta{Name: clusterNetworkName},
			Spec:       configv1.NetworkSpec{ServiceNetwork: []string{"172.30.0.0/16"}},
		},
	)
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	values := BuildAgentValues(t.Context(), c, "gh-ns", "203.0.113.10:9092")
	assert.ElementsMatch(t, []string{"10.96.0.1/32", "192.168.1.10/32"}, values.APIServerCIDRs, "API server CIDRs")
	assert.ElementsMatch(t, []string{"203.0.113.10/32"}, values.ExternalKafkaCIDRs, "Kafka CIDRs")
	assert.Equal(t, []string{"172.30.0.0/16"}, values.WebhookEgressCIDRs, "webhook CIDRs")
}

func TestBuildAddonAgentValues(t *testing.T) {
	scheme := newTestScheme(t)
	objs := []runtime.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kafka-kafka-bootstrap",
				Namespace: "gh-ns",
			},
			Spec: corev1.ServiceSpec{ClusterIP: "172.30.10.5"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	cidrs := BuildAddonAgentValues(t.Context(), c,
		"kafka-kafka-bootstrap.gh-ns.svc:9092", "gh-ns")
	assert.Equal(t, []string{"172.30.10.5/32"}, cidrs, "addon Kafka CIDRs")
}

func TestBuildAddonAgentValues_EmptyBootstrap(t *testing.T) {
	scheme := newTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	cidrs := BuildAddonAgentValues(t.Context(), c, "", "gh-ns")
	assert.Nil(t, cidrs, "expected nil CIDRs for empty bootstrap server")
}

func TestBuildAddonAgentValues_Fallback(t *testing.T) {
	scheme := newTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	cidrs := BuildAddonAgentValues(t.Context(), c, ":9092", "gh-ns")
	assert.Nil(t, cidrs, "expected nil CIDRs when bootstrap resolution fails")
}
