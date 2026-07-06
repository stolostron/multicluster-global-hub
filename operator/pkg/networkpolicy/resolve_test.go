/*
Copyright 2023 Red Hat, Inc.

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
	"net"
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

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme), "register corev1 scheme")
	require.NoError(t, discoveryv1.AddToScheme(scheme), "register discoveryv1 scheme")
	require.NoError(t, configv1.AddToScheme(scheme), "register configv1 scheme")
	return scheme
}

func TestResolveAPIServerCIDRs(t *testing.T) {
	scheme := newTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(kubernetesServiceObjects()...).Build()
	cidrs, err := ResolveAPIServerCIDRs(t.Context(), c)
	require.NoError(t, err, "resolve kubernetes API server CIDRs")
	assert.ElementsMatch(t, []string{"10.96.0.1/32", "192.168.1.10/32"}, cidrs, "unexpected API server CIDRs")
}

func TestResolveBootstrapServerCIDRs_IPAndService(t *testing.T) {
	scheme := newTestScheme(t)
	objs := []runtime.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kafka-kafka-bootstrap",
				Namespace: "multicluster-global-hub",
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: "172.30.10.5",
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
	cidrs, err := ResolveBootstrapServerCIDRs(t.Context(), c,
		"203.0.113.10:9092,kafka-kafka-bootstrap.multicluster-global-hub.svc:9092",
		"multicluster-global-hub")
	require.NoError(t, err, "resolve bootstrap server CIDRs")
	assert.ElementsMatch(t, []string{"172.30.10.5/32", "203.0.113.10/32"}, cidrs, "unexpected bootstrap CIDRs")
}

func TestResolveBootstrapServerCIDRs_Empty(t *testing.T) {
	scheme := newTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	_, err := ResolveBootstrapServerCIDRs(t.Context(), c, "  ", "ns")
	require.Error(t, err, "expected error for empty bootstrap server")
}

func TestResolvePostgresCIDRs_Service(t *testing.T) {
	scheme := newTestScheme(t)
	objs := []runtime.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hoh-rw",
				Namespace: "multicluster-global-hub-postgres",
			},
			Spec: corev1.ServiceSpec{ClusterIP: "172.30.5.56"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	cidrs, err := ResolvePostgresCIDRs(t.Context(), c, "gh-ns",
		"postgresql://postgres:secret@hoh-rw.multicluster-global-hub-postgres.svc:5432/hoh")
	require.NoError(t, err, "resolve postgres CIDRs")
	assert.Equal(t, []string{"172.30.5.56/32"}, cidrs, "unexpected postgres CIDRs")
}

func TestResolvePostgresCIDRs_EmptyURI(t *testing.T) {
	scheme := newTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	_, err := ResolvePostgresCIDRs(t.Context(), c, "gh-ns", "  ")
	require.Error(t, err, "expected error for empty postgres URI")
}

func TestResolvePostgresCIDRs_InvalidURIDoesNotLeakCredentials(t *testing.T) {
	scheme := newTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	_, err := ResolvePostgresCIDRs(t.Context(), c, "gh-ns", "://postgres:supersecret@bad-uri")
	require.Error(t, err, "expected error for invalid postgres URI")
	assert.NotContains(t, err.Error(), "supersecret", "error must not contain database credentials")
}

func TestParseServiceHost(t *testing.T) {
	tests := []struct {
		host     string
		wantNS   string
		wantName string
	}{
		{"kafka-kafka-bootstrap.gh-ns.svc", "gh-ns", "kafka-kafka-bootstrap"},
		{"kafka-kafka-bootstrap.gh-ns.svc.cluster.local", "gh-ns", "kafka-kafka-bootstrap"},
		{"kafka-kafka-bootstrap.svc", "gh-ns", "kafka-kafka-bootstrap"},
		{"example.com", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			ns, name := parseServiceHost(tt.host, "gh-ns")
			assert.Equal(t, tt.wantNS, ns, "unexpected namespace")
			assert.Equal(t, tt.wantName, name, "unexpected service name")
		})
	}
}

func TestBrokerHost(t *testing.T) {
	assert.Equal(t, "broker.example.com", brokerHost("broker.example.com:9092"), "broker host with port")
	assert.Equal(t, "203.0.113.1", brokerHost("203.0.113.1:9093"), "IP broker host")
	assert.Equal(t, "broker", brokerHost("broker"), "broker host without port")
}

func TestResolveAPIServerCIDRs_NotFound(t *testing.T) {
	scheme := newTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	_, err := ResolveAPIServerCIDRs(t.Context(), c)
	require.Error(t, err, "expected error when kubernetes service is missing")
}

func TestResolveServiceNetworkCIDRs(t *testing.T) {
	scheme := newTestScheme(t)
	objs := []runtime.Object{
		&configv1.Network{
			ObjectMeta: metav1.ObjectMeta{Name: clusterNetworkName},
			Spec: configv1.NetworkSpec{
				ServiceNetwork: []string{"172.30.0.0/16"},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	cidrs, err := ResolveServiceNetworkCIDRs(t.Context(), c)
	require.NoError(t, err, "resolve service network CIDRs")
	assert.Equal(t, []string{"172.30.0.0/16"}, cidrs, "unexpected service network CIDRs")
}

func TestResolveServiceNetworkCIDRs_NotFound(t *testing.T) {
	scheme := newTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	cidrs, err := ResolveServiceNetworkCIDRs(t.Context(), c)
	require.NoError(t, err, "missing Network CR should not error")
	assert.Nil(t, cidrs, "expected nil CIDRs when Network CR is absent")
}

func TestResolveServiceNetworkCIDRs_InvalidCIDR(t *testing.T) {
	scheme := newTestScheme(t)
	objs := []runtime.Object{
		&configv1.Network{
			ObjectMeta: metav1.ObjectMeta{Name: clusterNetworkName},
			Spec: configv1.NetworkSpec{
				ServiceNetwork: []string{"not-a-cidr"},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	_, err := ResolveServiceNetworkCIDRs(t.Context(), c)
	require.Error(t, err, "expected error for invalid service network CIDR")
}

func TestResolveServiceCIDRs_ServiceOnly(t *testing.T) {
	scheme := newTestScheme(t)
	objs := []runtime.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kafka-kafka-bootstrap",
				Namespace: "multicluster-global-hub",
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: "172.30.10.5",
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	cidrs, err := resolveServiceCIDRs(t.Context(), c, "multicluster-global-hub", "kafka-kafka-bootstrap")
	require.NoError(t, err, "resolve service CIDRs from ClusterIP")
	assert.Equal(t, []string{"172.30.10.5/32"}, cidrs, "unexpected service CIDRs")
}

func TestAddHostCIDRAndSortedCIDRs(t *testing.T) {
	cidrs := map[string]struct{}{}
	addHostCIDR(cidrs, nil)
	addHostCIDR(cidrs, parseTestIP(t, "10.0.0.1"))
	addHostCIDR(cidrs, parseTestIP(t, "2001:db8::1"))

	assert.Equal(t, []string{"10.0.0.1/32", "2001:db8::1/128"}, sortedCIDRs(cidrs), "unexpected sorted CIDRs")
}

func parseTestIP(t *testing.T, raw string) net.IP {
	t.Helper()
	ip := net.ParseIP(raw)
	require.NotNil(t, ip, "parse test IP %q", raw)
	return ip
}
