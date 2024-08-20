// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package certificates

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/openshift/library-go/pkg/crypto"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestCertAgent(t *testing.T) {
	cert, key, err := newSigningCertKeyPair("testing-mcgh", 365*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverCACerts,
			Namespace: utils.GetDefaultNamespace(),
		},
		Data: map[string][]byte{
			"tls.crt": cert,
			"tls.key": key,
		},
	}

	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Secret{})
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(caSecret).Build()

	agent := &GlobalHubAgent{client: client}
	agent.Manifests(nil, nil)
	options := agent.GetAgentAddonOptions()
	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterName,
		},
	}
	configs := options.Registration.CSRConfigurations(cluster)
	expectedCSRs := 2
	if len(configs) != expectedCSRs {
		t.Fatalf("expected %d CSRs, found %d", expectedCSRs, len(configs))
	}

	caHashOrgUnit := fmt.Sprintf("ca-hash-%x", sha256.Sum256(cert))

	kubeAPISignerExpectedRegConfig := v1alpha1.RegistrationConfig{
		SignerName: "kubernetes.io/kube-apiserver-client",
		Subject: v1alpha1.Subject{
			User: "system:open-cluster-management:cluster:test:addon:multicluster-global-hub-controller:agent:global-hub-agent",
			Groups: []string{
				"system:open-cluster-management:cluster:test:addon:multicluster-global-hub-controller",
				"system:open-cluster-management:addon:multicluster-global-hub-controller",
				"system:authenticated",
			},
			OrganizationUnits: []string{caHashOrgUnit},
		},
	}
	assert.Contains(t, configs, kubeAPISignerExpectedRegConfig)

	obsSignerExpectedRegConfig := v1alpha1.RegistrationConfig{
		SignerName: "open-cluster-management.io/globalhub-signer",
		Subject: v1alpha1.Subject{
			User:              "inventory-api",
			Groups:            nil,
			OrganizationUnits: []string{"globalhub", caHashOrgUnit},
		},
	}
	assert.Contains(t, configs, obsSignerExpectedRegConfig)
}

func newSigningCertKeyPair(signerName string, validity time.Duration) (certData, keyData []byte, err error) {
	ca, err := crypto.MakeSelfSignedCAConfigForDuration(signerName, validity)
	if err != nil {
		return nil, nil, err
	}

	certBytes := &bytes.Buffer{}
	keyBytes := &bytes.Buffer{}
	if err := ca.WriteCertConfig(certBytes, keyBytes); err != nil {
		return nil, nil, err
	}

	return certBytes.Bytes(), keyBytes.Bytes(), nil
}
